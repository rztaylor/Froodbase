// Package retrieval answers read requests for the caller: get_record and
// get_history in slice 1, with search (identifier, full-text and relationship
// channels, authorisation then mode filtering, fusion and the explanation
// trace) arriving in later slices.
//
// It owns the shaping of records into the wire types every tool returns
// (TR-14): kind, lifecycle state, support with its counts, roots by source
// kind and threshold verdict, scope, validity, verification date, citations and, for a
// deduction, the derived flag and its sources (PR-13). Authorisation
// is the store's row-level security plus the caller's areas; a record outside
// them is not found, never "forbidden" (TR-75). Depends on internal/store,
// internal/domain, internal/authz, internal/contribute (for the caller and
// area mapping) and internal/wire.
package retrieval
