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

func TestIndexRecordArgs(t *testing.T) {
	args := IndexRecordArgs{RecordID: "r1", RecordType: "assertion", AreaID: "a1", Revision: 2}
	if args.Kind() != KindIndexRecord || KindIndexRecord != "index_record" {
		t.Errorf("Kind() = %q", args.Kind())
	}
	if !args.InsertOpts().UniqueOpts.ByArgs {
		t.Error("index_record is not unique by args")
	}
}

func TestReviewSweepArgs(t *testing.T) {
	at := time.Date(2026, 10, 15, 8, 30, 45, 0, time.UTC)
	a := ReviewSweepFor(at, time.Hour)
	if a.Kind() != KindReviewSweep || KindReviewSweep != "review_sweep" {
		t.Fatalf("Kind() = %q", a.Kind())
	}
	if want := time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC); !a.ScheduledFor.Equal(want) {
		t.Errorf("ScheduledFor = %v, want the slot %v", a.ScheduledFor, want)
	}
	// Two schedulers in the same slot build the same job, so one is inserted.
	if b := ReviewSweepFor(at.Add(20*time.Minute), time.Hour); b != a {
		t.Errorf("same slot built %+v and %+v", a, b)
	}
	if b := ReviewSweepFor(at.Add(time.Hour), time.Hour); b == a {
		t.Error("the next slot built the same job")
	}
	// A slot is never shorter than a minute.
	if b := ReviewSweepFor(at, 0); !b.ScheduledFor.Equal(time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC)) {
		t.Errorf("zero interval slot = %v, want the minute", b.ScheduledFor)
	}
	opts := a.InsertOpts().UniqueOpts
	if !opts.ByArgs || opts.ByState != nil {
		t.Errorf("review_sweep unique opts = %+v, want unique by slot in every state", opts)
	}
}

func TestReconsiderDerivedArgs(t *testing.T) {
	args := ReconsiderDerivedArgs{RecordID: "r1", AreaID: "a1"}
	if args.Kind() != KindReconsiderDerived || KindReconsiderDerived != "reconsider_derived" {
		t.Errorf("Kind() = %q", args.Kind())
	}
	opts := args.InsertOpts().UniqueOpts
	if !opts.ByArgs || len(opts.ByState) != len(waitingStates) {
		t.Errorf("reconsider_derived unique opts = %+v, want unique by record while waiting or running", opts)
	}
}
