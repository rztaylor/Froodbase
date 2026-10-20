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
}

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
			Description: "Record findings in a knowledge area as one atomic change set: entities, assertions about them and the shareable evidence (https sources with a revision) behind them. " +
				"Each assertion has a kind that never changes: observation (what was seen; cites evidence, never derived), decision (what was approved or intended) or inference (what was concluded). " +
				"When you conclude something from records you have read, record it as an inference (or a decision) with derived_from naming those assertions (refs in this call or ids in the area), evidence of its own, or both: it starts with the distinct evidence roots of its sources and stays labelled derived. " +
				"Deriving from another area, an unknown id or in a loop is refused. The observations list is the older form of assertions of kind observation. " +
				"Every record is active at once with the support its distinct evidence roots give it; there is no approval step, and below the kind's threshold it is still stored and returned. Reports, conversations and transcripts are never sent: contribute individual findings with their own citations. " +
				"Pass a fresh idempotency_key per change set; retrying with the same key returns the same result without writing again. The result lists every record written with its id, kind, lifecycle state and support: list those ids in your report.",
			Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		}, wrap(h.Contribute))
	}
	if h.GetRecord != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        ToolGetRecord,
			Description: "Read one record (an entity, an assertion or an evidence record) by id with its kind, lifecycle state, support, scope, validity, verification date, citations and, for a deduction, derived: true with the ids it was derived_from. Records outside your knowledge areas are reported as not found.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(h.GetRecord))
	}
	if h.Search != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name: ToolSearch,
			Description: "Search the knowledge areas you can read by an entity name (exact or near-miss; a near-miss is a candidate for you to confirm, never resolved for you) or by words in assertion bodies. " +
				"Results carry kind, lifecycle state, support, scope, validity, verification date, citations and which channel matched. Nothing is reported about records you cannot read. Optional area limits the search to one area key.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, wrap(h.Search))
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
