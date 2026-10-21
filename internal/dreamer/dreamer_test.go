package dreamer

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/store/jobs"
)

func TestPeriodicJobsConfigured(t *testing.T) {
	// River exposes no accessors on PeriodicJob; building the list without a
	// panic proves the ids are unique and each schedule is set.
	if got := len(PeriodicJobs(Schedule{ReviewSweepInterval: time.Hour})); got != 2 {
		t.Fatalf("periodic jobs = %d, want 2 (heartbeat, review_sweep)", got)
	}
	if HeartbeatInterval != time.Minute {
		t.Fatalf("HeartbeatInterval = %v, want 1m", HeartbeatInterval)
	}
	if jobs.HeartbeatFor(time.Now()).Kind() != jobs.KindHeartbeat {
		t.Fatal("heartbeat constructor builds the wrong kind")
	}
}

func TestScheduleValidate(t *testing.T) {
	for _, tc := range []struct {
		interval time.Duration
		ok       bool
	}{
		{0, false},
		{-time.Hour, false},
		{30 * time.Second, false},
		{time.Minute, true},
		{time.Hour, true},
		{24 * time.Hour, true},
	} {
		err := Schedule{ReviewSweepInterval: tc.interval}.Validate()
		if (err == nil) != tc.ok {
			t.Errorf("Schedule{%v}.Validate() = %v, want ok %v", tc.interval, err, tc.ok)
		}
	}
	// The dreamer refuses to start without a valid schedule, before it
	// touches the database.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := New(log, nil, Schedule{}); err == nil || !strings.Contains(err.Error(), "review sweep interval") {
		t.Errorf("New with no schedule: err = %v, want a review sweep interval error", err)
	}
}
