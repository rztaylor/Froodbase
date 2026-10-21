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

// minReviewSweepInterval is the shortest review sweep interval accepted;
// internal/config bounds the configured value more tightly.
const minReviewSweepInterval = time.Minute

const stopTimeout = 30 * time.Second

// Schedule is how often the configurable periodic jobs run; the heartbeat's
// interval is fixed. froodbase-dreamer fills it from internal/config.
type Schedule struct {
	// ReviewSweepInterval is how often the review sweep runs, at least
	// minReviewSweepInterval.
	ReviewSweepInterval time.Duration
}

// Validate checks every interval is set and not too short.
func (s Schedule) Validate() error {
	if s.ReviewSweepInterval < minReviewSweepInterval {
		return fmt.Errorf("dreamer: review sweep interval %s is shorter than %s", s.ReviewSweepInterval, minReviewSweepInterval)
	}
	return nil
}

// Dreamer runs the workers.
type Dreamer struct {
	log    *slog.Logger
	pool   *store.Pool
	client *river.Client[pgx.Tx]
	sched  Schedule
}

// New builds the worker client with every job kind registered and the
// periodic schedules configured. It does not start working.
func New(log *slog.Logger, pool *store.Pool, sched Schedule) (*Dreamer, error) {
	if err := sched.Validate(); err != nil {
		return nil, err
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &heartbeatWorker{log: log})
	river.AddWorker(workers, &indexWorker{log: log, pool: pool})
	river.AddWorker(workers, &rebuildWorker{log: log, pool: pool})
	river.AddWorker(workers, &reconsiderWorker{log: log, pool: pool})
	river.AddWorker(workers, &reviewSweepWorker{log: log, pool: pool})

	client, err := river.NewClient(riverpgxv5.New(pool.Pgx()), &river.Config{
		Logger:       log,
		Workers:      workers,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
		PeriodicJobs: PeriodicJobs(sched),
	})
	if err != nil {
		return nil, fmt.Errorf("dreamer: building River client: %w", err)
	}
	return &Dreamer{log: log, pool: pool, client: client, sched: sched}, nil
}

// PeriodicJobs lists every periodic schedule. River inserts them on the
// elected leader only, and each is unique per slot as a second guard, so
// two dreamers never both run one. Exposed so tests can check the
// configuration without a database.
func PeriodicJobs(sched Schedule) []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(HeartbeatInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.HeartbeatFor(time.Now()), nil
			},
			&river.PeriodicJobOpts{ID: jobs.KindHeartbeat, RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(sched.ReviewSweepInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.ReviewSweepFor(time.Now(), sched.ReviewSweepInterval), nil
			},
			&river.PeriodicJobOpts{ID: jobs.KindReviewSweep, RunOnStart: true},
		),
	}
}

// Run works jobs until ctx is cancelled, then stops gracefully.
func (d *Dreamer) Run(ctx context.Context) error {
	if err := d.client.Start(ctx); err != nil {
		return fmt.Errorf("dreamer: starting workers: %w", err)
	}
	d.log.Info("froodbase-dreamer workers started", "heartbeat_interval", HeartbeatInterval.String(), "review_sweep_interval", d.sched.ReviewSweepInterval.String())
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
