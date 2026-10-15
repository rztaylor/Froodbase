// Package dreamer owns the background work of Froodbase: the River worker
// that froodbase-dreamer runs against the database, the periodic schedules,
// and the health endpoint that reports queue depth.
//
// Job kinds and their workers: heartbeat (every minute, slice 0). Later
// slices add index_record (per write), review_sweep (periodic), gap_scan
// (periodic per area) and reconsider_derived (per state change). Periodic
// jobs are inserted only by River's elected leader, so two dreamer instances
// never both schedule them, and the heartbeat is unique per minute as a
// second guard. Argument types live in internal/store/jobs so froodbase-api
// can enqueue jobs in its write transactions. This package never speaks for
// itself: nothing it does becomes accepted knowledge without a reviewer.
// Depends on internal/store (and store/jobs) and internal/version.
package dreamer
