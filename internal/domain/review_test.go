package domain_test

import (
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/domain/domaintest"
)

// The shared review-due table.
func TestDueForReviewCases(t *testing.T) {
	for _, tc := range domaintest.DueCases() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := tc.Record.Validate(); err != nil {
				t.Fatalf("case record invalid: %v", err)
			}
			if got := tc.Record.DueForReview(tc.Now); got != tc.Want {
				t.Errorf("DueForReview() = %v, want %v", got, tc.Want)
			}
		})
	}
}

// Age alone never makes a record due: without a review date, no amount of
// time since it was recorded or verified does (PR-33).
func TestDueForReviewIgnoresAge(t *testing.T) {
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	r := domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleActive,
		Temporal: domain.Temporal{RecordedAt: old, VerifiedAt: &old}}
	if r.DueForReview(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Error("a record with no review date is due for review")
	}
}
