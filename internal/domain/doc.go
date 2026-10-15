// Package domain holds the knowledge model as plain Go types and the rules
// they obey, with no I/O: knowledge areas, entities and their names,
// assertions, evidence with roots, and contributions.
//
// It owns the vocabulary (entity types, epistemic kinds, source kinds, access
// classes), record validation, evidence shareability (allowed source kinds and
// URI patterns; an access class never wider than its source's) and
// corroboration over distinct evidence roots. Constructors set recorded_at and
// never derive validity from it. Scope overlap and query modes arrive in slice
// 2; relationships, lessons and lifecycle in later slices.
//
// Not here: persistence (store), entity resolution and acceptance policy
// (contribute), retrieval, authorisation (authz) and id generation, which
// callers supply. It imports only the standard library.
//
// Requirements: PR-01, PR-03, PR-06, PR-10 to PR-12, TR-20, TR-22, TR-24,
// TR-25, TR-61.
package domain
