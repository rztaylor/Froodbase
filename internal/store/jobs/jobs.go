package jobs

import (
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// KindHeartbeat is the periodic liveness job of the dreamer.
const KindHeartbeat = "heartbeat"

// HeartbeatArgs is the heartbeat job. It carries the minute it was scheduled
// for, which is also its uniqueness key: two schedulers inserting in the
// same minute produce one job.
type HeartbeatArgs struct {
	ScheduledFor time.Time `json:"scheduled_for" river:"unique"`
}

// Kind implements river.JobArgs.
func (HeartbeatArgs) Kind() string { return KindHeartbeat }

// InsertOpts makes the heartbeat unique per scheduled minute while it is
// still pending or running.
func (HeartbeatArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// HeartbeatFor builds the heartbeat for the minute containing t.
func HeartbeatFor(t time.Time) HeartbeatArgs {
	return HeartbeatArgs{ScheduledFor: t.UTC().Truncate(time.Minute)}
}

// KindReviewSweep is the periodic review sweep (PR-33, architecture design
// section 9): the dreamer marks the records of every area that are due for
// review or stale under their area's freshness policy, changing none of
// them.
const KindReviewSweep = "review_sweep"

// ReviewSweepArgs is one review sweep. ScheduledFor is the start of the
// schedule slot it belongs to and its uniqueness key in every state, so
// schedulers inserting in the same slot produce one sweep. The sweep reads
// the clock when it runs, not ScheduledFor.
type ReviewSweepArgs struct {
	ScheduledFor time.Time `json:"scheduled_for" river:"unique"`
}

// Kind implements river.JobArgs.
func (ReviewSweepArgs) Kind() string { return KindReviewSweep }

// InsertOpts makes the sweep unique per schedule slot.
func (ReviewSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// ReviewSweepFor builds the sweep for the slot of the given interval that
// contains t; an interval under a minute counts as a minute. An on-demand
// sweep passes the exact time instead, as its own slot.
func ReviewSweepFor(t time.Time, interval time.Duration) ReviewSweepArgs {
	interval = max(interval, time.Minute)
	return ReviewSweepArgs{ScheduledFor: t.UTC().Truncate(interval)}
}

// KindIndexRecord is the index-maintenance job enqueued in the transaction
// that writes a record (the outbox pattern, D5): the dreamer refreshes
// whatever search structures the record's revision needs.
const KindIndexRecord = "index_record"

// IndexRecordArgs names the record to index. The record id is the
// uniqueness key while a job waits or runs, so a burst of writes to one
// record collapses into one job; the worker indexes whatever revision is
// current when it reads the record. River requires the running state in
// the uniqueness set, so a write that commits between that read and the
// job's completion stays stale until the next write or a rebuild; the
// window is one short transaction and staleness is observable (TR-57).
// Revision is the revision that enqueued the job, for logs.
type IndexRecordArgs struct {
	RecordID   string `json:"record_id" river:"unique"`
	RecordType string `json:"record_type"`
	AreaID     string `json:"area_id"`
	Revision   int    `json:"revision"`
}

// Kind implements river.JobArgs.
func (IndexRecordArgs) Kind() string { return KindIndexRecord }

// InsertOpts makes the job unique per record while it waits or runs.
func (IndexRecordArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: waitingStates}}
}

// waitingStates are the job states in which a unique job counts as already
// enqueued: every state River requires, but not completed, so each later
// revision can enqueue its own job.
var waitingStates = []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled}

// KindReconsiderDerived is the propagation job (TR-44) enqueued in the
// transaction that changes a record's support, such as gaining a new
// evidence root: the dreamer walks every record derived from it, directly
// or transitively, and brings each up to date with its sources.
const KindReconsiderDerived = "reconsider_derived"

// ReconsiderDerivedArgs names the changed record and its area. The record id
// is the uniqueness key while a job waits or runs, so several changes to one
// record before the dreamer reaches it collapse into one walk; the job reads
// the sources' current state rather than the change that enqueued it, so
// nothing is lost by collapsing. The job completes itself inside the
// transaction that holds the changed record against further root
// additions, so a change either lands before the walk reads it or enqueues
// a new job after the walk has completed.
type ReconsiderDerivedArgs struct {
	RecordID string `json:"record_id" river:"unique"`
	AreaID   string `json:"area_id"`
}

// Kind implements river.JobArgs.
func (ReconsiderDerivedArgs) Kind() string { return KindReconsiderDerived }

// InsertOpts makes the job unique per record while it waits or runs.
func (ReconsiderDerivedArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: waitingStates}}
}

// KindRebuildIndex re-indexes every record of one area.
const KindRebuildIndex = "rebuild_index"

// RebuildIndexArgs names the area to rebuild. Unique per area while waiting or running.
type RebuildIndexArgs struct {
	AreaID string `json:"area_id" river:"unique"`
}

// Kind implements river.JobArgs.
func (RebuildIndexArgs) Kind() string { return KindRebuildIndex }

// InsertOpts makes the rebuild unique per area while it waits or runs.
func (RebuildIndexArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: waitingStates}}
}
