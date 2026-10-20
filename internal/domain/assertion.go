package domain

import (
	"strings"
	"time"
)

// EpistemicKind says what kind of claim an assertion is (PR-03, TR-22). The
// kind is explicit on every assertion, visible in every answer and never
// changes. Observations cite evidence; decisions and inferences may cite
// evidence, be derived from other assertions, or both (PR-13, PR-42).
type EpistemicKind string

// The three epistemic kinds.
const (
	KindObservation EpistemicKind = "observation" // what was seen
	KindDecision    EpistemicKind = "decision"    // what was approved or intended
	KindInference   EpistemicKind = "inference"   // what was concluded
)

// EpistemicKinds lists the kinds in a stable order.
func EpistemicKinds() []EpistemicKind {
	return []EpistemicKind{KindObservation, KindDecision, KindInference}
}

// Valid reports whether k is one of the three kinds.
func (k EpistemicKind) Valid() bool {
	for _, c := range EpistemicKinds() {
		if c == k {
			return true
		}
	}
	return false
}

// ParseEpistemicKind returns the kind named by s, or ErrUnknownEpistemicKind.
func ParseEpistemicKind(s string) (EpistemicKind, error) {
	k := EpistemicKind(s)
	if !k.Valid() {
		return "", ErrUnknownEpistemicKind
	}
	return k, nil
}

// Assertion is a claim about a subject entity (TR-22): its epistemic kind, a
// Markdown body, the scope it applies in and the temporal fields. Evidence is
// linked to it separately, through claim evidence; the assertion itself does
// not embed its sources.
type Assertion struct {
	ID              ID
	AreaID          ID
	SubjectEntityID ID
	Kind            EpistemicKind
	BodyMarkdown    string
	Scope           Scope
	Temporal
}

// NewAssertion builds an assertion of the given kind recorded at
// recordedAt. It sets RecordedAt only: ValidFrom and ValidTo stay nil because
// the time a fact was written down says nothing about when it applied
// (TR-32). A contributor who knows the validity sets it afterwards.
func NewAssertion(kind EpistemicKind, id, area, subject ID, bodyMarkdown string, scope Scope, recordedAt time.Time) (Assertion, error) {
	a := Assertion{
		ID:              id,
		AreaID:          area,
		SubjectEntityID: subject,
		Kind:            kind,
		BodyMarkdown:    bodyMarkdown,
		Scope:           scope,
		Temporal:        Temporal{RecordedAt: recordedAt},
	}
	if err := a.Validate(); err != nil {
		return Assertion{}, err
	}
	return a, nil
}

// NewObservation builds an observation: what was seen. See NewAssertion.
func NewObservation(id, area, subject ID, bodyMarkdown string, scope Scope, recordedAt time.Time) (Assertion, error) {
	return NewAssertion(KindObservation, id, area, subject, bodyMarkdown, scope, recordedAt)
}

// NewDecision builds a decision: what was approved or intended. See NewAssertion.
func NewDecision(id, area, subject ID, bodyMarkdown string, scope Scope, recordedAt time.Time) (Assertion, error) {
	return NewAssertion(KindDecision, id, area, subject, bodyMarkdown, scope, recordedAt)
}

// NewInference builds an inference: what was concluded. See NewAssertion.
func NewInference(id, area, subject ID, bodyMarkdown string, scope Scope, recordedAt time.Time) (Assertion, error) {
	return NewAssertion(KindInference, id, area, subject, bodyMarkdown, scope, recordedAt)
}

// CheckGrounds checks what an assertion of kind k rests on: the number of
// evidence records it cites and of assertions it is derived from. An
// observation is what was seen, so it cites evidence of its own (PR-43) and
// is never derived; a decision or an inference may cite evidence, be derived
// from other assertions, or both, but needs at least one (PR-13, PR-42).
func (k EpistemicKind) CheckGrounds(evidence, sources int) error {
	switch {
	case !k.Valid():
		return ErrUnknownEpistemicKind
	case k == KindObservation && sources > 0:
		return ErrObservationDerived
	case k == KindObservation && evidence == 0:
		return ErrNoEvidence
	case evidence == 0 && sources == 0:
		return ErrNoGrounds
	}
	return nil
}

// Validate checks the identifiers, the kind, the body and the temporal fields.
func (a Assertion) Validate() error {
	if err := a.ID.Validate(); err != nil {
		return fieldErr("assertion id", err)
	}
	if err := a.AreaID.Validate(); err != nil {
		return fieldErr("area id", err)
	}
	if err := a.SubjectEntityID.Validate(); err != nil {
		return fieldErr("subject entity id", err)
	}
	if !a.Kind.Valid() {
		return fieldErr("epistemic kind", ErrUnknownEpistemicKind)
	}
	if strings.TrimSpace(a.BodyMarkdown) == "" {
		return fieldErr("body", ErrEmptyBody)
	}
	return a.Temporal.Validate()
}
