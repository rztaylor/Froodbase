package domain

import "time"

// Mode is how a query treats time (TR-53): what is true now, what was true
// at a given time, or everything with a label.
type Mode string

// The three query modes.
const (
	ModeCurrent    Mode = "current"    // in force and valid now
	ModeAsOf       Mode = "as_of"      // valid at a given time, by validity time
	ModeHistorical Mode = "historical" // everything, each labelled
)

// Modes lists the modes in a stable order.
func Modes() []Mode {
	return []Mode{ModeCurrent, ModeAsOf, ModeHistorical}
}

// Valid reports whether m is one of the three modes.
func (m Mode) Valid() bool {
	for _, c := range Modes() {
		if c == m {
			return true
		}
	}
	return false
}

// ParseMode returns the mode named by s, or ErrUnknownMode.
func ParseMode(s string) (Mode, error) {
	m := Mode(s)
	if !m.Valid() {
		return "", ErrUnknownMode
	}
	return m, nil
}

// Query is a mode with, for as_of, the validity time asked about.
type Query struct {
	Mode Mode
	AsOf time.Time
}

// CurrentQuery asks what is true now.
func CurrentQuery() Query { return Query{Mode: ModeCurrent} }

// AsOfQuery asks what was valid at t, by validity time.
func AsOfQuery(t time.Time) Query { return Query{Mode: ModeAsOf, AsOf: t} }

// HistoricalQuery asks for everything, labelled.
func HistoricalQuery() Query { return Query{Mode: ModeHistorical} }

// Validate checks the mode and that a time is given for as_of and only for it.
func (q Query) Validate() error {
	switch {
	case !q.Mode.Valid():
		return ErrUnknownMode
	case q.Mode == ModeAsOf && q.AsOf.IsZero():
		return ErrNoAsOf
	case q.Mode != ModeAsOf && !q.AsOf.IsZero():
		return ErrAsOfNotAllowed
	}
	return nil
}

// TemporalStatus labels a record relative to now, in every mode, so a past
// record is never presented as current state (PR-24, PR-25).
type TemporalStatus string

// The three statuses.
const (
	StatusCurrent    TemporalStatus = "current"    // in force and valid now
	StatusHistorical TemporalStatus = "historical" // superseded, retracted, or valid only before now
	StatusUpcoming   TemporalStatus = "upcoming"   // in force, valid only from a time after now
)

// ModeRecord is what the mode predicate and the stale rule read of a
// record: its kind (for the freshness policy), its lifecycle state and its
// temporal fields. Scope is filtered separately, with Scope.Overlaps.
// Supersession is read from the state: a superseded record keeps its
// validity unless its contributor asserted an end (TR-33).
type ModeRecord struct {
	Kind  EpistemicKind
	State LifecycleState
	Temporal
}

// StatusAt labels the record relative to now: current when it is in force
// and its validity includes now; upcoming when it is in force and valid only
// from later; historical otherwise.
func (r ModeRecord) StatusAt(now time.Time) TemporalStatus {
	switch {
	case !r.State.InForce():
		return StatusHistorical
	case r.ValidAt(now):
		return StatusCurrent
	case r.ValidFrom != nil && now.Before(*r.ValidFrom):
		return StatusUpcoming
	}
	return StatusHistorical
}

// InMode reports whether the query's mode keeps the record (TR-31, TR-34,
// TR-53). current keeps an active or disputed record whose validity
// includes now, a null end being "not known to have ended"; superseded,
// retracted and expired records are left out. as_of keeps a record whose
// validity includes the as_of time, whenever it was recorded, unless it was
// retracted; a superseded record is kept, labelled, because superseding
// does not end its validity. historical keeps everything. An invalid query
// keeps nothing.
func (r ModeRecord) InMode(q Query, now time.Time) bool {
	switch q.Mode {
	case ModeCurrent:
		return r.StatusAt(now) == StatusCurrent
	case ModeAsOf:
		return (r.State.InForce() || r.State == LifecycleSuperseded) && r.ValidAt(q.AsOf)
	case ModeHistorical:
		return true
	}
	return false
}

// ModeResult is the predicate's answer for one record: whether the mode
// keeps it, its status relative to now, and whether it is stale.
type ModeResult struct {
	Included bool
	Status   TemporalStatus
	Stale    bool
}

// Classify applies the mode predicate, the status label and the stale rule
// to a record under the area's freshness policy. A record that is current
// now and was last checked longer ago than its kind allows is stale: it is
// still returned, flagged, never dropped (TR-31). Stale applies only to a
// current record, in whichever mode it was found.
func Classify(r ModeRecord, q Query, policy FreshnessPolicy, now time.Time) ModeResult {
	status := r.StatusAt(now)
	return ModeResult{
		Included: r.InMode(q, now),
		Status:   status,
		Stale:    status == StatusCurrent && policy.Stale(r.Kind, r.Temporal, now),
	}
}

// DueForReview reports whether the record is due for review at now: it is
// in force and the review date someone set for it has come (PR-33,
// architecture design section 9). Age alone never makes a record due, and
// being due changes nothing about it: the review sweep marks it so a
// reviewer can find it. Validity plays no part; a record no longer in force
// needs no review.
func (r ModeRecord) DueForReview(now time.Time) bool {
	return r.State.InForce() && r.ReviewDueAt != nil && !now.Before(*r.ReviewDueAt)
}
