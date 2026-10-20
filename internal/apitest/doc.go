// Package apitest starts froodbase-api in-process for scenario tests: the
// real HTTP server, basic auth against a sample users file, the real tool
// handlers over a disposable Testcontainers database, and a dreamer that
// works jobs in-process, so a test can drive the service exactly as
// froodbase-mcp does through internal/client.
//
// It owns the harness only: fixtures, server lifecycle, clients per sample
// user. Every file but this one is behind the integration build tag. The
// slice scenario tests that use it live here too, one per slice, each
// proving the slice's exit criteria from docs/planning/07-local-mvp.md.
// Tests only; nothing in the binaries imports it. Depends on
// internal/apihandlers, internal/authz/staticusers, internal/client,
// internal/dreamer, internal/httpserver, internal/mcpserver,
// internal/store/storetest and internal/version.
package apitest
