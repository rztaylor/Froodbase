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
