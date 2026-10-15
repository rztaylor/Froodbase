package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/authz"
)

func connect(ctx context.Context, t *testing.T, s *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestWhoamiListedAndCalled(t *testing.T) {
	ac := &authz.Context{UserID: "alice", DisplayName: "Alice", Grants: map[string][]authz.Role{"support": {authz.RoleReviewer}}}
	ctx := authz.WithContext(context.Background(), ac)
	cs := connect(ctx, t, New("test", APIHandlers()))

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != ToolWhoami {
		t.Fatalf("tools = %+v, want only %s", tools.Tools, ToolWhoami)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: ToolWhoami})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var out WhoamiOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.UserID != "alice" || out.DisplayName != "Alice" || out.Grants["support"][0] != "reviewer" {
		t.Fatalf("whoami = %+v", out)
	}
}

func TestWhoamiWithoutCallerIsToolError(t *testing.T) {
	ctx := context.Background()
	cs := connect(ctx, t, New("test", APIHandlers()))
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: ToolWhoami})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected a tool error without an authorisation context")
	}
}

func TestNilHandlerOmitsTool(t *testing.T) {
	ctx := context.Background()
	cs := connect(ctx, t, New("test", Handlers{}))
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 0 {
		t.Fatalf("tools = %+v, want none", tools.Tools)
	}
}
