package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
)

// SearchFilter is the mode and scope a search keeps (TR-50, TR-53), and the
// reference time the mode and status are relative to. The filter runs in
// SQL after row-level security and before the limit, through the same
// functions for every channel (froodbase_in_mode, froodbase_scope_overlaps).
// An unspecified scope keeps every scope; the zero Query is refused.
type SearchFilter struct {
	Query domain.Query
	Scope domain.Scope
	Now   time.Time
}

func (f SearchFilter) asOf() *time.Time {
	if f.Query.Mode != domain.ModeAsOf {
		return nil
	}
	t := f.Query.AsOf.UTC()
	return &t
}

func (f SearchFilter) validate() error {
	if err := f.Query.Validate(); err != nil {
		return err
	}
	if f.Now.IsZero() {
		return ErrNoReferenceTime
	}
	return nil
}

// NameHit is one identifier-channel candidate: an entity whose name matched
// the query exactly or within the trigram threshold, with the name's status
// relative to now (the entity's lifecycle state with the name's validity).
type NameHit struct {
	EntityID   domain.ID
	AreaID     domain.ID
	Name       string
	Similarity float64
	Exact      bool
	Status     domain.TemporalStatus
}

// BodyHit is one full-text candidate: an assertion whose body matched, its
// status relative to now and whether it is stale under the area's
// freshness policy.
type BodyHit struct {
	AssertionID domain.ID
	AreaID      domain.ID
	Rank        float64
	Status      domain.TemporalStatus
	Stale       bool
}

// SearchNames runs the identifier channel over the caller's areas (RLS
// filters before ranking, TR-52) and the filter's mode and scope. Names at
// or above threshold similarity are returned best first, with exact matches
// ahead.
func (p *Pool) SearchNames(ctx context.Context, caller Caller, query string, threshold float64, limit int, f SearchFilter) ([]NameHit, error) {
	if err := f.validate(); err != nil {
		return nil, err
	}
	var out []NameHit
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.SearchEntityNames(ctx, gen.SearchEntityNamesParams{
			Query: query, Threshold: threshold, Lim: int32(limit),
			Mode: string(f.Query.Mode), AsOf: f.asOf(), NowAt: f.Now.UTC(), Scope: scopeBytes(f.Scope),
		})
		if err != nil {
			return err
		}
		out = make([]NameHit, 0, len(rows))
		for _, r := range rows {
			out = append(out, NameHit{
				EntityID: domain.ID(r.EntityID), AreaID: domain.ID(r.AreaID), Name: r.Name, Similarity: r.Sim, Exact: r.Exact,
				Status: domain.TemporalStatus(r.Status),
			})
		}
		return nil
	})
	return out, err
}

// SearchBodies runs the full-text channel over the caller's areas and the
// filter's mode and scope.
func (p *Pool) SearchBodies(ctx context.Context, caller Caller, query string, limit int, f SearchFilter) ([]BodyHit, error) {
	if err := f.validate(); err != nil {
		return nil, err
	}
	var out []BodyHit
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.SearchAssertionBodies(ctx, gen.SearchAssertionBodiesParams{
			Query: query, Lim: int32(limit),
			Mode: string(f.Query.Mode), AsOf: f.asOf(), NowAt: f.Now.UTC(), Scope: scopeBytes(f.Scope),
		})
		if err != nil {
			return err
		}
		out = make([]BodyHit, 0, len(rows))
		for _, r := range rows {
			out = append(out, BodyHit{
				AssertionID: domain.ID(r.ID), AreaID: domain.ID(r.AreaID), Rank: r.Rank,
				Status: domain.TemporalStatus(r.Status), Stale: r.Stale,
			})
		}
		return nil
	})
	return out, err
}
