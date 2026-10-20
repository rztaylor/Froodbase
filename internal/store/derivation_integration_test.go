//go:build integration

package store_test

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

// deduction builds an assertion of kind about subject derived from sources.
func deduction(t *testing.T, kind domain.EpistemicKind, area, subject domain.ID, body string, sources ...domain.ID) (domain.Assertion, []domain.Derivation) {
	t.Helper()
	a, err := domain.NewAssertion(kind, domain.ID(store.NewID()), area, subject, body, domain.Scope{Environment: "prod"}, t1)
	if err != nil {
		t.Fatal(err)
	}
	var links []domain.Derivation
	for _, s := range sources {
		links = append(links, domain.Derivation{DerivedID: a.ID, SourceID: s, AreaID: area, Rationale: "concluded during INC-1", RecordedAt: t1})
	}
	return a, links
}

func contributionOf(area domain.ID, assertions []domain.Assertion, links []domain.Derivation) store.Contribution {
	return store.Contribution{
		Contribution:  domain.Contribution{ID: domain.ID(store.NewID()), AreaID: area, ActorUserID: "alice", ActorAgentID: "claude", Client: "test", RecordedAt: t1},
		Assertions:    assertions,
		ClaimEvidence: map[domain.ID][]domain.ID{},
		Derivations:   links,
		Rationale:     "concluded during INC-1",
	}
}

func rootsOf(t *testing.T, events []domain.SupportEvent) map[domain.ID]string {
	t.Helper()
	out := map[domain.ID]string{}
	for _, e := range events {
		if e.Type != domain.SupportRootAdded || len(e.EvidenceRootIDs) != 1 {
			t.Errorf("unexpected support event %+v", e)
			continue
		}
		out[e.EvidenceRootIDs[0]] = e.Rationale
	}
	return out
}

// A deduction inherits one root_added per distinct root of its sources plus
// its own citations; a root shared by sources counts once, the rationale
// names the sources, and a deduction derived from a deduction written in the
// same call (listed first) inherits the full set.
func TestDeductionInheritsDistinctRoots(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	f := newFixture(t, areaA, "The API returned 503s.") // roots ev1 (with ev2 restating it) and ev3
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}
	g := newFixture(t, areaA, "The queue backed up.")
	g.entity = f.entity
	g.contribution.Entities = nil
	g.assertion.SubjectEntityID = f.entity.ID()
	g.contribution.Assertions = []domain.Assertion{g.assertion}
	// g cites f's ev1 (a shared root) and its own ev3.
	g.ev3.SourceURI = "https://dev.azure.com/org/p/_workitems/edit/43"
	g.contribution.Evidence = []domain.Evidence{g.ev3}
	g.contribution.ClaimEvidence = map[domain.ID][]domain.ID{g.assertion.ID: {f.ev1.ID, g.ev3.ID}}
	if _, err := db.App.Contribute(ctx, callerA, g.contribution); err != nil {
		t.Fatal(err)
	}

	inf, infLinks := deduction(t, domain.KindInference, areaA.ID, f.entity.ID(), "The 503s were caused by the queue backing up.", f.assertion.ID, g.assertion.ID)
	dec, decLinks := deduction(t, domain.KindDecision, areaA.ID, f.entity.ID(), "Scale the queue before the next release.", inf.ID)
	direct := evidence(areaA.ID, "https://dev.azure.com/org/p/_workitems/edit/77", "")
	direct.SourceKind = domain.SourceTicket
	c := contributionOf(areaA.ID, []domain.Assertion{dec, inf}, append(decLinks, infLinks...))
	c.Evidence = []domain.Evidence{direct}
	c.ClaimEvidence[inf.ID] = []domain.ID{direct.ID}
	res, err := db.App.Contribute(ctx, callerA, c)
	if err != nil {
		t.Fatal(err)
	}
	w := domain.DefaultSupportConfig().EventWeights[domain.SupportRootAdded]
	// Roots: ev1 (f and g), f.ev3, g.ev3, and the direct ticket: four.
	if res.Roots[inf.ID] != 4 || res.Scores[inf.ID] != 4*w || res.Roots[dec.ID] != 4 || res.Scores[dec.ID] != 4*w {
		t.Errorf("result roots %v scores %v, want 4 roots each", res.Roots, res.Scores)
	}
	if got := res.Written[1]; got.ID != inf.ID || got.Kind != string(domain.KindInference) {
		t.Errorf("written[1] = %+v, want the inference first: sources are written before what is derived from them", got)
	}

	events, err := db.App.SupportEvents(ctx, callerA, inf.ID)
	if err != nil {
		t.Fatal(err)
	}
	roots := rootsOf(t, events)
	fs, gs := string(f.assertion.ID), string(g.assertion.ID)
	pair := "inherited from " + strings.Join(sorted(fs, gs), ", ")
	want := map[domain.ID]string{f.ev1.ID: pair, f.ev3.ID: "inherited from " + fs, g.ev3.ID: "inherited from " + gs, direct.ID: c.Rationale}
	if !reflect.DeepEqual(roots, want) {
		t.Errorf("inference roots = %v, want %v", roots, want)
	}
	decEvents, err := db.App.SupportEvents(ctx, callerA, dec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if roots := rootsOf(t, decEvents); len(roots) != 4 || roots[direct.ID] != "inherited from "+string(inf.ID) {
		t.Errorf("decision roots = %v, want the inference's four, inherited from it", roots)
	}

	links, err := db.App.Derivations(ctx, callerA, inf.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range links {
		if l.AreaID != areaA.ID || l.Rationale != "concluded during INC-1" || l.RecordedAt.IsZero() {
			t.Errorf("derivation %+v is not well formed", l)
		}
		got = append(got, string(l.SourceID))
	}
	if !reflect.DeepEqual(sorted(got...), sorted(fs, gs)) {
		t.Errorf("inference sources = %v, want %s and %s", got, fs, gs)
	}
	if links, _ := db.App.Derivations(ctx, callerA, f.assertion.ID); len(links) != 0 {
		t.Errorf("an observation has derivations: %v", links)
	}
	if a, err := db.App.GetAssertion(ctx, callerA, inf.ID); err != nil || a.LifecycleState != store.LifecycleActive || a.Assertion.Kind != domain.KindInference {
		t.Errorf("inference = %+v, %v; want active inference", a, err)
	}
	if score, err := db.App.RecomputeSupportScore(ctx, callerA, inf.ID); err != nil || score != 4*w {
		t.Errorf("recomputed score %v, %v; want %v", score, err, 4*w)
	}

	// Another area sees no derivation rows, and the service role cannot
	// change one.
	areaB, callerB := area(ctx, t, db, "area-b")
	if n := countAs(ctx, t, db.App, []string{string(areaB.ID)}, "select count(*) from derivation"); n != 0 {
		t.Errorf("area b sees %d derivation rows, want 0", n)
	}
	if links, err := db.App.Derivations(ctx, callerB, inf.ID); err != nil || len(links) != 0 {
		t.Errorf("area b reads derivations %v, %v; want none", links, err)
	}
	if n := countAs(ctx, t, db.App, []string{string(areaA.ID)}, "select count(*) from derivation"); n != 3 {
		t.Errorf("area a sees %d derivation rows, want 3", n)
	}
	for _, stmt := range []string{"update derivation set rationale = 'x'", "delete from derivation"} {
		if _, err := db.App.Pgx().Exec(ctx, stmt); err == nil {
			t.Errorf("%q succeeded as the service role; derivations are append-only", stmt)
		}
	}
}

// Deriving from an unknown id, from an assertion in another area or in a
// cycle is refused and nothing is written.
func TestDerivationRefusals(t *testing.T) {
	ctx := ctxT(t)
	db := storetest.Open(t)
	areaA, callerA := area(ctx, t, db, "area-a")
	areaB, callerB := area(ctx, t, db, "area-b")
	f := newFixture(t, areaA, "The API returned 503s.")
	if _, err := db.App.Contribute(ctx, callerA, f.contribution); err != nil {
		t.Fatal(err)
	}
	other := newFixture(t, areaB, "Area b saw timeouts.")
	if _, err := db.App.Contribute(ctx, callerB, other.contribution); err != nil {
		t.Fatal(err)
	}
	both := store.Caller{UserID: "alice", AgentID: "claude", AreaIDs: []string{string(areaA.ID), string(areaB.ID)}}
	subject := f.entity.ID()

	unknown, unknownLinks := deduction(t, domain.KindInference, areaA.ID, subject, "From nowhere.", f.assertion.ID, domain.ID(store.NewID()))
	cross, crossLinks := deduction(t, domain.KindInference, areaA.ID, subject, "From another area.", f.assertion.ID, other.assertion.ID)
	a, aLinks := deduction(t, domain.KindInference, areaA.ID, subject, "A from B.")
	b, bLinks := deduction(t, domain.KindInference, areaA.ID, subject, "B from A.", a.ID)
	aLinks = append(aLinks, domain.Derivation{DerivedID: a.ID, SourceID: b.ID, AreaID: areaA.ID, RecordedAt: t1})
	self, _ := deduction(t, domain.KindInference, areaA.ID, subject, "From itself.")
	selfLinks := []domain.Derivation{{DerivedID: self.ID, SourceID: self.ID, AreaID: areaA.ID, RecordedAt: t1}}

	tests := []struct {
		name   string
		caller store.Caller
		c      store.Contribution
		want   error
	}{
		{"unknown id", callerA, contributionOf(areaA.ID, []domain.Assertion{unknown}, unknownLinks), store.ErrNotFound},
		{"another area, caller holds both", both, contributionOf(areaA.ID, []domain.Assertion{cross}, crossLinks), store.ErrCrossArea},
		{"another area, caller holds one", callerA, contributionOf(areaA.ID, []domain.Assertion{cross}, crossLinks), store.ErrNotFound},
		{"cycle", callerA, contributionOf(areaA.ID, []domain.Assertion{a, b}, append(aLinks, bLinks...)), domain.ErrDerivationCycle},
		{"self", callerA, contributionOf(areaA.ID, []domain.Assertion{self}, selfLinks), domain.ErrDerivationCycle},
	}
	count := func() int {
		return countAdmin(ctx, t, db.Admin, "select (select count(*) from assertion) + (select count(*) from derivation) + (select count(*) from support_event) + (select count(*) from contribution) + (select count(*) from revision)")
	}
	before := count()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.App.Contribute(ctx, tt.caller, tt.c); !errors.Is(err, tt.want) {
				t.Errorf("Contribute() = %v, want %v", err, tt.want)
			}
			if after := count(); after != before {
				t.Errorf("a refused contribution wrote %d rows", after-before)
			}
		})
	}

	// The database refuses a cross-area link even from the owner.
	_, err := db.Admin.Pgx().Exec(ctx, "insert into derivation (derived_id, source_id, area_id, recorded_at) values ($1, $2, $3, now())",
		string(f.assertion.ID), string(other.assertion.ID), string(areaA.ID))
	if err == nil {
		t.Error("the database accepted a derivation whose source is in another area")
	}
}

func sorted(s ...string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}
