// Command froodbase-mcp is the local MCP server that runs on each engineer's
// computer, launched by Claude Code (or another MCP client) over stdio.
//
// It owns process startup for the stdio transport and the wiring of the tool
// definitions in internal/mcpserver to the forwarding client in
// internal/client, which calls froodbase-api with the person's credentials
// from FROODBASE_API_URL, FROODBASE_USERNAME and FROODBASE_PASSWORD. When the
// API cannot be reached, tools return the explicit unavailable result rather
// than failing. Later: Entra sign-in and the token cache, and the local
// report-ingestion preview. Retrieval, authorisation and storage belong to
// froodbase-api; this binary never opens the database. Keep main small: parse
// flags, wire dependencies from internal packages, and map errors to exit
// codes.
package main
