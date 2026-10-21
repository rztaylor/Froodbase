//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/domain/domaintest"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

// The SQL functions of migration 7 answer to the same tables as the domain
// rules (internal/domain/domaintest): a case added there is checked here.

func TestSQLScopeOverlapsMatchesDomainCases(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	for _, c := range domaintest.OverlapCases() {
		for _, order := range [][2]domain.Scope{{c.A, c.B}, {c.B, c.A}} {
			var got bool
			if err := db.App.Pgx().QueryRow(ctx, "select froodbase_scope_overlaps($1::jsonb, $2::jsonb)",
				store.ScopeBytes(order[0]), store.ScopeBytes(order[1])).Scan(&got); err != nil {
				t.Fatalf("%s: %v", c.Name, err)
			}
			if got != c.Want {
				t.Errorf("%s: froodbase_scope_overlaps(%+v, %+v) = %v, want %v", c.Name, order[0], order[1], got, c.Want)
			}
		}
	}
	// A scope stored before keys were checked, or with a key the domain
	// does not know, is still read by its six keys alone.
	var got bool
	if err := db.App.Pgx().QueryRow(ctx, `select froodbase_scope_overlaps('{"environment":"prod","colour":"blue"}', '{"environment":"PROD"}')`).Scan(&got); err != nil || !got {
		t.Errorf("unknown key: overlaps = %v, %v; want true", got, err)
	}
}

func TestSQLModeMatchesDomainCases(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	for _, c := range domaintest.ModeCases() {
		var asOf *time.Time
		if c.Query.Mode == domain.ModeAsOf {
			asOf = &c.Query.AsOf
		}
		policy, err := json.Marshal(c.Policy.MaxAgeDays)
		if err != nil {
			t.Fatal(err)
		}
		r := c.Record
		var got domain.ModeResult
		var status string
		err = db.App.Pgx().QueryRow(ctx, `select
			froodbase_in_mode($1, $2, $3, $4, $5, $6),
			froodbase_temporal_status($1, $2, $3, $6),
			froodbase_stale($7, $1, $2, $3, $8, $9, $10::jsonb, $6)`,
			string(r.State), r.ValidFrom, r.ValidTo, string(c.Query.Mode), asOf, c.Now,
			string(r.Kind), r.RecordedAt, r.VerifiedAt, policy,
		).Scan(&got.Included, &status, &got.Stale)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		got.Status = domain.TemporalStatus(status)
		if got != c.Want {
			t.Errorf("%s: SQL = %+v, want %+v", c.Name, got, c.Want)
		}
	}
}

func TestSQLModeRefusesNothingSilently(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	// An unknown mode keeps nothing, as domain.ModeRecord.InMode does.
	var got bool
	if err := db.App.Pgx().QueryRow(ctx, "select froodbase_in_mode('active', null, null, 'latest', null, now())").Scan(&got); err != nil || got {
		t.Errorf("unknown mode: in_mode = %v, %v; want false", got, err)
	}
	// The default freshness policy matches the domain's.
	var policy []byte
	if err := db.App.Pgx().QueryRow(ctx, "select froodbase_freshness_policy(gen_random_uuid())").Scan(&policy); err != nil {
		t.Fatal(err)
	}
	var days map[domain.EpistemicKind]int
	if err := json.Unmarshal(policy, &days); err != nil {
		t.Fatal(err)
	}
	if want := domain.DefaultFreshnessPolicy().MaxAgeDays; !reflect.DeepEqual(days, want) {
		t.Errorf("service default freshness policy = %v, want %v", days, want)
	}
}

// pitchArea holds the pitch's four records as stored, with the failure
// superseded by the deployment (its validity unchanged, TR-33).
type pitchArea struct {
	area   domain.Area
	caller store.Caller
	entity *domain.Entity
	ids    map[string]domain.ID // pitch name -> assertion id
}

func (p pitchArea) names(ids []domain.ID) []string {
	var out []string
	for _, id := range ids {
		for name, pid := range p.ids {
			if pid == id {
				out = append(out, name)
			}
		}
	}
	slices.Sort(out)
	return out
}

func seedPitch(ctx context.Context, t *testing.T, db storetest.DB, key string) pitchArea {
	t.Helper()
	a, caller := area(ctx, t, db, key)
	e, err := domain.NewEntity(domain.ID(store.NewID()), a.ID, domain.EntityService, domain.EntityName{Name: "Callback Service"})
	if err != nil {
		t.Fatal(err)
	}
	p := pitchArea{area: a, caller: caller, entity: e, ids: map[string]domain.ID{}}
	c := store.Contribution{
		Contribution:  domain.Contribution{ID: domain.ID(store.NewID()), AreaID: a.ID, ActorUserID: "alice", Client: "test", RecordedAt: t0},
		Entities:      []*domain.Entity{e},
		ClaimEvidence: map[domain.ID][]domain.ID{},
		Refs:          map[domain.ID]string{},
	}
	for i, r := range domaintest.PitchRecords() {
		ev := evidence(a.ID, "https://github.com/org/repo/blob/main/pitch.go", "")
		ev.SourceLocator = r.Name
		as := domain.Assertion{
			ID: domain.ID(store.NewID()), AreaID: a.ID, SubjectEntityID: e.ID(), Kind: r.Record.Kind,
			BodyMarkdown: r.Body, Scope: r.Scope, Temporal: r.Record.Temporal,
		}
		if err := as.Validate(); err != nil {
			t.Fatalf("pitch %d: %v", i, err)
		}
		c.Evidence = append(c.Evidence, ev)
		c.Assertions = append(c.Assertions, as)
		c.ClaimEvidence[as.ID] = []domain.ID{ev.ID}
		p.ids[r.Name] = as.ID
	}
	if _, err := db.App.Contribute(ctx, caller, c); err != nil {
		t.Fatalf("seeding the pitch: %v", err)
	}
	// Supersession arrives with slice 4; the state is the signal until then.
	for name, id := range p.ids {
		state := string(domaintest.Pitch(name).Record.State)
		if _, err := db.Admin.Pgx().Exec(ctx, "update assertion set lifecycle_state = $2 where id = $1", string(id), state); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

var pitchNow = domaintest.Day(2026, 10, 14)

func bodyIDs(hits []store.BodyHit) []domain.ID {
	out := make([]domain.ID, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.AssertionID)
	}
	return out
}

func TestSearchBodiesInEachMode(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	p := seedPitch(ctx, t, db, "pitch")
	for _, tc := range []struct {
		name  string
		query domain.Query
		scope domain.Scope
		want  []string
	}{
		{"current", domain.CurrentQuery(), domain.Scope{}, []string{domaintest.PitchDeployment, domaintest.PitchMerge}},
		{"current in production", domain.CurrentQuery(), domain.Scope{Environment: "prod"}, []string{domaintest.PitchDeployment}},
		{"as_of 3 Sep", domain.AsOfQuery(domaintest.Day(2026, 9, 3)), domain.Scope{}, []string{domaintest.PitchFailure, domaintest.PitchImport}},
		{"as_of 3 Sep on 2.4", domain.AsOfQuery(domaintest.Day(2026, 9, 3)), domain.Scope{VersionMin: "2.4", VersionMax: "2.4"}, nil},
		{"as_of 10 Sep", domain.AsOfQuery(domaintest.Day(2026, 9, 10)), domain.Scope{}, []string{domaintest.PitchDeployment, domaintest.PitchFailure, domaintest.PitchMerge}},
		{"historical", domain.HistoricalQuery(), domain.Scope{}, []string{domaintest.PitchDeployment, domaintest.PitchFailure, domaintest.PitchImport, domaintest.PitchMerge}},
		{"historical on 2.3", domain.HistoricalQuery(), domain.Scope{VersionMin: "2.3", VersionMax: "2.3"}, []string{domaintest.PitchFailure, domaintest.PitchImport, domaintest.PitchMerge}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := db.App.SearchBodies(ctx, p.caller, "callback", 10, store.SearchFilter{Query: tc.query, Scope: tc.scope, Now: pitchNow})
			if err != nil {
				t.Fatal(err)
			}
			if got := p.names(bodyIDs(hits)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SearchBodies = %v, want %v", got, tc.want)
			}
			for _, h := range hits {
				name := p.names([]domain.ID{h.AssertionID})[0]
				want := domain.Classify(domaintest.Pitch(name).Record, tc.query, domain.DefaultFreshnessPolicy(), pitchNow)
				if h.Status != want.Status || h.Stale != want.Stale {
					t.Errorf("%s: status %s stale %v, want %s %v", name, h.Status, h.Stale, want.Status, want.Stale)
				}
			}
		})
	}
	// Historical labels each record relative to now.
	hits, err := db.App.SearchBodies(ctx, p.caller, "callback", 10, store.SearchFilter{Query: domain.HistoricalQuery(), Now: pitchNow})
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]domain.TemporalStatus{}
	for _, h := range hits {
		labels[p.names([]domain.ID{h.AssertionID})[0]] = h.Status
	}
	want := map[string]domain.TemporalStatus{
		domaintest.PitchFailure: domain.StatusHistorical, domaintest.PitchMerge: domain.StatusCurrent,
		domaintest.PitchDeployment: domain.StatusCurrent, domaintest.PitchImport: domain.StatusHistorical,
	}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("historical labels = %v, want %v", labels, want)
	}
	// The mode filter runs before the limit: a limit of one in current mode
	// still finds a current record, however the superseded ones rank.
	hits, err = db.App.SearchBodies(ctx, p.caller, "callback failure", 1, store.SearchFilter{Query: domain.CurrentQuery(), Now: pitchNow})
	if err != nil || len(hits) != 1 || hits[0].Status != domain.StatusCurrent {
		t.Errorf("current, limit 1 = %+v, %v; want one current record", hits, err)
	}
}

func TestSearchBodiesStaleUnderTheAreaPolicy(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	p := seedPitch(ctx, t, db, "pitch")
	late := domaintest.Day(2026, 12, 31)
	stale := func(now time.Time) map[string]bool {
		t.Helper()
		hits, err := db.App.SearchBodies(ctx, p.caller, "callback", 10, store.SearchFilter{Query: domain.CurrentQuery(), Now: now})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, h := range hits {
			out[p.names([]domain.ID{h.AssertionID})[0]] = h.Stale
		}
		return out
	}
	// By 31 December neither live record has been checked for 90 days:
	// both are still returned, flagged stale.
	if got, want := stale(late), map[string]bool{domaintest.PitchMerge: true, domaintest.PitchDeployment: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("stale on 31 Dec = %v, want %v", got, want)
	}
	if got, want := stale(pitchNow), map[string]bool{domaintest.PitchMerge: false, domaintest.PitchDeployment: false}; !reflect.DeepEqual(got, want) {
		t.Errorf("stale on 14 Oct = %v, want %v", got, want)
	}
	// The area's own policy replaces the default: 30 days for observations.
	policy := domain.FreshnessPolicy{MaxAgeDays: map[domain.EpistemicKind]int{domain.KindObservation: 30}}
	if err := db.App.PutFreshnessPolicy(ctx, p.caller, p.area.ID, policy); err != nil {
		t.Fatal(err)
	}
	got, err := db.App.FreshnessPolicy(ctx, p.caller, p.area.ID)
	if err != nil || !reflect.DeepEqual(got, policy) {
		t.Fatalf("FreshnessPolicy = %+v, %v; want %+v", got, err, policy)
	}
	if got, want := stale(pitchNow), map[string]bool{domaintest.PitchMerge: true, domaintest.PitchDeployment: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("stale on 14 Oct under 30 days = %v, want %v", got, want)
	}
	// Another area keeps the default.
	other, otherCaller := area(ctx, t, db, "other")
	if got, err := db.App.FreshnessPolicy(ctx, otherCaller, other.ID); err != nil || !reflect.DeepEqual(got, domain.DefaultFreshnessPolicy()) {
		t.Errorf("other area's policy = %+v, %v; want the default", got, err)
	}
	// A caller cannot set another area's policy, and an invalid one is refused.
	if err := db.App.PutFreshnessPolicy(ctx, otherCaller, p.area.ID, policy); err == nil {
		t.Error("PutFreshnessPolicy into an area the caller lacks succeeded")
	}
	if err := db.App.PutFreshnessPolicy(ctx, p.caller, p.area.ID, domain.FreshnessPolicy{MaxAgeDays: map[domain.EpistemicKind]int{domain.KindObservation: -1}}); err == nil {
		t.Error("a negative freshness limit was accepted")
	}
}

func TestClassifyAssertionAsOfByValidityTime(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	p := seedPitch(ctx, t, db, "pitch")
	imp := p.ids[domaintest.PitchImport] // recorded 13 Oct, valid 2 to 9 Sep
	for _, tc := range []struct {
		name string
		at   time.Time
		want bool
	}{
		{"before its validity", domaintest.Day(2026, 9, 1), false},
		{"inside its validity, before it was recorded", domaintest.Day(2026, 9, 3), true},
		{"at its end", domaintest.Day(2026, 9, 9), false},
		{"after it was recorded", domaintest.Day(2026, 10, 14), false},
	} {
		got, err := db.App.ClassifyAssertion(ctx, p.caller, imp, domain.AsOfQuery(tc.at), pitchNow)
		if err != nil {
			t.Fatal(err)
		}
		if got.Included != tc.want || got.Status != domain.StatusHistorical {
			t.Errorf("%s: %+v, want included %v and historical", tc.name, got, tc.want)
		}
	}
	// Another area's caller cannot classify it: it is not found.
	_, other := area(ctx, t, db, "other")
	if _, err := db.App.ClassifyAssertion(ctx, other, imp, domain.HistoricalQuery(), pitchNow); err == nil {
		t.Error("ClassifyAssertion across areas succeeded")
	}
}

func TestSearchNamesInEachMode(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	a, caller := area(ctx, t, db, "names")
	renamed := domaintest.Day(2026, 6, 1)
	e, err := domain.NewEntity(domain.ID(store.NewID()), a.ID, domain.EntityService,
		domain.EntityName{Name: "Import Tool", ValidTo: &renamed, Scope: domain.Scope{Environment: "prod"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddName(domain.EntityName{Name: "Ingest Service", ValidFrom: &renamed, Scope: domain.Scope{Environment: "prod"}}); err != nil {
		t.Fatal(err)
	}
	c := store.Contribution{
		Contribution: domain.Contribution{ID: domain.ID(store.NewID()), AreaID: a.ID, ActorUserID: "alice", Client: "test", RecordedAt: t0},
		Entities:     []*domain.Entity{e},
	}
	if _, err := db.App.Contribute(ctx, caller, c); err != nil {
		t.Fatal(err)
	}
	search := func(name string, q domain.Query, scope domain.Scope) []domain.TemporalStatus {
		t.Helper()
		hits, err := db.App.SearchNames(ctx, caller, name, domain.NameSimilarityThreshold, 10, store.SearchFilter{Query: q, Scope: scope, Now: pitchNow})
		if err != nil {
			t.Fatal(err)
		}
		var out []domain.TemporalStatus
		for _, h := range hits {
			if h.Name == name {
				out = append(out, h.Status)
			}
		}
		return out
	}
	cur, hist := []domain.TemporalStatus{domain.StatusCurrent}, []domain.TemporalStatus{domain.StatusHistorical}
	for _, tc := range []struct {
		name, query string
		q           domain.Query
		scope       domain.Scope
		want        []domain.TemporalStatus
	}{
		{"current name, current", "Ingest Service", domain.CurrentQuery(), domain.Scope{}, cur},
		{"old name, current", "Import Tool", domain.CurrentQuery(), domain.Scope{}, nil},
		{"old name, historical", "Import Tool", domain.HistoricalQuery(), domain.Scope{}, hist},
		{"old name, as_of before the rename", "Import Tool", domain.AsOfQuery(domaintest.Day(2026, 5, 1)), domain.Scope{}, hist},
		{"new name, as_of before the rename", "Ingest Service", domain.AsOfQuery(domaintest.Day(2026, 5, 1)), domain.Scope{}, nil},
		{"current name, staging scope", "Ingest Service", domain.CurrentQuery(), domain.Scope{Environment: "staging"}, nil},
		{"current name, production scope", "Ingest Service", domain.CurrentQuery(), domain.Scope{Environment: "PROD"}, cur},
	} {
		if got := search(tc.query, tc.q, tc.scope); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: statuses %v, want %v", tc.name, got, tc.want)
		}
	}
	modes, err := db.App.ClassifyNames(ctx, caller, e.ID(), domain.AsOfQuery(domaintest.Day(2026, 5, 1)), pitchNow)
	if err != nil {
		t.Fatal(err)
	}
	want := []store.NameMode{
		{Position: 0, Included: true, Status: domain.StatusHistorical},
		{Position: 1, Included: false, Status: domain.StatusCurrent},
	}
	if !reflect.DeepEqual(modes, want) {
		t.Errorf("ClassifyNames as_of 1 May = %+v, want %+v", modes, want)
	}
}
