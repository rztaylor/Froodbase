package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/wire"
)

// ServerName is the MCP implementation name reported to clients.
const ServerName = "froodbase"

// Tool names. The set is finite by design (TR-12); add a constant here when a
// slice adds a tool.
const (
	ToolWhoami     = "whoami"
	ToolContribute = "contribute"
	ToolGetRecord  = "get_record"
	ToolGetHistory = "get_history"
	ToolSearch     = "search"
	ToolGetAsOf    = "get_as_of"
)

// WhoamiOutput is the whoami tool's structured result.
type WhoamiOutput struct {
	UserID      string              `json:"user_id"`
	DisplayName string              `json:"display_name"`
	AgentID     string              `json:"agent_id,omitempty"`
	Grants      map[string][]string `json:"grants"`
}

// Unavailable is the structured result every tool returns, with IsError
// false, when froodbase-api cannot be reached from froodbase-mcp. The agent
// is expected to continue with packaged and local knowledge and to note the
// outage in its report (TR-102).
type Unavailable struct {
	FroodbaseUnavailable bool   `json:"froodbase_unavailable"`
	APIURL               string `json:"api_url"`
	Message              string `json:"message"`
	Cause                string `json:"cause,omitempty"`
}

// UnavailableMessage is the guidance carried by Unavailable.Message.
const UnavailableMessage = "Froodbase is unavailable; continue with packaged and local knowledge and record in the report that Froodbase could not be reached."

// UnavailableError is returned by a forwarding handler when the API cannot
// be reached. Tool wrappers convert it into an Unavailable result rather
// than a tool error.
type UnavailableError struct {
	APIURL string
	Err    error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("froodbase-api at %s unavailable: %v", e.APIURL, e.Err)
}

// Unwrap exposes the cause.
func (e *UnavailableError) Unwrap() error { return e.Err }

// Result converts the error to the structured result.
func (e *UnavailableError) Result() Unavailable {
	cause := ""
	if e.Err != nil {
		cause = e.Err.Error()
	}
	return Unavailable{FroodbaseUnavailable: true, APIURL: e.APIURL, Message: UnavailableMessage, Cause: cause}
}

// Handlers supplies the behaviour behind each tool. A nil handler leaves the
// tool out of the server.
type Handlers struct {
	Whoami     func(ctx context.Context) (WhoamiOutput, error)
	Contribute func(ctx context.Context, in wire.ContributeInput) (wire.ContributeOutput, error)
	GetRecord  func(ctx context.Context, in wire.GetRecordInput) (wire.Record, error)
	GetHistory func(ctx context.Context, in wire.GetHistoryInput) (wire.HistoryOutput, error)
	Search     func(ctx context.Context, in wire.SearchInput) (wire.SearchOutput, error)
	GetAsOf    func(ctx context.Context, in wire.GetAsOfInput) (wire.GetAsOfOutput, error)
}

// recordLabels is how every reading tool's description explains a record's
// temporal fields (PR-22, TR-31, TR-53).
const recordLabels = "Each record carries status relative to now: current (in force and valid now; no valid_to means not known to have ended, which is not the same as verified current), historical (superseded, retracted or valid only before now: never present it as current state) or upcoming (valid only from later). " +
	"validity_unknown: true means nobody gave its validity dates, so it is not ruled out at any time and is not verified current; verified_at says when it was last checked, and stale: true means it is current but unchecked for longer than its area's freshness policy allows: it is still returned, so say it may be out of date. " +
	"due_for_review: true means the periodic review sweep found it past its review date, and stale_at when the sweep first found it stale (review_swept_at says when its area was last swept): flags for a reviewer, never a change to the record, so report them and still use the record as its status says."

// New builds an MCP server exposing the tools whose handlers are set.
func New(version string, h Handlers) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: version}, nil)
	if h.Whoami != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        ToolWhoami,
			Description: "Report who Froodbase thinks the caller is and which knowledge areas and roles they hold.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(func(ctx context.Context, _ struct{}) (WhoamiOutput, error) { return h.Whoami(ctx) }))
	}
	if h.Contribute != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name: ToolContribute,
			Description: "Record findings in a knowledge area as one atomic change set: entities, assertions about them and the evidence behind them, which the area's audience must be able to check: an https source at a revision (repository, ticket, telemetry_query, document); " +
				"a command you ran (source_kind command: source_uri is what was observed as azure:///<ARM id>, k8s://<cluster>/<namespace>/<kind>/<name>, sql://<server>/<database> or host://<name>; locator is the exact command with secrets removed; observed_at when it ran; excerpt the output relied on; each run is its own root); " +
				"or the person you act for's own statement (source_kind statement: excerpt is what they said, observed_at when; source_uri person://<their user id> is filled in; never someone else's, and it counts for less than an observed source). Never include secrets: anything that looks like a password, token or key is refused. " +
				"Each assertion has a kind that never changes: observation (what was seen; cites evidence, never derived), decision (what was approved or intended) or inference (what was concluded). " +
				"When you conclude something from records you have read, record it as an inference (or a decision) with derived_from naming those assertions (refs in this call or ids in the area), evidence of its own, or both: it starts with the distinct evidence roots of its sources and stays labelled derived. " +
				"Deriving from another area, an unknown id or in a loop is refused. The observations list is the older form of assertions of kind observation. " +
				"Give an assertion valid_from and valid_to (RFC 3339; valid_to is exclusive) only when the source says when it held, and scope (environment, region, version_min, version_max, cluster, tenant; versions such as 2.3 or v2.4.1, both bounds for one version) when it applies only somewhere: leave out what you do not know. A missing date stays unknown and is never taken from when you record it; such a record is marked validity_unknown. " +
				"Every record is active at once with the support its distinct evidence roots give it; there is no approval step, and below the kind's threshold it is still stored and returned. Reports, conversations and transcripts are never sent: contribute individual findings with their own citations. " +
				"Leave out what the server fills in: ref on any record nothing in the call refers to; source_kind when source_uri is a work item, Jira or GitHub issue, a GitHub, GitLab, Bitbucket or Azure DevOps repository URL, a Confluence, SharePoint or Azure DevOps wiki page, or one of the command or person schemes; access_class when it is area (say restricted when the source is); source_revision for a repository URL with a full commit SHA in it, and for commands and statements. Search before creating an entity, and cite existing records by id. " +
				"To add a new source to a claim already recorded (found with search), do not record the claim again, which earns nothing: list it under links as {claim: <its id>, evidence: [<evidence refs in this call or evidence ids>]}. The claim gains one root per distinct source it did not count yet, keeps its revision, and deductions derived from it gain the root shortly after; evidence it already counts adds nothing. " +
				"Pass a fresh idempotency_key per change set; retrying with the same key returns the same result without writing again. The result lists every record written with its id, kind, lifecycle state and support, and under linked each existing claim with roots_added and its support: list those ids in your report.",
			Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		}, wrap(h.Contribute))
	}
	if h.GetRecord != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        ToolGetRecord,
			Description: "Read one record (an entity, an assertion or an evidence record) by id with its kind, lifecycle state, support, scope, validity, verification date, citations and, for a deduction, derived: true with the ids it was derived_from. Records outside your knowledge areas are reported as not found. " + recordLabels,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(h.GetRecord))
	}
	if h.Search != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name: ToolSearch,
			Description: "Search the knowledge areas you can read by an entity name (exact or near-miss; a near-miss is a candidate for you to confirm, never resolved for you) or by words in assertion bodies. " +
				"Results carry kind, lifecycle state, support, scope, validity, verification date, citations and which channel matched. Nothing is reported about records you cannot read. Optional area limits the search to one area key. " +
				"mode says which time you ask about: current (the default) keeps what is in force and valid now; as_of with as_of (RFC 3339) keeps what was valid at that time by when it applied, not when it was recorded, so a record imported later about an earlier time is included, and superseded records are kept, labelled; historical keeps everything. " +
				"Use current for what is true now, as_of for what was true then (an incident's time), historical for past cases as analogues. Optional scope (environment, region, version_min, version_max, cluster, tenant) keeps records whose scope overlaps it; a dimension either side leaves out matches anything. " + recordLabels,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(h.Search))
	}
	if h.GetAsOf != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name: ToolGetAsOf,
			Description: "Read one assertion or entity by id as valid at as_of (RFC 3339), judged by when it applied (its validity), not when it was recorded: a record recorded later but valid earlier is valid then. " +
				"valid: true returns the record (an entity with only the names valid then); valid: false gives the reason (valid only later, ended before, or retracted) and no record. A record whose validity is unknown is not ruled out at any time and comes back marked validity_unknown. " +
				"The record is what Froodbase knows now about that time; get_history shows what it held at each recording. " + recordLabels,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(h.GetAsOf))
	}
	if h.GetHistory != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        ToolGetHistory,
			Description: "Read a record's append-only history: every content revision with its actor, time and rationale, and for a claim its support events (distinct roots added, cited or inherited from its sources, verifications, outcomes, challenges) and the assertions it was derived from.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(h.GetHistory))
	}
	return s
}

// wrap adapts a typed handler to the SDK. Outputs are returned as any so that
// an UnavailableError can be reported as an Unavailable result with the same
// tool; every other error is a tool error.
func wrap[In, Out any](h func(ctx context.Context, in In) (Out, error)) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := h(ctx, in)
		var ue *UnavailableError
		if errors.As(err, &ue) {
			return nil, ue.Result(), nil
		}
		if err != nil {
			return nil, nil, err
		}
		return nil, out, nil
	}
}

// ErrNoCaller is returned when a tool runs without an authorisation context.
var ErrNoCaller = authz.ErrNoContext

// WhoamiHandlers returns handlers for whoami alone, reading the
// authorisation context the HTTP middleware attached; froodbase-api adds the
// other tools through internal/apihandlers.
func WhoamiHandlers() Handlers {
	return Handlers{
		Whoami: func(ctx context.Context) (WhoamiOutput, error) {
			ac := authz.FromContext(ctx)
			if ac == nil {
				return WhoamiOutput{}, authz.ErrNoContext
			}
			return WhoamiFromContext(ac), nil
		},
	}
}

// WhoamiFromContext converts an authorisation context to the tool output.
func WhoamiFromContext(ac *authz.Context) WhoamiOutput {
	grants := make(map[string][]string, len(ac.Grants))
	for _, area := range ac.Areas() {
		roles := make([]string, 0, len(ac.Grants[area]))
		for _, r := range ac.Grants[area] {
			roles = append(roles, string(r))
		}
		grants[area] = roles
	}
	return WhoamiOutput{UserID: ac.UserID, DisplayName: ac.DisplayName, AgentID: ac.AgentID, Grants: grants}
}
