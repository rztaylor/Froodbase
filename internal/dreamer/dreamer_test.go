package dreamer

import (
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/store/jobs"
)

func TestPeriodicJobsConfigured(t *testing.T) {
	// River exposes no accessors on PeriodicJob; building the list without a
	// panic proves the ids are unique and each schedule is set.
	if got := len(PeriodicJobs()); got != 1 {
		t.Fatalf("periodic jobs = %d, want 1 (heartbeat)", got)
	}
	if HeartbeatInterval != time.Minute {
		t.Fatalf("HeartbeatInterval = %v, want 1m", HeartbeatInterval)
	}
	if jobs.HeartbeatFor(time.Now()).Kind() != jobs.KindHeartbeat {
		t.Fatal("heartbeat constructor builds the wrong kind")
	}
}
