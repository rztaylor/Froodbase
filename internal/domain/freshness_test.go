package domain

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultFreshnessPolicy(t *testing.T) {
	p := DefaultFreshnessPolicy()
	if err := p.Validate(); err != nil {
		t.Fatalf("default policy invalid: %v", err)
	}
	tests := []struct {
		kind  EpistemicKind
		want  time.Duration
		limit bool
	}{
		{KindObservation, 90 * 24 * time.Hour, true},
		{KindInference, 90 * 24 * time.Hour, true},
		{KindDecision, 0, false},
	}
	for _, tt := range tests {
		got, ok := p.MaxAge(tt.kind)
		if got != tt.want || ok != tt.limit {
			t.Errorf("MaxAge(%s) = %v, %v; want %v, %v", tt.kind, got, ok, tt.want, tt.limit)
		}
	}
}

func TestFreshnessPolicyValidate(t *testing.T) {
	tests := []struct {
		name string
		p    FreshnessPolicy
		want error
	}{
		{"empty: no limits", FreshnessPolicy{}, nil},
		{"zero is no limit", FreshnessPolicy{MaxAgeDays: map[EpistemicKind]int{KindDecision: 0}}, nil},
		{"positive", FreshnessPolicy{MaxAgeDays: map[EpistemicKind]int{KindDecision: 365}}, nil},
		{"negative", FreshnessPolicy{MaxAgeDays: map[EpistemicKind]int{KindObservation: -1}}, ErrInvalidFreshnessPolicy},
		{"unknown kind", FreshnessPolicy{MaxAgeDays: map[EpistemicKind]int{"lesson": 30}}, ErrUnknownEpistemicKind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.p.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

// Stale is strictly older than the limit, and a day is 24 hours.
func TestFreshnessPolicyStaleBoundary(t *testing.T) {
	p := FreshnessPolicy{MaxAgeDays: map[EpistemicKind]int{KindObservation: 1}}
	now := t1
	tests := []struct {
		name     string
		verified time.Time
		want     bool
	}{
		{"exactly a day", now.Add(-24 * time.Hour), false},
		{"a microsecond more", now.Add(-24*time.Hour - time.Microsecond), true},
		{"just now", now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := Temporal{RecordedAt: t0, VerifiedAt: ptr(tt.verified)}
			if got := p.Stale(KindObservation, tm, now); got != tt.want {
				t.Errorf("Stale() = %v, want %v", got, tt.want)
			}
		})
	}
}
