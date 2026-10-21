package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
)

// The review sweep (slice 2, PR-33): flags beside the records marking those
// due for review and those stale under their area's freshness policy. The
// flags are projections in review_flag and review_sweep (migration 8); the
// sweep never writes a record, a revision or a support event.

// ReviewSweepResult is what one sweep of an area found: the assertions it
// read, how many are due for review and how many stale at SweptAt, and how
// many flag rows it changed.
type ReviewSweepResult struct {
	AreaID  domain.ID
	SweptAt time.Time
	Records int
	Due     int
	Stale   int
	Changed int
}

// ReviewFlags are the sweep's marks on one assertion: DueForReview as of
// the area's last sweep, StaleAt when a sweep first found it stale (nil
// when it is not stale now), and SweptAt when its area was last swept (nil
// when never). They lag the record by up to one sweep; the live stale label
// is ClassifyAssertion's.
type ReviewFlags struct {
	DueForReview bool
	StaleAt      *time.Time
	SweptAt      *time.Time
}

// SweepReview marks every assertion of an area due for review or stale at
// now, through froodbase_due_for_review and froodbase_stale under the
// area's freshness policy, the same functions the live labels use, and
// records the sweep. Only flag rows that change are written. An area the
// caller does not hold is ErrNotFound. Sweeping is idempotent at a given
// now.
func (p *Pool) SweepReview(ctx context.Context, caller Caller, areaID domain.ID, now time.Time) (ReviewSweepResult, error) {
	if err := areaID.Validate(); err != nil {
		return ReviewSweepResult{}, fmt.Errorf("area id: %w", err)
	}
	res := ReviewSweepResult{AreaID: areaID, SweptAt: now.UTC()}
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		if _, err := q.GetArea(ctx, string(areaID)); err != nil {
			return err
		}
		row, err := q.SweepReviewFlags(ctx, gen.SweepReviewFlagsParams{NowAt: res.SweptAt, AreaID: string(areaID)})
		if err != nil {
			return err
		}
		res.Records, res.Due, res.Stale, res.Changed = int(row.Records), int(row.Due), int(row.Stale), int(row.Changed)
		return q.UpsertReviewSweep(ctx, gen.UpsertReviewSweepParams{
			AreaID: string(areaID), SweptAt: res.SweptAt, Records: row.Records, DueCount: row.Due, StaleCount: row.Stale,
		})
	})
	if err != nil {
		return ReviewSweepResult{}, err
	}
	return res, nil
}

// ReviewFlags reads the sweep's marks on one assertion. An assertion the
// caller cannot read is ErrNotFound; one the sweep never marked has none.
func (p *Pool) ReviewFlags(ctx context.Context, caller Caller, id domain.ID) (ReviewFlags, error) {
	var out ReviewFlags
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		row, err := q.GetReviewFlags(ctx, string(id))
		if err != nil {
			return err
		}
		out = ReviewFlags{DueForReview: row.DueForReview, StaleAt: row.StaleAt, SweptAt: row.SweptAt}
		return nil
	})
	return out, err
}

// AllAreaIDs lists every knowledge area's id, for the periodic sweep, which
// then sweeps each with a caller holding that area alone. It reads through
// app_all_area_ids, which returns ids only.
func (p *Pool) AllAreaIDs(ctx context.Context) ([]domain.ID, error) {
	rows, err := p.pool.Query(ctx, "select a::text from app_all_area_ids() a")
	if err != nil {
		return nil, fmt.Errorf("store: listing areas: %w", err)
	}
	ids, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ID, error) {
		var id string
		err := r.Scan(&id)
		return domain.ID(id), err
	})
	if err != nil {
		return nil, fmt.Errorf("store: listing areas: %w", err)
	}
	return ids, nil
}
