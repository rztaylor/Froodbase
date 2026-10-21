package retrieval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/contribute"
	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/wire"
)

// Limits on search results.
const (
	DefaultSearchLimit = 10
	MaxSearchLimit     = 50
)

// Errors a caller may match.
var (
	// ErrEmptyQuery means the search had nothing to look for.
	ErrEmptyQuery = errors.New("retrieval: query is empty")
	// ErrInvalidQuery means the mode, as_of time or scope is malformed; the
	// message says how.
	ErrInvalidQuery = errors.New("retrieval: invalid query")
)

// Search runs the identifier and full-text channels over the caller's
// areas, identifier hits first (TR-50, TR-51), keeping what the mode and
// scope keep (TR-53). Authorisation is applied by the store before any
// ranking (TR-52), and the mode and scope filter after it and before the
// limit, in SQL; results never mention records the caller cannot read, and
// there is no total count. Every result is labelled with its status relative
// to now and, for a claim, whether it is stale. Fusion across channels
// arrives with a later slice.
func (s *Service) Search(ctx context.Context, ac *authz.Context, in wire.SearchInput) (wire.SearchOutput, error) {
	if ac == nil {
		return wire.SearchOutput{}, authz.ErrNoContext
	}
	query := domain.NormaliseQuery(in.Query)
	if query == "" {
		return wire.SearchOutput{}, ErrEmptyQuery
	}
	q, err := queryOf(in.Mode, in.AsOf)
	if err != nil {
		return wire.SearchOutput{}, err
	}
	scope := domain.Scope{}
	if in.Scope != nil {
		scope = contribute.ScopeFromWire(*in.Scope)
		if err := scope.Validate(); err != nil {
			return wire.SearchOutput{}, fmt.Errorf("%w: scope: %v", ErrInvalidQuery, err)
		}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	caller, err := contribute.CallerFor(ctx, s.areas, ac)
	if err != nil {
		return wire.SearchOutput{}, err
	}
	if in.Area != "" {
		id, err := s.areas.AreaID(ctx, in.Area)
		if err != nil {
			return wire.SearchOutput{}, err
		}
		// Narrow the caller to the one area; a caller without it sees nothing.
		narrowed := store.Caller{UserID: caller.UserID, AgentID: caller.AgentID}
		for _, a := range caller.AreaIDs {
			if a == string(id) {
				narrowed.AreaIDs = []string{a}
			}
		}
		caller = narrowed
	}
	filter := store.SearchFilter{Query: q, Scope: scope, Now: s.now().UTC()}
	out := wire.SearchOutput{Query: query, Mode: string(q.Mode), Results: []wire.SearchResult{}}
	if q.Mode == domain.ModeAsOf {
		at := q.AsOf.UTC()
		out.AsOf = &at
	}

	names, err := s.pool.SearchNames(ctx, caller, query, domain.NameSimilarityThreshold, limit, filter)
	if err != nil {
		return wire.SearchOutput{}, err
	}
	seen := map[domain.ID]bool{}
	for _, hit := range names {
		if seen[hit.EntityID] {
			continue
		}
		seen[hit.EntityID] = true
		e, err := s.pool.GetEntity(ctx, caller, hit.EntityID)
		if err != nil {
			return wire.SearchOutput{}, err
		}
		rec, err := s.entityRecord(ctx, e)
		if err != nil {
			return wire.SearchOutput{}, err
		}
		rec.Status = string(hit.Status)
		out.Results = append(out.Results, wire.SearchResult{Record: rec, Match: wire.Match{
			Channel: wire.ChannelIdentifier, Name: hit.Name, Similarity: hit.Similarity, Candidate: !hit.Exact,
		}})
	}

	bodies, err := s.pool.SearchBodies(ctx, caller, query, limit, filter)
	if err != nil {
		return wire.SearchOutput{}, err
	}
	for _, hit := range bodies {
		a, err := s.pool.GetAssertion(ctx, caller, hit.AssertionID)
		if err != nil {
			return wire.SearchOutput{}, err
		}
		rec, err := s.assertionRecord(ctx, caller, a, domain.ModeResult{Status: hit.Status, Stale: hit.Stale})
		if err != nil {
			return wire.SearchOutput{}, err
		}
		out.Results = append(out.Results, wire.SearchResult{Record: rec, Match: wire.Match{Channel: wire.ChannelFullText, Rank: hit.Rank}})
	}
	return out, nil
}

// queryOf reads a tool's mode and as_of time: current by default, as_of
// only with a time and a time only with as_of.
func queryOf(mode string, asOf *time.Time) (domain.Query, error) {
	m := domain.ModeCurrent
	if strings.TrimSpace(mode) != "" {
		var err error
		if m, err = domain.ParseMode(strings.TrimSpace(mode)); err != nil {
			return domain.Query{}, fmt.Errorf("%w: mode %q is not current, as_of or historical", ErrInvalidQuery, mode)
		}
	}
	q := domain.Query{Mode: m}
	if asOf != nil {
		q.AsOf = asOf.UTC()
	}
	switch err := q.Validate(); {
	case errors.Is(err, domain.ErrNoAsOf):
		return domain.Query{}, fmt.Errorf("%w: mode as_of needs as_of, the time to ask about (RFC 3339, for example 2026-09-03T00:00:00Z)", ErrInvalidQuery)
	case errors.Is(err, domain.ErrAsOfNotAllowed):
		return domain.Query{}, fmt.Errorf("%w: as_of is given but mode is %s; set mode to as_of to ask what was valid at that time", ErrInvalidQuery, m)
	case err != nil:
		return domain.Query{}, fmt.Errorf("%w: %v", ErrInvalidQuery, err)
	}
	return q, nil
}

// AuditRestrictedRead is the audit event written when a caller reads
// evidence with the restricted access class (TR-74).
const AuditRestrictedRead = "restricted_read"

// auditRestricted records one event naming the restricted evidence the
// caller was shown with a claim.
func (s *Service) auditRestricted(ctx context.Context, caller store.Caller, areaID domain.ID, claimID domain.ID, evidence []domain.Evidence) error {
	var ids []string
	for _, ev := range evidence {
		if ev.AccessClass == domain.AccessRestricted {
			ids = append(ids, string(ev.ID))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return s.pool.RecordAudit(ctx, caller, string(areaID), AuditRestrictedRead, map[string]any{
		"claim_id": string(claimID), "evidence_ids": ids, "via": strings.TrimSpace("get_record or search"),
	})
}
