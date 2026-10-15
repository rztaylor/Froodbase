// Package client is the HTTP client froodbase-mcp uses to call froodbase-api.
//
// It owns the connection to the API's Streamable HTTP MCP endpoint through
// the MCP Go SDK client: basic auth for the signed-in person, the client
// version header on every request, and the mapping of transport failures to
// the explicit "Froodbase unavailable" result (connection errors and 5xx
// responses become mcpserver.UnavailableError; 401 and 403 become an
// authentication error the person must fix). It exposes one typed method per
// forwarded tool, decoding the API's structured result. It performs no
// authorisation and holds no knowledge logic; the API decides everything.
// Entra sign-in and the token cache arrive here with the cloud move. Depends
// on internal/mcpserver (result types) and internal/version.
package client
