//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

var (
	t0 = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 9, 9, 12, 30, 0, 0, time.UTC)
)

// recordTables are every table that holds records or their history; each
// must have row-level security enabled.
var recordTables = []string{
	"knowledge_area", "entity", "entity_name", "assertion", "evidence", "claim_evidence",
	"contribution", "contribution_claims", "revision", "audit_event", "support_event", "support_config",
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// area creates a knowledge area through the admin pool and returns a caller
// holding it.
func area(ctx context.Context, t *testing.T, db storetest.DB, key string) (domain.Area, store.Caller) {
	t.Helper()
	a := domain.Area{ID: domain.ID(store.NewID()), Key: key, Name: strings.ToUpper(key)}
	if err := db.Admin.CreateArea(ctx, store.Caller{UserID: "operator"}, a); err != nil {
		t.Fatalf("CreateArea(%s): %v", key, err)
	}
	return a, store.Caller{UserID: "alice", AgentID: "claude", AreaIDs: []string{string(a.ID)}}
}

// fixture is one contribution: an entity, an observation about it, and the
// evidence behind it (ev1 a root, ev2 restating ev1, ev3 another root).
type fixture struct {
	contribution  store.Contribution
	entity        *domain.Entity
	assertion     domain.Assertion
	ev1, ev2, ev3 domain.Evidence
}

func evidence(area domain.ID, uri string, root domain.ID) domain.Evidence {
	return domain.Evidence{
		ID: domain.ID(store.NewID()), AreaID: area, SourceKind: domain.SourceRepository, SourceURI: uri, SourceRevision: "abc123",
		SourceLocator: "L1-L10", Excerpt: "...", AccessClass: domain.AccessArea, RootEvidenceID: root, RecordedAt: t0,
	}
}

func newFixture(t *testing.T, area domain.Area, body string) fixture {
	t.Helper()
	e, err := domain.NewEntity(domain.ID(store.NewID()), area.ID, domain.EntityService, domain.EntityName{Name: "Callback Service " + area.Key, Scope: domain.Scope{Environment: "prod"}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := domain.NewObservation(domain.ID(store.NewID()), area.ID, e.ID(), body, domain.Scope{Environment: "prod", VersionMin: "2.3"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	ev1 := evidence(area.ID, "https://github.com/org/repo/blob/main/a.go", "")
	ev2 := evidence(area.ID, "https://github.com/org/repo/blob/main/b.go", ev1.ID)
	ev3 := evidence(area.ID, "https://dev.azure.com/org/p/_workitems/edit/42", "")
	ev3.SourceKind = domain.SourceTicket
	c := store.Contribution{
		Contribution:  domain.Contribution{ID: domain.ID(store.NewID()), AreaID: area.ID, ActorUserID: "alice", ActorAgentID: "claude", Client: "test", RecordedAt: t0},
		Entities:      []*domain.Entity{e},
		Assertions:    []domain.Assertion{a},
		Evidence:      []domain.Evidence{ev1, ev2, ev3},
		ClaimEvidence: map[domain.ID][]domain.ID{a.ID: {ev1.ID, ev2.ID, ev3.ID}},
		Rationale:     "seen during INC-1",
	}
	return fixture{contribution: c, entity: e, assertion: a, ev1: ev1, ev2: ev2, ev3: ev3}
}

// countAs runs a count query as the service role with the given caller areas.
func countAs(ctx context.Context, t *testing.T, pool *store.Pool, areas []string, query string, args ...any) int {
	t.Helper()
	var n int
	err := pgx.BeginFunc(ctx, pool.Pgx(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config($1, $2, true)", store.SessionVariable, strings.Join(areas, ",")); err != nil {
			return err
		}
		return tx.QueryRow(ctx, query, args...).Scan(&n)
	})
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func countAdmin(ctx context.Context, t *testing.T, pool *store.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.Pgx().QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestMigrationCreatesRecordTablesWithRLSAndServiceRole(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	for _, table := range recordTables {
		var enabled bool
		if err := db.Admin.Pgx().QueryRow(ctx, "select relrowsecurity from pg_class where relname = $1 and relkind = 'r'", table).Scan(&enabled); err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
		if !enabled {
			t.Errorf("table %s has no row-level security", table)
		}
		if n := countAdmin(ctx, t, db.Admin, "select count(*) from pg_policies where tablename = $1", table); n == 0 {
			t.Errorf("table %s has no policies", table)
		}
	}
	var super bool
	if err := db.Admin.Pgx().QueryRow(ctx, "select rolsuper or rolbypassrls from pg_roles where rolname = 'froodbase_app'").Scan(&super); err != nil {
		t.Fatal(err)
	}
	if super {
		t.Error("froodbase_app is a superuser or bypasses RLS")
	}
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from pg_tables where tableowner = 'froodbase_app'"); n != 0 {
		t.Errorf("froodbase_app owns %d tables, want none", n)
	}
	// The seeded service default is the domain default.
	cfg, err := db.App.CurrentSupportConfig(ctx, store.Caller{UserID: "anyone"}, domain.ID(store.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	if want := domain.DefaultSupportConfig(); !reflect.DeepEqual(cfg, want) {
		t.Errorf("seeded support config = %+v, want %+v", cfg, want)
	}
}

func TestRLSIsolatesAreasIncludingJoins(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	areaB, callerB := area(ctx, t, db, "area-b")
	fa, fb := newFixture(t, areaA, "A: the API returned 503s."), newFixture(t, areaB, "B: the queue backed up.")
	if _, err := db.App.Contribute(ctx, callerA, fa.contribution); err != nil {
		t.Fatal(err)
	}
	if _, err := db.App.Contribute(ctx, callerB, fb.contribution); err != nil {
		t.Fatal(err)
	}

	// The caller holding A reads A and sees nothing of B, through every read.
	if _, err := db.App.GetAssertion(ctx, callerA, fa.assertion.ID); err != nil {
		t.Errorf("own assertion: %v", err)
	}
	if _, err := db.App.GetAssertion(ctx, callerA, fb.assertion.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other area's assertion: err = %v, want ErrNotFound", err)
	}
	if _, err := db.App.GetEntity(ctx, callerA, fb.entity.ID()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other area's entity: err = %v, want ErrNotFound", err)
	}
	if _, err := db.App.GetEvidence(ctx, callerA, fb.ev1.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other area's evidence: err = %v, want ErrNotFound", err)
	}
	for name, fn := range map[string]func() (int, error){
		"history":        func() (int, error) { h, err := db.App.History(ctx, callerA, fb.assertion.ID); return len(h), err },
		"audit":          func() (int, error) { h, err := db.App.AuditEvents(ctx, callerA, fb.assertion.ID); return len(h), err },
		"support events": func() (int, error) { h, err := db.App.SupportEvents(ctx, callerA, fb.assertion.ID); return len(h), err },
		"claim evidence": func() (int, error) { h, err := db.App.ClaimEvidence(ctx, callerA, fb.assertion.ID); return len(h), err },
	} {
		if n, err := fn(); err != nil || n != 0 {
			t.Errorf("%s of other area's record: %d rows, %v; want 0, nil", name, n, err)
		}
	}
	if h, err := db.App.History(ctx, callerA, fa.assertion.ID); err != nil || len(h) != 1 {
		t.Errorf("own history: %d rows, %v; want 1", len(h), err)
	}

	// Joins to names and evidence leak nothing: counts over the join tables
	// are zero for B and complete for A, and nothing at all with no areas.
	joins := map[string]string{
		"names":          "select count(*) from entity_name n join entity e on e.id = n.entity_id",
		"claim evidence": "select count(*) from claim_evidence ce join evidence e on e.id = ce.evidence_id join assertion a on a.id = ce.claim_id",
		"contributions":  "select count(*) from contribution_claims cc join contribution c on c.id = cc.contribution_id join assertion a on a.id = cc.claim_id",
		"support":        "select count(*) from support_event s join assertion a on a.id = s.record_id",
		"revisions":      "select count(*) from revision r join assertion a on a.id = r.record_id",
		"areas":          "select count(*) from knowledge_area",
	}
	bothAreas := []string{string(areaA.ID), string(areaB.ID)}
	for name, q := range joins {
		onlyA := countAs(ctx, t, db.App, callerA.AreaIDs, q)
		both := countAs(ctx, t, db.App, bothAreas, q)
		none := countAs(ctx, t, db.App, nil, q)
		if onlyA == 0 || both != 2*onlyA || none != 0 {
			t.Errorf("%s: A=%d both=%d none=%d; want A>0, both=2A, none=0", name, onlyA, both, none)
		}
	}
	// Area B's rows in particular are invisible to A, with an explicit filter.
	if n := countAs(ctx, t, db.App, callerA.AreaIDs, "select count(*) from assertion where area_id = $1", string(areaB.ID)); n != 0 {
		t.Errorf("A sees %d assertions of B", n)
	}
	// The admin pool, which runs migrations, sees everything: tests of
	// visibility must use the app pool.
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from assertion"); n != 2 {
		t.Errorf("admin sees %d assertions, want 2", n)
	}
}

func TestServiceRoleCannotBypassRLS(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	areaB, callerB := area(ctx, t, db, "area-b")
	fb := newFixture(t, areaB, "B: the queue backed up.")
	if _, err := db.App.Contribute(ctx, callerB, fb.contribution); err != nil {
		t.Fatal(err)
	}
	app := db.App.Pgx()
	tests := []struct {
		name string
		sql  string
	}{
		{"disable RLS", "alter table entity disable row level security"},
		{"drop policy", "drop policy area_isolation on entity"},
		{"turn row security off", "set row_security = off; select count(*) from assertion"},
		{"bypass with set role", "set role froodbase"},
		{"update a revision", "update revision set rationale = 'x'"},
		{"delete an audit event", "delete from audit_event"},
		{"update a support event", "update support_event set rationale = 'x'"},
		{"delete evidence", "delete from evidence"},
		{"delete an assertion", "delete from assertion"},
		{"write the service default config", "insert into support_config (area_id, version, event_weights, use_without_challenge_cap, threshold_by_kind, allowed_contributors) values (null, 'x', '{}', 0, '{}', '{}')"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pgx.BeginFunc(ctx, app, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, "select set_config($1, $2, true)", store.SessionVariable, strings.Join(callerB.AreaIDs, ",")); err != nil {
					return err
				}
				for _, stmt := range strings.Split(tt.sql, ";") {
					if _, err := tx.Exec(ctx, stmt); err != nil {
						return err
					}
				}
				return nil
			})
			if err == nil {
				t.Errorf("%q succeeded as froodbase_app", tt.sql)
			}
		})
	}
	// Writing into an area the session does not hold is refused by the
	// policy itself, before any application check.
	err := pgx.BeginFunc(ctx, app, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config($1, $2, true)", store.SessionVariable, string(areaA.ID)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "insert into entity (id, area_id, entity_type, recorded_at, updated_at) values ($1, $2, 'service', now(), now())", store.NewID(), string(areaB.ID))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Errorf("insert into another area: err = %v, want a row-level security violation", err)
	}
	// And the writer reports it as ErrForbidden when its own check is bypassed
	// by a caller claiming area B but contributing to it with A's records.
	fa := newFixture(t, areaA, "A: contributed by someone holding only B")
	if _, err := db.App.Contribute(ctx, callerB, fa.contribution); !errors.Is(err, store.ErrForbidden) {
		t.Errorf("Contribute outside the caller's areas: err = %v, want ErrForbidden", err)
	}
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from entity where area_id = $1", string(areaA.ID)); n != 0 {
		t.Errorf("forbidden contribution left %d entities behind", n)
	}
	_ = callerA
}

func TestContributeIsAtomic(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	nothingLeft := func(t *testing.T, when string) {
		t.Helper()
		for _, table := range []string{"entity", "entity_name", "assertion", "evidence", "claim_evidence", "contribution", "contribution_claims", "revision", "support_event"} {
			if n := countAdmin(ctx, t, db.Admin, "select count(*) from "+table); n != 0 {
				t.Errorf("%s: %s has %d rows, want 0", when, table, n)
			}
		}
		if n := countAdmin(ctx, t, db.Admin, "select count(*) from audit_event where event_type <> 'area_created'"); n != 0 {
			t.Errorf("%s: %d audit events left behind", when, n)
		}
		if n := countAdmin(ctx, t, db.Admin, "select count(*) from river_job where kind = 'index_record'"); n != 0 {
			t.Errorf("%s: %d index jobs left behind", when, n)
		}
	}

	// A failure injected at each step of the write leaves nothing behind.
	for _, step := range []string{"revision", "audit", "job", "commit"} {
		boom := errors.New("injected failure at " + step)
		db.App.SetFailAt(func(s string) error {
			if s == step {
				return boom
			}
			return nil
		})
		f := newFixture(t, areaA, "A: the API returned 503s.")
		if _, err := db.App.Contribute(ctx, callerA, f.contribution); !errors.Is(err, boom) {
			t.Fatalf("step %s: err = %v, want the injected failure", step, err)
		}
		nothingLeft(t, "after failure at "+step)
	}
	db.App.SetFailAt(nil)

	// A genuine failure part-way through (duplicate evidence after the
	// entity and the first evidence are in) rolls everything back too.
	f := newFixture(t, areaA, "A: the API returned 503s.")
	dup := f.ev1
	dup.ID = domain.ID(store.NewID())
	dup.RootEvidenceID = ""
	f.contribution.Evidence = append(f.contribution.Evidence, dup)
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); !errors.Is(err, store.ErrDuplicateEvidence) {
		t.Fatalf("duplicate evidence: err = %v, want ErrDuplicateEvidence", err)
	}
	nothingLeft(t, "after duplicate evidence")

	// The same contribution without the duplicate commits whole.
	f.contribution.Evidence = f.contribution.Evidence[:3]
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"entity": 1, "entity_name": 1, "assertion": 1, "evidence": 3, "claim_evidence": 3, "contribution": 1, "contribution_claims": 1, "revision": 5, "support_event": 2} {
		if n := countAdmin(ctx, t, db.Admin, "select count(*) from "+table); n != want {
			t.Errorf("%s has %d rows, want %d", table, n, want)
		}
	}
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from river_job where kind = 'index_record'"); n != 2 {
		t.Errorf("%d index jobs, want 2 (entity and assertion)", n)
	}
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from river_job where kind = 'index_record' and args->>'record_id' = $1 and (args->>'revision')::int = 1", string(f.assertion.ID)); n != 1 {
		t.Errorf("index job for the assertion's revision 1: %d, want 1", n)
	}
}

func TestRevisionsAreAppendOnly(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f := newFixture(t, areaA, "The API returned 503s.")
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}

	updated := f.assertion
	updated.BodyMarkdown = "The API returned 503s between 10:00 and 10:20."
	updated.ValidFrom = &t0
	rev, err := db.App.UpdateAssertion(ctx, callerA, updated, 1, "narrowed the window")
	if err != nil || rev != 2 {
		t.Fatalf("UpdateAssertion = %d, %v; want 2, nil", rev, err)
	}
	// A stale update is refused (TR-90).
	if _, err := db.App.UpdateAssertion(ctx, callerA, updated, 1, "stale"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Errorf("stale update: err = %v, want ErrRevisionConflict", err)
	}
	// Kind, subject and area are immutable.
	moved := updated
	moved.Kind = domain.KindInference
	if _, err := db.App.UpdateAssertion(ctx, callerA, moved, 2, "kind change"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Errorf("changing the kind: err = %v, want ErrRevisionConflict", err)
	}

	cur, err := db.App.GetAssertion(ctx, callerA, f.assertion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Revision != 2 || cur.Assertion.BodyMarkdown != updated.BodyMarkdown || cur.Assertion.ValidFrom == nil || !cur.Assertion.ValidFrom.Equal(t0) {
		t.Errorf("current = %+v, want revision 2 with the new body and validity", cur)
	}
	if !cur.Assertion.RecordedAt.Equal(t0) {
		t.Errorf("recorded_at moved to %v on update", cur.Assertion.RecordedAt)
	}
	history, err := db.App.History(ctx, callerA, f.assertion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Revision != 1 || history[1].Revision != 2 {
		t.Fatalf("history = %+v, want revisions 1 and 2", history)
	}
	first, err := store.AssertionFromSnapshot(history[0].Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if first.Assertion.BodyMarkdown != f.assertion.BodyMarkdown || first.Assertion.ValidFrom != nil || first.Revision != 1 {
		t.Errorf("revision 1 snapshot = %+v, want the original body with unknown validity", first)
	}
	second, err := store.AssertionFromSnapshot(history[1].Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if second.Assertion.BodyMarkdown != updated.BodyMarkdown || second.Revision != 2 || history[1].Rationale != "narrowed the window" || history[1].ActorUserID != "alice" || history[1].ActorAgentID != "claude" {
		t.Errorf("revision 2 = %+v / %+v", second, history[1])
	}

	// Entities version the same way; renaming adds a name, it never overwrites.
	e := f.entity
	if err := e.AddName(domain.EntityName{Name: "callback-svc", ValidFrom: &t1}); err != nil {
		t.Fatal(err)
	}
	rev, err = db.App.UpdateEntity(ctx, callerA, e, 1, "alias")
	if err != nil || rev != 2 {
		t.Fatalf("UpdateEntity = %d, %v; want 2, nil", rev, err)
	}
	got, err := db.App.GetEntity(ctx, callerA, e.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || len(got.Entity.Names) != 2 || got.Entity.Names[1].Name != "callback-svc" {
		t.Errorf("entity = %+v, want revision 2 with two names", got)
	}
	eh, err := db.App.History(ctx, callerA, e.ID())
	if err != nil || len(eh) != 2 {
		t.Fatalf("entity history = %d rows, %v; want 2", len(eh), err)
	}
	old, err := store.EntityFromSnapshot(eh[0].Snapshot)
	if err != nil || len(old.Entity.Names) != 1 {
		t.Errorf("entity revision 1 = %+v, %v; want one name", old, err)
	}
	if _, err := db.App.UpdateEntity(ctx, callerA, e, 1, "stale"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Errorf("stale entity update: err = %v", err)
	}
	// Evidence is immutable and has exactly its first revision.
	if h, err := db.App.History(ctx, callerA, f.ev1.ID); err != nil || len(h) != 1 || h[0].RecordType != store.RecordEvidence {
		t.Errorf("evidence history = %+v, %v; want one revision", h, err)
	}
	// Index jobs are unique per record while they wait (FRB-12): the second
	// revision of each record collapsed into the job its first revision
	// enqueued, so two records have two jobs between them.
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from river_job where kind = 'index_record'"); n != 2 {
		t.Errorf("%d index jobs, want 2 (one per record, bursts collapsed)", n)
	}
}

func TestAuditEventPerWrite(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f := newFixture(t, areaA, "The API returned 503s.")
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}
	if _, err := db.App.UpdateAssertion(ctx, callerA, f.assertion, 1, "touch"); err != nil {
		t.Fatal(err)
	}
	ev4 := evidence(areaA.ID, "https://github.com/org/repo/blob/main/c.go", "")
	if _, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev4}, "more evidence"); err != nil {
		t.Fatal(err)
	}
	types := func(id domain.ID) []string {
		events, err := db.App.AuditEvents(ctx, callerA, id)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(events))
		for _, e := range events {
			if e.ActorUserID != "alice" || e.ActorAgentID != "claude" || e.AreaID != string(areaA.ID) {
				t.Errorf("audit event %+v lacks actor, agent or area", e)
			}
			out = append(out, fmt.Sprintf("%s@%d", e.EventType, e.RecordRevision))
		}
		return out
	}
	want := map[domain.ID][]string{
		f.entity.ID():  {"entity_created@1"},
		f.assertion.ID: {"assertion_created@1", "assertion_updated@2", "evidence_linked@2"},
		f.ev1.ID:       {"evidence_recorded@1"},
		f.ev3.ID:       {"evidence_recorded@1"},
		ev4.ID:         {"evidence_recorded@1"},
	}
	for id, w := range want {
		if got := types(id); !reflect.DeepEqual(got, w) {
			t.Errorf("audit events of %s = %v, want %v", id, got, w)
		}
	}
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from audit_event where event_type = 'area_created' and area_id = $1 and actor_user_id = 'operator'", string(areaA.ID)); n != 1 {
		t.Errorf("area_created events = %d, want 1", n)
	}
	// Audit events are the writer's record of the write, so one per record
	// per write and nothing else.
	if n := countAdmin(ctx, t, db.Admin, "select count(*) from audit_event"); n != 1+3+3+1+1 {
		t.Errorf("total audit events = %d, want 9", n)
	}
}

func TestOneRootAddedPerDistinctRoot(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f := newFixture(t, areaA, "The API returned 503s.")
	res, err := db.App.Contribute(ctx, callerA, f.contribution)
	if err != nil {
		t.Fatal(err)
	}
	cfg := domain.DefaultSupportConfig()
	w := cfg.EventWeights[domain.SupportRootAdded]
	// ev1 and ev2 share a root; ev3 is another: two roots, two events.
	if res.ConfigVersion != domain.DefaultSupportConfigVersion || res.Scores[f.assertion.ID] != 2*w {
		t.Errorf("result = %+v, want score %v under %s", res, 2*w, domain.DefaultSupportConfigVersion)
	}
	roots := func() []domain.ID {
		events, err := db.App.SupportEvents(ctx, callerA, f.assertion.ID)
		if err != nil {
			t.Fatal(err)
		}
		var out []domain.ID
		for _, e := range events {
			if e.Type != domain.SupportRootAdded || len(e.EvidenceRootIDs) != 1 || e.ActorUserID != "alice" || e.ActorAgentID != "claude" ||
				e.ConfigVersion != domain.DefaultSupportConfigVersion || e.Rationale == "" || e.Scope.Environment != "prod" {
				t.Errorf("support event %+v is not a well-formed root_added by alice", e)
			}
			out = append(out, e.EvidenceRootIDs[0])
		}
		return out
	}
	if got := roots(); !reflect.DeepEqual(got, []domain.ID{f.ev1.ID, f.ev3.ID}) && !reflect.DeepEqual(got, []domain.ID{f.ev3.ID, f.ev1.ID}) {
		t.Errorf("roots = %v, want ev1 and ev3", got)
	}
	if events, _ := db.App.SupportEvents(ctx, callerA, f.assertion.ID); len(events) != 2 || events[0].Rationale != f.contribution.Rationale {
		t.Errorf("events carry rationale %q, want the contribution's", events[0].Rationale)
	}

	// Another restatement of ev1 earns nothing; a fourth source is a new root.
	ev4 := evidence(areaA.ID, "https://github.com/org/repo/blob/main/d.go", f.ev1.ID)
	score, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev4}, "restated")
	if err != nil || score != 2*w {
		t.Errorf("link restatement: score %v, %v; want %v", score, err, 2*w)
	}
	if n := len(roots()); n != 2 {
		t.Errorf("after a restatement: %d root_added events, want 2", n)
	}
	ev5 := evidence(areaA.ID, "https://portal.azure.com/#blade/q/1", "")
	ev5.SourceKind = domain.SourceTelemetryQuery
	score, err = db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev5, ev4}, "telemetry")
	if err != nil || score != 3*w {
		t.Errorf("link new root: score %v, %v; want %v", score, err, 3*w)
	}
	if n := len(roots()); n != 3 {
		t.Errorf("after a new root: %d root_added events, want 3", n)
	}
	// Linking the same evidence again is idempotent, and the database itself
	// refuses a second root_added for a root.
	if _, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev5}, "again"); err != nil {
		t.Errorf("relink: %v", err)
	}
	if n := len(roots()); n != 3 {
		t.Errorf("after relinking: %d root_added events, want 3", n)
	}
	_, err = db.Admin.Pgx().Exec(ctx, "insert into support_event (record_id, area_id, event_type, evidence_root_ids, actor_user_id, config_version, at) values ($1, $2, 'root_added', $3::uuid[], 'x', 'service-1', now())",
		string(f.assertion.ID), string(areaA.ID), []string{string(f.ev1.ID)})
	if err == nil {
		t.Error("the database accepted a second root_added for the same root")
	}
	// A claim with no evidence enters active with no support.
	g := newFixture(t, areaA, "Unsupported claim.")
	g.contribution.Evidence = nil
	g.contribution.ClaimEvidence = nil
	res, err = db.App.Contribute(ctx, callerA, g.contribution)
	if err != nil || res.Scores[g.assertion.ID] != 0 {
		t.Errorf("unsupported claim: %+v, %v", res, err)
	}
	if a, err := db.App.GetAssertion(ctx, callerA, g.assertion.ID); err != nil || a.LifecycleState != store.LifecycleActive {
		t.Errorf("unsupported claim state = %+v, %v; want active", a, err)
	}
}

func TestSupportScoreIsRecomputableUnderItsConfigVersion(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f := newFixture(t, areaA, "The API returned 503s.")
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}
	check := func(when string) store.AssertionRecord {
		t.Helper()
		cur, err := db.App.GetAssertion(ctx, callerA, f.assertion.ID)
		if err != nil {
			t.Fatal(err)
		}
		recomputed, err := db.App.RecomputeSupportScore(ctx, callerA, f.assertion.ID)
		if err != nil {
			t.Fatalf("%s: recompute: %v", when, err)
		}
		if recomputed != cur.SupportScore {
			t.Errorf("%s: recomputed %v, stored %v", when, recomputed, cur.SupportScore)
		}
		// The same projection, done by hand from the history and the named version.
		events, err := db.App.SupportEvents(ctx, callerA, f.assertion.ID)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := db.App.SupportConfig(ctx, callerA, areaA.ID, cur.SupportConfigVersion)
		if err != nil {
			t.Fatal(err)
		}
		byHand, err := domain.SupportScore(events, cfg)
		if err != nil || byHand != cur.SupportScore {
			t.Errorf("%s: by hand %v, %v; stored %v", when, byHand, err, cur.SupportScore)
		}
		return cur
	}
	cur := check("after contribute")
	if cur.SupportConfigVersion != domain.DefaultSupportConfigVersion || cur.SupportScore != 2 {
		t.Errorf("after contribute: %v under %q, want 2 under %q", cur.SupportScore, cur.SupportConfigVersion, domain.DefaultSupportConfigVersion)
	}

	// The area versions its own configuration; the next write projects under
	// it and names it, and the history still explains the score.
	cfg := domain.DefaultSupportConfig()
	cfg.Version = "area-a-2"
	cfg.EventWeights[domain.SupportRootAdded] = 2
	if err := db.App.PutSupportConfig(ctx, callerA, areaA.ID, cfg); err != nil {
		t.Fatal(err)
	}
	ev4 := evidence(areaA.ID, "https://github.com/org/repo/blob/main/d.go", "")
	score, err := db.App.LinkEvidence(ctx, callerA, f.assertion.ID, []domain.Evidence{ev4}, "another source")
	if err != nil || score != 6 {
		t.Fatalf("LinkEvidence under the new version = %v, %v; want 6 (three roots at weight 2)", score, err)
	}
	cur = check("after reconfiguration")
	if cur.SupportConfigVersion != "area-a-2" {
		t.Errorf("config version = %q, want area-a-2", cur.SupportConfigVersion)
	}
	// Earlier events keep the version they were appended under.
	events, err := db.App.SupportEvents(ctx, callerA, f.assertion.ID)
	if err != nil || len(events) != 3 || events[0].ConfigVersion != domain.DefaultSupportConfigVersion || events[2].ConfigVersion != "area-a-2" {
		t.Errorf("events = %+v, %v", events, err)
	}
	// Another area is unaffected by A's configuration.
	areaB, callerB := area(ctx, t, db, "area-b")
	if c, err := db.App.CurrentSupportConfig(ctx, callerB, areaB.ID); err != nil || c.Version != domain.DefaultSupportConfigVersion {
		t.Errorf("B's config = %+v, %v; want the service default", c, err)
	}
	if _, err := db.App.SupportConfig(ctx, callerB, areaB.ID, "area-a-2"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("B reading A's config version: err = %v, want ErrNotFound", err)
	}
}

func TestAreaIDsResolveGrantKeys(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, _ := area(ctx, t, db, "support")
	areaB, _ := area(ctx, t, db, "platform")
	got, err := db.App.AreaIDs(ctx, []string{"support", "platform", "nope"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"support": string(areaA.ID), "platform": string(areaB.ID)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AreaIDs = %v, want %v", got, want)
	}
	// Without the function, the service role sees no areas at all.
	if n := countAs(ctx, t, db.App, nil, "select count(*) from knowledge_area"); n != 0 {
		t.Errorf("knowledge_area visible without caller areas: %d rows", n)
	}
}
