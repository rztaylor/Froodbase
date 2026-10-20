package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
)

// IndexState is the projection bookkeeping of one record (TR-57).
type IndexState struct {
	RecordID   domain.ID
	RecordType string
	AreaID     domain.ID
	Revision   int
	IndexedAt  time.Time
}

// RecordRevision names a record and its current revision.
type RecordRevision struct {
	ID         domain.ID
	RecordType string
	Revision   int
}

// IndexStaleness is the projection status across every area.
type IndexStaleness struct {
	Total int
	Stale int
}

// IndexRecord refreshes a record's projections and marks it indexed at its
// current revision. The caller must cover the record's area; a record the
// caller cannot see is ErrNotFound. In this slice the full-text and name
// projections are generated columns, so the work is the bookkeeping that
// makes staleness observable and proves the outbox path (D5).
func (p *Pool) IndexRecord(ctx context.Context, caller Caller, recordID domain.ID, recordType string, areaID domain.ID, at time.Time) (int, error) {
	var revision int
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rev, err := q.GetRecordRevision(ctx, string(recordID))
		if err != nil {
			return err
		}
		revision = int(rev)
		return q.UpsertIndexState(ctx, gen.UpsertIndexStateParams{
			RecordID: string(recordID), RecordType: recordType, AreaID: string(areaID), Revision: rev, IndexedAt: at.UTC(),
		})
	})
	return revision, err
}

// RebuildIndex re-indexes every record of an area at its current revision
// and returns how many it touched. Safe to run while writes continue: a
// write that lands after a record was read enqueues its own index job.
func (p *Pool) RebuildIndex(ctx context.Context, caller Caller, areaID domain.ID, at time.Time) (int, error) {
	n := 0
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.ListAreaRecordRevisions(ctx, string(areaID))
		if err != nil {
			return err
		}
		for _, r := range rows {
			if err := q.UpsertIndexState(ctx, gen.UpsertIndexStateParams{
				RecordID: r.ID, RecordType: r.RecordType, AreaID: string(areaID), Revision: r.Revision, IndexedAt: at.UTC(),
			}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// GetIndexState reads a record's bookkeeping; ErrNotFound when never indexed.
func (p *Pool) GetIndexState(ctx context.Context, recordID domain.ID) (IndexState, error) {
	row, err := gen.New(p.pool).GetIndexState(ctx, string(recordID))
	if err != nil {
		return IndexState{}, mapErr(err)
	}
	return IndexState{RecordID: domain.ID(row.RecordID), RecordType: row.RecordType, AreaID: domain.ID(row.AreaID), Revision: int(row.Revision), IndexedAt: row.IndexedAt}, nil
}

// Staleness counts records across every area and how many are not indexed
// at their current revision, through a definer function that exposes
// counts only.
func (p *Pool) Staleness(ctx context.Context) (IndexStaleness, error) {
	var s IndexStaleness
	if err := p.pool.QueryRow(ctx, "select total, stale from app_index_staleness()").Scan(&s.Total, &s.Stale); err != nil {
		return IndexStaleness{}, fmt.Errorf("store: reading index staleness: %w", err)
	}
	return s, nil
}
