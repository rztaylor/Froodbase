// Package store owns PostgreSQL access for froodbase-api and
// froodbase-dreamer.
//
// It opens and closes the pgx connection pool, embeds the goose migrations
// under migrations/ and applies them forward-only, and reports migration
// status. Later slices add the sqlc-generated queries (under gen/, exempt from
// the boundary rule), the row-level-security session variable set per
// transaction, the transactional writer that records revisions, audit events
// and River jobs together, and the Testcontainers test helper. It holds no
// domain rules: validity, scope and lifecycle semantics come from
// internal/domain, which is the only package inside the module it may import.
// Nothing in cmd/ opens a database connection except through this package.
package store
