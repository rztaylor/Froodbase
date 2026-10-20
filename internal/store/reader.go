package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
)

// The reader. Every method runs in a transaction with the caller's areas
// set, so a record outside those areas is simply absent (ErrNotFound) and a
// listing never counts it (TR-71, TR-75).

// RevisionRecord is one entry of a record's history (TR-35).
type RevisionRecord struct {
	RecordID     domain.ID
	RecordType   string
	Revision     int
	AreaID       domain.ID
	Snapshot     json.RawMessage
	ActorUserID  string
	ActorAgentID string
	Rationale    string
	RecordedAt   time.Time
}

// AuditRecord is one operational audit event (TR-74).
type AuditRecord struct {
	ID             string
	AreaID         string
	EventType      string
	ActorUserID    string
	ActorAgentID   string
	RecordID       string
	RecordType     string
	RecordRevision int
	Detail         json.RawMessage
	At             time.Time
}

// GetEntity reads an entity with its current names.
func (p *Pool) GetEntity(ctx context.Context, caller Caller, id domain.ID) (EntityRecord, error) {
	var rec EntityRecord
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		row, err := q.GetEntity(ctx, string(id))
		if err != nil {
			return err
		}
		names, err := q.ListEntityNames(ctx, string(id))
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return fmt.Errorf("entity %s has no names: %w", id, domain.ErrEmptyName)
		}
		var e *domain.Entity
		for i, n := range names {
			scope, err := scopeFromBytes(n.Scope)
			if err != nil {
				return err
			}
			name := domain.EntityName{Name: n.Name, Scope: scope, ValidFrom: n.ValidFrom, ValidTo: n.ValidTo}
			if i == 0 {
				if e, err = domain.NewEntity(domain.ID(row.ID), domain.ID(row.AreaID), domain.EntityType(row.EntityType), name); err != nil {
					return err
				}
				continue
			}
			if err := e.AddName(name); err != nil {
				return err
			}
		}
		rec = EntityRecord{Entity: e, LifecycleState: row.LifecycleState, Revision: int(row.Revision)}
		return nil
	})
	return rec, err
}

// GetAssertion reads an assertion with its support fields.
func (p *Pool) GetAssertion(ctx context.Context, caller Caller, id domain.ID) (AssertionRecord, error) {
	var rec AssertionRecord
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		row, err := q.GetAssertion(ctx, string(id))
		if err != nil {
			return err
		}
		rec, err = assertionFromRow(row)
		return err
	})
	return rec, err
}

// GetEvidence reads one evidence record.
func (p *Pool) GetEvidence(ctx context.Context, caller Caller, id domain.ID) (domain.Evidence, error) {
	var ev domain.Evidence
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		row, err := q.GetEvidence(ctx, string(id))
		if err != nil {
			return err
		}
		ev = evidenceFromRow(row)
		return nil
	})
	return ev, err
}

// ClaimEvidence lists the evidence linked to an assertion, through the
// claim_evidence join, so a caller outside the area sees none of it.
func (p *Pool) ClaimEvidence(ctx context.Context, caller Caller, claimID domain.ID) ([]domain.Evidence, error) {
	var out []domain.Evidence
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.ListEvidenceForClaim(ctx, string(claimID))
		if err != nil {
			return err
		}
		out = make([]domain.Evidence, 0, len(rows))
		for _, r := range rows {
			out = append(out, evidenceFromRow(r))
		}
		return nil
	})
	return out, err
}

// History lists a record's revisions, oldest first (TR-35). An unknown or
// invisible record has an empty history.
func (p *Pool) History(ctx context.Context, caller Caller, recordID domain.ID) ([]RevisionRecord, error) {
	var out []RevisionRecord
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.ListRevisions(ctx, string(recordID))
		if err != nil {
			return err
		}
		out = make([]RevisionRecord, 0, len(rows))
		for _, r := range rows {
			out = append(out, RevisionRecord{
				RecordID: domain.ID(r.RecordID), RecordType: r.RecordType, Revision: int(r.Revision), AreaID: domain.ID(r.AreaID),
				Snapshot: json.RawMessage(r.Snapshot), ActorUserID: r.ActorUserID, ActorAgentID: deref(r.ActorAgentID), Rationale: r.Rationale, RecordedAt: r.RecordedAt,
			})
		}
		return nil
	})
	return out, err
}

// AuditEvents lists the audit events of a record, oldest first.
func (p *Pool) AuditEvents(ctx context.Context, caller Caller, recordID domain.ID) ([]AuditRecord, error) {
	var out []AuditRecord
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.ListAuditEventsForRecord(ctx, optional(string(recordID)))
		if err != nil {
			return err
		}
		out = make([]AuditRecord, 0, len(rows))
		for _, r := range rows {
			out = append(out, AuditRecord{
				ID: r.ID, AreaID: deref(r.AreaID), EventType: r.EventType, ActorUserID: r.ActorUserID, ActorAgentID: deref(r.ActorAgentID),
				RecordID: deref(r.RecordID), RecordType: deref(r.RecordType), RecordRevision: int(derefInt(r.RecordRevision)), Detail: json.RawMessage(r.Detail), At: r.At,
			})
		}
		return nil
	})
	return out, err
}

// SupportEvents lists an assertion's support history, oldest first (TR-29).
func (p *Pool) SupportEvents(ctx context.Context, caller Caller, claimID domain.ID) ([]domain.SupportEvent, error) {
	var out []domain.SupportEvent
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		rows, err := q.ListSupportEvents(ctx, string(claimID))
		if err != nil {
			return err
		}
		out = make([]domain.SupportEvent, 0, len(rows))
		for _, r := range rows {
			ev, err := supportEventFromRow(r)
			if err != nil {
				return err
			}
			out = append(out, ev)
		}
		return nil
	})
	return out, err
}

// SupportConfig returns the configuration version an area's score was
// projected under: the area's own row of that version if it has one, else
// the service default of that version (TR-43).
func (p *Pool) SupportConfig(ctx context.Context, caller Caller, areaID domain.ID, version string) (domain.SupportConfig, error) {
	var cfg domain.SupportConfig
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		row, err := q.SupportConfigVersion(ctx, gen.SupportConfigVersionParams{Version: version, AreaID: string(areaID)})
		if err != nil {
			return err
		}
		cfg, err = supportConfigFromRow(row.Version, row.EventWeights, row.ThresholdByKind, row.AllowedContributors, row.UseWithoutChallengeCap)
		return err
	})
	return cfg, err
}

// CurrentSupportConfig returns the configuration a write in the area would
// use now: the area's newest version, else the service default.
func (p *Pool) CurrentSupportConfig(ctx context.Context, caller Caller, areaID domain.ID) (domain.SupportConfig, error) {
	var cfg domain.SupportConfig
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		var err error
		cfg, err = p.currentSupportConfig(ctx, q, string(areaID))
		return err
	})
	return cfg, err
}

// RecomputeSupportScore projects an assertion's score again from its
// events under the configuration version stored on it, without writing. It
// must equal the stored score; a difference means the history or the
// configuration was tampered with.
func (p *Pool) RecomputeSupportScore(ctx context.Context, caller Caller, claimID domain.ID) (float64, error) {
	var score float64
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		cur, err := q.GetAssertion(ctx, string(claimID))
		if err != nil {
			return err
		}
		row, err := q.SupportConfigVersion(ctx, gen.SupportConfigVersionParams{Version: cur.SupportConfigVersion, AreaID: cur.AreaID})
		if err != nil {
			return fmt.Errorf("store: support config %q: %w", cur.SupportConfigVersion, mapErr(err))
		}
		cfg, err := supportConfigFromRow(row.Version, row.EventWeights, row.ThresholdByKind, row.AllowedContributors, row.UseWithoutChallengeCap)
		if err != nil {
			return err
		}
		rows, err := q.ListSupportEvents(ctx, cur.ID)
		if err != nil {
			return err
		}
		events := make([]domain.SupportEvent, 0, len(rows))
		for _, r := range rows {
			ev, err := supportEventFromRow(r)
			if err != nil {
				return err
			}
			events = append(events, ev)
		}
		score, err = domain.SupportScore(events, cfg)
		return err
	})
	return score, err
}

func assertionFromRow(row gen.GetAssertionRow) (AssertionRecord, error) {
	scope, err := scopeFromBytes(row.Scope)
	if err != nil {
		return AssertionRecord{}, err
	}
	return AssertionRecord{
		Assertion: domain.Assertion{
			ID: domain.ID(row.ID), AreaID: domain.ID(row.AreaID), SubjectEntityID: domain.ID(row.SubjectEntityID), Kind: domain.EpistemicKind(row.EpistemicKind),
			BodyMarkdown: row.BodyMarkdown, Scope: scope,
			Temporal: domain.Temporal{ValidFrom: row.ValidFrom, ValidTo: row.ValidTo, RecordedAt: row.RecordedAt, VerifiedAt: row.VerifiedAt, ReviewDueAt: row.ReviewDueAt},
		},
		LifecycleState: row.LifecycleState, Revision: int(row.Revision), SupportScore: row.SupportScore,
		SupportConfigVersion: row.SupportConfigVersion, NeedsReconsideration: row.NeedsReconsideration,
	}, nil
}

func evidenceFromRow(r gen.Evidence) domain.Evidence {
	return domain.Evidence{
		ID: domain.ID(r.ID), AreaID: domain.ID(r.AreaID), SourceKind: domain.SourceKind(r.SourceKind), SourceURI: r.SourceUri, SourceRevision: r.SourceRevision,
		SourceLocator: r.SourceLocator, Excerpt: deref(r.Excerpt), AccessClass: domain.AccessClass(r.AccessClass), RootEvidenceID: domain.ID(deref(r.RootEvidenceID)),
		ObservedAt: r.ObservedAt, RecordedAt: r.RecordedAt,
	}
}

func supportEventFromRow(r gen.SupportEvent) (domain.SupportEvent, error) {
	scope, err := scopeFromBytes(r.Scope)
	if err != nil {
		return domain.SupportEvent{}, err
	}
	roots := make([]domain.ID, 0, len(r.EvidenceRootIds))
	for _, id := range r.EvidenceRootIds {
		roots = append(roots, domain.ID(id))
	}
	return domain.SupportEvent{
		Type: domain.SupportEventType(r.EventType), EvidenceRootIDs: roots, Scope: scope, ActorUserID: domain.ID(deref(r.ActorUserID)),
		ActorAgentID: domain.ID(deref(r.ActorAgentID)), JobName: deref(r.JobName), ConfigVersion: r.ConfigVersion, Rationale: r.Rationale, At: r.At,
	}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}
