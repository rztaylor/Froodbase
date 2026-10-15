package domain

import (
	"errors"
	"testing"
	"time"
)

var (
	t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
)

func ptr(t time.Time) *time.Time { return &t }

func TestEntityNameValidAt(t *testing.T) {
	tests := []struct {
		name string
		n    EntityName
		at   time.Time
		want bool
	}{
		{"open both ends", EntityName{Name: "api"}, t1, true},
		{"from only, before", EntityName{Name: "api", ValidFrom: ptr(t1)}, t0, false},
		{"from only, at start", EntityName{Name: "api", ValidFrom: ptr(t1)}, t1, true},
		{"from only, after", EntityName{Name: "api", ValidFrom: ptr(t1)}, t2, true},
		{"to only, before end", EntityName{Name: "api", ValidTo: ptr(t1)}, t0, true},
		{"to only, at end is excluded", EntityName{Name: "api", ValidTo: ptr(t1)}, t1, false},
		{"window, inside", EntityName{Name: "api", ValidFrom: ptr(t0), ValidTo: ptr(t2)}, t1, true},
		{"window, after", EntityName{Name: "api", ValidFrom: ptr(t0), ValidTo: ptr(t1)}, t2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.n.ValidAt(tt.at); got != tt.want {
				t.Errorf("ValidAt(%v) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestEntityNameValidate(t *testing.T) {
	tests := []struct {
		name string
		n    EntityName
		want error
	}{
		{"ok open", EntityName{Name: "api"}, nil},
		{"ok window", EntityName{Name: "api", ValidFrom: ptr(t0), ValidTo: ptr(t1)}, nil},
		{"empty name", EntityName{}, ErrEmptyName},
		{"blank name", EntityName{Name: "  "}, ErrEmptyName},
		{"from after to", EntityName{Name: "api", ValidFrom: ptr(t1), ValidTo: ptr(t0)}, ErrInvalidValidity},
		{"from equals to", EntityName{Name: "api", ValidFrom: ptr(t1), ValidTo: ptr(t1)}, ErrInvalidValidity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.n.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseEntityType(t *testing.T) {
	for _, s := range []string{"system", "service", "component", "team", "environment", "incident"} {
		if _, err := ParseEntityType(s); err != nil {
			t.Errorf("ParseEntityType(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "Service", "person"} {
		if _, err := ParseEntityType(s); !errors.Is(err, ErrUnknownEntityType) {
			t.Errorf("ParseEntityType(%q) = %v, want ErrUnknownEntityType", s, err)
		}
	}
}

func TestNewEntity(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		area ID
		typ  EntityType
		n    EntityName
		want error
	}{
		{"ok", "e1", "a1", EntityService, EntityName{Name: "api"}, nil},
		{"empty id", "", "a1", EntityService, EntityName{Name: "api"}, ErrEmptyID},
		{"empty area", "e1", "", EntityService, EntityName{Name: "api"}, ErrEmptyID},
		{"unknown type", "e1", "a1", "person", EntityName{Name: "api"}, ErrUnknownEntityType},
		{"bad name", "e1", "a1", EntityService, EntityName{}, ErrEmptyName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewEntity(tt.id, tt.area, tt.typ, tt.n)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewEntity() error = %v, want %v", err, tt.want)
			}
			if err == nil && e.ID() != tt.id {
				t.Errorf("ID() = %q, want %q", e.ID(), tt.id)
			}
		})
	}
}

// Names are not identifiers (TR-20): renaming adds a name and leaves the id alone.
func TestEntityNamesAt(t *testing.T) {
	e, err := NewEntity("e1", "a1", EntitySystem, EntityName{Name: "old", ValidTo: ptr(t1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddName(EntityName{Name: "new", ValidFrom: ptr(t1)}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddName(EntityName{Name: "alias"}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddName(EntityName{}); !errors.Is(err, ErrEmptyName) {
		t.Errorf("AddName(empty) = %v, want ErrEmptyName", err)
	}
	tests := []struct {
		at   time.Time
		want []string
	}{
		{t0, []string{"old", "alias"}},
		{t1, []string{"new", "alias"}},
		{t2, []string{"new", "alias"}},
	}
	for _, tt := range tests {
		got := e.NamesAt(tt.at)
		if len(got) != len(tt.want) {
			t.Errorf("NamesAt(%v) = %v, want %v", tt.at, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("NamesAt(%v) = %v, want %v", tt.at, got, tt.want)
				break
			}
		}
	}
	if e.ID() != "e1" {
		t.Errorf("ID changed to %q after renames", e.ID())
	}
}

func TestAreaValidate(t *testing.T) {
	tests := []struct {
		name string
		a    Area
		want error
	}{
		{"ok", Area{ID: "a1", Key: "support", Name: "Support"}, nil},
		{"empty id", Area{Key: "support", Name: "Support"}, ErrEmptyID},
		{"empty key", Area{ID: "a1", Name: "Support"}, ErrEmptyName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.a.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
