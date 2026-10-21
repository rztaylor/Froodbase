package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/mcpserver"
	"github.com/rztaylor/froodbase/internal/version"
	"github.com/rztaylor/froodbase/internal/wire"
)

const callTimeout = 30 * time.Second

// AuthError means the API rejected the credentials; the person must fix
// FROODBASE_USERNAME or FROODBASE_PASSWORD. It is a tool error, not an
// unavailable result.
type AuthError struct {
	Status int
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("froodbase-api rejected the credentials (HTTP %d); check FROODBASE_USERNAME and FROODBASE_PASSWORD", e.Status)
}

// Client calls froodbase-api on behalf of one signed-in person.
type Client struct {
	apiURL string
	http   *http.Client
}

// New builds a client for the MCP endpoint at apiURL with basic auth.
func New(apiURL, username, password string) *Client {
	return &Client{
		apiURL: apiURL,
		http: &http.Client{Transport: &transport{
			base:     http.DefaultTransport,
			username: username,
			password: password,
			apiURL:   apiURL,
		}},
	}
}

// APIURL is the endpoint the client talks to.
func (c *Client) APIURL() string { return c.apiURL }

// transport adds basic auth and the version header, and classifies failures.
type transport struct {
	base               http.RoundTripper
	username, password string
	apiURL             string
}

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.SetBasicAuth(t.username, t.password)
	r.Header.Set(version.ClientHeader, version.Version)
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, &mcpserver.UnavailableError{APIURL: t.apiURL, Err: err}
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		_ = resp.Body.Close()
		return nil, &AuthError{Status: resp.StatusCode}
	case resp.StatusCode >= 500:
		_ = resp.Body.Close()
		return nil, &mcpserver.UnavailableError{APIURL: t.apiURL, Err: fmt.Errorf("HTTP %d from the API", resp.StatusCode)}
	}
	return resp, nil
}

// CallTool opens a session, calls one tool and closes the session. The API
// serves stateless sessions, so this costs one handshake round trip per
// call, which is acceptable for the local MVP.
func (c *Client) CallTool(ctx context.Context, name string, args any) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	mc := mcp.NewClient(&mcp.Implementation{Name: "froodbase-mcp", Version: version.Version}, nil)
	session, err := mc.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             c.apiURL,
		HTTPClient:           c.http,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return nil, classify(c.apiURL, err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, classify(c.apiURL, err)
	}
	return res, nil
}

// classify surfaces the transport's own error types from the SDK's wrapping
// and treats anything else that is not an API tool error as unavailable.
func classify(apiURL string, err error) error {
	var ue *mcpserver.UnavailableError
	if errors.As(err, &ue) {
		return ue
	}
	var ae *AuthError
	if errors.As(err, &ae) {
		return ae
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &mcpserver.UnavailableError{APIURL: apiURL, Err: err}
	}
	return &mcpserver.UnavailableError{APIURL: apiURL, Err: err}
}

// Whoami forwards the whoami tool.
func (c *Client) Whoami(ctx context.Context) (mcpserver.WhoamiOutput, error) {
	var out mcpserver.WhoamiOutput
	err := c.call(ctx, mcpserver.ToolWhoami, struct{}{}, &out)
	return out, err
}

// Contribute forwards the contribute tool.
func (c *Client) Contribute(ctx context.Context, in wire.ContributeInput) (wire.ContributeOutput, error) {
	var out wire.ContributeOutput
	err := c.call(ctx, mcpserver.ToolContribute, in, &out)
	return out, err
}

// GetRecord forwards the get_record tool.
func (c *Client) GetRecord(ctx context.Context, in wire.GetRecordInput) (wire.Record, error) {
	var out wire.Record
	err := c.call(ctx, mcpserver.ToolGetRecord, in, &out)
	return out, err
}

// Search forwards the search tool.
func (c *Client) Search(ctx context.Context, in wire.SearchInput) (wire.SearchOutput, error) {
	var out wire.SearchOutput
	err := c.call(ctx, mcpserver.ToolSearch, in, &out)
	return out, err
}

// GetAsOf forwards the get_as_of tool.
func (c *Client) GetAsOf(ctx context.Context, in wire.GetAsOfInput) (wire.GetAsOfOutput, error) {
	var out wire.GetAsOfOutput
	err := c.call(ctx, mcpserver.ToolGetAsOf, in, &out)
	return out, err
}

// GetHistory forwards the get_history tool.
func (c *Client) GetHistory(ctx context.Context, in wire.GetHistoryInput) (wire.HistoryOutput, error) {
	var out wire.HistoryOutput
	err := c.call(ctx, mcpserver.ToolGetHistory, in, &out)
	return out, err
}

// call runs a tool and decodes its structured content into out. A tool error
// from the API is returned as an error with the API's message.
func (c *Client) call(ctx context.Context, name string, args any, out any) error {
	res, err := c.CallTool(ctx, name, args)
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("froodbase-api: %s", textOf(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return fmt.Errorf("client: encoding %s result: %w", name, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("client: decoding %s result: %w", name, err)
	}
	return nil
}

func textOf(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			return t.Text
		}
	}
	return "tool error"
}
