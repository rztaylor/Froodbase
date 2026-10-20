//go:build integration

package storetest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rztaylor/froodbase/internal/store"
)

// Image is the PostgreSQL image used, the same as compose.yaml.
const Image = "pgvector/pgvector:pg16"

const (
	adminUser     = "froodbase"
	adminPassword = "froodbase-test"
	appUser       = "froodbase_app" // the service role the migrations create
	appPassword   = "froodbase-app-test"
	templateDB    = "froodbase_template"
	startTimeout  = 3 * time.Minute
)

var (
	once      sync.Once
	startErr  error
	maintURL  string // connection URL to the maintenance database
	hostPort  string // host:port of the container
	dbCounter atomic.Int64
)

// DB is a fresh, migrated database with two pools on it: Admin connects as
// the owning superuser (it bypasses row-level security, so use it to set up
// and to inspect what a write left behind) and App connects as the service
// role froodbase_app, which the policies apply to. Tests of what a caller
// can see must use App.
type DB struct {
	Admin *store.Pool
	App   *store.Pool
}

// Open returns a DB on a fresh database with every migration applied. The
// database is dropped when the test ends.
func Open(t testing.TB) DB {
	t.Helper()
	name := create(t, true)
	return DB{Admin: openPool(t, adminURL(name)), App: openPool(t, appURL(name))}
}

// Pool returns the admin pool of a fresh, migrated database, for tests that
// do not exercise row-level security.
func Pool(t testing.TB) *store.Pool {
	t.Helper()
	return Open(t).Admin
}

// EmptyPool returns a store.Pool on a fresh database with no migrations
// applied, for tests of the migration path itself.
func EmptyPool(t testing.TB) *store.Pool {
	t.Helper()
	return openPool(t, adminURL(create(t, false)))
}

func openPool(t testing.TB, url string) *store.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := store.Open(ctx, url)
	if err != nil {
		t.Fatalf("storetest: opening pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// create makes a fresh database, cloned from the migrated template when
// migrated is true, and drops it when the test ends.
func create(t testing.TB, migrated bool) string {
	t.Helper()
	once.Do(start)
	if startErr != nil {
		t.Fatalf("storetest: %v", startErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	name := fmt.Sprintf("test_%d_%d", os.Getpid(), dbCounter.Add(1))
	admin, err := pgx.Connect(ctx, maintURL)
	if err != nil {
		t.Fatalf("storetest: connecting as admin: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	create := fmt.Sprintf("CREATE DATABASE %s", name)
	if migrated {
		create += " TEMPLATE " + templateDB
	}
	if _, err := admin.Exec(ctx, create); err != nil {
		t.Fatalf("storetest: creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		admin, err := pgx.Connect(ctx, maintURL)
		if err != nil {
			return
		}
		defer func() { _ = admin.Close(ctx) }()
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return name
}

// start runs the container and prepares the migrated template database.
func start() {
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	c, err := postgres.Run(ctx, Image,
		postgres.WithUsername(adminUser),
		postgres.WithPassword(adminPassword),
		postgres.WithDatabase("postgres"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		startErr = fmt.Errorf("starting %s (is Docker running?): %w", Image, err)
		return
	}
	host, err := c.Host(ctx)
	if err != nil {
		startErr = err
		return
	}
	port, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		startErr = err
		return
	}
	hostPort = fmt.Sprintf("%s:%s", host, port.Port())
	maintURL = adminURL("postgres")

	admin, err := pgx.Connect(ctx, maintURL)
	if err != nil {
		startErr = fmt.Errorf("connecting as admin: %w", err)
		return
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+templateDB); err != nil {
		startErr = fmt.Errorf("creating template: %w", err)
		return
	}
	pool, err := store.Open(ctx, adminURL(templateDB))
	if err != nil {
		startErr = fmt.Errorf("opening template: %w", err)
		return
	}
	_, _, err = pool.Migrate(ctx)
	pool.Close() // the template must have no connections when it is cloned
	if err != nil {
		startErr = fmt.Errorf("migrating template: %w", err)
		return
	}
	// The migration creates the service role without a login; give it one
	// so tests can connect as the role the policies apply to. Compose does
	// the same through its init script.
	if _, err := admin.Exec(ctx, fmt.Sprintf("ALTER ROLE %s LOGIN PASSWORD '%s'", appUser, appPassword)); err != nil {
		startErr = fmt.Errorf("enabling login for %s: %w", appUser, err)
		return
	}
}

func adminURL(db string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", adminUser, adminPassword, hostPort, db)
}

func appURL(db string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", appUser, appPassword, hostPort, db)
}
