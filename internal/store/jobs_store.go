package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

// JobInserter enqueues background jobs. The River client it wraps has no
// workers; working jobs is froodbase-dreamer's role.
type JobInserter struct {
	client *river.Client[pgx.Tx]
}

// Jobs returns the inserter bound to this pool.
func (p *Pool) Jobs() *JobInserter {
	return p.jobs
}

func newJobInserter(p *Pool) (*JobInserter, error) {
	client, err := river.NewClient(riverpgxv5.New(p.pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("store: building job inserter: %w", err)
	}
	return &JobInserter{client: client}, nil
}

// InsertTx enqueues a job inside tx, so it is committed or rolled back with
// the record change that caused it (the outbox pattern, D5).
func (j *JobInserter) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs) (*rivertype.JobInsertResult, error) {
	res, err := j.client.InsertTx(ctx, tx, args, nil)
	if err != nil {
		return nil, fmt.Errorf("store: enqueueing %s: %w", args.Kind(), err)
	}
	return res, nil
}

// Insert enqueues a job outside a transaction, for periodic and operator
// triggered work.
func (j *JobInserter) Insert(ctx context.Context, args river.JobArgs) (*rivertype.JobInsertResult, error) {
	res, err := j.client.Insert(ctx, args, nil)
	if err != nil {
		return nil, fmt.Errorf("store: enqueueing %s: %w", args.Kind(), err)
	}
	return res, nil
}

// QueueDepth counts jobs waiting or running.
func (p *Pool) QueueDepth(ctx context.Context) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx,
		"select count(*) from river_job where state in ('available', 'scheduled', 'retryable', 'running', 'pending')").Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: reading queue depth: %w", err)
	}
	return n, nil
}

// migrateRiver applies River's own schema (the river_job and related tables)
// after the goose migrations. River versions its schema itself; the migration
// is forward-only here like everything else.
func (p *Pool) migrateRiver(ctx context.Context) (int, error) {
	m, err := rivermigrate.New(riverpgxv5.New(p.pool), nil)
	if err != nil {
		return 0, fmt.Errorf("store: building River migrator: %w", err)
	}
	res, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return 0, fmt.Errorf("store: applying River migrations: %w", err)
	}
	return len(res.Versions), nil
}

// riverVersion reports the newest River schema version applied, 0 if none.
func (p *Pool) riverVersion(ctx context.Context) (int, error) {
	m, err := rivermigrate.New(riverpgxv5.New(p.pool), nil)
	if err != nil {
		return 0, fmt.Errorf("store: building River migrator: %w", err)
	}
	versions, err := m.ExistingVersions(ctx)
	if err != nil {
		// River reports an error when its tables do not exist yet.
		return 0, nil //nolint:nilerr // absence of the schema is version 0
	}
	if len(versions) == 0 {
		return 0, nil
	}
	return versions[len(versions)-1].Version, nil
}
