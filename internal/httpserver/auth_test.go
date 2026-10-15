package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/mcpserver"
)

type fakeAuth struct{}

func (fakeAuth) Authenticate(user, pass string) (authz.Principal, bool) {
	if user == "alice" && pass == "secret" {
		return authz.Principal{Subject: "alice"}, true
	}
	if user == "ghost" && pass == "secret" {
		return authz.Principal{Subject: "ghost"}, true
	}
	return authz.Principal{}, false
}

func (fakeAuth) Build(_ context.Context, p authz.Principal) (*authz.Context, error) {
	if p.Subject != "alice" {
		return nil, errors.New("unknown")
	}
	return &authz.Context{UserID: "alice", DisplayName: "Alice", Grants: map[string][]authz.Role{"support": {authz.RoleReviewer}}}, nil
}

func TestBasicAuth(t *testing.T) {
	var seen *authz.Context
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		seen = authz.FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	h := BasicAuth(fakeAuth{}, fakeAuth{}, next)

	tests := []struct {
		name       string
		user, pass string
		noAuth     bool
		want       int
	}{
		{name: "no credentials", noAuth: true, want: http.StatusUnauthorized},
		{name: "wrong password", user: "alice", pass: "nope", want: http.StatusUnauthorized},
		{name: "unknown user", user: "mallory", pass: "secret", want: http.StatusUnauthorized},
		{name: "authenticated but no context", user: "ghost", pass: "secret", want: http.StatusUnauthorized},
		{name: "ok", user: "alice", pass: "secret", want: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, seen = 0, nil
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if !tt.noAuth {
				req.SetBasicAuth(tt.user, tt.pass)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			if tt.want == http.StatusUnauthorized {
				if calls != 0 {
					t.Fatal("handler ran without valid credentials")
				}
				if rec.Header().Get("WWW-Authenticate") == "" {
					t.Fatal("missing WWW-Authenticate header")
				}
				return
			}
			if calls != 1 || seen == nil || seen.UserID != "alice" {
				t.Fatalf("handler calls = %d, context = %+v", calls, seen)
			}
		})
	}
}

type basicAuthTransport struct{ user, pass string }

func (b basicAuthTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.SetBasicAuth(b.user, b.pass)
	return http.DefaultTransport.RoundTrip(r)
}

// TestWhoamiOverHTTP proves the authorisation context set by the middleware
// reaches the tool handler through the SDK's Streamable HTTP transport.
func TestWhoamiOverHTTP(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := New(log)
	mcpServer := mcpserver.New("test", mcpserver.APIHandlers())
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	s.Mount(MCPPath, BasicAuth(fakeAuth{}, fakeAuth{}, mcpHandler))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL + MCPPath,
		HTTPClient: &http.Client{Transport: basicAuthTransport{"alice", "secret"}},
		MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: mcpserver.ToolWhoami})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	out, ok := res.StructuredContent.(map[string]any)
	if !ok || out["user_id"] != "alice" {
		t.Fatalf("whoami = %#v", res.StructuredContent)
	}

	// Unauthenticated connect fails before any tool runs.
	bad := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	if _, err := bad.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + MCPPath, MaxRetries: -1}, nil); err == nil {
		t.Fatal("connect without credentials succeeded")
	}
}
