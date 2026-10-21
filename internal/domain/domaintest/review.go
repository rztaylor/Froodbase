package domaintest

import (
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
)

// DueCase is one row of the review-due table: a record, the reference time
// (now) and whether the record is due for review then.
type DueCase struct {
	Name   string
	Record domain.ModeRecord
	Now    time.Time
	Want   bool
}

// DueCases is the review-due table (PR-33, architecture design section 9):
// a record in force whose review_due_at has come is due for review, at the
// moment itself and after; no date, a later date, or a record no longer in
// force is not. Validity, verification and recording time play no part.
func DueCases() []DueCase {
	now := Day(2026, 12, 1)
	due := func(state domain.LifecycleState, reviewDue *time.Time) domain.ModeRecord {
		return domain.ModeRecord{Kind: domain.KindObservation, State: state,
			Temporal: domain.Temporal{RecordedAt: Day(2026, 1, 1), ReviewDueAt: reviewDue}}
	}
	cases := []DueCase{
		{"due: no review date", due(domain.LifecycleActive, nil), now, false},
		{"due: review date a day ago", due(domain.LifecycleActive, at(now.Add(-24*time.Hour))), now, true},
		{"due: review date exactly now", due(domain.LifecycleActive, at(now)), now, true},
		{"due: review date a microsecond from now", due(domain.LifecycleActive, at(now.Add(time.Microsecond))), now, false},
		{"due: review date next month", due(domain.LifecycleActive, at(Day(2027, 1, 1))), now, false},
		{"due: a decision too", domain.ModeRecord{Kind: domain.KindDecision, State: domain.LifecycleActive,
			Temporal: domain.Temporal{RecordedAt: Day(2026, 1, 1), ReviewDueAt: at(Day(2026, 6, 1))}}, now, true},
		{"due: verified yesterday, review date passed", domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleActive,
			Temporal: domain.Temporal{RecordedAt: Day(2026, 1, 1), VerifiedAt: at(now.Add(-24 * time.Hour)), ReviewDueAt: at(Day(2026, 6, 1))}}, now, true},
		{"due: validity ended, still in force", domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleActive,
			Temporal: domain.Temporal{ValidFrom: at(Day(2026, 1, 1)), ValidTo: at(Day(2026, 3, 1)), RecordedAt: Day(2026, 1, 1), ReviewDueAt: at(Day(2026, 6, 1))}}, now, true},
		{"due: validity upcoming, in force", domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleActive,
			Temporal: domain.Temporal{ValidFrom: at(Day(2027, 1, 1)), RecordedAt: Day(2026, 1, 1), ReviewDueAt: at(Day(2026, 6, 1))}}, now, true},
	}
	for _, st := range domain.LifecycleStates() {
		cases = append(cases, DueCase{"due: " + string(st) + ", review date passed", due(st, at(Day(2026, 6, 1))), now, st.InForce()})
	}
	return cases
}
