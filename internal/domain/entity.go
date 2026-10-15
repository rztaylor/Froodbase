package domain

import (
	"strings"
	"time"
)

// EntityType classifies an entity (PR-01).
type EntityType string

// The documented entity types.
const (
	EntitySystem      EntityType = "system"
	EntityService     EntityType = "service"
	EntityComponent   EntityType = "component"
	EntityTeam        EntityType = "team"
	EntityEnvironment EntityType = "environment"
	EntityIncident    EntityType = "incident"
)

// EntityTypes lists the documented entity types in a stable order.
func EntityTypes() []EntityType {
	return []EntityType{EntitySystem, EntityService, EntityComponent, EntityTeam, EntityEnvironment, EntityIncident}
}

// Valid reports whether t is a documented entity type.
func (t EntityType) Valid() bool {
	for _, k := range EntityTypes() {
		if k == t {
			return true
		}
	}
	return false
}

// ParseEntityType returns the entity type named by s, or ErrUnknownEntityType.
func ParseEntityType(s string) (EntityType, error) {
	t := EntityType(s)
	if !t.Valid() {
		return "", ErrUnknownEntityType
	}
	return t, nil
}

// EntityName is one name an entity carries, with the window in which it
// applied. Names are not identifiers (TR-20): an entity keeps its ID through
// every rename, and may carry several names at once (an alias, or names that
// differ by scope). A nil ValidFrom or ValidTo means no known start or end.
type EntityName struct {
	Name      string
	Scope     Scope
	ValidFrom *time.Time
	ValidTo   *time.Time
}

// Validate checks that the name is present and its window is well formed.
func (n EntityName) Validate() error {
	if strings.TrimSpace(n.Name) == "" {
		return fieldErr("entity name", ErrEmptyName)
	}
	return validateWindow(n.ValidFrom, n.ValidTo)
}

// ValidAt reports whether the name applied at t. The window is half-open:
// ValidFrom is included and ValidTo excluded.
func (n EntityName) ValidAt(t time.Time) bool {
	return windowContains(n.ValidFrom, n.ValidTo, t)
}

// Entity is a thing knowledge is about: a system, service, component, team,
// environment or incident, with a stable identity that does not change when
// its name does (PR-01, PR-06, TR-20). The identifier is fixed at
// construction and only readable afterwards.
type Entity struct {
	id     ID
	AreaID ID
	Type   EntityType
	Names  []EntityName
}

// NewEntity builds an entity in an area with its first name. It validates the
// identifiers, the type and the name.
func NewEntity(id, area ID, typ EntityType, name EntityName) (*Entity, error) {
	if err := id.Validate(); err != nil {
		return nil, fieldErr("entity id", err)
	}
	if err := area.Validate(); err != nil {
		return nil, fieldErr("area id", err)
	}
	if !typ.Valid() {
		return nil, fieldErr("entity type", ErrUnknownEntityType)
	}
	e := &Entity{id: id, AreaID: area, Type: typ}
	if err := e.AddName(name); err != nil {
		return nil, err
	}
	return e, nil
}

// ID returns the entity's immutable identifier.
func (e *Entity) ID() ID { return e.id }

// AddName records another name for the entity, validating it first. Earlier
// names are kept: renaming is an addition with validity windows, never an
// overwrite.
func (e *Entity) AddName(n EntityName) error {
	if err := n.Validate(); err != nil {
		return err
	}
	e.Names = append(e.Names, n)
	return nil
}

// NamesAt lists the names that applied at t, in the order they were added.
func (e *Entity) NamesAt(t time.Time) []string {
	var out []string
	for _, n := range e.Names {
		if n.ValidAt(t) {
			out = append(out, n.Name)
		}
	}
	return out
}
