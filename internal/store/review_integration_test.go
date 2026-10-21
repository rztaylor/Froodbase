//go:build integration

package store_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/domain/domaintest"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

// The review sweep (FRB-16): flags as projections beside the records, never
// revisions, computed by the same SQL functions as the live labels.

var sweepNow = domaintest.Day(2026, 12, 1)

func daysBefore(n int) time.Time { return sweepNow.Add(-time.Duration(n) * 24 * time.Hour) }

func TestSQLDueForReviewMatchesDomainCases(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	for _, c := range domaintest.DueCases() {
		var got bool
		if err := db.App.Pgx().QueryRow(ctx, "select froodbase_due_for_review($1, $2, $3)",
			string(c.Record.State), c.Record.ReviewDueAt, c.Now).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if got != c.Want {
			t.Errorf("%s: froodbase_due_for_review = %v, want %v", c.Name, got, c.Want)
		}
	}
}

// reviewSpec is one seeded assertion: its kind and state, and the review
// clock set on it (nil leaves the field null).
type reviewSpec struct {
	kind      domain.EpistemicKind
	state     domain.LifecycleState
	verified  *time.Time
	reviewDue *time.Time
}

func ptrTime(t time.Time) *time.Time { return &t }

// seedReview writes one assertion per spec in a new area and sets each
// one's state and review clock through the admin pool, as the review tools
// of later slices will. It returns the area, a caller holding it and the
// assertion id per name.
func seedReview(ctx context.Context, t *testing.T, db storetest.DB, key string, specs map[string]reviewSpec) (domain.Area, store.Caller, map[string]domain.ID) {
	t.Helper()
	a, caller := area(ctx, t, db, key)
	e, err := domain.NewEntity(domain.ID(store.NewID()), a.ID, domain.EntityService, domain.EntityName{Name: "Review Service " + key})
	if err != nil {
		t.Fatal(err)
	}
	c := store.Contribution{
		Contribution:  domain.Contribution{ID: domain.ID(store.NewID()), AreaID: a.ID, ActorUserID: "alice", Client: "test", RecordedAt: t0},
		Entities:      []*domain.Entity{e},
		ClaimEvidence: map[domain.ID][]domain.ID{},
		Refs:          map[domain.ID]string{},
	}
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	slices.Sort(names)
	ids := map[string]domain.ID{}
	for _, name := range names {
		ev := evidence(a.ID, "https://github.com/org/repo/blob/main/review.go", "")
		ev.SourceLocator = name
		as := domain.Assertion{
			ID: domain.ID(store.NewID()), AreaID: a.ID, SubjectEntityID: e.ID(), Kind: specs[name].kind,
			BodyMarkdown: "Review case " + name, Temporal: domain.Temporal{RecordedAt: t0},
		}
		c.Evidence = append(c.Evidence, ev)
		c.Assertions = append(c.Assertions, as)
		c.ClaimEvidence[as.ID] = []domain.ID{ev.ID}
		ids[name] = as.ID
	}
	if _, err := db.App.Contribute(ctx, caller, c); err != nil {
		t.Fatalf("seeding %s: %v", key, err)
	}
	for name, s := range specs {
		if _, err := db.Admin.Pgx().Exec(ctx, "update assertion set lifecycle_state = $2, verified_at = $3, review_due_at = $4 where id = $1",
			string(ids[name]), string(s.state), s.verified, s.reviewDue); err != nil {
			t.Fatal(err)
		}
	}
	return a, caller, ids
}

// recordState is everything about the records and their history that the
// sweep must leave alone.
func recordState(ctx context.Context, t *testing.T, db storetest.DB) []string {
	t.Helper()
	rows, err := db.Admin.Pgx().Query(ctx, `
		select 'assertion ' || a.id || ' ' || a.lifecycle_state || ' ' || coalesce(a.valid_from::text, '-') || ' ' || coalesce(a.valid_to::text, '-')
		       || ' ' || coalesce(a.verified_at::text, '-') || ' ' || coalesce(a.review_due_at::text, '-') || ' r' || a.revision
		       || ' ' || a.updated_at || ' ' || a.support_score || ' ' || a.needs_reconsideration
		from assertion a
		union all select 'revision ' || record_id || ' r' || revision || ' ' || md5(snapshot::text) from revision
		union all select 'support_event ' || id from support_event
		union all select 'audit_event ' || id from audit_event
		union all select 'entity ' || id || ' ' || lifecycle_state || ' r' || revision from entity
		order by 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func reviewSpecs() map[string]reviewSpec {
	obs, dec, inf := domain.KindObservation, domain.KindDecision, domain.KindInference
	active, retracted, superseded := domain.LifecycleActive, domain.LifecycleRetracted, domain.LifecycleSuperseded
	return map[string]reviewSpec{
		"past due":                  {obs, active, ptrTime(daysBefore(1)), ptrTime(daysBefore(1))},
		"due later":                 {obs, active, ptrTime(daysBefore(1)), ptrTime(sweepNow.Add(30 * 24 * time.Hour))},
		"verified 100 days ago":     {obs, active, ptrTime(daysBefore(100)), nil},
		"inference 91 days old":     {inf, active, ptrTime(daysBefore(91)), nil},
		"verified 10 days ago":      {obs, active, ptrTime(daysBefore(10)), nil},
		"decision 1000 days old":    {dec, active, ptrTime(daysBefore(1000)), ptrTime(daysBefore(5))},
		"retracted, old, past due":  {obs, retracted, ptrTime(daysBefore(400)), ptrTime(daysBefore(5))},
		"superseded, old, past due": {obs, superseded, ptrTime(daysBefore(400)), ptrTime(daysBefore(5))},
	}
}

func TestSweepReviewMarksDueAndStaleWithoutChangingRecords(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	a, caller, ids := seedReview(ctx, t, db, "review", reviewSpecs())
	sweeper := store.Caller{UserID: "froodbase-dreamer", AreaIDs: []string{string(a.ID)}, Job: jobs.KindReviewSweep}

	before := recordState(ctx, t, db)
	res, err := db.App.SweepReview(ctx, sweeper, a.ID, sweepNow)
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != len(ids) || res.Due != 2 || res.Stale != 2 || res.Changed != 4 {
		t.Errorf("sweep = %+v, want 8 records, 2 due, 2 stale, 4 changed", res)
	}
	if after := recordState(ctx, t, db); !reflect.DeepEqual(after, before) {
		t.Errorf("the sweep changed records or history:\nbefore %v\nafter  %v", before, after)
	}

	wantDue := map[string]bool{"past due": true, "decision 1000 days old": true}
	wantStale := map[string]bool{"verified 100 days ago": true, "inference 91 days old": true}
	for name, id := range ids {
		flags, err := db.App.ReviewFlags(ctx, caller, id)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if flags.DueForReview != wantDue[name] {
			t.Errorf("%s: due_for_review = %v, want %v", name, flags.DueForReview, wantDue[name])
		}
		if wantStale[name] != (flags.StaleAt != nil) || (flags.StaleAt != nil && !flags.StaleAt.Equal(sweepNow)) {
			t.Errorf("%s: stale_at = %v, want stale %v at %s", name, flags.StaleAt, wantStale[name], sweepNow)
		}
		if flags.SweptAt == nil || !flags.SweptAt.Equal(sweepNow) {
			t.Errorf("%s: swept_at = %v, want %s", name, flags.SweptAt, sweepNow)
		}
		// The live stale label and the sweep agree: one SQL function.
		live, err := db.App.ClassifyAssertion(ctx, caller, id, domain.CurrentQuery(), sweepNow)
		if err != nil {
			t.Fatal(err)
		}
		if live.Stale != (flags.StaleAt != nil) {
			t.Errorf("%s: live stale %v, sweep stale_at %v", name, live.Stale, flags.StaleAt)
		}
	}

	// Sweeping again changes no flag row, and a record stale since the
	// first sweep keeps that time.
	later := sweepNow.Add(24 * time.Hour)
	res, err = db.App.SweepReview(ctx, sweeper, a.ID, later)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed != 0 || res.Due != 2 || res.Stale != 2 {
		t.Errorf("second sweep = %+v, want 2 due, 2 stale, nothing changed", res)
	}
	flags, err := db.App.ReviewFlags(ctx, caller, ids["verified 100 days ago"])
	if err != nil || flags.StaleAt == nil || !flags.StaleAt.Equal(sweepNow) || !flags.SweptAt.Equal(later) {
		t.Errorf("after the second sweep: %+v, %v; want stale_at %s, swept_at %s", flags, err, sweepNow, later)
	}

	// A check and a new review date clear the flags at the next sweep; the
	// records themselves were changed by those writes, not by the sweep.
	if _, err := db.Admin.Pgx().Exec(ctx, "update assertion set verified_at = $2 where id = $1", string(ids["verified 100 days ago"]), later); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Admin.Pgx().Exec(ctx, "update assertion set review_due_at = $2 where id = $1", string(ids["past due"]), later.Add(30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	before = recordState(ctx, t, db)
	if res, err = db.App.SweepReview(ctx, sweeper, a.ID, later); err != nil || res.Changed != 2 || res.Due != 1 || res.Stale != 1 {
		t.Errorf("sweep after a check = %+v, %v; want 1 due, 1 stale, 2 changed", res, err)
	}
	if after := recordState(ctx, t, db); !reflect.DeepEqual(after, before) {
		t.Error("the sweep that cleared flags changed records or history")
	}
	for _, name := range []string{"verified 100 days ago", "past due"} {
		if flags, err := db.App.ReviewFlags(ctx, caller, ids[name]); err != nil || flags.DueForReview || flags.StaleAt != nil {
			t.Errorf("%s after a check: %+v, %v; want no flags", name, flags, err)
		}
	}
}

func TestSweepReviewReadsTheAreaPolicyAndStaysInItsArea(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	spec := map[string]reviewSpec{"verified 45 days ago": {domain.KindObservation, domain.LifecycleActive, ptrTime(daysBefore(45)), ptrTime(daysBefore(1))}}
	strict, strictCaller, strictIDs := seedReview(ctx, t, db, "strict", spec)
	lax, laxCaller, laxIDs := seedReview(ctx, t, db, "lax", spec)

	// The strict area's own policy: observations go stale after 30 days.
	if err := db.App.PutFreshnessPolicy(ctx, strictCaller, strict.ID,
		domain.FreshnessPolicy{MaxAgeDays: map[domain.EpistemicKind]int{domain.KindObservation: 30}}); err != nil {
		t.Fatal(err)
	}
	sweep := func(a domain.Area) store.ReviewSweepResult {
		t.Helper()
		res, err := db.App.SweepReview(ctx, store.Caller{UserID: "froodbase-dreamer", AreaIDs: []string{string(a.ID)}, Job: jobs.KindReviewSweep}, a.ID, sweepNow)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := sweep(strict); res.Stale != 1 || res.Due != 1 {
		t.Errorf("strict sweep = %+v, want 1 stale under the 30-day policy, 1 due", res)
	}
	// Sweeping one area marks nothing in another.
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from review_flag where area_id = $1", string(lax.ID)); n != 0 {
		t.Errorf("sweeping %s wrote %d flags in %s", strict.Key, n, lax.Key)
	}
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from review_sweep where area_id = $1", string(lax.ID)); n != 0 {
		t.Errorf("sweeping %s recorded a sweep of %s", strict.Key, lax.Key)
	}
	flags, err := db.App.ReviewFlags(ctx, laxCaller, laxIDs["verified 45 days ago"])
	if err != nil || flags.DueForReview || flags.StaleAt != nil || flags.SweptAt != nil {
		t.Errorf("unswept area's record: %+v, %v; want no flags and no sweep", flags, err)
	}
	// The default policy (90 days) leaves the same record fresh.
	if res := sweep(lax); res.Stale != 0 || res.Due != 1 {
		t.Errorf("lax sweep = %+v, want nothing stale under the default, 1 due", res)
	}
	got, err := db.App.FreshnessPolicy(ctx, laxCaller, lax.ID)
	if err != nil || !reflect.DeepEqual(got, domain.DefaultFreshnessPolicy()) {
		t.Errorf("lax policy = %+v, %v; want the default", got, err)
	}

	// A caller without the area neither sweeps it nor reads its flags.
	if _, err := db.App.SweepReview(ctx, laxCaller, strict.ID, sweepNow); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("sweeping an area the caller lacks: %v, want ErrNotFound", err)
	}
	if _, err := db.App.ReviewFlags(ctx, laxCaller, strictIDs["verified 45 days ago"]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("reading flags across areas: %v, want ErrNotFound", err)
	}

	// Every area is listed for the sweep's fan-out, whoever asks.
	all, err := db.App.AllAreaIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(all, strict.ID) || !slices.Contains(all, lax.ID) {
		t.Errorf("AllAreaIDs = %v, want both areas", all)
	}
}
