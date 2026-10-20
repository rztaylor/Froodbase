package wire

import (
	"encoding/json"
	"time"
)

// Scope is where a fact applies; an empty field means unspecified (TR-26).
type Scope struct {
	Environment string `json:"environment,omitempty"`
	Region      string `json:"region,omitempty"`
	VersionMin  string `json:"version_min,omitempty"`
	VersionMax  string `json:"version_max,omitempty"`
	Cluster     string `json:"cluster,omitempty"`
	Tenant      string `json:"tenant,omitempty"`
}

// IsZero reports whether every dimension is unspecified.
func (s Scope) IsZero() bool { return s == Scope{} }

// Name is one name of an entity, optionally limited to a scope and a
// validity window.
type Name struct {
	Name      string     `json:"name"`
	Scope     *Scope     `json:"scope,omitempty"`
	ValidFrom *time.Time `json:"valid_from,omitempty"`
	ValidTo   *time.Time `json:"valid_to,omitempty"`
}

// EntityInput is an entity to create. Ref is a name local to the call that
// observations and the result use to refer to it.
type EntityInput struct {
	Ref   string `json:"ref"`
	Type  string `json:"type"`
	Names []Name `json:"names"`
}

// EvidenceInput is a citation of a shareable source to record. Root, when
// set, is the ref or id of the original observation this evidence restates,
// so corroboration counts it once (PR-12).
type EvidenceInput struct {
	Ref            string     `json:"ref"`
	SourceKind     string     `json:"source_kind"`
	SourceURI      string     `json:"source_uri"`
	SourceRevision string     `json:"source_revision"`
	Locator        string     `json:"locator,omitempty"`
	Excerpt        string     `json:"excerpt,omitempty"`
	AccessClass    string     `json:"access_class"`
	Root           string     `json:"root,omitempty"`
	ObservedAt     *time.Time `json:"observed_at,omitempty"`
}

// ObservationInput is an observation to record about a subject entity
// (a ref from this call or the id of an existing entity in the area), with
// the refs or ids of the evidence that supports it. Validity dates are only
// set when the contributor knows them; they are never inferred (TR-32).
type ObservationInput struct {
	Ref          string     `json:"ref"`
	Subject      string     `json:"subject"`
	BodyMarkdown string     `json:"body_markdown"`
	Scope        *Scope     `json:"scope,omitempty"`
	ValidFrom    *time.Time `json:"valid_from,omitempty"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	Evidence     []string   `json:"evidence"`
}

// ContributeInput is one atomic change set for one knowledge area (TR-60).
type ContributeInput struct {
	Area             string             `json:"area"`
	IdempotencyKey   string             `json:"idempotency_key"`
	InvestigationRef string             `json:"investigation_ref,omitempty"`
	Rationale        string             `json:"rationale,omitempty"`
	Entities         []EntityInput      `json:"entities,omitempty"`
	Evidence         []EvidenceInput    `json:"evidence,omitempty"`
	Observations     []ObservationInput `json:"observations,omitempty"`
}

// Support is a claim's earned support (PR-15, TR-14): the score, the
// configuration version it was computed under, the counts behind it and
// whether it meets the area's threshold for the claim's kind.
type Support struct {
	Score          float64 `json:"score"`
	ConfigVersion  string  `json:"config_version"`
	Roots          int     `json:"roots"`
	Verifications  int     `json:"verifications"`
	Outcomes       int     `json:"outcomes"`
	Events         int     `json:"events"`
	MeetsThreshold bool    `json:"meets_threshold"`
}

// Written is one record a contribution wrote, with what an agent needs to
// list it in a report and a person needs to inspect or retract it (TR-60).
type Written struct {
	Ref            string   `json:"ref,omitempty"`
	ID             string   `json:"id"`
	RecordType     string   `json:"record_type"`
	Kind           string   `json:"kind,omitempty"`
	LifecycleState string   `json:"lifecycle_state,omitempty"`
	Support        *Support `json:"support,omitempty"`
}

// ContributeOutput is the result of contribute.
type ContributeOutput struct {
	ContributionID string    `json:"contribution_id"`
	Area           string    `json:"area"`
	ConfigVersion  string    `json:"config_version"`
	Replayed       bool      `json:"replayed"`
	Records        []Written `json:"records"`
}

// GetRecordInput asks for one record by id.
type GetRecordInput struct {
	ID string `json:"id"`
}

// Citation is evidence as cited by a claim (PR-11, TR-24).
type Citation struct {
	EvidenceID     string     `json:"evidence_id"`
	SourceKind     string     `json:"source_kind"`
	SourceURI      string     `json:"source_uri"`
	SourceRevision string     `json:"source_revision"`
	Locator        string     `json:"locator,omitempty"`
	Excerpt        string     `json:"excerpt,omitempty"`
	AccessClass    string     `json:"access_class"`
	RootEvidenceID string     `json:"root_evidence_id,omitempty"`
	ObservedAt     *time.Time `json:"observed_at,omitempty"`
	RecordedAt     time.Time  `json:"recorded_at"`
}

// EntityView is the entity-specific part of a record.
type EntityView struct {
	Type  string `json:"type"`
	Names []Name `json:"names"`
}

// Record is one record as every tool returns it (TR-14). Fields that do
// not apply to the record type are omitted.
type Record struct {
	ID                   string      `json:"id"`
	RecordType           string      `json:"record_type"`
	Area                 string      `json:"area"`
	AreaID               string      `json:"area_id"`
	Kind                 string      `json:"kind,omitempty"`
	LifecycleState       string      `json:"lifecycle_state"`
	Revision             int         `json:"revision"`
	Support              *Support    `json:"support,omitempty"`
	Scope                *Scope      `json:"scope,omitempty"`
	ValidFrom            *time.Time  `json:"valid_from,omitempty"`
	ValidTo              *time.Time  `json:"valid_to,omitempty"`
	ValidityUnknown      bool        `json:"validity_unknown,omitempty"`
	RecordedAt           time.Time   `json:"recorded_at"`
	VerifiedAt           *time.Time  `json:"verified_at,omitempty"`
	ReviewDueAt          *time.Time  `json:"review_due_at,omitempty"`
	NeedsReconsideration bool        `json:"needs_reconsideration,omitempty"`
	Subject              string      `json:"subject,omitempty"`
	BodyMarkdown         string      `json:"body_markdown,omitempty"`
	Citations            []Citation  `json:"citations,omitempty"`
	Entity               *EntityView `json:"entity,omitempty"`
	Evidence             *Citation   `json:"evidence,omitempty"`
}

// GetHistoryInput asks for a record's history by id.
type GetHistoryInput struct {
	ID string `json:"id"`
}

// Revision is one content revision of a record (TR-35).
type Revision struct {
	Revision     int             `json:"revision"`
	ActorUserID  string          `json:"actor_user_id"`
	ActorAgentID string          `json:"actor_agent_id,omitempty"`
	Rationale    string          `json:"rationale,omitempty"`
	RecordedAt   time.Time       `json:"recorded_at"`
	Snapshot     json.RawMessage `json:"snapshot"`
}

// SupportEvent is one entry of a claim's support history (TR-29).
type SupportEvent struct {
	Type            string    `json:"type"`
	EvidenceRootIDs []string  `json:"evidence_root_ids,omitempty"`
	Scope           *Scope    `json:"scope,omitempty"`
	ActorUserID     string    `json:"actor_user_id,omitempty"`
	ActorAgentID    string    `json:"actor_agent_id,omitempty"`
	JobName         string    `json:"job_name,omitempty"`
	ConfigVersion   string    `json:"config_version"`
	Rationale       string    `json:"rationale,omitempty"`
	At              time.Time `json:"at"`
}

// HistoryOutput is a record's revisions and, for a claim, its support events.
type HistoryOutput struct {
	ID            string         `json:"id"`
	RecordType    string         `json:"record_type"`
	Area          string         `json:"area"`
	Revisions     []Revision     `json:"revisions"`
	SupportEvents []SupportEvent `json:"support_events,omitempty"`
}

// SearchInput is a search over the caller's knowledge areas (TR-50). Area,
// when set, limits the search to one area key. Modes and scope filters
// arrive with slice 2.
type SearchInput struct {
	Query string `json:"query"`
	Area  string `json:"area,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// Channel names of search results.
const (
	ChannelIdentifier = "identifier"
	ChannelFullText   = "full_text"
)

// Match says how a result matched: the channel, the name it matched on for
// the identifier channel with its similarity, and whether it is a near-miss
// candidate the caller must confirm rather than an exact match (D13).
type Match struct {
	Channel    string  `json:"channel"`
	Name       string  `json:"name,omitempty"`
	Similarity float64 `json:"similarity,omitempty"`
	Candidate  bool    `json:"candidate"`
	Rank       float64 `json:"rank,omitempty"`
}

// SearchResult is one record with how it matched.
type SearchResult struct {
	Record Record `json:"record"`
	Match  Match  `json:"match"`
}

// SearchOutput lists results, identifier hits first. It carries no total
// and nothing about records the caller may not read (PR-62).
type SearchOutput struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}
