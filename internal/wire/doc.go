// Package wire holds the input and output shapes of every Froodbase tool:
// what a caller sends to contribute, get_record, get_history, get_as_of and
// search and what comes back, as plain structs with JSON tags.
//
// It is the one vocabulary shared by the tool definitions
// (internal/mcpserver), the engines that serve them in froodbase-api
// (internal/contribute, internal/retrieval through internal/apihandlers) and
// the forwarding client in froodbase-mcp (internal/client), so the two
// binaries agree on the contract without either importing the other's
// machinery. It carries no behaviour beyond trivial helpers and imports only
// the standard library. Every record returned carries kind, lifecycle state,
// support, scope, validity, verification date and citations as TR-14
// requires, where the record type has them, its status relative to now and
// stale flag, and a derived claim names the assertions it was derived from
// (PR-13).
package wire
