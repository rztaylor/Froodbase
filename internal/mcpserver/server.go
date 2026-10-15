package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/authz"
)

// ServerName is the MCP implementation name reported to clients.
const ServerName = "froodbase"

// Tool names. The set is finite by design (TR-12); add a constant here when a
// slice adds a tool.
const (
	ToolWhoami = "whoami"
)

// WhoamiOutput is the whoami tool's structured result.
type WhoamiOutput struct {
	UserID      string              `json:"user_id"`
	DisplayName string              `json:"display_name"`
	AgentID     string              `json:"agent_id,omitempty"`
	Grants      map[string][]string `json:"grants"`
}

// Unavailable is the structured result every tool returns, with IsError
// false, when froodbase-api cannot be reached from froodbase-mcp. The agent
// is expected to continue with packaged and local knowledge and to note the
// outage in its report (TR-102).
type Unavailable struct {
	FroodbaseUnavailable bool   `json:"froodbase_unavailable"`
	APIURL               string `json:"api_url"`
	Message              string `json:"message"`
	Cause                string `json:"cause,omitempty"`
}

// UnavailableMessage is the guidance carried by Unavailable.Message.
const UnavailableMessage = "Froodbase is unavailable; continue with packaged and local knowledge and record in the report that Froodbase could not be reached."

// UnavailableError is returned by a forwarding handler when the API cannot
// be reached. Tool wrappers convert it into an Unavailable result rather
// than a tool error.
type UnavailableError struct {
	APIURL string
	Err    error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("froodbase-api at %s unavailable: %v", e.APIURL, e.Err)
}

// Unwrap exposes the cause.
func (e *UnavailableError) Unwrap() error { return e.Err }

// Result converts the error to the structured result.
func (e *UnavailableError) Result() Unavailable {
	cause := ""
	if e.Err != nil {
		cause = e.Err.Error()
	}
	return Unavailable{FroodbaseUnavailable: true, APIURL: e.APIURL, Message: UnavailableMessage, Cause: cause}
}

// Handlers supplies the behaviour behind each tool. A nil handler leaves the
// tool out of the server.
type Handlers struct {
	Whoami func(ctx context.Context) (WhoamiOutput, error)
}

// New builds an MCP server exposing the tools whose handlers are set.
func New(version string, h Handlers) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: version}, nil)
	if h.Whoami != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        ToolWhoami,
			Description: "Report who Froodbase thinks the caller is and which knowledge areas and roles they hold.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(func(ctx context.Context, _ struct{}) (WhoamiOutput, error) { return h.Whoami(ctx) }))
	}
	return s
}

// wrap adapts a typed handler to the SDK. Outputs are returned as any so that
// an UnavailableError can be reported as an Unavailable result with the same
// tool; every other error is a tool error.
func wrap[In, Out any](h func(ctx context.Context, in In) (Out, error)) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := h(ctx, in)
		var ue *UnavailableError
		if errors.As(err, &ue) {
			return nil, ue.Result(), nil
		}
		if err != nil {
			return nil, nil, err
		}
		return nil, out, nil
	}
}

// ErrNoCaller is returned when a tool runs without an authorisation context,
// which means the transport did not authenticate the request.
var ErrNoCaller = errors.New("mcpserver: no authorisation context on the request")

// APIHandlers returns the handlers froodbase-api serves: they read the
// authorisation context the HTTP middleware attached.
func APIHandlers() Handlers {
	return Handlers{
		Whoami: func(ctx context.Context) (WhoamiOutput, error) {
			ac := authz.FromContext(ctx)
			if ac == nil {
				return WhoamiOutput{}, ErrNoCaller
			}
			return WhoamiFromContext(ac), nil
		},
	}
}

// WhoamiFromContext converts an authorisation context to the tool output.
func WhoamiFromContext(ac *authz.Context) WhoamiOutput {
	grants := make(map[string][]string, len(ac.Grants))
	for _, area := range ac.Areas() {
		roles := make([]string, 0, len(ac.Grants[area]))
		for _, r := range ac.Grants[area] {
			roles = append(roles, string(r))
		}
		grants[area] = roles
	}
	return WhoamiOutput{UserID: ac.UserID, DisplayName: ac.DisplayName, AgentID: ac.AgentID, Grants: grants}
}
