package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
)

// NameHit is one identifier-channel candidate: an entity whose name matched
// the query exactly or within the trigram threshold.
type NameHit struct {
	EntityID   domain.ID
	AreaID     domain.ID
	Name       string
	Similarity float64
	Exact      bool
}

// BodyHit is one full-text candidate: an assertion whose body matched.
type BodyHit struct {
	AssertionID domain.ID
	AreaID      domain.ID
	Rank        float64
}

// SearchNames runs the identifier channel over the caller's areas (RLS
// filters before ranking, TR-52). Names at or above threshold similarity
// are returned best first, with exact matches ahead.
func (p *Pool) SearchNames(ctx context.Context, caller Caller, query string, threshold float64, limit int) ([]NameHit, error) {
	var out []NameHit
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.SearchEntityNames(ctx, gen.SearchEntityNamesParams{Query: query, Threshold: threshold, Lim: int32(limit)})
		if err != nil {
			return err
		}
		out = make([]NameHit, 0, len(rows))
		for _, r := range rows {
			out = append(out, NameHit{EntityID: domain.ID(r.EntityID), AreaID: domain.ID(r.AreaID), Name: r.Name, Similarity: r.Sim, Exact: r.Exact})
		}
		return nil
	})
	return out, err
}

// SearchBodies runs the full-text channel over the caller's areas.
func (p *Pool) SearchBodies(ctx context.Context, caller Caller, query string, limit int) ([]BodyHit, error) {
	var out []BodyHit
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.SearchAssertionBodies(ctx, gen.SearchAssertionBodiesParams{Query: query, Lim: int32(limit)})
		if err != nil {
			return err
		}
		out = make([]BodyHit, 0, len(rows))
		for _, r := range rows {
			out = append(out, BodyHit{AssertionID: domain.ID(r.ID), AreaID: domain.ID(r.AreaID), Rank: r.Rank})
		}
		return nil
	})
	return out, err
}
