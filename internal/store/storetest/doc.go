// Package storetest gives integration tests a disposable PostgreSQL.
//
// It starts one pgvector/pgvector:pg16 container per test binary through
// Testcontainers for Go, applies every migration once to a template
// database, and hands each test its own database cloned from that template
// (Pool) or an empty one (EmptyPool) so tests never share state. Databases
// are dropped and pools closed in t.Cleanup; the container is reaped by
// Testcontainers when the process exits. Everything but this file is behind
// the integration build tag, so the unit gate compiles without Docker. Used
// by tests only; nothing in the binaries imports it. Depends on
// internal/store.
package storetest
