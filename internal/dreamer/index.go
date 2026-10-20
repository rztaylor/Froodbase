package dreamer

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
)

// JobUser is the actor a dreamer job runs as. A job acts within the area it
// was enqueued for: its caller holds that area and nothing else.
const JobUser = "froodbase-dreamer"

func areaCaller(areaID string) store.Caller {
	return store.Caller{UserID: JobUser, AreaIDs: []string{areaID}}
}

// indexWorker refreshes one record's projections at its current revision
// (TR-57, TR-58) and records when it did so.
type indexWorker struct {
	river.WorkerDefaults[jobs.IndexRecordArgs]
	log  *slog.Logger
	pool *store.Pool
}

func (w *indexWorker) Work(ctx context.Context, job *river.Job[jobs.IndexRecordArgs]) error {
	a := job.Args
	rev, err := w.pool.IndexRecord(ctx, areaCaller(a.AreaID), domain.ID(a.RecordID), a.RecordType, domain.ID(a.AreaID), time.Now())
	if err != nil {
		return err
	}
	w.log.Info("index_record", "job_id", job.ID, "record_id", a.RecordID, "record_type", a.RecordType, "area_id", a.AreaID, "enqueued_revision", a.Revision, "indexed_revision", rev)
	return nil
}

// rebuildWorker re-indexes every record of an area.
type rebuildWorker struct {
	river.WorkerDefaults[jobs.RebuildIndexArgs]
	log  *slog.Logger
	pool *store.Pool
}

func (w *rebuildWorker) Work(ctx context.Context, job *river.Job[jobs.RebuildIndexArgs]) error {
	n, err := w.pool.RebuildIndex(ctx, areaCaller(job.Args.AreaID), domain.ID(job.Args.AreaID), time.Now())
	if err != nil {
		return err
	}
	w.log.Info("rebuild_index", "job_id", job.ID, "area_id", job.Args.AreaID, "records", n)
	return nil
}

// Staleness reports the projection status across every area.
func (d *Dreamer) Staleness(ctx context.Context) (store.IndexStaleness, error) {
	return d.pool.Staleness(ctx)
}
