package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/store/gen"
)

// Caller is who a transaction runs for: the human user, the agent acting
// for them, and the ids of the knowledge areas they hold a grant in. The
// areas become the app.caller_areas session variable for the transaction,
// which every row-level-security policy compares against (TR-71). It is
// built by the API from the authorisation context; the store never decides
// who holds what.
type Caller struct {
	UserID  string
	AgentID string
	AreaIDs []string
}

// SessionVariable is the PostgreSQL setting the policies read.
const SessionVariable = "app.caller_areas"

func (c Caller) validate() error {
	if strings.TrimSpace(c.UserID) == "" {
		return ErrNoCaller
	}
	return nil
}

func (c Caller) hasArea(id string) bool {
	for _, a := range c.AreaIDs {
		if a == id {
			return true
		}
	}
	return false
}

func (c Caller) agentID() *string {
	if c.AgentID == "" {
		return nil
	}
	a := c.AgentID
	return &a
}

// setCallerAreas sets the session variable for the rest of tx. set_config
// with is_local = true scopes it to the transaction, so a pooled connection
// carries nothing over to its next user.
func setCallerAreas(ctx context.Context, tx pgx.Tx, areas []string) error {
	if _, err := tx.Exec(ctx, "select set_config($1, $2, true)", SessionVariable, strings.Join(areas, ",")); err != nil {
		return fmt.Errorf("store: setting caller areas: %w", err)
	}
	return nil
}

// inTx runs fn inside a transaction with the caller's areas set, committing
// when fn returns nil and rolling back otherwise.
func (p *Pool) inTx(ctx context.Context, caller Caller, fn func(ctx context.Context, tx pgx.Tx, q *gen.Queries) error) error {
	if err := caller.validate(); err != nil {
		return err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := setCallerAreas(ctx, tx, caller.AreaIDs); err != nil {
		return err
	}
	if err := fn(ctx, tx, gen.New(tx)); err != nil {
		return mapErr(err)
	}
	if err := p.hook("commit"); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: committing: %w", err)
	}
	return nil
}

// AreaIDs maps grant keys (how the users file and later Entra groups name
// areas) to area ids, for building a Caller. Keys with no area are absent
// from the result. It runs through app_area_ids, which reads past the policy
// on knowledge_area for exactly this purpose.
func (p *Pool) AreaIDs(ctx context.Context, keys []string) (map[string]string, error) {
	rows, err := p.pool.Query(ctx, "select key, id from app_area_ids($1::text[])", keys)
	if err != nil {
		return nil, fmt.Errorf("store: resolving area keys: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string, len(keys))
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("store: resolving area keys: %w", err)
		}
		out[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: resolving area keys: %w", err)
	}
	return out, nil
}

// hook lets tests inject a failure at a named step of a write; nil otherwise.
func (p *Pool) hook(step string) error {
	if p.failAt == nil {
		return nil
	}
	return p.failAt(step)
}
