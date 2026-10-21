package dreamer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
)

// reconsiderWorker propagates a change in one record's support to every
// record derived from it (PR-13, TR-44). It completes its own job inside
// the walk's transaction, so a root added to the record while the walk
// runs either is read by it or, arriving after the commit, enqueues a new
// job rather than being folded into this finished one.
type reconsiderWorker struct {
	river.WorkerDefaults[jobs.ReconsiderDerivedArgs]
	log  *slog.Logger
	pool *store.Pool
}

func (w *reconsiderWorker) Work(ctx context.Context, job *river.Job[jobs.ReconsiderDerivedArgs]) error {
	a := job.Args
	caller := areaCaller(a.AreaID)
	caller.Job = jobs.KindReconsiderDerived
	res, err := w.pool.ReconsiderDerived(ctx, caller, domain.ID(a.RecordID), func(ctx context.Context, tx pgx.Tx) error {
		_, err := river.JobCompleteTx[*riverpgxv5.Driver](ctx, tx, job)
		return err
	})
	if errors.Is(err, store.ErrNotFound) {
		// The record is not in the job's area: retrying cannot help.
		return river.JobCancel(fmt.Errorf("reconsider_derived %s in area %s: %w", a.RecordID, a.AreaID, err))
	}
	if err != nil {
		return err
	}
	gained := 0
	for _, roots := range res.Gained {
		gained += len(roots)
	}
	w.log.Info("reconsider_derived", "job_id", job.ID, "record_id", a.RecordID, "area_id", a.AreaID,
		"walked", len(res.Walked), "changed", len(res.Gained), "roots_added", gained, "config_version", res.ConfigVersion)
	return nil
}

// ReconsiderWaiting counts propagation jobs waiting to run.
func (d *Dreamer) ReconsiderWaiting(ctx context.Context) (int, error) {
	return d.pool.WaitingJobs(ctx, jobs.KindReconsiderDerived)
}
