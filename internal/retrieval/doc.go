// Package retrieval answers read requests for the caller: get_record,
// get_history, get_as_of (a record as valid at a time, by validity time) and
// search (identifier and full-text channels, authorisation then the mode and
// scope filter, current by default; relationship channels, fusion and the
// explanation trace arrive in later slices).
//
// It owns the shaping of records into the wire types every tool returns
// (TR-14): kind, lifecycle state, support with its counts, roots by source
// kind and threshold verdict, scope, validity, verification date, citations and, for a
// deduction, the derived flag and its sources (PR-13), labelled with the
// status relative to now and the stale flag the store's SQL gives it. Authorisation
// is the store's row-level security plus the caller's areas; a record outside
// them is not found, never "forbidden" (TR-75). Depends on internal/store,
// internal/domain, internal/authz, internal/contribute (for the caller, area
// mapping and scope translation) and internal/wire.
package retrieval
