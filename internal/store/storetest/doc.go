// Package storetest gives integration tests a disposable PostgreSQL.
//
// It starts one pgvector/pgvector:pg16 container per test binary through
// Testcontainers for Go, applies every migration once to a template
// database, and hands each test its own database cloned from that template
// (Open: an Admin pool as the owning superuser and an App pool as the
// service role the row-level-security policies apply to) or an empty one
// (EmptyPool) so tests never share state. Tests of what a caller can see
// must use App; Admin bypasses the policies and is for set-up and
// inspection. Databases are dropped and pools closed in t.Cleanup; the
// container is reaped when the process exits. Everything but this file is
// behind the integration build tag, so the unit gate compiles without
// Docker. Used by tests only. Depends on internal/store.
package storetest
