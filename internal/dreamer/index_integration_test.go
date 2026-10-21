//go:build integration

package dreamer

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

func TestIndexJobsKeepProjectionsCurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db := storetest.Open(t)
	pool := db.App

	area := domain.Area{ID: domain.ID(uuid.NewString()), Key: "demo", Name: "Demo"}
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

	// Before any worker runs: two records, both stale, two index jobs pending.
	st, err := pool.Staleness(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 2 || st.Stale != 2 {
		t.Fatalf("staleness before = %+v, want 2/2", st)
	}
	// A second write to the entity before the worker runs leaves one job for it.
	ent2 := *ent
	if err := ent2.AddName(domain.EntityName{Name: "Importer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.UpdateEntity(ctx, caller, &ent2, 1, "alias"); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := db.Admin.Pgx().QueryRow(ctx, "select count(*) from river_job where kind = $1 and args->>'record_id' = $2 and state in ('available','scheduled','pending','retryable')",
		jobs.KindIndexRecord, string(ent.ID())).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("pending index jobs for the entity = %d, want 1 (burst collapsed)", pending)
	}

	d, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), pool, testSchedule)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Run(runCtx) }()
	waitFor(t, func() bool {
		s, err := pool.Staleness(ctx)
		return err == nil && s.Total == 2 && s.Stale == 0
	}, 60*time.Second, "records indexed by the worker")

	is, err := pool.GetIndexState(ctx, ent.ID())
	if err != nil {
		t.Fatal(err)
	}
	if is.Revision != 2 || is.IndexedAt.IsZero() || is.RecordType != store.RecordEntity {
		t.Fatalf("index state = %+v, want revision 2", is)
	}

	// Rebuild covers every record of the area and is unique per area while waiting.
	before := is.IndexedAt
	time.Sleep(10 * time.Millisecond)
	if _, err := pool.Jobs().Insert(ctx, jobs.RebuildIndexArgs{AreaID: string(area.ID)}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		s, err := pool.GetIndexState(ctx, obs.ID)
		return err == nil && s.IndexedAt.After(before)
	}, 60*time.Second, "rebuild re-indexed the observation")

	rec := httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	var body struct {
		IndexTotal int `json:"index_total"`
		IndexStale int `json:"index_stale"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || body.IndexTotal != 2 || body.IndexStale != 0 {
		t.Fatalf("health = %d %s", rec.Code, rec.Body.String())
	}

	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
