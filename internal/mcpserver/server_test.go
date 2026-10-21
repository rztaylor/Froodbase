package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/wire"
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
	cs := connect(ctx, t, New("test", WhoamiHandlers()))

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
	cs := connect(ctx, t, New("test", WhoamiHandlers()))
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

func TestUnavailableBecomesResultNotError(t *testing.T) {
	ctx := context.Background()
	cs := connect(ctx, t, New("test", Handlers{Whoami: func(context.Context) (WhoamiOutput, error) {
		return WhoamiOutput{}, &UnavailableError{APIURL: "http://127.0.0.1:1/mcp", Err: errors.New("connection refused")}
	}}))
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: ToolWhoami})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unavailable reported as a tool error: %+v", res.Content)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var out Unavailable
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !out.FroodbaseUnavailable || out.APIURL != "http://127.0.0.1:1/mcp" || out.Message != UnavailableMessage || out.Cause != "connection refused" {
		t.Fatalf("unavailable = %+v", out)
	}
}

// TestContributeSchemaRequiresOnlyWhatCannotBeInferred checks that the
// contribute input schema leaves out of required what the server fills in
// (refs, the evidence kind, revision and access class, a statement's
// source), so a client that validates against it can send a plain change
// set, and still requires the rest.
func TestContributeSchemaRequiresOnlyWhatCannotBeInferred(t *testing.T) {
	ctx := context.Background()
	cs := connect(ctx, t, New("test", Handlers{Contribute: func(context.Context, wire.ContributeInput) (wire.ContributeOutput, error) {
		return wire.ContributeOutput{}, nil
	}}))
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(tools.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	type schema struct {
		Required   []string           `json:"required"`
		Properties map[string]*schema `json:"properties"`
		Items      *schema            `json:"items"`
	}
	var root schema
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	item := func(list string) *schema {
		p := root.Properties[list]
		if p == nil || p.Items == nil {
			t.Fatalf("schema has no %s items: %s", list, raw)
		}
		return p.Items
	}
	for _, c := range []struct {
		where    string
		required []string
	}{
		{"", []string{"area", "idempotency_key"}},
		{"entities", []string{"names", "type"}},
		{"evidence", nil},
		{"assertions", []string{"body_markdown", "kind", "subject"}},
		{"observations", []string{"body_markdown", "subject"}},
		{"links", []string{"claim", "evidence"}},
	} {
		s := &root
		if c.where != "" {
			s = item(c.where)
		}
		got := append([]string(nil), s.Required...)
		sort.Strings(got)
		if !reflect.DeepEqual(got, c.required) && (len(got) != 0 || len(c.required) != 0) {
			t.Errorf("%s required = %v, want %v", c.where, got, c.required)
		}
	}
}

// TestTemporalToolSchemas checks that search takes its mode, as_of and
// scope as optional fields (current is the default) and that get_as_of is
// listed and requires the id and the time.
func TestTemporalToolSchemas(t *testing.T) {
	ctx := context.Background()
	cs := connect(ctx, t, New("test", Handlers{
		Search: func(context.Context, wire.SearchInput) (wire.SearchOutput, error) { return wire.SearchOutput{}, nil },
		GetAsOf: func(_ context.Context, in wire.GetAsOfInput) (wire.GetAsOfOutput, error) {
			return wire.GetAsOfOutput{ID: in.ID, AsOf: *in.AsOf, Valid: true}, nil
		},
	}))
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	type schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	got := map[string]schema{}
	for _, tool := range tools.Tools {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var s schema
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		sort.Strings(s.Required)
		got[tool.Name] = s
	}
	if s, ok := got[ToolSearch]; !ok || !reflect.DeepEqual(s.Required, []string{"query"}) || s.Properties["mode"] == nil || s.Properties["as_of"] == nil || s.Properties["scope"] == nil {
		t.Errorf("search schema = %+v, want query required and mode, as_of and scope optional", s)
	}
	if s, ok := got[ToolGetAsOf]; !ok || !reflect.DeepEqual(s.Required, []string{"as_of", "id"}) {
		t.Errorf("get_as_of schema = %+v, want id and as_of required", s)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: ToolGetAsOf, Arguments: map[string]any{"id": "x", "as_of": "2026-09-03T00:00:00Z"}})
	if err != nil || res.IsError {
		t.Fatalf("get_as_of call = %+v, %v", res, err)
	}
}
