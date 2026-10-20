package store

import "time"

// SetFailAt makes the next writes fail at the named step ("revision",
// "audit", "job" or "commit") so tests can prove a failure anywhere leaves
// nothing behind. nil clears it.
func (p *Pool) SetFailAt(f func(step string) error) { p.failAt = f }

// SetClock replaces the writer's clock.
func (p *Pool) SetClock(now func() time.Time) { p.now = now }
