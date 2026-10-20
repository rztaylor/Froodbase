package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNewAssertionPerKind(t *testing.T) {
	ctors := map[EpistemicKind]func(id, area, subject ID, body string, scope Scope, at time.Time) (Assertion, error){
		KindObservation: NewObservation,
		KindDecision:    NewDecision,
		KindInference:   NewInference,
	}
	for kind, ctor := range ctors {
		t.Run(string(kind), func(t *testing.T) {
			a, err := ctor("as1", "a1", "e1", "Body.", Scope{Environment: "prod"}, t1)
			if err != nil {
				t.Fatal(err)
			}
			if a.Kind != kind {
				t.Errorf("Kind = %q, want %q", a.Kind, kind)
			}
			if !a.RecordedAt.Equal(t1) || !a.ValidityUnknown() {
				t.Errorf("temporal fields = %+v, want recorded_at only (TR-32)", a.Temporal)
			}
			if _, err := ctor("as1", "a1", "e1", " ", Scope{}, t1); !errors.Is(err, ErrEmptyBody) {
				t.Errorf("empty body: %v, want ErrEmptyBody", err)
			}
		})
	}
	if _, err := NewAssertion("guess", "as1", "a1", "e1", "Body.", Scope{}, t1); !errors.Is(err, ErrUnknownEpistemicKind) {
		t.Errorf("NewAssertion(guess) = %v, want ErrUnknownEpistemicKind", err)
	}
}

// An observation is what was seen, so it cites evidence and is never derived;
// a decision or an inference may cite evidence, be derived, or both, but
// needs at least one ground (PR-03, PR-13, PR-43).
func TestCheckGrounds(t *testing.T) {
	tests := []struct {
		kind              EpistemicKind
		evidence, sources int
		want              error
	}{
		{KindObservation, 1, 0, nil},
		{KindObservation, 0, 0, ErrNoEvidence},
		{KindObservation, 0, 2, ErrObservationDerived},
		{KindObservation, 1, 1, ErrObservationDerived},
		{KindDecision, 1, 0, nil},
		{KindDecision, 0, 1, nil},
		{KindDecision, 2, 2, nil},
		{KindDecision, 0, 0, ErrNoGrounds},
		{KindInference, 0, 2, nil},
		{KindInference, 1, 0, nil},
		{KindInference, 1, 1, nil},
		{KindInference, 0, 0, ErrNoGrounds},
		{"guess", 1, 0, ErrUnknownEpistemicKind},
	}
	for _, tt := range tests {
		if err := tt.kind.CheckGrounds(tt.evidence, tt.sources); !errors.Is(err, tt.want) {
			t.Errorf("%s.CheckGrounds(%d, %d) = %v, want %v", tt.kind, tt.evidence, tt.sources, err, tt.want)
		}
	}
}

func TestDerivationValidate(t *testing.T) {
	ok := Derivation{DerivedID: "d", SourceID: "s", AreaID: "a1", RecordedAt: t1}
	tests := []struct {
		name   string
		mutate func(*Derivation)
		want   error
	}{
		{"ok", func(*Derivation) {}, nil},
		{"no derived", func(d *Derivation) { d.DerivedID = "" }, ErrEmptyID},
		{"no source", func(d *Derivation) { d.SourceID = " " }, ErrEmptyID},
		{"no area", func(d *Derivation) { d.AreaID = "" }, ErrEmptyID},
		{"self", func(d *Derivation) { d.SourceID = d.DerivedID }, ErrDerivationCycle},
		{"no recorded_at", func(d *Derivation) { d.RecordedAt = time.Time{} }, ErrNoRecordedAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ok
			tt.mutate(&d)
			if err := d.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func links(pairs ...ID) []Derivation {
	var out []Derivation
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Derivation{DerivedID: pairs[i], SourceID: pairs[i+1]})
	}
	return out
}

// Records written together are ordered so every source comes before what is
// derived from it; sources outside the set (existing records) impose no
// order, and a cycle among the set is refused.
func TestOrderByDerivation(t *testing.T) {
	tests := []struct {
		name  string
		ids   []ID
		links []Derivation
		want  []ID
		err   error
	}{
		{"no links keeps input order", []ID{"a", "b", "c"}, nil, []ID{"a", "b", "c"}, nil},
		{"source after derived moves first", []ID{"inf", "o1", "o2"}, links("inf", "o1", "inf", "o2"), []ID{"o1", "o2", "inf"}, nil},
		{"chain", []ID{"c", "b", "a"}, links("c", "b", "b", "a"), []ID{"a", "b", "c"}, nil},
		{"existing sources impose nothing", []ID{"inf"}, links("inf", "old1", "inf", "old2"), []ID{"inf"}, nil},
		{"diamond", []ID{"top", "l", "r", "base"}, links("top", "l", "top", "r", "l", "base", "r", "base"), []ID{"base", "l", "r", "top"}, nil},
		{"self", []ID{"a"}, links("a", "a"), nil, ErrDerivationCycle},
		{"two-cycle", []ID{"a", "b"}, links("a", "b", "b", "a"), nil, ErrDerivationCycle},
		{"three-cycle beside a good record", []ID{"ok", "a", "b", "c"}, links("a", "b", "b", "c", "c", "a"), nil, ErrDerivationCycle},
		{"derived not in the set", []ID{"a"}, links("x", "a"), nil, ErrUnknownDerived},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OrderByDerivation(tt.ids, tt.links)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
		})
	}
}

// A deduction's roots are the union of its sources' distinct roots and the
// roots of its own evidence (PR-12, TR-25): a root shared by several sources
// or also cited directly counts once, and each root names where it came from.
func TestDerivedRoots(t *testing.T) {
	ev := func(id, root ID) Evidence { return Evidence{ID: id, RootEvidenceID: root} }
	tests := []struct {
		name    string
		direct  []Evidence
		sources map[ID][]ID
		want    []RootOrigin
	}{
		{"nothing", nil, nil, nil},
		{"direct only, restatement counts once", []Evidence{ev("r1", ""), ev("x", "r1")}, nil,
			[]RootOrigin{{Root: "r1", Direct: true}}},
		{"two sources, disjoint roots", nil, map[ID][]ID{"o1": {"r1"}, "o2": {"r2"}},
			[]RootOrigin{{Root: "r1", Sources: []ID{"o1"}}, {Root: "r2", Sources: []ID{"o2"}}}},
		{"shared root counts once and names both sources", nil, map[ID][]ID{"o2": {"r1", "r2"}, "o1": {"r1"}},
			[]RootOrigin{{Root: "r1", Sources: []ID{"o1", "o2"}}, {Root: "r2", Sources: []ID{"o2"}}}},
		{"direct and inherited", []Evidence{ev("r3", ""), ev("r1", "")}, map[ID][]ID{"o1": {"r1", "r2"}},
			[]RootOrigin{{Root: "r1", Direct: true, Sources: []ID{"o1"}}, {Root: "r2", Sources: []ID{"o1"}}, {Root: "r3", Direct: true}}},
		{"source with a repeated root", nil, map[ID][]ID{"o1": {"r1", "r1"}},
			[]RootOrigin{{Root: "r1", Sources: []ID{"o1"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DerivedRoots(tt.direct, tt.sources)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DerivedRoots() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRootOriginRationale(t *testing.T) {
	tests := []struct {
		origin RootOrigin
		call   string
		want   string
	}{
		{RootOrigin{Root: "r", Direct: true}, "seen during INC-1", "seen during INC-1"},
		{RootOrigin{Root: "r", Direct: true, Sources: []ID{"o1"}}, "seen", "seen"},
		{RootOrigin{Root: "r", Sources: []ID{"o1"}}, "seen", "inherited from o1"},
		{RootOrigin{Root: "r", Sources: []ID{"o1", "o2"}}, "", "inherited from o1, o2"},
	}
	for _, tt := range tests {
		if got := tt.origin.Rationale(tt.call); got != tt.want {
			t.Errorf("%+v.Rationale(%q) = %q, want %q", tt.origin, tt.call, got, tt.want)
		}
	}
}
