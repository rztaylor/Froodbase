// Package apihandlers wires the tool handlers froodbase-api serves: it reads
// the authorisation context the HTTP middleware attached and calls the
// engines (internal/contribute, internal/retrieval), returning the wire
// types the tool definitions in internal/mcpserver expect.
//
// It also owns the knowledge area directory: the mapping between area keys
// (how grants name areas) and area ids, cached from the store, and the
// start-up step that creates an area for every key the users file grants,
// so the local MVP needs no admin tool to make areas exist. Nothing here
// decides authorisation or touches SQL. froodbase-mcp does not import it.
// Depends on internal/contribute, internal/retrieval, internal/store,
// internal/authz, internal/mcpserver and internal/wire.
package apihandlers
