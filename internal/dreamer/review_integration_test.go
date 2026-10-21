//go:build integration

package dreamer

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

// testSchedule is the dreamer's schedule in integration tests: the default
// interval, so the review sweep runs once at start and then not again.
var testSchedule = Schedule{ReviewSweepInterval: time.Hour}

// seedObservation writes one observation in a new area and returns the
// area and the assertion's id.
func seedObservation(ctx context.Context, t *testing.T, pool *store.Pool, key string) (domain.Area, store.Caller, domain.ID) {
	t.Helper()
	area := domain.Area{ID: domain.ID(uuid.NewString()), Key: key, Name: key}
	if err := pool.CreateArea(ctx, store.Caller{UserID: "setup"}, area); err != nil {
		t.Fatal(err)
	}
	caller := store.Caller{UserID: "bob", AreaIDs: []string{string(area.ID)}}
	ent, err := domain.NewEntity(domain.ID(uuid.NewString()), area.ID, domain.EntityService, domain.EntityName{Name: "Import Tool"})
	if err != nil {
		t.Fatal(err)
	}
	ev := domain.Evidence{ID: domain.ID(uuid.NewString()), AreaID: area.ID, SourceKind: domain.SourceTicket, SourceURI: "https://dev.azure.com/o/p/_workitems/edit/1", SourceRevision: "r1", AccessClass: domain.AccessArea, RecordedAt: time.Now()}
	obs, err := domain.NewObservation(domain.ID(uuid.NewString()), area.ID, ent.ID(), "Callback failure on 2.3", domain.Scope{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c := store.Contribution{
		Contribution:  domain.Contribution{ID: domain.ID(uuid.NewString()), AreaID: area.ID, ActorUserID: "bob", RecordedAt: time.Now()},
		Entities:      []*domain.Entity{ent},
		Evidence:      []domain.Evidence{ev},
		Assertions:    []domain.Assertion{obs},
		ClaimEvidence: map[domain.ID][]domain.ID{obs.ID: {ev.ID}},
	}
	if _, err := pool.Contribute(ctx, caller, c); err != nil {
		t.Fatal(err)
	}
	return area, caller, obs.ID
}

// The periodic review sweep runs on start and marks, in each area, a
// record past its review date and one last checked longer ago than the
// policy allows, through a job that acts in one area at a time.
func TestReviewSweepJobMarksEveryArea(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db := storetest.Open(t)
	pool := db.App
	_, dueCaller, due := seedObservation(ctx, t, pool, "due")
	_, staleCaller, stale := seedObservation(ctx, t, pool, "stale")
	if _, err := db.Admin.Pgx().Exec(ctx, "update assertion set review_due_at = now() - interval '1 day' where id = $1", string(due)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Admin.Pgx().Exec(ctx, "update assertion set verified_at = now() - interval '100 days' where id = $1", string(stale)); err != nil {
		t.Fatal(err)
	}

	d, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), pool, testSchedule)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Run(runCtx) }()

	// RunOnStart inserts this slot's sweep; wait for it to complete.
	slot := jobs.ReviewSweepFor(time.Now(), testSchedule.ReviewSweepInterval).ScheduledFor
	deadline := time.Now().Add(90 * time.Second)
	for {
		var n int
		if err := db.Admin.Pgx().QueryRow(ctx, "select count(*) from river_job where kind = $1 and state = 'completed'",
			jobs.KindReviewSweep).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no review sweep for slot %s completed within the deadline", slot)
		}
		time.Sleep(250 * time.Millisecond)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if f, err := pool.ReviewFlags(ctx, dueCaller, due); err != nil || !f.DueForReview || f.StaleAt != nil || f.SweptAt == nil {
		t.Errorf("record past its review date: %+v, %v; want due, not stale, swept", f, err)
	}
	if f, err := pool.ReviewFlags(ctx, staleCaller, stale); err != nil || f.DueForReview || f.StaleAt == nil || f.SweptAt == nil {
		t.Errorf("record verified 100 days ago: %+v, %v; want stale, not due, swept", f, err)
	}
	// The sweep wrote no support event and no revision: each record has
	// only the revision its contribution wrote.
	var revisions, events int
	if err := db.Admin.Pgx().QueryRow(ctx, "select (select count(*) from revision where record_id in ($1, $2)), (select count(*) from support_event where job_name = $3)",
		string(due), string(stale), jobs.KindReviewSweep).Scan(&revisions, &events); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 || events != 0 {
		t.Errorf("after the sweep: %d revisions of the two records, %d support events by the sweep; want 2 and 0", revisions, events)
	}
	// Sweeping again through the dreamer changes nothing.
	results, err := d.SweepReview(ctx, time.Now())
	if err != nil || len(results) != 2 {
		t.Fatalf("SweepReview = %+v, %v; want two areas", results, err)
	}
	for _, r := range results {
		if r.Changed != 0 || r.Records != 1 {
			t.Errorf("second sweep of %s = %+v, want one record and nothing changed", r.AreaID, r)
		}
	}
}
