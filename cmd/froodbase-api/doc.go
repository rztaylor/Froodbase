// Command froodbase-api is the remote Froodbase service.
//
// It owns process startup for the HTTPS API and Streamable HTTP MCP endpoint: token validation, the authorisation context, synchronous retrieval, transactional writes and audit. Background work belongs to froodbase-dreamer.
// Keep main small: parse flags, wire dependencies from internal packages, and
// map errors to exit codes.
package main
