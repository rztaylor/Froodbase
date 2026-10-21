package dreamer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
)

// reviewSweepWorker marks, in every area, the records due for review and
// those stale under the area's freshness policy (PR-33, TR-31). Age makes a
// record due for review; it never changes, ends or removes it: the sweep
// writes flags beside the records and nothing else.
type reviewSweepWorker struct {
	river.WorkerDefaults[jobs.ReviewSweepArgs]
	log  *slog.Logger
	pool *store.Pool
}

// reviewSweepTimeout bounds one sweep of every area. River's default of a
// minute could cut a large sweep short on every attempt; each area commits
// on its own, so a sweep cut short has still marked the areas it reached.
const reviewSweepTimeout = 15 * time.Minute

// Timeout implements river.Worker.
func (w *reviewSweepWorker) Timeout(*river.Job[jobs.ReviewSweepArgs]) time.Duration {
	return reviewSweepTimeout
}

func (w *reviewSweepWorker) Work(ctx context.Context, job *river.Job[jobs.ReviewSweepArgs]) error {
	results, err := sweepReview(ctx, w.pool, time.Now())
	due, stale, changed := 0, 0, 0
	for _, r := range results {
		due, stale, changed = due+r.Due, stale+r.Stale, changed+r.Changed
		w.log.Info("review_sweep area", "job_id", job.ID, "area_id", r.AreaID, "records", r.Records, "due", r.Due, "stale", r.Stale, "changed", r.Changed)
	}
	w.log.Info("review_sweep", "job_id", job.ID, "scheduled_for", job.Args.ScheduledFor.Format(time.RFC3339),
		"areas", len(results), "due", due, "stale", stale, "changed", changed)
	return err
}

// SweepReview runs the review sweep now, as the periodic job does, at the
// given reference time: every area in turn, each with a caller holding that
// area alone. Tests use it to sweep at a chosen time; the job passes the
// clock.
func (d *Dreamer) SweepReview(ctx context.Context, now time.Time) ([]store.ReviewSweepResult, error) {
	return sweepReview(ctx, d.pool, now)
}

// sweepReview sweeps every area at now. An area that fails does not stop
// the others; the errors are returned together so River retries the job,
// and sweeping again is harmless.
func sweepReview(ctx context.Context, pool *store.Pool, now time.Time) ([]store.ReviewSweepResult, error) {
	areas, err := pool.AllAreaIDs(ctx)
	if err != nil {
		return nil, err
	}
	var results []store.ReviewSweepResult
	var errs []error
	for _, id := range areas {
		caller := areaCaller(string(id))
		caller.Job = jobs.KindReviewSweep
		r, err := pool.SweepReview(ctx, caller, id, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("review_sweep area %s: %w", id, err))
			continue
		}
		results = append(results, r)
	}
	return results, errors.Join(errs...)
}
