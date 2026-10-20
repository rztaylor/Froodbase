package jobs

import (
	"time"

	"github.com/riverqueue/river"
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

// IndexRecordArgs names the record and the revision to index. All fields
// are the uniqueness key: one pending job per revision of a record.
type IndexRecordArgs struct {
	RecordID   string `json:"record_id" river:"unique"`
	RecordType string `json:"record_type" river:"unique"`
	AreaID     string `json:"area_id" river:"unique"`
	Revision   int    `json:"revision" river:"unique"`
}

// Kind implements river.JobArgs.
func (IndexRecordArgs) Kind() string { return KindIndexRecord }

// InsertOpts makes the job unique per record revision while pending or running.
func (IndexRecordArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}
