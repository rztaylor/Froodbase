package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
)

// Temporal reads (slice 2): one record through the SQL mode predicate, and
// the per-area freshness policy the stale rule reads. The rules are the SQL
// functions of migration 7, which answer to the domain's case tables.

// ClassifyAssertion runs one assertion through the mode predicate, the
// status label and the stale rule under its area's freshness policy, at
// now. A record the caller cannot read is ErrNotFound.
func (p *Pool) ClassifyAssertion(ctx context.Context, caller Caller, id domain.ID, query domain.Query, now time.Time) (domain.ModeResult, error) {
	f := SearchFilter{Query: query, Now: now}
	if err := f.validate(); err != nil {
		return domain.ModeResult{}, err
	}
	var out domain.ModeResult
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		row, err := q.ClassifyAssertion(ctx, gen.ClassifyAssertionParams{Mode: string(query.Mode), AsOf: f.asOf(), NowAt: now.UTC(), ID: string(id)})
		if err != nil {
			return err
		}
		out = domain.ModeResult{Included: row.Included, Status: domain.TemporalStatus(row.Status), Stale: row.Stale}
		return nil
	})
	return out, err
}

// NameMode is one entity name through the mode predicate: its position in
// the entity's names, whether the mode keeps it and its status relative to
// now, read with the entity's lifecycle state.
type NameMode struct {
	Position int
	Included bool
	Status   domain.TemporalStatus
}

// ClassifyNames runs each name of an entity through the mode predicate, in
// name order. An entity the caller cannot read has none.
func (p *Pool) ClassifyNames(ctx context.Context, caller Caller, entityID domain.ID, query domain.Query, now time.Time) ([]NameMode, error) {
	f := SearchFilter{Query: query, Now: now}
	if err := f.validate(); err != nil {
		return nil, err
	}
	var out []NameMode
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.ClassifyEntityNames(ctx, gen.ClassifyEntityNamesParams{Mode: string(query.Mode), AsOf: f.asOf(), NowAt: now.UTC(), EntityID: string(entityID)})
		if err != nil {
			return err
		}
		out = make([]NameMode, 0, len(rows))
		for _, r := range rows {
			out = append(out, NameMode{Position: int(r.Position), Included: r.Included, Status: domain.TemporalStatus(r.Status)})
		}
		return nil
	})
	return out, err
}

// FreshnessPolicy reads the policy that applies in an area: its newest, or
// the service default (domain.DefaultFreshnessPolicy) when it has none.
func (p *Pool) FreshnessPolicy(ctx context.Context, caller Caller, areaID domain.ID) (domain.FreshnessPolicy, error) {
	var out domain.FreshnessPolicy
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		b, err := q.CurrentFreshnessPolicy(ctx, string(areaID))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &out.MaxAgeDays); err != nil {
			return fmt.Errorf("store: decoding freshness policy: %w", err)
		}
		return out.Validate()
	})
	return out, err
}

// PutFreshnessPolicy records a new freshness policy for an area. Policies
// are append-only like support configurations: the newest applies. Changing
// it flags records stale or not; it changes no record.
func (p *Pool) PutFreshnessPolicy(ctx context.Context, caller Caller, areaID domain.ID, policy domain.FreshnessPolicy) error {
	if err := areaID.Validate(); err != nil {
		return fmt.Errorf("area id: %w", err)
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	days := policy.MaxAgeDays
	if days == nil {
		days = map[domain.EpistemicKind]int{}
	}
	b, err := json.Marshal(days)
	if err != nil {
		return err
	}
	return p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		return q.InsertFreshnessPolicy(ctx, gen.InsertFreshnessPolicyParams{
			AreaID: string(areaID), MaxAgeDays: b, ActorUserID: optional(caller.UserID), RecordedAt: p.now().UTC(),
		})
	})
}
