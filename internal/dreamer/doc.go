// Package dreamer owns the background work of Froodbase: the River worker
// that froodbase-dreamer runs against the database, the periodic schedules,
// and the health endpoint that reports queue depth, index staleness and
// waiting propagation jobs.
//
// Job kinds and their workers: heartbeat (every minute, slice 0);
// index_record (per write, unique per record while waiting) and
// rebuild_index (per area, on demand), which keep the projection
// bookkeeping in index_state current and make staleness observable on the
// health endpoint (slice 1); reconsider_derived (per root added to a record
// with dependants, unique per record while waiting), which walks the records
// derived from it and passes the new roots on, completing itself in the
// walk's transaction (slice 1b; the weakening events extend it);
// review_sweep (periodic on the Schedule's interval, slice 2), which marks
// each area's records due for review or stale beside them and never
// changes a record. Later slices add gap_scan (periodic per area). A job
// acts within the area it was enqueued for: its store caller holds that
// area id and nothing else, so row-level security applies to jobs as to
// people; the review sweep, enqueued for no area, sweeps each area with a
// caller holding that area alone. Periodic jobs are inserted only by
// River's elected leader, so two dreamer instances never both schedule
// them, and each is unique per slot as a second guard. Argument types live in internal/store/jobs so froodbase-api
// can enqueue jobs in its write transactions. This package never speaks for
// itself: nothing it does becomes accepted knowledge without a reviewer.
// Depends on internal/store (and store/jobs) and internal/version.
package dreamer
