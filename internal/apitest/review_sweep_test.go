//go:build integration

package apitest

import (
	"context"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/wire"
)

// TestReviewSweepFlagsOnRecords: the dreamer's review sweep marks a record
// past its review date as due for review and a record unchecked for longer
// than the area's freshness policy as stale, and get_record and search show
// the marks, while the records keep their state, validity and revisions
// (PR-33). The review dates and checks are set through the admin pool, as
// the review tools of slice 3 will.
func TestReviewSweepFlagsOnRecords(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	env.WaitStartupSweep(t)
	bob := env.Client("bob")
	out, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-review-sweep", Rationale: "review sweep",
		Entities: []wire.EntityInput{{Ref: "svc", Type: "service", Names: []wire.Name{{Name: "Ledger Service"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "runbook", SourceURI: "https://github.com/org/repo/blob/main/docs/ledger.md", SourceRevision: "main@2026-10-01"},
			{Ref: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/501", SourceRevision: "2"},
		},
		Assertions: []wire.AssertionInput{
			{Ref: "due", Kind: "observation", Subject: "svc", Evidence: []string{"runbook"}, BodyMarkdown: "Ledger reconciliation runs nightly at 02:00"},
			{Ref: "old", Kind: "observation", Subject: "svc", Evidence: []string{"ticket"}, BodyMarkdown: "Ledger exports are capped at 10,000 rows"},
		},
	})
	if err != nil {
		t.Fatalf("contribute: %v", err)
	}
	w := written(out)
	due, old := w["due"].ID, w["old"].ID
	if _, err := env.DB.Admin.Pgx().Exec(ctx, "update assertion set review_due_at = now() - interval '1 day' where id = $1", due); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Admin.Pgx().Exec(ctx, "update assertion set verified_at = now() - interval '100 days' where id = $1", old); err != nil {
		t.Fatal(err)
	}
	get := func(id string) wire.Record {
		t.Helper()
		rec, err := bob.GetRecord(ctx, wire.GetRecordInput{ID: id})
		if err != nil {
			t.Fatalf("get_record %s: %v", id, err)
		}
		return rec
	}

	// Before the sweep (the start-up sweep ran before these records were
	// seeded): the live stale label is already there; the sweep's marks
	// are not.
	if r := get(due); r.DueForReview || r.StaleAt != nil {
		t.Errorf("before the sweep, due = %+v; want no marks", r)
	}
	if r := get(old); !r.Stale || r.StaleAt != nil {
		t.Errorf("before the sweep, old = %+v; want live stale and no stale_at", r)
	}

	sweptAt := time.Now()
	if _, err := env.Dreamer.SweepReview(ctx, sweptAt); err != nil {
		t.Fatal(err)
	}
	r := get(due)
	if !r.DueForReview || r.StaleAt != nil || r.ReviewSweptAt == nil || !r.ReviewSweptAt.Equal(sweptAt.UTC().Truncate(time.Microsecond)) {
		t.Errorf("after the sweep, due = %+v; want due for review, not stale, swept at %s", r, sweptAt)
	}
	if r.LifecycleState != "active" || r.Revision != 1 || r.ValidTo != nil || r.Status != wire.StatusCurrent {
		t.Errorf("the sweep changed the record: %+v", r)
	}
	r = get(old)
	if r.DueForReview || !r.Stale || r.StaleAt == nil || r.Revision != 1 || r.LifecycleState != "active" {
		t.Errorf("after the sweep, old = %+v; want stale at the sweep, unchanged otherwise", r)
	}
	hist, err := bob.GetHistory(ctx, wire.GetHistoryInput{ID: due})
	if err != nil || len(hist.Revisions) != 1 {
		t.Errorf("history of the due record: %d revisions, %v; want 1", len(hist.Revisions), err)
	}

	// Search shows the same marks; the stale record is still returned in
	// current mode.
	res, err := bob.Search(ctx, wire.SearchInput{Query: "ledger"})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]wire.Record{}
	for _, h := range res.Results {
		found[h.Record.ID] = h.Record
	}
	if !found[due].DueForReview || found[old].StaleAt == nil || !found[old].Stale {
		t.Errorf("search results: due %+v, old %+v; want the sweep's marks", found[due], found[old])
	}
}
