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
	templateDB    = "froodbase_template"
	startTimeout  = 3 * time.Minute
)

var (
	once      sync.Once
	startErr  error
	adminURL  string // connection URL to the maintenance database
	hostPort  string // host:port of the container
	dbCounter atomic.Int64
)

// Pool returns a store.Pool on a fresh database with every migration
// applied. The database is dropped when the test ends.
func Pool(t testing.TB) *store.Pool {
	t.Helper()
	return open(t, true)
}

// EmptyPool returns a store.Pool on a fresh database with no migrations
// applied, for tests of the migration path itself.
func EmptyPool(t testing.TB) *store.Pool {
	t.Helper()
	return open(t, false)
}

func open(t testing.TB, migrated bool) *store.Pool {
	t.Helper()
	once.Do(start)
	if startErr != nil {
		t.Fatalf("storetest: %v", startErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	name := fmt.Sprintf("test_%d_%d", os.Getpid(), dbCounter.Add(1))
	admin, err := pgx.Connect(ctx, adminURL)
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

	pool, err := store.Open(ctx, url(name))
	if err != nil {
		t.Fatalf("storetest: opening %s: %v", name, err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			return
		}
		defer func() { _ = admin.Close(ctx) }()
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return pool
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
	adminURL = url("postgres")

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		startErr = fmt.Errorf("connecting as admin: %w", err)
		return
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+templateDB); err != nil {
		startErr = fmt.Errorf("creating template: %w", err)
		return
	}
	pool, err := store.Open(ctx, url(templateDB))
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
}

func url(db string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", adminUser, adminPassword, hostPort, db)
}
