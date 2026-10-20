package retrieval

import (
	"context"
	"fmt"
	"strings"

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

// ErrEmptyQuery means the search had nothing to look for.
var ErrEmptyQuery = fmt.Errorf("retrieval: query is empty")

// Search runs the identifier and full-text channels over the caller's
// areas, identifier hits first (TR-50, TR-51). Authorisation is applied by
// the store before any ranking (TR-52); results never mention records the
// caller cannot read, and there is no total count. Fusion across channels
// and the mode filter arrive with later slices.
func (s *Service) Search(ctx context.Context, ac *authz.Context, in wire.SearchInput) (wire.SearchOutput, error) {
	if ac == nil {
		return wire.SearchOutput{}, authz.ErrNoContext
	}
	query := domain.NormaliseQuery(in.Query)
	if query == "" {
		return wire.SearchOutput{}, ErrEmptyQuery
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
	out := wire.SearchOutput{Query: query, Results: []wire.SearchResult{}}

	names, err := s.pool.SearchNames(ctx, caller, query, domain.NameSimilarityThreshold, limit)
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
		out.Results = append(out.Results, wire.SearchResult{Record: rec, Match: wire.Match{
			Channel: wire.ChannelIdentifier, Name: hit.Name, Similarity: hit.Similarity, Candidate: !hit.Exact,
		}})
	}

	bodies, err := s.pool.SearchBodies(ctx, caller, query, limit)
	if err != nil {
		return wire.SearchOutput{}, err
	}
	for _, hit := range bodies {
		a, err := s.pool.GetAssertion(ctx, caller, hit.AssertionID)
		if err != nil {
			return wire.SearchOutput{}, err
		}
		rec, err := s.assertionRecord(ctx, caller, a)
		if err != nil {
			return wire.SearchOutput{}, err
		}
		out.Results = append(out.Results, wire.SearchResult{Record: rec, Match: wire.Match{Channel: wire.ChannelFullText, Rank: hit.Rank}})
	}
	return out, nil
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
