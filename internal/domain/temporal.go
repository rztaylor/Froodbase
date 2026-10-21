package domain

import "time"

// Temporal holds the two clocks of a record (TR-30): when the fact applied
// (ValidFrom, ValidTo; nil means unknown or no known end) and when Froodbase
// learned it (RecordedAt, never zero), plus the review clock (VerifiedAt,
// ReviewDueAt). Validity is never populated from RecordedAt (TR-32); a record
// whose validity nobody supplied keeps nil dates and reports ValidityUnknown.
// A nil ValidTo means "not known to have ended", never "verified current"
// (TR-31): whether a record is current is the mode predicate's question, and
// how recently it was checked is the stale rule's.
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

// ValidAt reports whether the validity window includes at. The window is
// half-open, [ValidFrom, ValidTo), and a nil bound is open, so a record whose
// validity is unknown is not ruled out at any time. Recording time plays no
// part (TR-34).
func (t Temporal) ValidAt(at time.Time) bool {
	return windowContains(t.ValidFrom, t.ValidTo, at)
}

// LastChecked is when the record was last known to hold: VerifiedAt, or
// RecordedAt for a record never verified since it was written. It measures
// freshness only and never stands in for validity.
func (t Temporal) LastChecked() time.Time {
	if t.VerifiedAt != nil {
		return *t.VerifiedAt
	}
	return t.RecordedAt
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
