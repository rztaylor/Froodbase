// Package jobs defines the argument types of every background job in
// Froodbase, one Go type per River job kind.
//
// It is the contract between the writers that enqueue jobs (froodbase-api
// through internal/store, inside the transaction that changes the record)
// and froodbase-dreamer, which works them. Types here carry only the data a
// job needs and its uniqueness rule; the behaviour lives in internal/dreamer.
// Job kinds: heartbeat (slice 0) and index_record (slice 1, enqueued by the
// store's writer). Later slices add review_sweep, gap_scan and
// reconsider_derived. Depends on River's types
// only; nothing inside the module.
package jobs
