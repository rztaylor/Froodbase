package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ID is the stable, opaque identifier of a record (PR-06). The store assigns
// it (a UUID in text form); the domain only requires it to be present.
type ID string

// Validate reports ErrEmptyID when the identifier is blank.
func (id ID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptyID
	}
	return nil
}

// Sentinel errors returned by validation. Callers match them with errors.Is.
var (
	// ErrEmptyID means a required identifier is blank.
	ErrEmptyID = errors.New("domain: identifier is empty")
	// ErrEmptyName means a name or key is blank.
	ErrEmptyName = errors.New("domain: name is empty")
	// ErrEmptyBody means an assertion has no Markdown body.
	ErrEmptyBody = errors.New("domain: body is empty")
	// ErrNoRecordedAt means a record has no recording time; recorded_at is
	// system time and never null (TR-30).
	ErrNoRecordedAt = errors.New("domain: recorded_at is not set")
	// ErrInvalidValidity means valid_from is not before valid_to.
	ErrInvalidValidity = errors.New("domain: valid_from must be before valid_to")
	// ErrUnknownEntityType means the entity type is not one of the documented types.
	ErrUnknownEntityType = errors.New("domain: unknown entity type")
	// ErrUnknownEpistemicKind means the kind is not observation, decision or inference.
	ErrUnknownEpistemicKind = errors.New("domain: unknown epistemic kind")
	// ErrNoEvidence means an observation cites no evidence of its own (PR-43).
	ErrNoEvidence = errors.New("domain: an observation must cite evidence of its own")
	// ErrObservationDerived means an observation names records it was
	// derived from; what was seen is not deduced, so record an inference.
	ErrObservationDerived = errors.New("domain: an observation is not derived from other records; record an inference or a decision")
	// ErrNoGrounds means a decision or inference neither cites evidence nor
	// names a record it was derived from.
	ErrNoGrounds = errors.New("domain: a decision or inference must cite evidence, name the records it was derived from, or both")
	// ErrDerivationCycle means a record is derived, directly or through
	// others, from itself.
	ErrDerivationCycle = errors.New("domain: derivation cycle")
	// ErrUnknownDerived means a derivation link names a derived record that
	// is not among the records being written.
	ErrUnknownDerived = errors.New("domain: derivation names a derived record that is not being written")
	// ErrUnknownSourceKind means the evidence source kind is not a documented shared kind (TR-61).
	ErrUnknownSourceKind = errors.New("domain: unknown source kind")
	// ErrUnknownAccessClass means the access class is not restricted, area or organisation.
	ErrUnknownAccessClass = errors.New("domain: unknown access class")
	// ErrLocalSource means the evidence URI points at a local file or the
	// contributor's own machine, which the audience cannot open (PR-11).
	ErrLocalSource = errors.New("domain: local source is not a shareable citation")
	// ErrURINotAllowed means the evidence URI does not match the allowed
	// pattern for its source kind (TR-61).
	ErrURINotAllowed = errors.New("domain: source URI does not match an allowed pattern")
	// ErrNoRevision means evidence does not say which revision or snapshot
	// of its source it cites (PR-10).
	ErrNoRevision = errors.New("domain: source revision is empty")
	// ErrAccessTooPermissive means evidence claims a wider audience than its source allows (TR-61).
	ErrAccessTooPermissive = errors.New("domain: access class is more permissive than the source's")
)

func fieldErr(field string, err error) error {
	return fmt.Errorf("%s: %w", field, err)
}
