// Package httpserver owns the HTTP listener of froodbase-api.
//
// It builds the router, serves the health endpoint (GET /healthz), runs the
// server until its context is cancelled and shuts it down gracefully. Future
// handlers (the Streamable HTTP MCP endpoint from internal/mcpserver, basic
// auth from internal/authz) are mounted here but defined in their own
// packages; this package holds no knowledge logic, no authorisation rules
// and no database access. The health endpoint reports process liveness only.
// Depends on internal/version for the build identity it reports; nothing
// inside the module depends on it except cmd/froodbase-api.
package httpserver
