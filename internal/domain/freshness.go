package domain

import "time"

// FreshnessPolicy is an area's setting for how long a record of each kind
// may go unchecked before it is stale (TR-31): the maximum age of its last
// check in whole days, by epistemic kind. A kind that is absent, or zero,
// has no limit. Staleness flags a record for review; it never removes it,
// changes its validity or its state, or costs it support (PR-33).
type FreshnessPolicy struct {
	MaxAgeDays map[EpistemicKind]int
}

// DefaultFreshnessDays is the default maximum age, in days, of
// observations and inferences; decisions have none (architecture design
// section 14).
const DefaultFreshnessDays = 90

// DefaultFreshnessPolicy is the policy an area without its own uses:
// observations and inferences go stale after 90 days unchecked, because
// what was seen, and what was concluded from it, describe a world that
// changes; decisions hold until someone changes them, so they have no limit.
func DefaultFreshnessPolicy() FreshnessPolicy {
	return FreshnessPolicy{MaxAgeDays: map[EpistemicKind]int{
		KindObservation: DefaultFreshnessDays,
		KindInference:   DefaultFreshnessDays,
	}}
}

// Validate checks that every kind is known and no age is negative.
func (p FreshnessPolicy) Validate() error {
	for k, days := range p.MaxAgeDays {
		if !k.Valid() {
			return fieldErr("freshness policy kind", ErrUnknownEpistemicKind)
		}
		if days < 0 {
			return ErrInvalidFreshnessPolicy
		}
	}
	return nil
}

// MaxAge returns the maximum age of a record of kind k, and false when the
// kind has no limit. A day is 24 hours, whatever the time zone.
func (p FreshnessPolicy) MaxAge(k EpistemicKind) (time.Duration, bool) {
	days := p.MaxAgeDays[k]
	if days <= 0 {
		return 0, false
	}
	return time.Duration(days) * 24 * time.Hour, true
}

// Stale reports whether a record of kind k, last checked at
// t.LastChecked(), is older than its kind allows at now. A record checked
// exactly the maximum age ago is not yet stale. It says nothing about
// whether the record is current: Classify asks that first.
func (p FreshnessPolicy) Stale(k EpistemicKind, t Temporal, now time.Time) bool {
	limit, ok := p.MaxAge(k)
	return ok && now.Sub(t.LastChecked()) > limit
}
