//go:build integration

package apitest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/client"
	"github.com/rztaylor/froodbase/internal/domain/domaintest"
	"github.com/rztaylor/froodbase/internal/wire"
)

// TestSlice2Exit is the slice 2 exit and the pitch's worked example, a
// mandatory acceptance test (PR-27): a callback failure reproduced in
// production on 2.3, the fix merged, 2.4 deployed and verified, and an older
// investigation imported weeks later. It drives the service through
// internal/client exactly as froodbase-mcp does, with the bodies, scopes and
// validity dates of domaintest.PitchRecords, the table the Go and SQL mode
// rules are checked against (architecture design section 6). The
// development guide's "The pitch's 2.3 to 2.4 story in Claude"
// (docs/dev/guides/pitch-walkthrough.md) makes the same calls from Claude.
//
// Validity dates are the pitch's; recording times are the service's own
// clock, in the pitch's order, so the import is recorded last. The answers
// are the same on any day after the import's validity ends (9 September
// 2026); staleness, which depends on the day the test runs, is not asserted.
//
// Supersession is declared, not derived, and no tool declares it until
// slice 4: the deployment's supersession of the failure is simulated by
// setting the failure's lifecycle state through the admin pool, as
// TestTwoClocks does, and this test moves to slice 4's correction tool when
// it exists. Nothing here depends on whether the failure's scope (prod, 2.3)
// overlaps the deployment's (prod, 2.4).
func TestSlice2Exit(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	env.WaitStartupSweep(t)
	bob := env.Client("bob")

	failure, merge, deployment, investigation := domaintest.Pitch(domaintest.PitchFailure), domaintest.Pitch(domaintest.PitchMerge),
		domaintest.Pitch(domaintest.PitchDeployment), domaintest.Pitch(domaintest.PitchImport)
	incident := *failure.Record.ValidFrom
	assertion := func(r domaintest.PitchRecord, subject string, evidence ...string) wire.AssertionInput {
		scope := wire.Scope(r.Scope)
		return wire.AssertionInput{
			Ref: r.Name, Kind: string(r.Record.Kind), Subject: subject, BodyMarkdown: r.Body, Scope: &scope,
			ValidFrom: r.Record.ValidFrom, ValidTo: r.Record.ValidTo, Evidence: evidence,
		}
	}
	contribute := func(key string, in wire.ContributeInput) wire.ContributeOutput {
		t.Helper()
		in.Area, in.IdempotencyKey = "demo", key
		out, err := bob.Contribute(ctx, in)
		if err != nil {
			t.Fatalf("contribute %s: %v", key, err)
		}
		return out
	}
	admin := func(sql string, args ...any) {
		t.Helper()
		if _, err := env.DB.Admin.Pgx().Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	ids := map[string]string{} // record id -> pitch name
	search := func(c *client.Client, in wire.SearchInput) map[string]wire.Record {
		t.Helper()
		in.Query, in.Limit = "callback", 50
		res, err := c.Search(ctx, in)
		if err != nil {
			t.Fatalf("search %+v: %v", in, err)
		}
		got := map[string]wire.Record{}
		for _, r := range res.Results {
			if name, ok := ids[r.Record.ID]; ok {
				got[name] = r.Record
			}
		}
		return got
	}
	labels := func(m map[string]wire.Record) map[string]string {
		out := map[string]string{}
		for name, r := range m {
			out[name] = r.Status + "/" + r.LifecycleState
		}
		return out
	}
	expect := func(what string, got map[string]wire.Record, want map[string]string) {
		t.Helper()
		if g := labels(got); !reflect.DeepEqual(g, want) {
			t.Errorf("%s = %v, want %v", what, g, want)
		}
	}
	get := func(id string) wire.Record {
		t.Helper()
		rec, err := bob.GetRecord(ctx, wire.GetRecordInput{ID: id})
		if err != nil {
			t.Fatalf("get_record %s: %v", id, err)
		}
		return rec
	}
	const (
		current, historical = wire.StatusCurrent + "/active", wire.StatusHistorical + "/active"
		superseded          = wire.StatusHistorical + "/superseded"
	)

	// 1. 2 September: the failure is reproduced in production on 2.3, from
	// two independent sources, and checked that day.
	out := contribute("pitch-failure", wire.ContributeInput{
		Rationale: "INC-12: callbacks failing in production",
		Entities:  []wire.EntityInput{{Ref: "svc", Type: "service", Names: []wire.Name{{Name: "Callback Service"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/410", SourceRevision: "3"},
			{Ref: "runbook", SourceURI: "https://github.com/org/callbacks/blob/main/runbooks/repro.md", SourceRevision: "main@2026-09-02"},
		},
		Assertions: []wire.AssertionInput{assertion(failure, "svc", "ticket", "runbook")},
	})
	w := written(out)
	svc, failureID := w["svc"].ID, w[failure.Name].ID
	ids[failureID] = failure.Name
	if s := w[failure.Name].Support; s == nil || s.Roots != 2 {
		t.Fatalf("failure written with support %+v, want two roots", s)
	}
	admin("update assertion set verified_at = $2 where id = $1", failureID, *failure.Record.VerifiedAt)

	// 2. 5 September: the fix is merged. The merge is about the repository,
	// not production: its scope does not overlap the failure's, so it is no
	// correction of it. The failure stays active and current in production,
	// and the two are never answers to each other's scope.
	out = contribute("pitch-merge", wire.ContributeInput{
		Rationale:  "fix merged",
		Evidence:   []wire.EvidenceInput{{Ref: "pr", SourceKind: "repository", SourceURI: "https://github.com/org/callbacks/pull/418", SourceRevision: "merged@2026-09-05"}},
		Assertions: []wire.AssertionInput{assertion(merge, svc, "pr")},
	})
	mergeID := written(out)[merge.Name].ID
	ids[mergeID] = merge.Name
	if r := get(failureID); r.LifecycleState != "active" || r.Revision != 1 || r.Status != wire.StatusCurrent || r.ValidTo != nil {
		t.Errorf("failure after the merge = %+v, want active, current, revision 1, no end date", r)
	}
	failureScope, mergeScope := wire.Scope(failure.Scope), wire.Scope(merge.Scope)
	expect("current after the merge", search(bob, wire.SearchInput{}), map[string]string{failure.Name: current, merge.Name: current})
	expect("current in production on 2.3, after the merge", search(bob, wire.SearchInput{Scope: &failureScope}), map[string]string{failure.Name: current})
	expect("historical in the repository", search(bob, wire.SearchInput{Mode: wire.ModeHistorical, Scope: &mergeScope}), map[string]string{merge.Name: current})

	// 3. 9 September: 2.4 is deployed to production and the remediation
	// verified. The deployment supersedes the failure; until slice 4's
	// correction tool declares that (with the reason "remediated in 2.4"),
	// the state is set directly. The failure's validity is unchanged: nobody
	// asserted an end date.
	out = contribute("pitch-deployment", wire.ContributeInput{
		Rationale:  "2.4 rolled out and verified",
		Evidence:   []wire.EvidenceInput{{Ref: "release", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/410", SourceRevision: "9"}},
		Assertions: []wire.AssertionInput{assertion(deployment, svc, "release")},
	})
	deploymentID := written(out)[deployment.Name].ID
	ids[deploymentID] = deployment.Name
	admin("update assertion set verified_at = $2 where id = $1", deploymentID, *deployment.Record.VerifiedAt)
	admin("update assertion set lifecycle_state = 'superseded' where id = $1", failureID)

	// 4. Now: the current answer leaves the superseded failure out; as of
	// the incident date the failure is the answer, labelled historical; and
	// historical mode returns everything, each labelled.
	expect("current", search(bob, wire.SearchInput{}), map[string]string{merge.Name: current, deployment.Name: current})
	expect("as_of the incident", search(bob, wire.SearchInput{Mode: wire.ModeAsOf, AsOf: &incident}), map[string]string{failure.Name: superseded})
	expect("historical", search(bob, wire.SearchInput{Mode: wire.ModeHistorical}),
		map[string]string{failure.Name: superseded, merge.Name: current, deployment.Name: current})
	if got, err := bob.GetAsOf(ctx, wire.GetAsOfInput{ID: failureID, AsOf: &incident}); err != nil || !got.Valid || got.Record == nil ||
		got.Record.Status != wire.StatusHistorical || got.Record.LifecycleState != "superseded" {
		t.Errorf("get_as_of the failure at the incident = %+v, %v; want valid, labelled historical and superseded", got, err)
	}

	// 5. 13 October: the older investigation is imported. It is recorded
	// today about 2 to 9 September, as an inference of its own and as a
	// further source for the failure. It is historical evidence: the
	// failure gains a root and nothing else, and neither record is current.
	before, err := bob.GetHistory(ctx, wire.GetHistoryInput{ID: failureID})
	if err != nil {
		t.Fatal(err)
	}
	out = contribute("pitch-import", wire.ContributeInput{
		Rationale:  "importing the earlier INC-12 investigation",
		Evidence:   []wire.EvidenceInput{{Ref: "investigation", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/77", SourceRevision: "1"}},
		Assertions: []wire.AssertionInput{assertion(investigation, svc, "investigation")},
		Links:      []wire.LinkInput{{Claim: failureID, Evidence: []string{"investigation"}}},
	})
	importID := written(out)[investigation.Name].ID
	ids[importID] = investigation.Name
	if len(out.Linked) != 1 || out.Linked[0].ID != failureID || out.Linked[0].RootsAdded != 1 || out.Linked[0].Revision != 1 ||
		out.Linked[0].Support == nil || out.Linked[0].Support.Roots != 3 {
		t.Errorf("linked = %+v, want the failure with one root added (three in all) at revision 1", out.Linked)
	}

	imp, dep := get(importID), get(deploymentID)
	if imp.Status != wire.StatusHistorical || imp.LifecycleState != "active" || imp.ValidityUnknown ||
		imp.ValidFrom == nil || !imp.ValidFrom.Equal(incident) || imp.ValidTo == nil || !imp.ValidTo.Equal(*investigation.Record.ValidTo) {
		t.Errorf("import = %+v, want active and historical, valid %s to %s", imp, incident, investigation.Record.ValidTo)
	}
	if !imp.RecordedAt.After(dep.RecordedAt) || !imp.RecordedAt.After(*imp.ValidTo) {
		t.Errorf("import recorded at %s, want after the deployment (%s) and after its validity ended", imp.RecordedAt, dep.RecordedAt)
	}
	f := get(failureID)
	if f.LifecycleState != "superseded" || f.Status != wire.StatusHistorical || f.Revision != 1 || f.ValidTo != nil || f.Support == nil || f.Support.Roots != 3 {
		t.Errorf("failure after the import = %+v, want superseded and historical, revision 1, no end date, three roots", f)
	}
	after, err := bob.GetHistory(ctx, wire.GetHistoryInput{ID: failureID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Revisions, before.Revisions) || len(after.SupportEvents) != len(before.SupportEvents)+1 ||
		after.SupportEvents[len(after.SupportEvents)-1].Type != "root_added" {
		t.Errorf("failure history after the import: %d revisions (was %d), support events %+v; want the same revisions and one root_added more",
			len(after.Revisions), len(before.Revisions), after.SupportEvents)
	}

	expect("current after the import", search(bob, wire.SearchInput{}), map[string]string{merge.Name: current, deployment.Name: current})
	expect("as_of the incident, after the import", search(bob, wire.SearchInput{Mode: wire.ModeAsOf, AsOf: &incident}),
		map[string]string{failure.Name: superseded, investigation.Name: historical})
	expect("historical after the import", search(bob, wire.SearchInput{Mode: wire.ModeHistorical}),
		map[string]string{failure.Name: superseded, merge.Name: current, deployment.Name: current, investigation.Name: historical})

	// 6. The dreamer's maintenance does not reopen it either: a review sweep
	// leaves the superseded failure unmarked and unchanged, and the current
	// answer stays the same.
	if _, err := env.Dreamer.SweepReview(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if f := get(failureID); f.LifecycleState != "superseded" || f.Revision != 1 || f.DueForReview || f.StaleAt != nil || f.ReviewSweptAt == nil {
		t.Errorf("failure after a review sweep = %+v, want swept, unmarked, superseded at revision 1", f)
	}
	expect("current after a review sweep", search(env.Client("carol"), wire.SearchInput{}), map[string]string{merge.Name: current, deployment.Name: current})
}
