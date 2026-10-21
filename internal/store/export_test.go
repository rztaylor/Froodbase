package store

import (
	"context"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
)

// SetFailAt makes the next writes fail at the named step ("revision",
// "audit", "job" or "commit") so tests can prove a failure anywhere leaves
// nothing behind. nil clears it.
func (p *Pool) SetFailAt(f func(step string) error) { p.failAt = f }

// SetClock replaces the writer's clock.
func (p *Pool) SetClock(now func() time.Time) { p.now = now }

// MigrateTo applies River's schema and the goose migrations up to and
// including version, so a test can seed data the way an older release left
// it before applying the rest.
func (p *Pool) MigrateTo(ctx context.Context, version int64) error {
	if _, err := p.migrateRiver(ctx); err != nil {
		return err
	}
	provider, db, err := p.provider()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	_, err = provider.UpTo(ctx, version)
	return err
}

// ScopeBytes is the stored JSONB form of a scope.
func ScopeBytes(s domain.Scope) []byte { return scopeBytes(s) }
