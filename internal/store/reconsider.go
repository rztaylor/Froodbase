package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
)

// Reconsideration (PR-13, TR-44): when a record's support changes, every
// record derived from it is brought up to date with its sources. This slice
// carries the strengthening direction: a dependant gains each root its
// direct sources have and it has not counted. The weakening events
// (support_retracted, source_changed, contradicted) and needs_reconsideration
// extend reconsiderOne; the walk stays as it is.

// Reconsidered reports what one reconsideration changed.
type Reconsidered struct {
	// RecordID is the changed record the walk started from.
	RecordID domain.ID
	// Walked lists every dependant reconsidered, sources first.
	Walked []domain.ID
	// Gained lists the roots each dependant gained; dependants that gained
	// nothing are absent.
	Gained map[domain.ID][]domain.ID
	// Scores is each walked dependant's score afterwards.
	Scores map[domain.ID]float64
	// ConfigVersion is the support configuration the scores were projected under.
	ConfigVersion string
}

// ReconsiderDerived walks every assertion derived from recordID, directly or
// transitively, sources first, and brings each up to date with its direct
// sources: a root_added event, naming the caller's job and the sources it
// came from, for every root of theirs it has not counted (directly, through
// another source, or as the same person's statement), and its score
// re-projected under the area's current configuration. It writes support
// events and stored scores only: no revisions, as no content changes. The
// changed record is held for share for the whole transaction, so a root
// added to it concurrently either commits before the walk reads it or
// waits until the walk has committed. finish, when set, runs last in the
// same transaction (the dreamer completes its job there). A record the
// caller cannot see is ErrNotFound; derivation links never leave an area,
// so the walk stays in the record's area.
func (p *Pool) ReconsiderDerived(ctx context.Context, caller Caller, recordID domain.ID, finish func(ctx context.Context, tx pgx.Tx) error) (*Reconsidered, error) {
	if err := recordID.Validate(); err != nil {
		return nil, fmt.Errorf("record id: %w", err)
	}
	res := &Reconsidered{RecordID: recordID, Gained: map[domain.ID][]domain.ID{}, Scores: map[domain.ID]float64{}}
	err := p.inTx(ctx, caller, func(ctx context.Context, tx pgx.Tx, q *gen.Queries) error {
		now := p.now().UTC()
		cur, err := q.GetAssertion(ctx, string(recordID))
		if err != nil {
			return err
		}
		if err := q.LockAssertionShare(ctx, cur.ID); err != nil {
			return err
		}
		cfg, err := p.currentSupportConfig(ctx, q, cur.AreaID)
		if err != nil {
			return err
		}
		res.ConfigVersion = cfg.Version
		order, err := dependants(ctx, q, recordID)
		if err != nil {
			return err
		}
		for _, id := range order {
			score, gained, err := p.reconsiderOne(ctx, tx, q, caller, id, cfg, now)
			if err != nil {
				return fmt.Errorf("reconsidering %s: %w", id, err)
			}
			res.Walked = append(res.Walked, id)
			res.Scores[id] = score
			if len(gained) > 0 {
				res.Gained[id] = gained
			}
		}
		if finish != nil {
			return finish(ctx, tx)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// dependants reads the derivation links leading from start, one step at a
// time until no new record is reached, and orders the records they reach
// with domain.Dependants. Each record is expanded once, so a cycle ends the
// walk rather than looping.
func dependants(ctx context.Context, q *gen.Queries, start domain.ID) ([]domain.ID, error) {
	var links []domain.Derivation
	reached := map[string]bool{string(start): true}
	for frontier := []string{string(start)}; len(frontier) > 0; {
		rows, err := q.ListDerivedFrom(ctx, frontier)
		if err != nil {
			return nil, err
		}
		frontier = nil
		for _, r := range rows {
			links = append(links, domain.Derivation{DerivedID: domain.ID(r.DerivedID), SourceID: domain.ID(r.SourceID), AreaID: domain.ID(r.AreaID), Rationale: r.Rationale, RecordedAt: r.RecordedAt})
			if !reached[r.DerivedID] {
				reached[r.DerivedID] = true
				frontier = append(frontier, r.DerivedID)
			}
		}
	}
	return domain.Dependants(start, links), nil
}

// reconsiderOne brings one dependant up to date with its direct sources'
// roots and returns its score and the roots it gained.
func (p *Pool) reconsiderOne(ctx context.Context, tx pgx.Tx, q *gen.Queries, caller Caller, id domain.ID, cfg domain.SupportConfig, now time.Time) (float64, []domain.ID, error) {
	cur, err := q.GetAssertion(ctx, string(id))
	if err != nil {
		return 0, nil, err
	}
	links, err := q.ListDerivationSources(ctx, cur.ID)
	if err != nil {
		return 0, nil, err
	}
	sourceRoots := make(map[domain.ID][]domain.ID, len(links))
	for _, l := range links {
		roots, err := q.ListSupportRoots(ctx, l.SourceID)
		if err != nil {
			return 0, nil, err
		}
		for _, r := range roots {
			sourceRoots[domain.ID(l.SourceID)] = append(sourceRoots[domain.ID(l.SourceID)], domain.ID(r))
		}
	}
	score, _, gained, err := p.addRoots(ctx, tx, q, caller, cur, domain.DerivedRoots(nil, sourceRoots), cfg, "", now)
	return score, gained, err
}
