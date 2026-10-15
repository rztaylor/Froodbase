package domain

import "strings"

// Area is a knowledge area: the unit that partitions records for
// authorisation. Every record belongs to exactly one area (TR-27); the
// (area, role) grants in internal/authz refer to areas by Key.
type Area struct {
	ID   ID
	Key  string // short handle used in grants, such as "support"
	Name string // display name
}

// Validate checks that the area has an identifier and a key.
func (a Area) Validate() error {
	if err := a.ID.Validate(); err != nil {
		return fieldErr("area id", err)
	}
	if strings.TrimSpace(a.Key) == "" {
		return fieldErr("area key", ErrEmptyName)
	}
	return nil
}
