//go:build integration

package store_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

// jobCaller is how the dreamer's reconsider_derived worker calls the store.
func jobCaller(area domain.Area) store.Caller {
	return store.Caller{UserID: "froodbase-dreamer", AreaIDs: []string{string(area.ID)}, Job: jobs.KindReconsiderDerived}
}

// waitingReconsider counts reconsider_derived jobs waiting for a record.
func waitingReconsider(ctx context.Context, t *testing.T, db storetest.DB, record domain.ID) int {
	t.Helper()
	return countAdmin(ctx, t, db.Admin, "select count(*) from river_job where kind = $1 and args->>'record_id' = $2 and state in ('available','scheduled','pending','retryable')",
		jobs.KindReconsiderDerived, string(record))
}

// chain writes the fixture's observation O, an inference I1 derived from it
// and an inference I2 derived from I1.
func chain(ctx context.Context, t *testing.T, db storetest.DB, area domain.Area, caller store.Caller) (fixture, domain.Assertion, domain.Assertion) {
	t.Helper()
	f := newFixture(t, area, "The API returned 503s.") // roots ev1 and ev3
	if _, err := db.App.Contribute(ctx, caller, f.contribution); err != nil {
		t.Fatal(err)
	}
	i1, l1 := deduction(t, domain.KindInference, area.ID, f.entity.ID(), "The 503s come from the gateway.", f.assertion.ID)
	i2, l2 := deduction(t, domain.KindInference, area.ID, f.entity.ID(), "The gateway needs more instances.", i1.ID)
	if _, err := db.App.Contribute(ctx, caller, contributionOf(area.ID, []domain.Assertion{i2, i1}, append(l2, l1...))); err != nil {
		t.Fatal(err)
	}
	return f, i1, i2
}

func revisionCount(ctx context.Context, t *testing.T, db storetest.DB) int {
	t.Helper()
	return countAdmin(ctx, t, db.Admin, "select count(*) from revision")
}

// A new independent root on an observation reaches every inference derived
// from it, directly or through another inference: one root_added each,
// named for the job and inherited from the direct source, scores
// re-projected, no revisions. Writing the deductions enqueues nothing;
// linking the root enqueues one job for the observation; a second walk
// changes nothing.
func TestReconsiderDerivedPropagatesNewRoots(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f, i1, i2 := chain(ctx, t, db, areaA, callerA)
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from river_job where kind = $1", jobs.KindReconsiderDerived); n != 0 {
		t.Fatalf("writing records enqueued %d reconsider_derived jobs, want 0", n)
	}

	ev4 := evidence(areaA.ID, "https://github.com/org/repo/blob/main/gateway.go", "")
	if _, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev4}, "gateway logs"); err != nil {
		t.Fatal(err)
	}
	if n := waitingReconsider(ctx, t, db, f.assertion.ID); n != 1 {
		t.Fatalf("waiting reconsider_derived jobs for the observation = %d, want 1", n)
	}
	// Before the walk the inferences still have the observation's two roots.
	if res, err := db.App.SupportEvents(ctx, callerA, i2.ID); err != nil || len(res) != 2 {
		t.Fatalf("i2 events before the walk = %d, %v; want 2", len(res), err)
	}

	revs := revisionCount(ctx, t, db)
	res, err := db.App.ReconsiderDerived(ctx, jobCaller(areaA), f.assertion.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Walked, []domain.ID{i1.ID, i2.ID}) {
		t.Errorf("walked %v, want [i1 i2]", res.Walked)
	}
	want := map[domain.ID][]domain.ID{i1.ID: {ev4.ID}, i2.ID: {ev4.ID}}
	if !reflect.DeepEqual(res.Gained, want) {
		t.Errorf("gained %v, want %v", res.Gained, want)
	}
	w := domain.DefaultSupportConfig().EventWeights[domain.SupportRootAdded]
	for _, id := range []domain.ID{i1.ID, i2.ID} {
		if res.Scores[id] != 3*w {
			t.Errorf("score of %s = %v, want %v", id, res.Scores[id], 3*w)
		}
		a, err := db.App.GetAssertion(ctx, callerA, id)
		if err != nil || a.SupportScore != 3*w || a.Revision != 1 {
			t.Errorf("stored %s = %+v, %v; want score %v at revision 1", id, a, err, 3*w)
		}
		if score, err := db.App.RecomputeSupportScore(ctx, callerA, id); err != nil || score != 3*w {
			t.Errorf("recomputed %s = %v, %v", id, score, err)
		}
	}
	if after := revisionCount(ctx, t, db); after != revs {
		t.Errorf("the walk wrote %d revisions, want none", after-revs)
	}
	for id, from := range map[domain.ID]domain.ID{i1.ID: f.assertion.ID, i2.ID: i1.ID} {
		events, err := db.App.SupportEvents(ctx, callerA, id)
		if err != nil {
			t.Fatal(err)
		}
		last := events[len(events)-1]
		if len(events) != 3 || last.EvidenceRootIDs[0] != ev4.ID || last.JobName != jobs.KindReconsiderDerived || last.ActorUserID != "" ||
			last.Rationale != "inherited from "+string(from) || last.SourceKind != domain.SourceRepository {
			t.Errorf("events of %s = %+v; want a third root_added for ev4 by the job, inherited from %s", id, events, from)
		}
	}
	// The walk enqueued nothing more for the inferences it changed.
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from river_job where kind = $1", jobs.KindReconsiderDerived); n != 1 {
		t.Errorf("reconsider_derived jobs = %d, want only the observation's", n)
	}

	again, err := db.App.ReconsiderDerived(ctx, jobCaller(areaA), f.assertion.ID, nil)
	if err != nil || len(again.Gained) != 0 || len(again.Walked) != 2 {
		t.Errorf("second walk = %+v, %v; want nothing gained", again, err)
	}
}

// A root a deduction already counts adds nothing: cited by the deduction
// directly, or already reaching it through another source.
func TestReconsiderDerivedSkipsCountedRoots(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f := newFixture(t, areaA, "The API returned 503s.")
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}
	g := newFixture(t, areaA, "The queue backed up.")
	g.entity = f.entity
	g.contribution.Entities = nil
	g.assertion.SubjectEntityID = f.entity.ID()
	g.contribution.Assertions = []domain.Assertion{g.assertion}
	g.ev1.SourceURI = "https://github.com/org/repo/blob/main/queue.go"
	g.ev3.SourceURI = "https://dev.azure.com/org/p/_workitems/edit/43"
	g.contribution.Evidence = []domain.Evidence{g.ev1, g.ev3}
	g.contribution.ClaimEvidence = map[domain.ID][]domain.ID{g.assertion.ID: {g.ev1.ID, g.ev3.ID}}
	if _, err := db.App.Contribute(ctx, callerA, g.contribution); err != nil {
		t.Fatal(err)
	}
	// The inference cites its own ticket and is derived from both observations.
	direct := evidence(areaA.ID, "https://dev.azure.com/org/p/_workitems/edit/77", "")
	direct.SourceKind = domain.SourceTicket
	inf, links := deduction(t, domain.KindInference, areaA.ID, f.entity.ID(), "The queue caused the 503s.", f.assertion.ID, g.assertion.ID)
	c := contributionOf(areaA.ID, []domain.Assertion{inf}, links)
	c.Evidence = []domain.Evidence{direct}
	c.ClaimEvidence[inf.ID] = []domain.ID{direct.ID}
	if _, err := db.App.Contribute(ctx, callerA, c); err != nil {
		t.Fatal(err)
	}
	before, err := db.App.SupportEvents(ctx, callerA, inf.ID)
	if err != nil || len(before) != 5 {
		t.Fatalf("inference events = %d, %v; want 5 roots", len(before), err)
	}

	// f gains the inference's direct ticket and g's queue file: both counted.
	if _, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{direct, g.ev1}, "also seen"); err != nil {
		t.Fatal(err)
	}
	res, err := db.App.ReconsiderDerived(ctx, jobCaller(areaA), f.assertion.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Walked, []domain.ID{inf.ID}) || len(res.Gained) != 0 {
		t.Errorf("walk = %+v; want the inference walked and nothing gained", res)
	}
	after, err := db.App.SupportEvents(ctx, callerA, inf.ID)
	if err != nil || len(after) != len(before) {
		t.Errorf("inference events after = %d, %v; want unchanged", len(after), err)
	}
}

// The walk stops on cycles: a loop between two deductions (which the
// writer never creates; inserted here as the owner) is walked once.
func TestReconsiderDerivedStopsOnCycles(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f, i1, i2 := chain(ctx, t, db, areaA, callerA)
	if _, err := db.Admin.Pgx().Exec(ctx, "insert into derivation (derived_id, source_id, area_id, recorded_at) values ($1, $2, $3, now())",
		string(i1.ID), string(i2.ID), string(areaA.ID)); err != nil {
		t.Fatal(err)
	}
	ev4 := evidence(areaA.ID, "https://github.com/org/repo/blob/main/gateway.go", "")
	if _, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev4}, "gateway logs"); err != nil {
		t.Fatal(err)
	}
	res, err := db.App.ReconsiderDerived(ctx, jobCaller(areaA), f.assertion.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Walked, []domain.ID{i1.ID, i2.ID}) {
		t.Errorf("walked %v, want each of the loop once", res.Walked)
	}
	if len(res.Gained[i1.ID]) != 1 || len(res.Gained[i2.ID]) != 1 {
		t.Errorf("gained %v, want ev4 once each", res.Gained)
	}
}

// A job acts within its area only: walking from a record in another area
// is ErrNotFound and writes nothing; and repeated root additions to one
// record before the dreamer runs leave one waiting job, while a record
// nothing is derived from enqueues none.
func TestReconsiderDerivedAreaAndUniqueness(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	areaB, _ := area(ctx, t, db, "area-b")
	f, _, i2 := chain(ctx, t, db, areaA, callerA)

	for i, uri := range []string{"https://github.com/org/repo/blob/main/x.go", "https://github.com/org/repo/blob/main/y.go"} {
		ev := evidence(areaA.ID, uri, "")
		if _, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev}, "more"); err != nil {
			t.Fatal(err)
		}
		if n := waitingReconsider(ctx, t, db, f.assertion.ID); n != 1 {
			t.Fatalf("after link %d: waiting jobs = %d, want 1", i+1, n)
		}
	}
	// i2 has nothing derived from it: a root linked to it enqueues nothing.
	ev := evidence(areaA.ID, "https://github.com/org/repo/blob/main/z.go", "")
	if _, err := db.App.LinkEvidence(ctx, callerA, i2.ID, []domain.Evidence{ev}, "direct"); err != nil {
		t.Fatal(err)
	}
	if n := waitingReconsider(ctx, t, db, i2.ID); n != 0 {
		t.Errorf("waiting jobs for a record with no dependants = %d, want 0", n)
	}
	if n, err := db.App.WaitingJobs(ctx, jobs.KindReconsiderDerived); err != nil || n != 1 {
		t.Errorf("WaitingJobs = %d, %v; want 1", n, err)
	}

	events := countAdmin(ctx, t, db.Admin, "select count(*) from support_event")
	if _, err := db.App.ReconsiderDerived(ctx, jobCaller(areaB), f.assertion.ID, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("walk from another area = %v, want ErrNotFound", err)
	}
	if after := countAdmin(ctx, t, db.Admin, "select count(*) from support_event"); after != events {
		t.Errorf("a refused walk wrote %d support events", after-events)
	}
}
