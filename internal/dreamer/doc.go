// Package dreamer owns the background work of Froodbase: the River worker
// that froodbase-dreamer runs against the database, the periodic schedules,
// and the health endpoint that reports queue depth.
//
// Job kinds and their workers: heartbeat (every minute, slice 0);
// index_record (per write, unique per record while waiting) and
// rebuild_index (per area, on demand), which keep the projection
// bookkeeping in index_state current and make staleness observable on the
// health endpoint (slice 1). Later slices add review_sweep (periodic),
// gap_scan (periodic per area) and reconsider_derived (per state change).
// A job acts within the area it was enqueued for: its store caller holds
// that area id and nothing else, so row-level security applies to jobs as
// to people. Periodic jobs are inserted only by River's elected leader, so
// two dreamer instances never both schedule them, and the heartbeat is
// unique per minute as a second guard. Argument types live in internal/store/jobs so froodbase-api
// can enqueue jobs in its write transactions. This package never speaks for
// itself: nothing it does becomes accepted knowledge without a reviewer.
// Depends on internal/store (and store/jobs) and internal/version.
package dreamer
