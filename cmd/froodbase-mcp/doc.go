// Command froodbase-mcp is the local MCP server that runs on each engineer's computer.
//
// It owns process startup for the stdio MCP transport, the Entra sign-in and token cache, forwarding tool calls to froodbase-api, and the local report-ingestion preview. Retrieval, authorisation and storage belong to froodbase-api; this binary never opens the database.
// Keep main small: parse flags, wire dependencies from internal packages, and
// map errors to exit codes.
package main
