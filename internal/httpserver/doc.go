// Package httpserver owns the HTTP listener of froodbase-api.
//
// It builds the router, serves the health endpoint (GET /healthz), runs the
// server until its context is cancelled and shuts it down gracefully, and
// provides the HTTP basic auth middleware that authenticates a request,
// builds the authz.Context and attaches it to the request. Handlers such as
// the Streamable HTTP MCP endpoint (built by internal/mcpserver on the MCP Go
// SDK) are mounted here but defined elsewhere; this package holds no
// knowledge logic, no authorisation rules beyond "no context, no access" and
// no database access. The health endpoint reports process liveness only.
// Depends on internal/authz and internal/version; nothing inside the module
// depends on it except cmd/froodbase-api.
package httpserver
