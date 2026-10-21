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

// ErrNotFound means no record with that id is visible to the caller.
var ErrNotFound = errors.New("retrieval: record not found")

// Service serves read requests.
type Service struct {
	pool  *store.Pool
	areas contribute.Areas
	now   func() time.Time
}

// New builds the service.
func New(pool *store.Pool, areas contribute.Areas) *Service {
	return &Service{pool: pool, areas: areas, now: time.Now}
}

// GetRecord reads one record of any type by id.
func (s *Service) GetRecord(ctx context.Context, ac *authz.Context, in wire.GetRecordInput) (wire.Record, error) {
	if ac == nil {
		return wire.Record{}, authz.ErrNoContext
	}
	id := domain.ID(strings.TrimSpace(in.ID))
	if id == "" {
		return wire.Record{}, fmt.Errorf("%w: id is required", ErrNotFound)
	}
	caller, err := contribute.CallerFor(ctx, s.areas, ac)
	if err != nil {
		return wire.Record{}, err
	}
	if a, err := s.pool.GetAssertion(ctx, caller, id); err == nil {
		mode, err := s.pool.ClassifyAssertion(ctx, caller, id, domain.HistoricalQuery(), s.now().UTC())
		if err != nil {
			return wire.Record{}, err
		}
		return s.assertionRecord(ctx, caller, a, mode)
	} else if !errors.Is(err, store.ErrNotFound) {
		return wire.Record{}, err
	}
	if e, err := s.pool.GetEntity(ctx, caller, id); err == nil {
		return s.entityRecord(ctx, e)
	} else if !errors.Is(err, store.ErrNotFound) {
		return wire.Record{}, err
	}
	if ev, err := s.pool.GetEvidence(ctx, caller, id); err == nil {
		return s.evidenceRecord(ctx, caller, ev)
	} else if !errors.Is(err, store.ErrNotFound) {
		return wire.Record{}, err
	}
	return wire.Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// GetHistory reads a record's revisions and, for a claim, its support events
// and the assertions it was derived from.
func (s *Service) GetHistory(ctx context.Context, ac *authz.Context, in wire.GetHistoryInput) (wire.HistoryOutput, error) {
	if ac == nil {
		return wire.HistoryOutput{}, authz.ErrNoContext
	}
	id := domain.ID(strings.TrimSpace(in.ID))
	caller, err := contribute.CallerFor(ctx, s.areas, ac)
	if err != nil {
		return wire.HistoryOutput{}, err
	}
	revs, err := s.pool.History(ctx, caller, id)
	if err != nil {
		return wire.HistoryOutput{}, err
	}
	if len(revs) == 0 {
		return wire.HistoryOutput{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	out := wire.HistoryOutput{ID: string(id), RecordType: revs[0].RecordType}
	if out.Area, err = s.areas.AreaKey(ctx, revs[0].AreaID); err != nil {
		return wire.HistoryOutput{}, err
	}
	for _, r := range revs {
		out.Revisions = append(out.Revisions, wire.Revision{
			Revision: r.Revision, ActorUserID: r.ActorUserID, ActorAgentID: r.ActorAgentID, Rationale: r.Rationale, RecordedAt: r.RecordedAt, Snapshot: r.Snapshot,
		})
	}
	if out.RecordType == store.RecordAssertion {
		events, err := s.pool.SupportEvents(ctx, caller, id)
		if err != nil {
			return wire.HistoryOutput{}, err
		}
		for _, e := range events {
			out.SupportEvents = append(out.SupportEvents, supportEventToWire(e))
		}
		links, err := s.pool.Derivations(ctx, caller, id)
		if err != nil {
			return wire.HistoryOutput{}, err
		}
		for _, l := range links {
			out.Derivations = append(out.Derivations, wire.Derivation{SourceID: string(l.SourceID), Rationale: l.Rationale, RecordedAt: l.RecordedAt})
		}
	}
	return out, nil
}

// assertionRecord shapes an assertion with its support, citations and
// derivations, labelled with the status and stale flag the SQL mode
// predicate gave it.
func (s *Service) assertionRecord(ctx context.Context, caller store.Caller, a store.AssertionRecord, mode domain.ModeResult) (wire.Record, error) {
	area, err := s.areas.AreaKey(ctx, a.Assertion.AreaID)
	if err != nil {
		return wire.Record{}, err
	}
	events, err := s.pool.SupportEvents(ctx, caller, a.Assertion.ID)
	if err != nil {
		return wire.Record{}, err
	}
	cfg, err := s.pool.SupportConfig(ctx, caller, a.Assertion.AreaID, a.SupportConfigVersion)
	if err != nil {
		return wire.Record{}, err
	}
	evidence, err := s.pool.ClaimEvidence(ctx, caller, a.Assertion.ID)
	if err != nil {
		return wire.Record{}, err
	}
	if err := s.auditRestricted(ctx, caller, a.Assertion.AreaID, a.Assertion.ID, evidence); err != nil {
		return wire.Record{}, err
	}
	links, err := s.pool.Derivations(ctx, caller, a.Assertion.ID)
	if err != nil {
		return wire.Record{}, err
	}
	rec := wire.Record{
		ID: string(a.Assertion.ID), RecordType: store.RecordAssertion, Area: area, AreaID: string(a.Assertion.AreaID), Kind: string(a.Assertion.Kind),
		LifecycleState: a.LifecycleState, Revision: a.Revision, Support: SupportSummary(a.SupportScore, a.SupportConfigVersion, events, cfg, a.Assertion.Kind),
		Scope: scopeToWire(a.Assertion.Scope), ValidFrom: a.Assertion.ValidFrom, ValidTo: a.Assertion.ValidTo, ValidityUnknown: a.Assertion.ValidityUnknown(),
		RecordedAt: a.Assertion.RecordedAt, VerifiedAt: a.Assertion.VerifiedAt, ReviewDueAt: a.Assertion.ReviewDueAt, NeedsReconsideration: a.NeedsReconsideration,
		Status: string(mode.Status), Stale: mode.Stale,
		Subject: string(a.Assertion.SubjectEntityID), BodyMarkdown: a.Assertion.BodyMarkdown,
	}
	for _, ev := range evidence {
		rec.Citations = append(rec.Citations, citationToWire(ev))
	}
	for _, l := range links {
		rec.DerivedFrom = append(rec.DerivedFrom, string(l.SourceID))
	}
	rec.Derived = len(rec.DerivedFrom) > 0
	return rec, nil
}

func (s *Service) entityRecord(ctx context.Context, e store.EntityRecord) (wire.Record, error) {
	area, err := s.areas.AreaKey(ctx, e.Entity.AreaID)
	if err != nil {
		return wire.Record{}, err
	}
	view := &wire.EntityView{Type: string(e.Entity.Type)}
	for _, n := range e.Entity.Names {
		view.Names = append(view.Names, wire.Name{Name: n.Name, Scope: scopeToWire(n.Scope), ValidFrom: n.ValidFrom, ValidTo: n.ValidTo})
	}
	// An entity has no validity of its own, only its names do: by itself it
	// is current while in force.
	status := domain.ModeRecord{State: domain.LifecycleState(e.LifecycleState)}.StatusAt(s.now())
	return wire.Record{
		ID: string(e.Entity.ID()), RecordType: store.RecordEntity, Area: area, AreaID: string(e.Entity.AreaID),
		LifecycleState: e.LifecycleState, Revision: e.Revision, Entity: view, Status: string(status),
	}, nil
}

func (s *Service) evidenceRecord(ctx context.Context, caller store.Caller, ev domain.Evidence) (wire.Record, error) {
	area, err := s.areas.AreaKey(ctx, ev.AreaID)
	if err != nil {
		return wire.Record{}, err
	}
	c := citationToWire(ev)
	if err := s.auditRestricted(ctx, caller, ev.AreaID, "", []domain.Evidence{ev}); err != nil {
		return wire.Record{}, err
	}
	return wire.Record{
		ID: string(ev.ID), RecordType: store.RecordEvidence, Area: area, AreaID: string(ev.AreaID), LifecycleState: store.LifecycleActive,
		Revision: 1, RecordedAt: ev.RecordedAt, Evidence: &c,
	}, nil
}

// SupportSummary counts a claim's events, weighs its roots by source kind and
// reports its threshold verdict.
func SupportSummary(score float64, version string, events []domain.SupportEvent, cfg domain.SupportConfig, kind domain.EpistemicKind) *wire.Support {
	out := &wire.Support{Score: score, ConfigVersion: version, Events: len(events), MeetsThreshold: cfg.MeetsThreshold(kind, score)}
	out.RootsBySourceKind = contribute.RootWeightsToWire(domain.RootsBySourceKind(events, cfg))
	for _, e := range events {
		switch e.Type {
		case domain.SupportRootAdded:
			out.Roots++
		case domain.SupportVerified:
			out.Verifications++
		case domain.SupportOutcomeSucceeded, domain.SupportOutcomeFailed:
			out.Outcomes++
		}
	}
	return out
}

func citationToWire(ev domain.Evidence) wire.Citation {
	return wire.Citation{
		EvidenceID: string(ev.ID), SourceKind: string(ev.SourceKind), SourceURI: ev.SourceURI, SourceRevision: ev.SourceRevision, Locator: ev.SourceLocator,
		Excerpt: ev.Excerpt, AccessClass: string(ev.AccessClass), RootEvidenceID: string(ev.RootEvidenceID), ObservedAt: ev.ObservedAt, RecordedAt: ev.RecordedAt,
	}
}

func supportEventToWire(e domain.SupportEvent) wire.SupportEvent {
	roots := make([]string, 0, len(e.EvidenceRootIDs))
	for _, r := range e.EvidenceRootIDs {
		roots = append(roots, string(r))
	}
	return wire.SupportEvent{
		Type: string(e.Type), EvidenceRootIDs: roots, Scope: scopeToWire(e.Scope), ActorUserID: string(e.ActorUserID), ActorAgentID: string(e.ActorAgentID),
		JobName: e.JobName, ConfigVersion: e.ConfigVersion, Rationale: e.Rationale, At: e.At,
	}
}

func scopeToWire(s domain.Scope) *wire.Scope {
	if s.IsUnspecified() {
		return nil
	}
	return &wire.Scope{Environment: s.Environment, Region: s.Region, VersionMin: s.VersionMin, VersionMax: s.VersionMax, Cluster: s.Cluster, Tenant: s.Tenant}
}
