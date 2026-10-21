// Package jobs defines the argument types of every background job in
// Froodbase, one Go type per River job kind.
//
// It is the contract between the writers that enqueue jobs (froodbase-api
// through internal/store, inside the transaction that changes the record)
// and froodbase-dreamer, which works them. Types here carry only the data a
// job needs and its uniqueness rule; the behaviour lives in internal/dreamer.
// Job kinds: heartbeat (slice 0), index_record and rebuild_index (slice 1,
// enqueued by the store's writer), and reconsider_derived (slice 1b,
// enqueued by the writer when a record with dependants gains a root).
// Later slices add review_sweep and gap_scan. Depends on River's types
// only; nothing inside the module.
package jobs
