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
// assertions and the result use to refer to it; when left out the server
// generates one (entity-1, entity-2, ...), so leave it out only when nothing
// in the call refers to the entity.
type EntityInput struct {
	Ref   string `json:"ref,omitempty"`
	Type  string `json:"type"`
	Names []Name `json:"names"`
}

// EvidenceInput is a citation, checkable by the area's audience, to record.
// Root, when set, is the ref or id of the original observation this
// evidence restates, so corroboration counts it once (PR-12).
//
// Source kinds repository, ticket, telemetry_query and document cite an
// https URL at a revision. A command observation (source_kind command) cites
// what was observed as source_uri (azure:///subscriptions/..., the ARM id;
// k8s://<cluster>/<namespace>/<kind>/<name>; sql://<server>/<database>;
// host://<name>), the exact command or query with secrets removed as
// locator, when it ran as observed_at and optionally the output relied on as
// excerpt. A statement (source_kind statement) records what the caller
// knows: source_uri person://<their user id> (filled in when empty), what
// they said as excerpt and when as observed_at. Both default
// source_revision to observed_at. Anything that looks like a secret is
// refused.
//
// The server fills in what it can tell from the rest (FRB-51): Ref as for
// entities (evidence-1, ...); SourceKind from a well-known SourceURI (work
// items, Jira and GitHub issues are ticket; GitHub, GitLab, Bitbucket and
// Azure DevOps repository URLs are repository; Confluence, SharePoint and
// Azure DevOps wiki pages are document; the command schemes are command and
// person:// is statement); AccessClass as area; and SourceRevision, for a
// repository URL that pins a full commit SHA, as that SHA. Anything else
// stays required.
type EvidenceInput struct {
	Ref            string     `json:"ref,omitempty"`
	SourceKind     string     `json:"source_kind,omitempty"`
	SourceURI      string     `json:"source_uri,omitempty"`
	SourceRevision string     `json:"source_revision,omitempty"`
	Locator        string     `json:"locator,omitempty"`
	Excerpt        string     `json:"excerpt,omitempty"`
	AccessClass    string     `json:"access_class,omitempty"`
	Root           string     `json:"root,omitempty"`
	ObservedAt     *time.Time `json:"observed_at,omitempty"`
}

// AssertionInput is a claim to record about a subject entity (a ref from
// this call or the id of an existing entity in the area). Kind is
// observation (what was seen), decision (what was approved or intended) or
// inference (what was concluded) and never changes (PR-03). Evidence lists
// the refs or ids of the evidence it cites; DerivedFrom lists the assertions
// it was derived from, as refs of assertions in this call or ids of
// assertions in the area (PR-13). An observation cites evidence and is never
// derived; a decision or inference cites evidence, is derived, or both.
// Validity dates are only set when the contributor knows them; they are
// never inferred (TR-32). Ref, when left out, is generated (assertion-1, ...).
type AssertionInput struct {
	Ref          string     `json:"ref,omitempty"`
	Kind         string     `json:"kind"`
	Subject      string     `json:"subject"`
	BodyMarkdown string     `json:"body_markdown"`
	Scope        *Scope     `json:"scope,omitempty"`
	ValidFrom    *time.Time `json:"valid_from,omitempty"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	Evidence     []string   `json:"evidence,omitempty"`
	DerivedFrom  []string   `json:"derived_from,omitempty"`
}

// ObservationInput is the slice 1 form of an observation, kept so existing
// callers keep working: it is an AssertionInput of kind observation.
type ObservationInput struct {
	Ref          string     `json:"ref,omitempty"`
	Subject      string     `json:"subject"`
	BodyMarkdown string     `json:"body_markdown"`
	Scope        *Scope     `json:"scope,omitempty"`
	ValidFrom    *time.Time `json:"valid_from,omitempty"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	Evidence     []string   `json:"evidence,omitempty"`
}

// LinkInput cites further evidence for a claim already recorded (FRB-57):
// Claim is the id of an assertion in the area, of any kind, and Evidence
// the refs of evidence in this call or ids of evidence in the area. The
// claim gains one root per distinct root it has not counted yet; evidence
// it already counts, directly, through a restatement or (for a statement)
// from the same person, adds nothing. Nothing else about the claim changes:
// no new revision, and records derived from it gain the root through the
// dreamer. Use it instead of recording the same claim again, which earns
// nothing.
type LinkInput struct {
	Claim    string   `json:"claim"`
	Evidence []string `json:"evidence"`
}

// ContributeInput is one atomic change set for one knowledge area (TR-60).
// Assertions is the general form; Observations is kept for existing callers
// and is written as assertions of kind observation, before Assertions.
// Links adds evidence to claims the area already holds; they are written
// before the new assertions, so a deduction in the same call derived from a
// linked claim inherits its new roots at once.
type ContributeInput struct {
	Area             string             `json:"area"`
	IdempotencyKey   string             `json:"idempotency_key"`
	InvestigationRef string             `json:"investigation_ref,omitempty"`
	Rationale        string             `json:"rationale,omitempty"`
	Entities         []EntityInput      `json:"entities,omitempty"`
	Evidence         []EvidenceInput    `json:"evidence,omitempty"`
	Assertions       []AssertionInput   `json:"assertions,omitempty"`
	Observations     []ObservationInput `json:"observations,omitempty"`
	Links            []LinkInput        `json:"links,omitempty"`
}

// Support is a claim's earned support (PR-15, TR-14): the score, the
// configuration version it was computed under, the counts behind it, its
// roots by source kind with the weight the configuration gives each, and
// whether it meets the area's threshold for the claim's kind.
type Support struct {
	Score             float64      `json:"score"`
	ConfigVersion     string       `json:"config_version"`
	Roots             int          `json:"roots"`
	RootsBySourceKind []RootWeight `json:"roots_by_source_kind,omitempty"`
	Verifications     int          `json:"verifications"`
	Outcomes          int          `json:"outcomes"`
	Events            int          `json:"events"`
	MeetsThreshold    bool         `json:"meets_threshold"`
}

// RootWeight is how many of a claim's roots are of one source kind and what
// each root of that kind weighs (1 counts in full). SourceKind is empty for
// roots recorded before kinds were.
type RootWeight struct {
	SourceKind string  `json:"source_kind,omitempty"`
	Roots      int     `json:"roots"`
	Weight     float64 `json:"weight"`
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

// Linked is an existing claim a contribution cited further evidence for:
// its kind, its revision (unchanged by the link), the evidence ids linked,
// how many roots that added and its support after the link.
type Linked struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Revision   int      `json:"revision"`
	Evidence   []string `json:"evidence"`
	RootsAdded int      `json:"roots_added"`
	Support    *Support `json:"support,omitempty"`
}

// ContributeOutput is the result of contribute. Records lists what the call
// wrote, Linked the existing claims it added evidence to.
type ContributeOutput struct {
	ContributionID string    `json:"contribution_id"`
	Area           string    `json:"area"`
	ConfigVersion  string    `json:"config_version"`
	Replayed       bool      `json:"replayed"`
	Records        []Written `json:"records"`
	Linked         []Linked  `json:"linked,omitempty"`
}

// GetRecordInput asks for one record by id.
type GetRecordInput struct {
	ID string `json:"id"`
}

// Citation is evidence as cited by a claim (PR-11, TR-24). For a command
// observation Locator is the command run and ObservedAt when; for a
// statement SourceURI names the person and Excerpt what they said.
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
//
// Status labels the record relative to now, whatever the mode it was found
// in (TR-53): current (in force and valid now; a missing valid_to means not
// known to have ended, never verified current), historical (superseded,
// retracted, or valid only before now) or upcoming (valid only from later).
// ValidityUnknown means nobody gave its validity dates: it is not ruled out
// at any time, and the dates were never taken from recorded_at (TR-32).
// Stale means the record is current but was last checked (verified_at, or
// recorded_at if never verified) longer ago than its area's freshness policy
// allows for its kind: it is still returned, flagged for review (TR-31).
// An entity found by name is labelled with that name's status.
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
	Status               string      `json:"status,omitempty"`
	Stale                bool        `json:"stale,omitempty"`
	NeedsReconsideration bool        `json:"needs_reconsideration,omitempty"`
	Subject              string      `json:"subject,omitempty"`
	BodyMarkdown         string      `json:"body_markdown,omitempty"`
	Derived              bool        `json:"derived,omitempty"`
	DerivedFrom          []string    `json:"derived_from,omitempty"`
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

// Derivation is a link from a deduction to an assertion it was derived
// from (PR-13).
type Derivation struct {
	SourceID   string    `json:"source_id"`
	Rationale  string    `json:"rationale,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}

// HistoryOutput is a record's revisions and, for a claim, its support events
// and the assertions it was derived from.
type HistoryOutput struct {
	ID            string         `json:"id"`
	RecordType    string         `json:"record_type"`
	Area          string         `json:"area"`
	Revisions     []Revision     `json:"revisions"`
	SupportEvents []SupportEvent `json:"support_events,omitempty"`
	Derivations   []Derivation   `json:"derivations,omitempty"`
}

// The search modes (TR-50, TR-53).
const (
	ModeCurrent    = "current"    // in force and valid now: the default
	ModeAsOf       = "as_of"      // valid at AsOf, by validity time, whenever recorded
	ModeHistorical = "historical" // everything, each labelled with its status
)

// The statuses a record is labelled with relative to now.
const (
	StatusCurrent    = "current"
	StatusHistorical = "historical"
	StatusUpcoming   = "upcoming"
)

// SearchInput is a search over the caller's knowledge areas (TR-50). Area,
// when set, limits the search to one area key. Mode is current (the
// default), as_of with AsOf, or historical. Scope, when set, keeps records
// whose scope overlaps it: an unspecified dimension on either side matches
// anything.
type SearchInput struct {
	Query string     `json:"query"`
	Area  string     `json:"area,omitempty"`
	Limit int        `json:"limit,omitempty"`
	Mode  string     `json:"mode,omitempty"`
	AsOf  *time.Time `json:"as_of,omitempty"`
	Scope *Scope     `json:"scope,omitempty"`
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

// SearchOutput lists results, identifier hits first, with the mode they
// were filtered by. It carries no total and nothing about records the caller
// may not read (PR-62).
type SearchOutput struct {
	Query   string         `json:"query"`
	Mode    string         `json:"mode"`
	AsOf    *time.Time     `json:"as_of,omitempty"`
	Results []SearchResult `json:"results"`
}

// GetAsOfInput asks for a record as valid at a time, by validity time
// (TR-34): an assertion or an entity, by id.
type GetAsOfInput struct {
	ID   string     `json:"id"`
	AsOf *time.Time `json:"as_of"`
}

// GetAsOfOutput is a record as valid at AsOf. Valid says whether it was:
// its validity includes AsOf (an unknown validity is not ruled out) and it
// was not retracted. Record is the record when it was, with an entity's
// names cut to those valid then; Reason says why not otherwise.
type GetAsOfOutput struct {
	ID     string    `json:"id"`
	AsOf   time.Time `json:"as_of"`
	Valid  bool      `json:"valid"`
	Reason string    `json:"reason,omitempty"`
	Record *Record   `json:"record,omitempty"`
}
