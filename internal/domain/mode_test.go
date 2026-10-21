package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/domain/domaintest"
)

// The shared mode table: inclusion, status and stale for every case.
func TestClassifyModeCases(t *testing.T) {
	for _, tc := range domaintest.ModeCases() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := tc.Query.Validate(); err != nil {
				t.Fatalf("case query invalid: %v", err)
			}
			if err := tc.Record.Validate(); err != nil {
				t.Fatalf("case record invalid: %v", err)
			}
			got := domain.Classify(tc.Record, tc.Query, tc.Policy, tc.Now)
			if got != tc.Want {
				t.Errorf("Classify() = %+v, want %+v", got, tc.Want)
			}
			if in := tc.Record.InMode(tc.Query, tc.Now); in != tc.Want.Included {
				t.Errorf("InMode() = %v, want %v", in, tc.Want.Included)
			}
		})
	}
}

// Moving a record's recording time anywhere never changes whether a mode
// keeps it or its status: validity alone decides (TR-32, TR-34).
func TestModeIgnoresRecordedAt(t *testing.T) {
	shifts := []time.Duration{-1000 * 24 * time.Hour, -time.Hour, time.Hour, 1000 * 24 * time.Hour}
	for _, tc := range domaintest.ModeCases() {
		for _, d := range shifts {
			r := tc.Record
			r.RecordedAt = r.RecordedAt.Add(d)
			got := domain.Classify(r, tc.Query, tc.Policy, tc.Now)
			if got.Included != tc.Want.Included || got.Status != tc.Want.Status {
				t.Errorf("%s, recorded_at moved by %v: %+v, want included %v status %s", tc.Name, d, got, tc.Want.Included, tc.Want.Status)
			}
		}
	}
}

// The pitch's acceptance points (PR-27), read straight from the table so a
// change to the cases that broke them would fail here by name.
func TestPitchStory(t *testing.T) {
	now := domaintest.Day(2026, 10, 14)
	p := domain.DefaultFreshnessPolicy()
	in := func(name string, q domain.Query) domain.ModeResult {
		return domain.Classify(domaintest.Pitch(name).Record, q, p, now)
	}
	if r := in(domaintest.PitchFailure, domain.CurrentQuery()); r.Included {
		t.Error("current returns the superseded failure")
	}
	if r := in(domaintest.PitchFailure, domain.AsOfQuery(domaintest.Day(2026, 9, 3))); !r.Included || r.Status != domain.StatusHistorical {
		t.Errorf("as_of the incident date: failure %+v, want included and historical", r)
	}
	if r := in(domaintest.PitchImport, domain.CurrentQuery()); r.Included {
		t.Error("the late import reopens the failure in current mode")
	}
	if r := in(domaintest.PitchDeployment, domain.CurrentQuery()); !r.Included || r.Status != domain.StatusCurrent {
		t.Errorf("current: deployment %+v, want included and current", r)
	}
	for _, rec := range domaintest.PitchRecords() {
		if r := in(rec.Name, domain.HistoricalQuery()); !r.Included {
			t.Errorf("historical leaves out %s", rec.Name)
		}
		if err := rec.Scope.Validate(); err != nil {
			t.Errorf("pitch %s scope: %v", rec.Name, err)
		}
	}
}

// Every lifecycle state meets every mode in the table.
func TestModeCasesCoverEveryStateAndMode(t *testing.T) {
	seen := map[[2]string]bool{}
	for _, tc := range domaintest.ModeCases() {
		seen[[2]string{string(tc.Record.State), string(tc.Query.Mode)}] = true
	}
	for _, s := range domain.LifecycleStates() {
		for _, m := range domain.Modes() {
			if !seen[[2]string{string(s), string(m)}] {
				t.Errorf("no mode case for state %s in mode %s", s, m)
			}
		}
	}
}

func TestClassifyInvalidQueryKeepsNothing(t *testing.T) {
	r := domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleActive, Temporal: domain.Temporal{RecordedAt: time.Now()}}
	if got := domain.Classify(r, domain.Query{Mode: "latest"}, domain.DefaultFreshnessPolicy(), time.Now()); got.Included {
		t.Errorf("unknown mode kept the record: %+v", got)
	}
}

func TestClassifyUnknownStateIsNotInForce(t *testing.T) {
	now := domaintest.Day(2026, 6, 1)
	r := domain.ModeRecord{Kind: domain.KindObservation, State: "proposed", Temporal: domain.Temporal{RecordedAt: now}}
	for _, q := range []domain.Query{domain.CurrentQuery(), domain.AsOfQuery(now)} {
		if got := domain.Classify(r, q, domain.DefaultFreshnessPolicy(), now); got.Included || got.Status != domain.StatusHistorical {
			t.Errorf("%s: unknown state %+v, want left out and historical", q.Mode, got)
		}
	}
}

func TestQueryValidate(t *testing.T) {
	at := domaintest.Day(2026, 9, 3)
	tests := []struct {
		name string
		q    domain.Query
		want error
	}{
		{"current", domain.CurrentQuery(), nil},
		{"as_of", domain.AsOfQuery(at), nil},
		{"historical", domain.HistoricalQuery(), nil},
		{"unknown mode", domain.Query{Mode: "latest"}, domain.ErrUnknownMode},
		{"empty mode", domain.Query{}, domain.ErrUnknownMode},
		{"as_of without a time", domain.Query{Mode: domain.ModeAsOf}, domain.ErrNoAsOf},
		{"current with a time", domain.Query{Mode: domain.ModeCurrent, AsOf: at}, domain.ErrAsOfNotAllowed},
		{"historical with a time", domain.Query{Mode: domain.ModeHistorical, AsOf: at}, domain.ErrAsOfNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.q.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	for _, m := range domain.Modes() {
		if got, err := domain.ParseMode(string(m)); err != nil || got != m {
			t.Errorf("ParseMode(%q) = %q, %v", m, got, err)
		}
	}
	for _, s := range []string{"", "Current", "asof", "now"} {
		if _, err := domain.ParseMode(s); !errors.Is(err, domain.ErrUnknownMode) {
			t.Errorf("ParseMode(%q) = %v, want ErrUnknownMode", s, err)
		}
	}
}

func TestParseLifecycleState(t *testing.T) {
	inForce := map[domain.LifecycleState]bool{domain.LifecycleActive: true, domain.LifecycleDisputed: true}
	for _, s := range domain.LifecycleStates() {
		if got, err := domain.ParseLifecycleState(string(s)); err != nil || got != s {
			t.Errorf("ParseLifecycleState(%q) = %q, %v", s, got, err)
		}
		if s.InForce() != inForce[s] {
			t.Errorf("%s.InForce() = %v", s, s.InForce())
		}
	}
	for _, s := range []string{"", "proposed", "accepted", "Active"} {
		if _, err := domain.ParseLifecycleState(s); !errors.Is(err, domain.ErrUnknownLifecycleState) {
			t.Errorf("ParseLifecycleState(%q) = %v, want ErrUnknownLifecycleState", s, err)
		}
	}
}
