//go:build integration

package store_test

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

// commandRun is a command observation of the import tool's web app at the
// time it ran.
func commandRun(area domain.ID, at time.Time) domain.Evidence {
	return domain.Evidence{
		ID: domain.ID(store.NewID()), AreaID: area, SourceKind: domain.SourceCommand,
		SourceURI:     "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool",
		SourceLocator: "az webapp config show --ids /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool --query http20Enabled",
		Excerpt:       "true", AccessClass: domain.AccessArea, ObservedAt: &at, RecordedAt: t1,
	}.WithDefaults()
}

// personSaid is a statement by person at a time.
func personSaid(area domain.ID, person, said string, at time.Time) domain.Evidence {
	return domain.Evidence{
		ID: domain.ID(store.NewID()), AreaID: area, SourceKind: domain.SourceStatement, SourceURI: domain.StatementURI(person),
		Excerpt: said, AccessClass: domain.AccessArea, ObservedAt: &at, RecordedAt: t1,
	}.WithDefaults()
}

// observationCiting is a fresh observation about a new entity citing ev.
func observationCiting(t *testing.T, a domain.Area, body string, ev ...domain.Evidence) (store.Contribution, domain.ID) {
	t.Helper()
	f := newFixture(t, a, body)
	f.contribution.Evidence = ev
	ids := make([]domain.ID, len(ev))
	for i, e := range ev {
		ids[i] = e.ID
	}
	f.contribution.ClaimEvidence = map[domain.ID][]domain.ID{f.assertion.ID: ids}
	return f.contribution, f.assertion.ID
}

func rootKinds(t *testing.T, db storetest.DB, caller store.Caller, claim domain.ID) []domain.SourceKind {
	t.Helper()
	events, err := db.App.SupportEvents(t.Context(), caller, claim)
	if err != nil {
		t.Fatal(err)
	}
	var out []domain.SourceKind
	for _, e := range events {
		if e.Type == domain.SupportRootAdded {
			out = append(out, e.SourceKind)
		}
	}
	return out
}

// A command observation is one root per run: the same command run again
// later is a second root, and each root_added event records its kind.
func TestCommandRunsAreSeparateRoots(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	a, caller := area(ctx, t, db, "ops")
	first := commandRun(a.ID, time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	c, claim := observationCiting(t, a, "HTTP/2 is enabled on the import tool web app.", first)
	res, err := db.App.Contribute(ctx, caller, c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Roots[claim] != 1 || res.Scores[claim] != 1 || !reflect.DeepEqual(res.RootKinds[claim], map[domain.SourceKind]int{domain.SourceCommand: 1}) {
		t.Errorf("one run: roots %d score %v kinds %v, want one command root scoring 1", res.Roots[claim], res.Scores[claim], res.RootKinds[claim])
	}
	again := commandRun(a.ID, time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
	score, err := db.App.LinkEvidence(ctx, caller, claim, []domain.Evidence{again}, "checked again")
	if err != nil || score != 2 {
		t.Errorf("second run: score %v, %v; want 2", score, err)
	}
	if got := rootKinds(t, db, caller, claim); !reflect.DeepEqual(got, []domain.SourceKind{domain.SourceCommand, domain.SourceCommand}) {
		t.Errorf("root kinds = %v, want two command roots", got)
	}
	// The same run recorded twice is the same evidence, not a new root.
	dup := commandRun(a.ID, time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
	c2, _ := observationCiting(t, a, "Another claim.", dup)
	if _, err := db.App.Contribute(ctx, caller, c2); !errors.Is(err, store.ErrDuplicateEvidence) {
		t.Errorf("same run recorded again: err = %v, want ErrDuplicateEvidence", err)
	}
}

// A statement root weighs what the area's configuration says (0.5 by
// default); one person's statements count as one root per claim, another
// person's statement is another root, and an area version can weigh them
// differently.
func TestStatementRootsAreWeighted(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	a, caller := area(ctx, t, db, "ops")
	at := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	said := personSaid(a.ID, "alice", "The import job runs nightly at 02:00.", at)
	c, claim := observationCiting(t, a, "The import job runs nightly.", said)
	res, err := db.App.Contribute(ctx, caller, c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scores[claim] != domain.DefaultStatementRootWeight || res.Roots[claim] != 1 {
		t.Errorf("one statement: score %v roots %d, want %v and 1", res.Scores[claim], res.Roots[claim], domain.DefaultStatementRootWeight)
	}
	cfg := domain.DefaultSupportConfig()
	if cfg.MeetsThreshold(domain.KindObservation, res.Scores[claim]) {
		t.Error("a statement alone meets the observation threshold")
	}

	// Alice saying it again, in other words, earns nothing more.
	again := personSaid(a.ID, "alice", "It runs every night at two.", at.Add(time.Hour))
	if score, err := db.App.LinkEvidence(ctx, caller, claim, []domain.Evidence{again}, "said again"); err != nil || score != domain.DefaultStatementRootWeight {
		t.Errorf("same person again: score %v, %v; want %v", score, err, domain.DefaultStatementRootWeight)
	}
	// Bob is a second line of evidence; a command run is an observed source.
	bob := personSaid(a.ID, "bob", "Nightly, yes.", at)
	run := commandRun(a.ID, at)
	score, err := db.App.LinkEvidence(ctx, caller, claim, []domain.Evidence{bob, run}, "corroborated")
	if want := 2*domain.DefaultStatementRootWeight + 1; err != nil || score != want {
		t.Errorf("bob and a command: score %v, %v; want %v", score, err, want)
	}
	if got, err := db.App.RecomputeSupportScore(ctx, caller, claim); err != nil || got != score {
		t.Errorf("recomputed %v, %v; want the stored %v", got, err, score)
	}

	// Two different things one person said at the same moment are two
	// statements; the same thing twice is one.
	other := personSaid(a.ID, "alice", "And the export job runs at 03:00.", at)
	c2, _ := observationCiting(t, a, "The export job runs nightly.", other)
	if _, err := db.App.Contribute(ctx, caller, c2); err != nil {
		t.Errorf("a different statement at the same moment: %v", err)
	}
	same := personSaid(a.ID, "alice", "The import job runs nightly at 02:00.", at)
	c3, _ := observationCiting(t, a, "Restated.", same)
	if _, err := db.App.Contribute(ctx, caller, c3); !errors.Is(err, store.ErrDuplicateEvidence) {
		t.Errorf("the same statement twice: err = %v, want ErrDuplicateEvidence", err)
	}

	// An area version that weighs statements at 0.25.
	quarter := domain.DefaultSupportConfig()
	quarter.Version = "ops-2"
	quarter.RootWeightBySourceKind[domain.SourceStatement] = 0.25
	if err := db.App.PutSupportConfig(ctx, caller, a.ID, quarter); err != nil {
		t.Fatal(err)
	}
	got, err := db.App.CurrentSupportConfig(ctx, caller, a.ID)
	if err != nil || !reflect.DeepEqual(got.RootWeightBySourceKind, quarter.RootWeightBySourceKind) {
		t.Fatalf("stored root weights = %v, %v; want %v", got.RootWeightBySourceKind, err, quarter.RootWeightBySourceKind)
	}
	c4, claim4 := observationCiting(t, a, "The import job is idempotent.", personSaid(a.ID, "alice", "Re-running the import is safe.", at))
	res, err = db.App.Contribute(ctx, caller, c4)
	if err != nil || math.Abs(res.Scores[claim4]-0.25) > 1e-9 || res.ConfigVersion != "ops-2" {
		t.Errorf("under ops-2: score %v under %s, %v; want 0.25", res.Scores[claim4], res.ConfigVersion, err)
	}
}

// The database refuses a source kind the domain does not know.
func TestEvidenceSourceKindConstraint(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	a, _ := area(ctx, t, db, "ops")
	for kind, ok := range map[string]bool{"command": true, "statement": true, "email": false} {
		_, err := db.Admin.Pgx().Exec(ctx,
			"insert into evidence (id, area_id, source_kind, source_uri, source_revision, access_class, recorded_at) values ($1, $2, $3, 'x://y', 'r', 'area', now())",
			store.NewID(), string(a.ID), kind)
		if (err == nil) != ok {
			t.Errorf("source kind %q: err = %v, want accepted %v", kind, err, ok)
		}
	}
}

// Migration 6 records the kind of every existing root from its evidence,
// gives every stored configuration the default root weights and lists the
// new kinds in the service default.
func TestMigrationBackfillsRootKinds(t *testing.T) {
	ctx := ctxT(t)
	pool := storetest.EmptyPool(t)
	if err := pool.MigrateTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	areaID, evID, claimID := store.NewID(), store.NewID(), store.NewID()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"insert into knowledge_area (id, key, name, recorded_at) values ($1, 'old', 'Old', now())", []any{areaID}},
		{"insert into evidence (id, area_id, source_kind, source_uri, source_revision, access_class, recorded_at) values ($1, $2, 'ticket', 'https://dev.azure.com/o/p/_workitems/edit/1', '1', 'area', now())", []any{evID, areaID}},
		{"insert into support_event (record_id, area_id, event_type, evidence_root_ids, actor_user_id, config_version, at) values ($1, $2, 'root_added', $3::uuid[], 'alice', 'service-1', now())", []any{claimID, areaID, []string{evID}}},
		{"insert into support_config (area_id, version, event_weights, use_without_challenge_cap, threshold_by_kind, allowed_contributors) select $1, 'old-2', event_weights, use_without_challenge_cap, threshold_by_kind, allowed_contributors from support_config where area_id is null", []any{areaID}},
	} {
		if _, err := pool.Pgx().Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
	if _, _, err := pool.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var kind string
	if err := pool.Pgx().QueryRow(ctx, "select source_kind from support_event where record_id = $1", claimID).Scan(&kind); err != nil || kind != "ticket" {
		t.Errorf("backfilled root kind = %q, %v; want ticket", kind, err)
	}
	caller := store.Caller{UserID: "alice", AreaIDs: []string{areaID}}
	want := domain.DefaultSupportConfig()
	for _, version := range []string{"service-1", "old-2"} {
		cfg, err := pool.SupportConfig(ctx, caller, domain.ID(areaID), version)
		if err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		if !reflect.DeepEqual(cfg.RootWeightBySourceKind, want.RootWeightBySourceKind) {
			t.Errorf("%s root weights = %v, want %v", version, cfg.RootWeightBySourceKind, want.RootWeightBySourceKind)
		}
	}
}
