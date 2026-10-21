// Package domain holds the knowledge model as plain Go types and the rules
// they obey, with no I/O: knowledge areas, entities and their names,
// assertions, derivations, evidence with roots, contributions, the support
// model and the temporal rules.
//
// It owns the vocabulary (entity types, epistemic kinds, source kinds, access
// classes, support event types, lifecycle states, query modes), record
// validation, what each kind must rest on, derivation order and cycles, the
// roots a deduction inherits and the records to reconsider when a source
// changes, checkable evidence (source kinds and URI patterns, access never
// wider than the source's, no secrets), corroboration and distinct roots, the
// support projection under a configuration (TR-29, TR-43), and the two
// clocks: scope keys and overlap (TR-26), the current, as_of and historical
// mode predicate with each record's status relative to now, and the stale
// rule under an area's freshness policy (TR-31 to TR-34, TR-53).
// Constructors set recorded_at and never derive validity from it. The
// shared case tables for overlap and modes live in domaintest so the SQL
// functions are checked against the same truth. Relationships, lessons and
// lifecycle transitions arrive in later slices.
//
// Not here: persistence (store), entity resolution and deciding which events
// to append (contribute), retrieval, authorisation (authz) and id generation,
// which callers supply. It imports only the standard library.
//
// Requirements: PR-01, PR-03, PR-06, PR-10 to PR-13, PR-15, PR-20 to PR-27,
// PR-41, PR-42, TR-20, TR-22, TR-24 to TR-26, TR-29 to TR-34, TR-43, TR-44,
// TR-53, TR-61.
package domain
