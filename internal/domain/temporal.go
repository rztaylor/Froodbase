package domain

import "time"

// Scope is where a fact applies: the fixed dimensions of the architecture's
// scope object. An empty string means unspecified for that dimension. In this
// slice Scope is carried and stored as given; the overlap rules that compare
// two scopes (TR-26) arrive in slice 2.
type Scope struct {
	Environment string
	Region      string
	VersionMin  string
	VersionMax  string
	Cluster     string
	Tenant      string
}

// IsUnspecified reports whether no dimension is set.
func (s Scope) IsUnspecified() bool {
	return s == Scope{}
}

// Temporal holds the two clocks of a record (TR-30): when the fact applied
// (ValidFrom, ValidTo; nil means unknown or no known end) and when Froodbase
// learned it (RecordedAt, never zero), plus the review clock (VerifiedAt,
// ReviewDueAt). Validity is never populated from RecordedAt (TR-32); a record
// whose validity nobody supplied keeps nil dates and reports ValidityUnknown.
type Temporal struct {
	ValidFrom   *time.Time
	ValidTo     *time.Time
	RecordedAt  time.Time
	VerifiedAt  *time.Time
	ReviewDueAt *time.Time
}

// ValidityUnknown reports whether neither validity date was supplied.
func (t Temporal) ValidityUnknown() bool {
	return t.ValidFrom == nil && t.ValidTo == nil
}

// Validate checks that the record has a recording time and a well-formed
// validity window.
func (t Temporal) Validate() error {
	if t.RecordedAt.IsZero() {
		return fieldErr("recorded_at", ErrNoRecordedAt)
	}
	return validateWindow(t.ValidFrom, t.ValidTo)
}

func validateWindow(from, to *time.Time) error {
	if from != nil && to != nil && !from.Before(*to) {
		return ErrInvalidValidity
	}
	return nil
}

// windowContains reports whether t lies in the half-open window [from, to).
func windowContains(from, to *time.Time, t time.Time) bool {
	if from != nil && t.Before(*from) {
		return false
	}
	if to != nil && !t.Before(*to) {
		return false
	}
	return true
}
