//go:build integration

package apitest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/apihandlers"
	"github.com/rztaylor/froodbase/internal/authz/staticusers"
	"github.com/rztaylor/froodbase/internal/client"
	"github.com/rztaylor/froodbase/internal/dreamer"
	"github.com/rztaylor/froodbase/internal/httpserver"
	"github.com/rztaylor/froodbase/internal/mcpserver"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
	"github.com/rztaylor/froodbase/internal/version"
)

// Password is every sample user's password.
const Password = "froodbase-dev"

// Users mirrors local/users.example.json plus dave, a reviewer in another
// area, so denied access can be exercised. The hash is bcrypt("froodbase-dev").
const Users = `{"users":[
 {"username":"alice","display_name":"Alice (reviewer, demo area)","password_hash":"$2a$10$HkYVU1Ax3GXp5pXEDDp0nuhVjSJmFyDWCt.adIMwFPXzhmeJKeF5e","grants":{"demo":["reviewer"]}},
 {"username":"bob","display_name":"Bob (contributor, demo area)","password_hash":"$2a$10$HkYVU1Ax3GXp5pXEDDp0nuhVjSJmFyDWCt.adIMwFPXzhmeJKeF5e","grants":{"demo":["contributor"]}},
 {"username":"carol","display_name":"Carol (reader, demo area)","password_hash":"$2a$10$HkYVU1Ax3GXp5pXEDDp0nuhVjSJmFyDWCt.adIMwFPXzhmeJKeF5e","grants":{"demo":["reader"]}},
 {"username":"dave","display_name":"Dave (reviewer, other area)","password_hash":"$2a$10$HkYVU1Ax3GXp5pXEDDp0nuhVjSJmFyDWCt.adIMwFPXzhmeJKeF5e","grants":{"other":["reviewer"]}}
]}`

// Env is a running in-process service.
type Env struct {
	// DB has the admin pool (bypasses row-level security, for inspection)
	// and the app pool the service uses.
	DB storetest.DB
	// Areas is the service's area directory.
	Areas *apihandlers.Areas
	// URL is the MCP endpoint.
	URL string
	// Dreamer works jobs in-process; it is started with the service.
	Dreamer *dreamer.Dreamer
}

// Start brings up the service on a fresh database, with the sample users'
// areas created, and a dreamer working jobs. Everything stops when the
// test ends.
func Start(t testing.TB) *Env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	db := storetest.Open(t)
	users, err := staticusers.Parse([]byte(Users))
	if err != nil {
		t.Fatal(err)
	}
	areas := apihandlers.NewAreas(db.App)
	if _, err := areas.Ensure(ctx, users.AreaKeys()); err != nil {
		t.Fatal(err)
	}

	mcpServer := mcpserver.New(version.Version, apihandlers.New(db.App, areas, "apitest/"+version.Version))
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httpserver.New(log)
	srv.Mount(httpserver.MCPPath, httpserver.BasicAuth(users, users, httpserver.ClientVersion(log, handler)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	d, err := dreamer.New(log, db.App, dreamer.Schedule{ReviewSweepInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
		}
	})

	return &Env{DB: db, Areas: areas, URL: ts.URL + httpserver.MCPPath, Dreamer: d}
}

// Client returns the forwarding client froodbase-mcp uses, signed in as
// one of the sample users.
func (e *Env) Client(user string) *client.Client {
	return client.New(e.URL, user, Password)
}

// WaitFor polls cond until it holds or the timeout passes.
func WaitFor(t testing.TB, cond func() bool, timeout time.Duration, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// WaitStartupSweep waits for the review sweep the dreamer runs at start to
// complete, so a test that seeds review dates or old checks afterwards and
// then sweeps at a time of its choosing (Dreamer.SweepReview) is not raced
// by it. Later periodic sweeps follow the default interval of an hour.
func (e *Env) WaitStartupSweep(t testing.TB) {
	t.Helper()
	ctx := context.Background()
	WaitFor(t, func() bool {
		var n int
		if err := e.DB.Admin.Pgx().QueryRow(ctx, "select count(*) from river_job where kind = $1 and state = 'completed'", jobs.KindReviewSweep).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}, 60*time.Second, "the start-up review sweep")
}

// Staleness reads the projection status.
func (e *Env) Staleness(ctx context.Context) (store.IndexStaleness, error) {
	return e.DB.App.Staleness(ctx)
}
