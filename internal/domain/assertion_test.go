package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseEpistemicKind(t *testing.T) {
	tests := []struct {
		in   string
		want EpistemicKind
		err  error
	}{
		{"observation", KindObservation, nil},
		{"decision", KindDecision, nil},
		{"inference", KindInference, nil},
		{"", "", ErrUnknownEpistemicKind},
		{"Observation", "", ErrUnknownEpistemicKind},
		{"guess", "", ErrUnknownEpistemicKind},
	}
	for _, tt := range tests {
		got, err := ParseEpistemicKind(tt.in)
		if !errors.Is(err, tt.err) || got != tt.want {
			t.Errorf("ParseEpistemicKind(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
		}
	}
	if kinds := EpistemicKinds(); len(kinds) != 3 {
		t.Errorf("EpistemicKinds() = %v, want three kinds", kinds)
	}
}

// The constructor never derives validity from the recording time (TR-32).
func TestNewObservationLeavesValidityUnknown(t *testing.T) {
	a, err := NewObservation("as1", "a1", "e1", "The API returned 503s.", Scope{Environment: "prod"}, t1)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindObservation {
		t.Errorf("Kind = %q, want observation", a.Kind)
	}
	if !a.RecordedAt.Equal(t1) {
		t.Errorf("RecordedAt = %v, want %v", a.RecordedAt, t1)
	}
	if a.ValidFrom != nil || a.ValidTo != nil {
		t.Errorf("validity set from recording time: from=%v to=%v", a.ValidFrom, a.ValidTo)
	}
	if a.VerifiedAt != nil || a.ReviewDueAt != nil {
		t.Errorf("verified or review-due set by constructor")
	}
	if !a.ValidityUnknown() {
		t.Error("ValidityUnknown() = false, want true")
	}
	if a.Scope.Environment != "prod" {
		t.Errorf("Scope not kept: %+v", a.Scope)
	}
}

func TestNewObservationRejects(t *testing.T) {
	tests := []struct {
		name       string
		id, area   ID
		subject    ID
		body       string
		recordedAt time.Time
		want       error
	}{
		{"empty id", "", "a1", "e1", "body", t1, ErrEmptyID},
		{"empty area", "as1", "", "e1", "body", t1, ErrEmptyID},
		{"empty subject", "as1", "a1", "", "body", t1, ErrEmptyID},
		{"empty body", "as1", "a1", "e1", "   ", t1, ErrEmptyBody},
		{"zero recorded_at", "as1", "a1", "e1", "body", time.Time{}, ErrNoRecordedAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewObservation(tt.id, tt.area, tt.subject, tt.body, Scope{}, tt.recordedAt)
			if !errors.Is(err, tt.want) {
				t.Errorf("NewObservation() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAssertionValidate(t *testing.T) {
	ok := Assertion{ID: "as1", AreaID: "a1", SubjectEntityID: "e1", Kind: KindDecision, BodyMarkdown: "Approved.", Temporal: Temporal{RecordedAt: t1}}
	tests := []struct {
		name   string
		mutate func(*Assertion)
		want   error
	}{
		{"ok", func(*Assertion) {}, nil},
		{"unknown kind", func(a *Assertion) { a.Kind = "guess" }, ErrUnknownEpistemicKind},
		{"from after to", func(a *Assertion) { a.ValidFrom, a.ValidTo = ptr(t2), ptr(t1) }, ErrInvalidValidity},
		{"window ok", func(a *Assertion) { a.ValidFrom, a.ValidTo = ptr(t0), ptr(t2) }, nil},
		{"no recorded_at", func(a *Assertion) { a.RecordedAt = time.Time{} }, ErrNoRecordedAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := ok
			tt.mutate(&a)
			if err := a.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestScopeIsUnspecified(t *testing.T) {
	if !(Scope{}).IsUnspecified() {
		t.Error("zero scope should be unspecified")
	}
	if (Scope{Tenant: "t"}).IsUnspecified() {
		t.Error("scope with a tenant is specified")
	}
}
