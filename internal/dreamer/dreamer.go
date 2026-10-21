package dreamer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
)

// HeartbeatInterval is how often the heartbeat job runs.
const HeartbeatInterval = time.Minute

const stopTimeout = 30 * time.Second

// Dreamer runs the workers.
type Dreamer struct {
	log    *slog.Logger
	pool   *store.Pool
	client *river.Client[pgx.Tx]
}

// New builds the worker client with every job kind registered and the
// periodic schedules configured. It does not start working.
func New(log *slog.Logger, pool *store.Pool) (*Dreamer, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &heartbeatWorker{log: log})
	river.AddWorker(workers, &indexWorker{log: log, pool: pool})
	river.AddWorker(workers, &rebuildWorker{log: log, pool: pool})
	river.AddWorker(workers, &reconsiderWorker{log: log, pool: pool})

	client, err := river.NewClient(riverpgxv5.New(pool.Pgx()), &river.Config{
		Logger:       log,
		Workers:      workers,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
		PeriodicJobs: PeriodicJobs(),
	})
	if err != nil {
		return nil, fmt.Errorf("dreamer: building River client: %w", err)
	}
	return &Dreamer{log: log, pool: pool, client: client}, nil
}

// PeriodicJobs lists every periodic schedule. Exposed so tests can check the
// configuration without a database.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(HeartbeatInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.HeartbeatFor(time.Now()), nil
			},
			&river.PeriodicJobOpts{ID: jobs.KindHeartbeat, RunOnStart: true},
		),
	}
}

// Run works jobs until ctx is cancelled, then stops gracefully.
func (d *Dreamer) Run(ctx context.Context) error {
	if err := d.client.Start(ctx); err != nil {
		return fmt.Errorf("dreamer: starting workers: %w", err)
	}
	d.log.Info("froodbase-dreamer workers started", "heartbeat_interval", HeartbeatInterval.String())
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	if err := d.client.Stop(stopCtx); err != nil {
		return fmt.Errorf("dreamer: stopping workers: %w", err)
	}
	return nil
}

// QueueDepth reports jobs waiting or running.
func (d *Dreamer) QueueDepth(ctx context.Context) (int, error) {
	return d.pool.QueueDepth(ctx)
}

// heartbeatWorker logs that the dreamer is alive. Its completed job row is
// the durable evidence.
type heartbeatWorker struct {
	river.WorkerDefaults[jobs.HeartbeatArgs]
	log *slog.Logger
}

func (w *heartbeatWorker) Work(_ context.Context, job *river.Job[jobs.HeartbeatArgs]) error {
	w.log.Info("heartbeat", "job_id", job.ID, "scheduled_for", job.Args.ScheduledFor.Format(time.RFC3339), "attempt", job.Attempt)
	return nil
}
