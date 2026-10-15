package jobs

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHeartbeatArgs(t *testing.T) {
	a := HeartbeatFor(time.Date(2026, 10, 15, 8, 30, 45, 0, time.UTC))
	if a.Kind() != KindHeartbeat {
		t.Fatalf("Kind() = %q", a.Kind())
	}
	if a.ScheduledFor != time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC) {
		t.Fatalf("ScheduledFor = %v, want truncated to the minute", a.ScheduledFor)
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var back HeartbeatArgs
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !back.ScheduledFor.Equal(a.ScheduledFor) {
		t.Fatalf("round trip = %v, want %v", back.ScheduledFor, a.ScheduledFor)
	}
	if !a.InsertOpts().UniqueOpts.ByArgs {
		t.Fatal("heartbeat is not unique by args")
	}
}
