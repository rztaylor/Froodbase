package mcpserver

import (
	"context"
	"errors"

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
	UserID      string              `json:"user_id" jsonschema:"the caller's stable user id"`
	DisplayName string              `json:"display_name" jsonschema:"the caller's display name"`
	AgentID     string              `json:"agent_id,omitempty" jsonschema:"the agent acting for the caller, if any"`
	Grants      map[string][]string `json:"grants" jsonschema:"knowledge area id to the roles held there"`
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
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, WhoamiOutput, error) {
			out, err := h.Whoami(ctx)
			return nil, out, err
		})
	}
	return s
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
