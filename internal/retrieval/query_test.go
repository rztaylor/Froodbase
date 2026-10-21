package retrieval

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
)

func TestQueryOf(t *testing.T) {
	at := time.Date(2026, 9, 3, 1, 0, 0, 0, time.FixedZone("BST", 3600))
	for _, c := range []struct {
		name string
		mode string
		asOf *time.Time
		want domain.Query
		says string
	}{
		{"default is current", "", nil, domain.CurrentQuery(), ""},
		{"current", "current", nil, domain.CurrentQuery(), ""},
		{"historical, spaces ignored", " historical ", nil, domain.HistoricalQuery(), ""},
		{"as_of in UTC", "as_of", &at, domain.AsOfQuery(at.UTC()), ""},
		{"as_of without a time", "as_of", nil, domain.Query{}, "needs as_of"},
		{"a time without as_of", "", &at, domain.Query{}, "set mode to as_of"},
		{"a time with historical", "historical", &at, domain.Query{}, "mode is historical"},
		{"unknown mode", "latest", nil, domain.Query{}, "not current, as_of or historical"},
	} {
		got, err := queryOf(c.mode, c.asOf)
		if c.says != "" {
			if !errors.Is(err, ErrInvalidQuery) || !strings.Contains(err.Error(), c.says) {
				t.Errorf("%s: err = %v, want ErrInvalidQuery saying %q", c.name, err, c.says)
			}
			continue
		}
		if err != nil || got != c.want || (got.Mode == domain.ModeAsOf && got.AsOf.Location() != time.UTC) {
			t.Errorf("%s: queryOf = %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
}

func TestNotValidReason(t *testing.T) {
	d := func(day int) *time.Time { t := time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC); return &t }
	for _, c := range []struct {
		name     string
		state    string
		from, to *time.Time
		at       time.Time
		says     string
	}{
		{"retracted", "retracted", nil, nil, *d(5), "retracted"},
		{"window after the time", "active", d(2), d(9), *d(1), "valid from 2026-09-02T00:00:00Z until 2026-09-09T00:00:00Z"},
		{"starts after the time", "active", d(9), nil, *d(1), "valid only from 2026-09-09"},
		{"ended before the time", "superseded", d(2), d(9), *d(20), "ended at 2026-09-09"},
	} {
		if got := notValidReason(c.state, c.from, c.to, c.at); !strings.Contains(got, c.says) {
			t.Errorf("%s: reason %q, want it to contain %q", c.name, got, c.says)
		}
	}
}
