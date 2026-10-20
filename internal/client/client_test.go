package client

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
	"github.com/rztaylor/froodbase/internal/httpserver"
	"github.com/rztaylor/froodbase/internal/mcpserver"
	"github.com/rztaylor/froodbase/internal/version"
)

type fakeAuth struct{}

func (fakeAuth) Authenticate(user, pass string) (authz.Principal, bool) {
	if user == "alice" && pass == "secret" {
		return authz.Principal{Subject: "alice"}, true
	}
	return authz.Principal{}, false
}

func (fakeAuth) Build(context.Context, authz.Principal) (*authz.Context, error) {
	return &authz.Context{UserID: "alice", DisplayName: "Alice", Grants: map[string][]authz.Role{"demo": {authz.RoleReviewer}}}, nil
}

// api starts an in-process froodbase-api MCP endpoint and records the
// client version header it saw.
func api(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var seen string
	s := httpserver.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpserver.New("test", mcpserver.WhoamiHandlers())
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	record := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(version.ClientHeader)
		h.ServeHTTP(w, r)
	})
	s.Mount(httpserver.MCPPath, httpserver.BasicAuth(fakeAuth{}, fakeAuth{}, record))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, &seen
}

func TestWhoamiForwardsWithAuthAndVersion(t *testing.T) {
	ts, seen := api(t)
	c := New(ts.URL+httpserver.MCPPath, "alice", "secret")
	out, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.UserID != "alice" || out.Grants["demo"][0] != "reviewer" {
		t.Fatalf("whoami = %+v", out)
	}
	if *seen != version.Version {
		t.Fatalf("version header = %q, want %q", *seen, version.Version)
	}
}

func TestWrongCredentialsIsAuthError(t *testing.T) {
	ts, _ := api(t)
	c := New(ts.URL+httpserver.MCPPath, "alice", "nope")
	_, err := c.Whoami(context.Background())
	var ae *AuthError
	if !errors.As(err, &ae) || ae.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want AuthError 401", err)
	}
	var ue *mcpserver.UnavailableError
	if errors.As(err, &ue) {
		t.Fatal("auth failure reported as unavailable")
	}
}

func TestConnectionRefusedIsUnavailable(t *testing.T) {
	ts, _ := api(t)
	url := ts.URL + httpserver.MCPPath
	ts.Close()
	c := New(url, "alice", "secret")
	_, err := c.Whoami(context.Background())
	var ue *mcpserver.UnavailableError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want UnavailableError", err)
	}
	if ue.APIURL != url || ue.Err == nil {
		t.Fatalf("unavailable = %+v", ue)
	}
}

func TestServerErrorIsUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	c := New(ts.URL, "alice", "secret")
	_, err := c.Whoami(context.Background())
	var ue *mcpserver.UnavailableError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want UnavailableError", err)
	}
}

func TestToolErrorPassesThrough(t *testing.T) {
	s := httpserver.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpserver.New("test", mcpserver.Handlers{Whoami: func(context.Context) (mcpserver.WhoamiOutput, error) {
			return mcpserver.WhoamiOutput{}, errors.New("no such caller")
		}})
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	s.Mount(httpserver.MCPPath, h)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	c := New(ts.URL+httpserver.MCPPath, "x", "y")
	_, err := c.Whoami(context.Background())
	if err == nil || err.Error() != "froodbase-api: no such caller" {
		t.Fatalf("err = %v", err)
	}
	var ue *mcpserver.UnavailableError
	if errors.As(err, &ue) {
		t.Fatal("tool error reported as unavailable")
	}
}
