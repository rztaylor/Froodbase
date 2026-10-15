package domain

import (
	"errors"
	"testing"
	"time"
)

func zeroTime() time.Time { return time.Time{} }

func TestContributionValidate(t *testing.T) {
	ok := Contribution{ID: "c1", ActorUserID: "u1", ActorAgentID: "claude", Client: "froodbase-mcp/0.1", InvestigationRef: "INC-1", RecordedAt: t1}
	tests := []struct {
		name   string
		mutate func(*Contribution)
		want   error
	}{
		{"ok", func(*Contribution) {}, nil},
		{"no agent is fine", func(c *Contribution) { c.ActorAgentID = "" }, nil},
		{"no investigation ref is fine", func(c *Contribution) { c.InvestigationRef = "" }, nil},
		{"empty id", func(c *Contribution) { c.ID = "" }, ErrEmptyID},
		{"no human actor", func(c *Contribution) { c.ActorUserID = "" }, ErrEmptyID},
		{"no recorded_at", func(c *Contribution) { c.RecordedAt = zeroTime() }, ErrNoRecordedAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ok
			tt.mutate(&c)
			if err := c.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
