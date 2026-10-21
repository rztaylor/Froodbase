//go:build integration

package dreamer

import (
	"context"
	"encoding/json"
	"fmt"
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

func repoEvidence(area domain.ID, path string) domain.Evidence {
	return domain.Evidence{ID: domain.ID(uuid.NewString()), AreaID: area, SourceKind: domain.SourceRepository,
		SourceURI: "https://github.com/org/repo/blob/main/" + path, SourceRevision: "abc123", AccessClass: domain.AccessArea, RecordedAt: time.Now()}
}

func reconsiderHealth(t *testing.T, d *Dreamer) int {
	t.Helper()
	rec := httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	var body struct {
		ReconsiderWaiting *int `json:"reconsider_waiting"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || body.ReconsiderWaiting == nil {
		t.Fatalf("health = %d %s", rec.Code, rec.Body.String())
	}
	return *body.ReconsiderWaiting
}

// An observation, an inference derived from it and a second inference
// derived from that: a new root linked to the observation reaches both
// inferences through the running dreamer, whose job completes and leaves
// nothing waiting; roots linked while the dreamer is working all arrive.
func TestReconsiderDerivedIsWorked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db := storetest.Open(t)
	pool := db.App

	area := domain.Area{ID: domain.ID(uuid.NewString()), Key: "demo", Name: "Demo"}
	if err := pool.CreateArea(ctx, store.Caller{UserID: "setup"}, area); err != nil {
		t.Fatal(err)
	}
	caller := store.Caller{UserID: "bob", AreaIDs: []string{string(area.ID)}}
	ent, err := domain.NewEntity(domain.ID(uuid.NewString()), area.ID, domain.EntityService, domain.EntityName{Name: "Gateway"})
	if err != nil {
		t.Fatal(err)
	}
	ev := repoEvidence(area.ID, "a.go")
	obs, err := domain.NewObservation(domain.ID(uuid.NewString()), area.ID, ent.ID(), "Gateway returned 503s", domain.Scope{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	i1, err := domain.NewInference(domain.ID(uuid.NewString()), area.ID, ent.ID(), "The gateway is undersized", domain.Scope{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	i2, err := domain.NewInference(domain.ID(uuid.NewString()), area.ID, ent.ID(), "Scaling the gateway would stop the 503s", domain.Scope{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	c := store.Contribution{
		Contribution:  domain.Contribution{ID: domain.ID(uuid.NewString()), AreaID: area.ID, ActorUserID: "bob", RecordedAt: now},
		Entities:      []*domain.Entity{ent},
		Evidence:      []domain.Evidence{ev},
		Assertions:    []domain.Assertion{obs, i1, i2},
		ClaimEvidence: map[domain.ID][]domain.ID{obs.ID: {ev.ID}},
		Derivations: []domain.Derivation{
			{DerivedID: i1.ID, SourceID: obs.ID, AreaID: area.ID, RecordedAt: now},
			{DerivedID: i2.ID, SourceID: i1.ID, AreaID: area.ID, RecordedAt: now},
		},
	}
	if _, err := pool.Contribute(ctx, caller, c); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.LinkEvidence(ctx, caller, obs.ID, []domain.Evidence{repoEvidence(area.ID, "b.go")}, "second file"); err != nil {
		t.Fatal(err)
	}

	d, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), pool, testSchedule)
	if err != nil {
		t.Fatal(err)
	}
	if n := reconsiderHealth(t, d); n != 1 {
		t.Fatalf("reconsider_waiting before the dreamer runs = %d, want 1", n)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Run(runCtx) }()

	roots := func(id domain.ID) int {
		events, err := pool.SupportEvents(ctx, caller, id)
		if err != nil {
			t.Fatal(err)
		}
		return len(events)
	}
	waitFor(t, func() bool { return roots(i1.ID) == 2 && roots(i2.ID) == 2 }, 60*time.Second, "both inferences gaining the new root")

	// Roots linked while the dreamer works: every one reaches the second
	// inference, however the links and the walks interleave.
	const more = 8
	for i := 0; i < more; i++ {
		if _, err := pool.LinkEvidence(ctx, caller, obs.ID, []domain.Evidence{repoEvidence(area.ID, fmt.Sprintf("more-%d.go", i))}, "more"); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, func() bool { return roots(i2.ID) == 2+more }, 60*time.Second, "every later root reaching the second inference")
	waitFor(t, func() bool { return reconsiderHealth(t, d) == 0 }, 30*time.Second, "no propagation job waiting")

	var completed int
	if err := db.Admin.Pgx().QueryRow(ctx, "select count(*) from river_job where kind = $1 and state = 'completed'", jobs.KindReconsiderDerived).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed < 2 {
		t.Errorf("completed reconsider_derived jobs = %d, want at least 2", completed)
	}
	a, err := pool.GetAssertion(ctx, caller, i2.ID)
	if err != nil || a.Revision != 1 {
		t.Errorf("second inference = %+v, %v; want revision 1 (support changes write no revision)", a, err)
	}

	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
