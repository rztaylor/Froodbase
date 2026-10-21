//go:build integration

package store_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

// linkOnly is a contribution that records evidence and links it, and
// evidence the area holds, to existing claims, writing no new claim.
func linkOnly(area domain.ID, evidence []domain.Evidence, links ...store.EvidenceLink) store.Contribution {
	return store.Contribution{
		Contribution: domain.Contribution{ID: domain.ID(store.NewID()), AreaID: area, ActorUserID: "alice", ActorAgentID: "claude", Client: "test", RecordedAt: t1},
		Evidence:     evidence,
		Links:        links,
		Rationale:    "corroborated later",
	}
}

// A contribution can cite new and existing evidence for a claim the area
// already holds (FRB-57): one root_added per distinct root the claim had not
// counted, the score re-projected, no new revision of the claim, an
// evidence_linked audit event, and reconsider_derived enqueued for the
// deductions built on it. Evidence the claim already counts, directly or
// through a restatement, adds nothing. A deduction written in the same call
// and derived from the claim inherits the new root at once.
func TestContributeLinksEvidenceToExistingClaims(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f, i1, _ := chain(ctx, t, db, areaA, callerA) // the observation has roots ev1 and ev3
	revisions := revisionCount(ctx, t, db)

	ev4 := evidence(areaA.ID, "https://github.com/org/repo/blob/main/e.go", "")
	ev5 := evidence(areaA.ID, "https://github.com/org/repo/blob/main/f.go", f.ev1.ID) // restates ev1
	i3, l3 := deduction(t, domain.KindInference, areaA.ID, f.entity.ID(), "The gateway drops long requests.", f.assertion.ID)
	c := linkOnly(areaA.ID, []domain.Evidence{ev4, ev5}, store.EvidenceLink{ClaimID: f.assertion.ID, EvidenceIDs: []domain.ID{ev4.ID, ev5.ID, f.ev2.ID}})
	c.Assertions, c.Derivations = []domain.Assertion{i3}, l3
	res, err := db.App.Contribute(ctx, callerA, c)
	if err != nil {
		t.Fatal(err)
	}
	w := domain.DefaultSupportConfig().EventWeights[domain.SupportRootAdded]
	want := []store.LinkedClaim{{ID: f.assertion.ID, Kind: string(domain.KindObservation), Revision: 1, EvidenceIDs: []domain.ID{ev4.ID, ev5.ID, f.ev2.ID}, RootsAdded: []domain.ID{ev4.ID}}}
	if !reflect.DeepEqual(res.Linked, want) || res.Scores[f.assertion.ID] != 3*w || res.Roots[f.assertion.ID] != 3 || res.RootKinds[f.assertion.ID][domain.SourceRepository] != 2 {
		t.Errorf("result = linked %+v, score %v, roots %d, kinds %v; want %+v with three roots", res.Linked, res.Scores[f.assertion.ID], res.Roots[f.assertion.ID], res.RootKinds[f.assertion.ID], want)
	}
	if res.Roots[i3.ID] != 3 {
		t.Errorf("the deduction written with the link has %d roots, want 3 (it inherits the new one)", res.Roots[i3.ID])
	}
	for _, rec := range res.Written {
		if rec.ID == f.assertion.ID {
			t.Errorf("the linked claim is listed as written: %+v", res.Written)
		}
	}
	if got := revisionCount(ctx, t, db) - revisions; got != 3 {
		t.Errorf("%d revisions written, want 3 (two evidence records and the new inference, none for the linked claim)", got)
	}
	a, err := db.App.GetAssertion(ctx, callerA, f.assertion.ID)
	if err != nil || a.Revision != 1 || a.SupportScore != 3*w {
		t.Errorf("linked claim = revision %d, score %v, %v; want revision 1 with score %v", a.Revision, a.SupportScore, err, 3*w)
	}
	linked, err := db.App.ClaimEvidence(ctx, callerA, f.assertion.ID)
	if err != nil || len(linked) != 5 {
		t.Errorf("claim evidence = %d, %v; want 5 (ev1, ev2, ev3, ev4, ev5)", len(linked), err)
	}
	audit, err := db.App.AuditEvents(ctx, callerA, f.assertion.ID)
	if err != nil || len(audit) == 0 || audit[len(audit)-1].EventType != store.AuditEvidenceLinked {
		t.Errorf("audit of the linked claim = %+v, %v; want evidence_linked last", audit, err)
	}
	if n := waitingReconsider(ctx, t, db, f.assertion.ID); n != 1 {
		t.Errorf("%d reconsider_derived jobs waiting for the observation, want 1 (i1 is derived from it)", n)
	}
	_ = i1

	// Evidence the claim already counts adds nothing and enqueues nothing new.
	events := func() int {
		e, err := db.App.SupportEvents(ctx, callerA, f.assertion.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(e)
	}
	before := events()
	res, err = db.App.Contribute(ctx, callerA, linkOnly(areaA.ID, nil, store.EvidenceLink{ClaimID: f.assertion.ID, EvidenceIDs: []domain.ID{f.ev2.ID, ev4.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Linked) != 1 || len(res.Linked[0].RootsAdded) != 0 || res.Scores[f.assertion.ID] != 3*w || events() != before {
		t.Errorf("relink of counted evidence = %+v, score %v, %d events; want nothing added", res.Linked, res.Scores[f.assertion.ID], events()-before)
	}
}

// A link to an unknown claim, to a claim or evidence in another area, or to
// something that is not a claim refuses the whole contribution, and nothing
// is written.
func TestContributeLinkRefusals(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	areaB, callerB := area(ctx, t, db, "area-b")
	f := newFixture(t, areaA, "The API returned 503s.")
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}
	other := newFixture(t, areaB, "Area b saw timeouts.")
	if _, err := db.App.Contribute(ctx, callerB, other.contribution); err != nil {
		t.Fatal(err)
	}
	both := store.Caller{UserID: "alice", AgentID: "claude", AreaIDs: []string{string(areaA.ID), string(areaB.ID)}}
	fresh := func() domain.Evidence { return evidence(areaA.ID, "https://github.com/org/repo/blob/main/g.go", "") }
	link := func(claim domain.ID, ev ...domain.ID) store.EvidenceLink {
		return store.EvidenceLink{ClaimID: claim, EvidenceIDs: ev}
	}

	tests := []struct {
		name   string
		caller store.Caller
		c      func() store.Contribution
		want   error
	}{
		{"unknown claim", callerA, func() store.Contribution {
			ev := fresh()
			return linkOnly(areaA.ID, []domain.Evidence{ev}, link(domain.ID(store.NewID()), ev.ID))
		}, store.ErrNotFound},
		{"an entity, not a claim", callerA, func() store.Contribution {
			ev := fresh()
			return linkOnly(areaA.ID, []domain.Evidence{ev}, link(f.entity.ID(), ev.ID))
		}, store.ErrNotFound},
		{"claim in another area, caller holds both", both, func() store.Contribution {
			ev := fresh()
			return linkOnly(areaA.ID, []domain.Evidence{ev}, link(other.assertion.ID, ev.ID))
		}, store.ErrCrossArea},
		{"claim in another area, caller holds one", callerA, func() store.Contribution {
			ev := fresh()
			return linkOnly(areaA.ID, []domain.Evidence{ev}, link(other.assertion.ID, ev.ID))
		}, store.ErrNotFound},
		{"evidence in another area, caller holds both", both, func() store.Contribution {
			ev := fresh()
			return linkOnly(areaA.ID, []domain.Evidence{ev}, link(f.assertion.ID, ev.ID, other.ev3.ID))
		}, store.ErrCrossArea},
		{"unknown evidence", callerA, func() store.Contribution {
			return linkOnly(areaA.ID, nil, link(f.assertion.ID, domain.ID(store.NewID())))
		}, store.ErrNotFound},
		{"a new claim citing evidence in another area, caller holds both", both, func() store.Contribution {
			g := newFixture(t, areaA, "Cites another area.")
			g.contribution.Evidence = nil
			g.contribution.ClaimEvidence = map[domain.ID][]domain.ID{g.assertion.ID: {other.ev3.ID}}
			return g.contribution
		}, store.ErrCrossArea},
	}
	count := func() int {
		return countAdmin(ctx, t, db.Admin, "select (select count(*) from evidence) + (select count(*) from claim_evidence) + (select count(*) from support_event) + (select count(*) from contribution) + (select count(*) from revision) + (select count(*) from audit_event) + (select count(*) from river_job where kind = $1)",
			jobs.KindReconsiderDerived)
	}
	before := count()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.App.Contribute(ctx, tt.caller, tt.c()); !errors.Is(err, tt.want) {
				t.Errorf("Contribute() = %v, want %v", err, tt.want)
			}
			if after := count(); after != before {
				t.Errorf("a refused contribution wrote %d rows", after-before)
			}
		})
	}
}
