package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// migrationsFS embeds the goose migrations. Files are named
// NNNNN_description.sql with strictly increasing versions.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationsDir is the directory inside the embedded filesystem.
const MigrationsDir = "migrations"

const openTimeout = 10 * time.Second

// Pool is the process-wide PostgreSQL connection pool.
type Pool struct {
	pool   *pgxpool.Pool
	jobs   *JobInserter
	now    func() time.Time
	failAt func(step string) error // test hook, nil in production
}

// Open connects to PostgreSQL with the given URL and verifies the connection
// with a ping. The URL is never included in errors because it carries the
// password.
func Open(ctx context.Context, databaseURL string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: parsing database URL: %w", redact(err))
	}
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: opening pool: %w", redact(err))
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: connecting to PostgreSQL: %w", redact(err))
	}
	p := &Pool{pool: pool, now: time.Now}
	jobs, err := newJobInserter(p)
	if err != nil {
		pool.Close()
		return nil, err
	}
	p.jobs = jobs
	return p, nil
}

// Close releases every connection.
func (p *Pool) Close() {
	p.pool.Close()
}

// Pgx exposes the underlying pgx pool to packages inside the module that run
// queries; cmd packages do not use it.
func (p *Pool) Pgx() *pgxpool.Pool {
	return p.pool
}

// Migrations returns the embedded migration filesystem rooted at
// MigrationsDir.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationsFS, MigrationsDir)
	if err != nil {
		panic("store: embedded migrations directory missing: " + err.Error())
	}
	return sub
}

// MigrationStatus is one embedded migration and whether it has been applied.
type MigrationStatus struct {
	Version   int64
	Source    string
	Applied   bool
	AppliedAt time.Time
}

// Migrate applies River's job queue schema, then every pending goose
// migration in version order. River goes first so a goose migration can
// grant the service role access to River's tables whether the database is
// new or being upgraded. Running it again with nothing pending is a no-op.
// It returns the goose versions applied; River steps applied are counted in
// the second result.
func (p *Pool) Migrate(ctx context.Context) ([]int64, int, error) {
	riverSteps, err := p.migrateRiver(ctx)
	if err != nil {
		return nil, 0, err
	}
	provider, db, err := p.provider()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = db.Close() }()
	results, err := provider.Up(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("store: applying migrations: %w", err)
	}
	versions := make([]int64, 0, len(results))
	for _, r := range results {
		versions = append(versions, r.Source.Version)
	}
	return versions, riverSteps, nil
}

// RiverSource is the Source reported by Status for River's own schema.
const RiverSource = "river (job queue schema)"

// Status lists every embedded migration with its applied state, in version
// order, followed by one entry for River's schema version.
func (p *Pool) Status(ctx context.Context) ([]MigrationStatus, error) {
	provider, db, err := p.provider()
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	statuses, err := provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: reading migration status: %w", err)
	}
	out := make([]MigrationStatus, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, MigrationStatus{
			Version:   s.Source.Version,
			Source:    s.Source.Path,
			Applied:   s.State == goose.StateApplied,
			AppliedAt: s.AppliedAt,
		})
	}
	rv, err := p.riverVersion(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, MigrationStatus{Version: int64(rv), Source: RiverSource, Applied: rv > 0})
	return out, nil
}

// provider builds a goose provider over a database/sql handle borrowed from
// the pool. The caller closes the handle; closing it does not close the pool.
func (p *Pool) provider() (*goose.Provider, *sql.DB, error) {
	db := stdlib.OpenDBFromPool(p.pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, db, Migrations())
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("store: building migration provider: %w", err)
	}
	return provider, db, nil
}
