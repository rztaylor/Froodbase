// Package domaintest holds the shared truth tables for the temporal rules of
// internal/domain: scope overlap (TR-26), the current, as_of and historical
// mode predicate with the status label and the stale rule (TR-31 to TR-34,
// TR-53), and the pitch's version 2.3 to 2.4 records (PR-27).
//
// It owns the cases and their expected results only, as plain Go values with
// no logic of its own: internal/domain's tests run them against the Go
// functions, and the store's integration tests run the same cases through the
// SQL functions, so both implementations answer to one table. A case added
// here is checked by both.
//
// Not here: the rules themselves (internal/domain), database access
// (internal/store, internal/store/storetest). Used by tests only. Depends on
// internal/domain and the standard library.
package domaintest
