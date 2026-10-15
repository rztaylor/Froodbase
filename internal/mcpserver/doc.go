// Package mcpserver owns the MCP tool surface of Froodbase: the finite set of
// tool names, their input and output schemas and descriptions, built on the
// MCP Go SDK.
//
// It defines each tool once and lets a caller supply the handlers, so
// froodbase-api serves the tools against its engines and froodbase-mcp serves
// the same definitions with handlers that forward to the API (slice 0 ticket
// FRB-5). Handlers read the caller's authz.Context from the request context;
// this package performs no authentication, holds no database access and
// contains no knowledge logic. The HTTP transport (Streamable HTTP, basic
// auth) is mounted by internal/httpserver; the stdio transport by
// cmd/froodbase-mcp. Depends on internal/authz and internal/version.
package mcpserver
