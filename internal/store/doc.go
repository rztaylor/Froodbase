// Package store owns PostgreSQL access for froodbase-api and
// froodbase-dreamer.
//
// It opens the pgx pool, applies the embedded goose migrations
// (forward-only) and River's schema, and inserts River jobs. It owns the
// record tables and their row-level security: every read and write runs in a
// transaction with the caller's areas in the app.caller_areas session
// variable, as the non-owning service role. The writer (Contribute, with
// links adding evidence to existing claims; UpdateEntity, UpdateAssertion,
// LinkEvidence) writes records, revision snapshots, audit events, support
// events (one root_added per distinct root, cited or inherited from a
// deduction's sources, with the root's source kind; one person's statements
// are one root per claim), the index_record job and, when a record with
// dependants gains a root, the reconsider_derived job together;
// ReconsiderDerived is that job's walk. Revisions, audit events, support
// events and derivation links are append-only. Temporal reads (SearchFilter,
// ClassifyAssertion, ClassifyNames) filter and label through the SQL forms of
// the domain's temporal rules (migration 7, checked against
// internal/domain/domaintest); FreshnessPolicy and PutFreshnessPolicy keep
// the per-area freshness policy. SweepReview, ReviewFlags and AllAreaIDs keep
// the review sweep's marks (migration 8): projections computed by the same
// SQL functions as the live labels, never revisions. Queries are generated
// by sqlc into gen/ (exempt from the boundary rule).
//
// It holds no domain rules and decides no authorisation: who holds which
// areas comes from the caller, validity and support projection from
// internal/domain, the only package in the module it imports besides
// store/jobs. The test helper lives in store/storetest.
package store
