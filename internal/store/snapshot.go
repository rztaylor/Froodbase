package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
)

// Snapshots are the JSONB bodies of revision rows (TR-35): the whole record
// as it was, in a shape the store controls, so get_history and as-of reads
// never depend on the live tables. Field names are stable; adding a field
// is fine, renaming one is a migration of the stored snapshots.

// LifecycleActive is the state every record enters (TR-40). The lifecycle
// state machine arrives in a later slice; until then records are active.
const LifecycleActive = "active"

// The record types named in revision, support_event and audit_event rows.
const (
	RecordEntity    = "entity"
	RecordAssertion = "assertion"
	RecordEvidence  = "evidence"
)

type scopeJSON struct {
	Environment string `json:"environment,omitempty"`
	Region      string `json:"region,omitempty"`
	VersionMin  string `json:"version_min,omitempty"`
	VersionMax  string `json:"version_max,omitempty"`
	Cluster     string `json:"cluster,omitempty"`
	Tenant      string `json:"tenant,omitempty"`
}

func scopeToJSON(s domain.Scope) scopeJSON {
	return scopeJSON{Environment: s.Environment, Region: s.Region, VersionMin: s.VersionMin, VersionMax: s.VersionMax, Cluster: s.Cluster, Tenant: s.Tenant}
}

func (s scopeJSON) domain() domain.Scope {
	return domain.Scope{Environment: s.Environment, Region: s.Region, VersionMin: s.VersionMin, VersionMax: s.VersionMax, Cluster: s.Cluster, Tenant: s.Tenant}
}

// scopeBytes is the JSONB column value of a scope: fixed keys, unset
// dimensions omitted, so an unspecified scope is {} (TR-26).
func scopeBytes(s domain.Scope) []byte {
	b, _ := json.Marshal(scopeToJSON(s)) // a struct of strings cannot fail
	return b
}

func scopeFromBytes(b []byte) (domain.Scope, error) {
	var s scopeJSON
	if len(b) == 0 {
		return domain.Scope{}, nil
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return domain.Scope{}, fmt.Errorf("store: decoding scope: %w", err)
	}
	return s.domain(), nil
}

type nameSnapshot struct {
	Name      string     `json:"name"`
	Scope     scopeJSON  `json:"scope"`
	ValidFrom *time.Time `json:"valid_from,omitempty"`
	ValidTo   *time.Time `json:"valid_to,omitempty"`
}

type entitySnapshot struct {
	ID             string         `json:"id"`
	AreaID         string         `json:"area_id"`
	EntityType     string         `json:"entity_type"`
	Names          []nameSnapshot `json:"names"`
	LifecycleState string         `json:"lifecycle_state"`
	Revision       int            `json:"revision"`
}

// EntityRecord is an entity as stored: the domain record plus the store's
// own fields.
type EntityRecord struct {
	Entity         *domain.Entity
	LifecycleState string
	Revision       int
}

func entitySnapshotBytes(r EntityRecord) ([]byte, error) {
	s := entitySnapshot{
		ID:             string(r.Entity.ID()),
		AreaID:         string(r.Entity.AreaID),
		EntityType:     string(r.Entity.Type),
		Names:          make([]nameSnapshot, 0, len(r.Entity.Names)),
		LifecycleState: r.LifecycleState,
		Revision:       r.Revision,
	}
	for _, n := range r.Entity.Names {
		s.Names = append(s.Names, nameSnapshot{Name: n.Name, Scope: scopeToJSON(n.Scope), ValidFrom: n.ValidFrom, ValidTo: n.ValidTo})
	}
	return json.Marshal(s)
}

// EntityFromSnapshot decodes a revision snapshot of an entity, for history
// and as-of reads.
func EntityFromSnapshot(b []byte) (EntityRecord, error) {
	var s entitySnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return EntityRecord{}, fmt.Errorf("store: decoding entity snapshot: %w", err)
	}
	if len(s.Names) == 0 {
		return EntityRecord{}, fmt.Errorf("store: decoding entity snapshot: %w", domain.ErrEmptyName)
	}
	first := s.Names[0]
	e, err := domain.NewEntity(domain.ID(s.ID), domain.ID(s.AreaID), domain.EntityType(s.EntityType),
		domain.EntityName{Name: first.Name, Scope: first.Scope.domain(), ValidFrom: first.ValidFrom, ValidTo: first.ValidTo})
	if err != nil {
		return EntityRecord{}, fmt.Errorf("store: decoding entity snapshot: %w", err)
	}
	for _, n := range s.Names[1:] {
		if err := e.AddName(domain.EntityName{Name: n.Name, Scope: n.Scope.domain(), ValidFrom: n.ValidFrom, ValidTo: n.ValidTo}); err != nil {
			return EntityRecord{}, fmt.Errorf("store: decoding entity snapshot: %w", err)
		}
	}
	return EntityRecord{Entity: e, LifecycleState: s.LifecycleState, Revision: s.Revision}, nil
}

type assertionSnapshot struct {
	ID              string     `json:"id"`
	AreaID          string     `json:"area_id"`
	SubjectEntityID string     `json:"subject_entity_id"`
	EpistemicKind   string     `json:"epistemic_kind"`
	BodyMarkdown    string     `json:"body_markdown"`
	Scope           scopeJSON  `json:"scope"`
	ValidFrom       *time.Time `json:"valid_from,omitempty"`
	ValidTo         *time.Time `json:"valid_to,omitempty"`
	RecordedAt      time.Time  `json:"recorded_at"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	ReviewDueAt     *time.Time `json:"review_due_at,omitempty"`
	LifecycleState  string     `json:"lifecycle_state"`
	Revision        int        `json:"revision"`
}

// AssertionRecord is an assertion as stored: the domain record, its
// lifecycle state, revision, and its support score with the configuration
// version the score was projected under (TR-43).
type AssertionRecord struct {
	Assertion            domain.Assertion
	LifecycleState       string
	Revision             int
	SupportScore         float64
	SupportConfigVersion string
	NeedsReconsideration bool
}

// assertionSnapshotBytes holds the record's content and lifecycle. Support
// is deliberately absent: it is a projection of the claim's support events,
// which have their own append-only history (TR-29).
func assertionSnapshotBytes(r AssertionRecord) ([]byte, error) {
	a := r.Assertion
	return json.Marshal(assertionSnapshot{
		ID: string(a.ID), AreaID: string(a.AreaID), SubjectEntityID: string(a.SubjectEntityID),
		EpistemicKind: string(a.Kind), BodyMarkdown: a.BodyMarkdown, Scope: scopeToJSON(a.Scope),
		ValidFrom: a.ValidFrom, ValidTo: a.ValidTo, RecordedAt: a.RecordedAt, VerifiedAt: a.VerifiedAt, ReviewDueAt: a.ReviewDueAt,
		LifecycleState: r.LifecycleState, Revision: r.Revision,
	})
}

// AssertionFromSnapshot decodes a revision snapshot of an assertion. The
// support fields are zero: support is read from the support events.
func AssertionFromSnapshot(b []byte) (AssertionRecord, error) {
	var s assertionSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return AssertionRecord{}, fmt.Errorf("store: decoding assertion snapshot: %w", err)
	}
	a := domain.Assertion{
		ID: domain.ID(s.ID), AreaID: domain.ID(s.AreaID), SubjectEntityID: domain.ID(s.SubjectEntityID),
		Kind: domain.EpistemicKind(s.EpistemicKind), BodyMarkdown: s.BodyMarkdown, Scope: s.Scope.domain(),
		Temporal: domain.Temporal{ValidFrom: s.ValidFrom, ValidTo: s.ValidTo, RecordedAt: s.RecordedAt, VerifiedAt: s.VerifiedAt, ReviewDueAt: s.ReviewDueAt},
	}
	if err := a.Validate(); err != nil {
		return AssertionRecord{}, fmt.Errorf("store: decoding assertion snapshot: %w", err)
	}
	return AssertionRecord{Assertion: a, LifecycleState: s.LifecycleState, Revision: s.Revision}, nil
}

type evidenceSnapshot struct {
	ID             string     `json:"id"`
	AreaID         string     `json:"area_id"`
	SourceKind     string     `json:"source_kind"`
	SourceURI      string     `json:"source_uri"`
	SourceRevision string     `json:"source_revision"`
	SourceLocator  string     `json:"source_locator,omitempty"`
	Excerpt        string     `json:"excerpt,omitempty"`
	AccessClass    string     `json:"access_class"`
	RootEvidenceID string     `json:"root_evidence_id,omitempty"`
	ObservedAt     *time.Time `json:"observed_at,omitempty"`
	RecordedAt     time.Time  `json:"recorded_at"`
	Revision       int        `json:"revision"`
}

func evidenceSnapshotBytes(e domain.Evidence) ([]byte, error) {
	return json.Marshal(evidenceSnapshot{
		ID: string(e.ID), AreaID: string(e.AreaID), SourceKind: string(e.SourceKind), SourceURI: e.SourceURI,
		SourceRevision: e.SourceRevision, SourceLocator: e.SourceLocator, Excerpt: e.Excerpt, AccessClass: string(e.AccessClass),
		RootEvidenceID: string(e.RootEvidenceID), ObservedAt: e.ObservedAt, RecordedAt: e.RecordedAt, Revision: 1,
	})
}

// EvidenceFromSnapshot decodes a revision snapshot of an evidence record.
func EvidenceFromSnapshot(b []byte) (domain.Evidence, error) {
	var s evidenceSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return domain.Evidence{}, fmt.Errorf("store: decoding evidence snapshot: %w", err)
	}
	e := domain.Evidence{
		ID: domain.ID(s.ID), AreaID: domain.ID(s.AreaID), SourceKind: domain.SourceKind(s.SourceKind), SourceURI: s.SourceURI,
		SourceRevision: s.SourceRevision, SourceLocator: s.SourceLocator, Excerpt: s.Excerpt, AccessClass: domain.AccessClass(s.AccessClass),
		RootEvidenceID: domain.ID(s.RootEvidenceID), ObservedAt: s.ObservedAt, RecordedAt: s.RecordedAt,
	}
	if err := e.Validate(); err != nil {
		return domain.Evidence{}, fmt.Errorf("store: decoding evidence snapshot: %w", err)
	}
	return e, nil
}

// Support configuration rows hold three JSONB documents; these are their shapes.

type allowedContributorsJSON struct {
	Classes     []string `json:"classes"`
	SourceKinds []string `json:"source_kinds"`
}

func supportConfigFromRow(version string, weights, thresholds, allowed []byte, useCap float64) (domain.SupportConfig, error) {
	cfg := domain.SupportConfig{Version: version, UseWithoutChallengeCap: useCap}
	if err := json.Unmarshal(weights, &cfg.EventWeights); err != nil {
		return domain.SupportConfig{}, fmt.Errorf("store: decoding support config weights: %w", err)
	}
	if err := json.Unmarshal(thresholds, &cfg.ThresholdByKind); err != nil {
		return domain.SupportConfig{}, fmt.Errorf("store: decoding support config thresholds: %w", err)
	}
	var ac allowedContributorsJSON
	if err := json.Unmarshal(allowed, &ac); err != nil {
		return domain.SupportConfig{}, fmt.Errorf("store: decoding support config contributors: %w", err)
	}
	cfg.AllowedContributors.Classes = ac.Classes
	for _, k := range ac.SourceKinds {
		cfg.AllowedContributors.SourceKinds = append(cfg.AllowedContributors.SourceKinds, domain.SourceKind(k))
	}
	if err := cfg.Validate(); err != nil {
		return domain.SupportConfig{}, fmt.Errorf("store: support config %q: %w", version, err)
	}
	return cfg, nil
}

func supportConfigToRow(cfg domain.SupportConfig) (weights, thresholds, allowed []byte, err error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, nil, err
	}
	if weights, err = json.Marshal(cfg.EventWeights); err != nil {
		return nil, nil, nil, err
	}
	if thresholds, err = json.Marshal(cfg.ThresholdByKind); err != nil {
		return nil, nil, nil, err
	}
	ac := allowedContributorsJSON{Classes: cfg.AllowedContributors.Classes, SourceKinds: make([]string, 0, len(cfg.AllowedContributors.SourceKinds))}
	for _, k := range cfg.AllowedContributors.SourceKinds {
		ac.SourceKinds = append(ac.SourceKinds, string(k))
	}
	if allowed, err = json.Marshal(ac); err != nil {
		return nil, nil, nil, err
	}
	return weights, thresholds, allowed, nil
}
