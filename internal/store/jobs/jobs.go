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
