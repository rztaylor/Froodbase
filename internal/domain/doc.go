// Package domain holds the knowledge model as plain Go types and the rules
// they obey, with no I/O: knowledge areas, entities and their names,
// assertions, derivations, evidence with roots, contributions, and the
// support model.
//
// It owns the vocabulary (entity types, epistemic kinds, source kinds, access
// classes, support event types), record validation, what each kind must rest
// on (an observation cites evidence; a decision or inference cites evidence,
// is derived from other assertions, or both), derivation order and cycles,
// the roots a deduction inherits from its sources, the walk order of the
// records to reconsider when a source changes, evidence that the
// audience can check (source kinds, including command observations and
// personal statements, their URI patterns and required fields; an access
// class never wider than its source's; nothing that looks like a secret),
// corroboration and distinct roots, and the support projection: the service
// default support configuration, its bounds, root weights by source kind,
// and the deterministic score of a claim's append-only support events under
// a configuration (TR-29, TR-43). Constructors set recorded_at and never derive
// validity from it. Scope overlap and query modes arrive in slice 2;
// relationships, lessons and lifecycle in later slices.
//
// Not here: persistence (store), entity resolution and deciding which events
// to append (contribute), retrieval, authorisation (authz) and id generation,
// which callers supply. It imports only the standard library.
//
// Requirements: PR-01, PR-03, PR-06, PR-10 to PR-13, PR-15, PR-41, PR-42,
// TR-20, TR-22, TR-24, TR-25, TR-29, TR-43, TR-44, TR-61.
package domain
