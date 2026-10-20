package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store/gen"
	"github.com/rztaylor/froodbase/internal/store/jobs"
)

// The writer. Every method is one transaction that writes the records, their
// revision snapshots, an audit event per write, the support events the
// evidence earns and the index job together, with the caller's areas set
// first so the policies apply (TR-35, TR-74, TR-90, D5). A failure anywhere
// rolls everything back.

// Audit event types the writer records.
const (
	AuditAreaCreated      = "area_created"
	AuditEntityCreated    = "entity_created"
	AuditEntityUpdated    = "entity_updated"
	AuditAssertionCreated = "assertion_created"
	AuditAssertionUpdated = "assertion_updated"
	AuditEvidenceRecorded = "evidence_recorded"
	AuditEvidenceLinked   = "evidence_linked"
)

// Contribution is one contribute call to write: the provenance episode and
// the records it asserts. Every record must carry the contribution's area.
// ClaimEvidence links each assertion to evidence by id, either records in
// Evidence or evidence the area already holds.
type Contribution struct {
	Contribution  domain.Contribution
	Entities      []*domain.Entity
	Assertions    []domain.Assertion
	Evidence      []domain.Evidence
	ClaimEvidence map[domain.ID][]domain.ID
	Rationale     string
}

// ContributionResult reports what the write produced.
type ContributionResult struct {
	// Scores is the projected support score of every assertion written.
	Scores map[domain.ID]float64
	// ConfigVersion is the support configuration the scores were projected under.
	ConfigVersion string
}

// CreateArea creates a knowledge area and audits it. The caller's areas for
// this transaction are extended with the new id so the policy on
// knowledge_area admits the row; authorising who may create areas is the
// API's job.
func (p *Pool) CreateArea(ctx context.Context, caller Caller, area domain.Area) error {
	if err := area.Validate(); err != nil {
		return err
	}
	caller.AreaIDs = append(append([]string(nil), caller.AreaIDs...), string(area.ID))
	return p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		now := p.now().UTC()
		if err := q.InsertArea(ctx, gen.InsertAreaParams{ID: string(area.ID), Key: area.Key, Name: area.Name, RecordedAt: now}); err != nil {
			return err
		}
		return p.audit(ctx, q, caller, string(area.ID), AuditAreaCreated, "", "", 0, map[string]any{"key": area.Key}, now)
	})
}

// Contribute writes a contribution: entities with their names, evidence,
// assertions linked to their evidence, the contribution and its claims, a
// revision and an audit event per record, one root_added support event per
// distinct evidence root of each assertion with the score projected under
// the area's current support configuration (TR-29, TR-43), and an index job
// per entity and assertion. Every assertion enters active (PR-40).
func (p *Pool) Contribute(ctx context.Context, caller Caller, c Contribution) (*ContributionResult, error) {
	if err := c.validate(caller); err != nil {
		return nil, err
	}
	res := &ContributionResult{Scores: make(map[domain.ID]float64, len(c.Assertions))}
	err := p.inTx(ctx, caller, func(ctx context.Context, tx pgx.Tx, q *gen.Queries) error {
		now := p.now().UTC()
		area := string(c.Contribution.AreaID)
		cfg, err := p.currentSupportConfig(ctx, q, area)
		if err != nil {
			return err
		}
		res.ConfigVersion = cfg.Version

		cc := c.Contribution
		if err := q.InsertContribution(ctx, gen.InsertContributionParams{
			ID: string(cc.ID), AreaID: area, ActorUserID: string(cc.ActorUserID), ActorAgentID: optional(string(cc.ActorAgentID)),
			Client: cc.Client, InvestigationRef: cc.InvestigationRef, RecordedAt: cc.RecordedAt.UTC(),
		}); err != nil {
			return err
		}

		for _, e := range c.Entities {
			if err := p.insertEntity(ctx, tx, q, caller, e, c.Rationale, now); err != nil {
				return err
			}
		}
		byID := make(map[domain.ID]domain.Evidence, len(c.Evidence))
		for _, ev := range c.Evidence {
			if err := p.insertEvidence(ctx, q, caller, ev, c.Rationale, now); err != nil {
				return err
			}
			byID[ev.ID] = ev
		}
		for _, a := range c.Assertions {
			evidence, err := p.resolveEvidence(ctx, q, byID, c.ClaimEvidence[a.ID])
			if err != nil {
				return err
			}
			score, err := p.insertAssertion(ctx, tx, q, caller, a, evidence, cfg, c.Rationale, now)
			if err != nil {
				return err
			}
			if err := q.InsertContributionClaim(ctx, gen.InsertContributionClaimParams{ContributionID: string(cc.ID), ClaimID: string(a.ID), AreaID: area}); err != nil {
				return err
			}
			res.Scores[a.ID] = score
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c Contribution) validate(caller Caller) error {
	if err := caller.validate(); err != nil {
		return err
	}
	if err := c.Contribution.Validate(); err != nil {
		return err
	}
	area := c.Contribution.AreaID
	if !caller.hasArea(string(area)) {
		return ErrForbidden
	}
	for _, e := range c.Entities {
		if e == nil || e.ID().Validate() != nil {
			return fmt.Errorf("entity: %w", domain.ErrEmptyID)
		}
		if e.AreaID != area {
			return fmt.Errorf("entity %s: %w", e.ID(), ErrForbidden)
		}
	}
	for _, ev := range c.Evidence {
		if err := ev.Validate(); err != nil {
			return err
		}
		if ev.AreaID != area {
			return fmt.Errorf("evidence %s: %w", ev.ID, ErrForbidden)
		}
	}
	for _, a := range c.Assertions {
		if err := a.Validate(); err != nil {
			return err
		}
		if a.AreaID != area {
			return fmt.Errorf("assertion %s: %w", a.ID, ErrForbidden)
		}
	}
	return nil
}

// UpdateEntity replaces the entity's names, based on expectedRevision, and
// writes revision expectedRevision+1 (TR-35, TR-90). The type and area are
// immutable and must match the stored record.
func (p *Pool) UpdateEntity(ctx context.Context, caller Caller, e *domain.Entity, expectedRevision int, rationale string) (int, error) {
	if e == nil || e.ID().Validate() != nil {
		return 0, fmt.Errorf("entity: %w", domain.ErrEmptyID)
	}
	if len(e.Names) == 0 {
		return 0, fmt.Errorf("entity %s: %w", e.ID(), domain.ErrEmptyName)
	}
	var revision int
	err := p.inTx(ctx, caller, func(ctx context.Context, tx pgx.Tx, q *gen.Queries) error {
		now := p.now().UTC()
		id := string(e.ID())
		cur, err := q.GetEntity(ctx, id)
		if err != nil {
			return err
		}
		if cur.AreaID != string(e.AreaID) || cur.EntityType != string(e.Type) {
			return fmt.Errorf("entity %s: type and area are immutable: %w", id, ErrRevisionConflict)
		}
		n, err := q.UpdateEntity(ctx, gen.UpdateEntityParams{ID: id, Revision: int32(expectedRevision), UpdatedAt: now})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrRevisionConflict
		}
		revision = expectedRevision + 1
		if err := q.DeleteEntityNames(ctx, id); err != nil {
			return err
		}
		if err := p.insertNames(ctx, q, e); err != nil {
			return err
		}
		rec := EntityRecord{Entity: e, LifecycleState: cur.LifecycleState, Revision: revision}
		snap, err := entitySnapshotBytes(rec)
		if err != nil {
			return err
		}
		return p.recordChange(ctx, tx, q, caller, change{
			recordID: id, recordType: RecordEntity, areaID: cur.AreaID, revision: revision,
			snapshot: snap, audit: AuditEntityUpdated, rationale: rationale, index: true,
		}, now)
	})
	return revision, err
}

// UpdateAssertion replaces the assertion's content and temporal fields,
// based on expectedRevision, and writes revision expectedRevision+1. Kind,
// subject and area are immutable (PR-03) and must match the stored record;
// support is untouched because content changes earn nothing (PR-42).
func (p *Pool) UpdateAssertion(ctx context.Context, caller Caller, a domain.Assertion, expectedRevision int, rationale string) (int, error) {
	if err := a.Validate(); err != nil {
		return 0, err
	}
	var revision int
	err := p.inTx(ctx, caller, func(ctx context.Context, tx pgx.Tx, q *gen.Queries) error {
		now := p.now().UTC()
		id := string(a.ID)
		cur, err := q.GetAssertion(ctx, id)
		if err != nil {
			return err
		}
		if cur.AreaID != string(a.AreaID) || cur.SubjectEntityID != string(a.SubjectEntityID) || cur.EpistemicKind != string(a.Kind) {
			return fmt.Errorf("assertion %s: kind, subject and area are immutable: %w", id, ErrRevisionConflict)
		}
		n, err := q.UpdateAssertion(ctx, gen.UpdateAssertionParams{
			ID: id, Revision: int32(expectedRevision), BodyMarkdown: a.BodyMarkdown, Scope: scopeBytes(a.Scope),
			ValidFrom: utcPtr(a.ValidFrom), ValidTo: utcPtr(a.ValidTo), VerifiedAt: utcPtr(a.VerifiedAt), ReviewDueAt: utcPtr(a.ReviewDueAt), UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrRevisionConflict
		}
		revision = expectedRevision + 1
		a.RecordedAt = cur.RecordedAt // recording time is when Froodbase first learned it
		snap, err := assertionSnapshotBytes(AssertionRecord{Assertion: a, LifecycleState: cur.LifecycleState, Revision: revision})
		if err != nil {
			return err
		}
		return p.recordChange(ctx, tx, q, caller, change{
			recordID: id, recordType: RecordAssertion, areaID: cur.AreaID, revision: revision,
			snapshot: snap, audit: AuditAssertionUpdated, rationale: rationale, index: true,
		}, now)
	})
	return revision, err
}

// LinkEvidence links further evidence to an existing assertion: records the
// evidence the area does not yet hold, links it, appends a root_added event
// for every distinct root the claim has not seen (repeated descendants of a
// known root add nothing, TR-29) and stores the score re-projected from the
// claim's whole history under the area's current configuration. It returns
// the new score. The assertion's content is unchanged, so no revision is
// written; the audit event and the support events are the history.
func (p *Pool) LinkEvidence(ctx context.Context, caller Caller, claimID domain.ID, evidence []domain.Evidence, rationale string) (float64, error) {
	if err := claimID.Validate(); err != nil {
		return 0, fmt.Errorf("claim id: %w", err)
	}
	var score float64
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		now := p.now().UTC()
		cur, err := q.GetAssertion(ctx, string(claimID))
		if err != nil {
			return err
		}
		cfg, err := p.currentSupportConfig(ctx, q, cur.AreaID)
		if err != nil {
			return err
		}
		for _, ev := range evidence {
			if err := ev.Validate(); err != nil {
				return err
			}
			if string(ev.AreaID) != cur.AreaID {
				return fmt.Errorf("evidence %s: %w", ev.ID, ErrForbidden)
			}
			if _, err := q.GetEvidence(ctx, string(ev.ID)); err == nil {
				continue
			} else if !isNoRows(err) {
				return err
			}
			if err := p.insertEvidence(ctx, q, caller, ev, rationale, now); err != nil {
				return err
			}
		}
		linked, err := p.linkEvidence(ctx, q, caller, cur, evidence, cfg, rationale, now)
		if err != nil {
			return err
		}
		score = linked
		return p.audit(ctx, q, caller, cur.AreaID, AuditEvidenceLinked, cur.ID, RecordAssertion, int(cur.Revision),
			map[string]any{"evidence_ids": evidenceIDs(evidence), "support_score": score, "rationale": rationale}, now)
	})
	return score, err
}

// PutSupportConfig records a new configuration version for an area (TR-43).
// Versions are append-only: a change is a new version, never an edit, so
// every stored score keeps the version it was projected under.
func (p *Pool) PutSupportConfig(ctx context.Context, caller Caller, areaID domain.ID, cfg domain.SupportConfig) error {
	if err := areaID.Validate(); err != nil {
		return fmt.Errorf("area id: %w", err)
	}
	weights, thresholds, allowed, err := supportConfigToRow(cfg)
	if err != nil {
		return err
	}
	return p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		area := string(areaID)
		return q.InsertSupportConfig(ctx, gen.InsertSupportConfigParams{
			AreaID: &area, Version: cfg.Version, EventWeights: weights, UseWithoutChallengeCap: cfg.UseWithoutChallengeCap,
			ThresholdByKind: thresholds, AllowedContributors: allowed, RecordedAt: p.now().UTC(),
		})
	})
}

// change is one record write to finish: its revision row, audit event and,
// for indexed records, the index job.
type change struct {
	recordID, recordType, areaID string
	revision                     int
	snapshot                     []byte
	audit, rationale             string
	index                        bool
}

func (p *Pool) recordChange(ctx context.Context, tx pgx.Tx, q *gen.Queries, caller Caller, c change, now time.Time) error {
	if err := p.hook("revision"); err != nil {
		return err
	}
	if err := q.InsertRevision(ctx, gen.InsertRevisionParams{
		RecordID: c.recordID, RecordType: c.recordType, Revision: int32(c.revision), AreaID: c.areaID, Snapshot: c.snapshot,
		ActorUserID: caller.UserID, ActorAgentID: caller.agentID(), Rationale: c.rationale, RecordedAt: now,
	}); err != nil {
		return err
	}
	if err := p.hook("audit"); err != nil {
		return err
	}
	if err := p.audit(ctx, q, caller, c.areaID, c.audit, c.recordID, c.recordType, c.revision, map[string]any{"rationale": c.rationale}, now); err != nil {
		return err
	}
	if !c.index {
		return nil
	}
	if err := p.hook("job"); err != nil {
		return err
	}
	_, err := p.jobs.InsertTx(ctx, tx, jobs.IndexRecordArgs{RecordID: c.recordID, RecordType: c.recordType, AreaID: c.areaID, Revision: c.revision})
	return err
}

func (p *Pool) audit(ctx context.Context, q *gen.Queries, caller Caller, areaID, eventType, recordID, recordType string, revision int, detail map[string]any, now time.Time) error {
	body, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("store: encoding audit detail: %w", err)
	}
	var rev *int32
	if revision > 0 {
		r := int32(revision)
		rev = &r
	}
	return q.InsertAuditEvent(ctx, gen.InsertAuditEventParams{
		AreaID: optional(areaID), EventType: eventType, ActorUserID: caller.UserID, ActorAgentID: caller.agentID(),
		RecordID: optional(recordID), RecordType: optional(recordType), RecordRevision: rev, Detail: body, At: now,
	})
}

func (p *Pool) insertEntity(ctx context.Context, tx pgx.Tx, q *gen.Queries, caller Caller, e *domain.Entity, rationale string, now time.Time) error {
	id := string(e.ID())
	if err := q.InsertEntity(ctx, gen.InsertEntityParams{ID: id, AreaID: string(e.AreaID), EntityType: string(e.Type), LifecycleState: LifecycleActive, RecordedAt: now}); err != nil {
		return err
	}
	if err := p.insertNames(ctx, q, e); err != nil {
		return err
	}
	snap, err := entitySnapshotBytes(EntityRecord{Entity: e, LifecycleState: LifecycleActive, Revision: 1})
	if err != nil {
		return err
	}
	return p.recordChange(ctx, tx, q, caller, change{
		recordID: id, recordType: RecordEntity, areaID: string(e.AreaID), revision: 1,
		snapshot: snap, audit: AuditEntityCreated, rationale: rationale, index: true,
	}, now)
}

func (p *Pool) insertNames(ctx context.Context, q *gen.Queries, e *domain.Entity) error {
	for i, n := range e.Names {
		if err := n.Validate(); err != nil {
			return err
		}
		if err := q.InsertEntityName(ctx, gen.InsertEntityNameParams{
			EntityID: string(e.ID()), AreaID: string(e.AreaID), Position: int32(i), Name: n.Name, Scope: scopeBytes(n.Scope), ValidFrom: utcPtr(n.ValidFrom), ValidTo: utcPtr(n.ValidTo),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pool) insertEvidence(ctx context.Context, q *gen.Queries, caller Caller, ev domain.Evidence, rationale string, now time.Time) error {
	if err := q.InsertEvidence(ctx, gen.InsertEvidenceParams{
		ID: string(ev.ID), AreaID: string(ev.AreaID), SourceKind: string(ev.SourceKind), SourceUri: ev.SourceURI, SourceRevision: ev.SourceRevision,
		SourceLocator: ev.SourceLocator, Excerpt: optional(ev.Excerpt), AccessClass: string(ev.AccessClass), RootEvidenceID: optional(string(ev.RootEvidenceID)),
		ObservedAt: utcPtr(ev.ObservedAt), RecordedAt: ev.RecordedAt.UTC(),
	}); err != nil {
		return err
	}
	snap, err := evidenceSnapshotBytes(ev)
	if err != nil {
		return err
	}
	return p.recordChange(ctx, nil, q, caller, change{
		recordID: string(ev.ID), recordType: RecordEvidence, areaID: string(ev.AreaID), revision: 1,
		snapshot: snap, audit: AuditEvidenceRecorded, rationale: rationale,
	}, now)
}

// resolveEvidence returns the evidence records behind ids: those written in
// this contribution, else those the area already holds.
func (p *Pool) resolveEvidence(ctx context.Context, q *gen.Queries, written map[domain.ID]domain.Evidence, ids []domain.ID) ([]domain.Evidence, error) {
	out := make([]domain.Evidence, 0, len(ids))
	for _, id := range ids {
		if ev, ok := written[id]; ok {
			out = append(out, ev)
			continue
		}
		row, err := q.GetEvidence(ctx, string(id))
		if err != nil {
			return nil, fmt.Errorf("evidence %s: %w", id, mapErr(err))
		}
		out = append(out, evidenceFromRow(row))
	}
	return out, nil
}

func (p *Pool) insertAssertion(ctx context.Context, tx pgx.Tx, q *gen.Queries, caller Caller, a domain.Assertion, evidence []domain.Evidence, cfg domain.SupportConfig, rationale string, now time.Time) (float64, error) {
	id := string(a.ID)
	if err := q.InsertAssertion(ctx, gen.InsertAssertionParams{
		ID: id, AreaID: string(a.AreaID), SubjectEntityID: string(a.SubjectEntityID), EpistemicKind: string(a.Kind), BodyMarkdown: a.BodyMarkdown,
		Scope: scopeBytes(a.Scope), ValidFrom: utcPtr(a.ValidFrom), ValidTo: utcPtr(a.ValidTo), RecordedAt: a.RecordedAt.UTC(), VerifiedAt: utcPtr(a.VerifiedAt),
		ReviewDueAt: utcPtr(a.ReviewDueAt), LifecycleState: LifecycleActive, SupportScore: 0, SupportConfigVersion: cfg.Version, UpdatedAt: now,
	}); err != nil {
		return 0, err
	}
	cur, err := q.GetAssertion(ctx, id)
	if err != nil {
		return 0, err
	}
	score, err := p.linkEvidence(ctx, q, caller, cur, evidence, cfg, rationale, now)
	if err != nil {
		return 0, err
	}
	snap, err := assertionSnapshotBytes(AssertionRecord{Assertion: a, LifecycleState: LifecycleActive, Revision: 1})
	if err != nil {
		return 0, err
	}
	return score, p.recordChange(ctx, tx, q, caller, change{
		recordID: id, recordType: RecordAssertion, areaID: string(a.AreaID), revision: 1,
		snapshot: snap, audit: AuditAssertionCreated, rationale: rationale, index: true,
	}, now)
}

// linkEvidence links evidence to the assertion, appends a root_added event
// for each root the claim has not seen, re-projects the score from every
// event under cfg and stores it. Idempotent for evidence already linked.
func (p *Pool) linkEvidence(ctx context.Context, q *gen.Queries, caller Caller, cur gen.GetAssertionRow, evidence []domain.Evidence, cfg domain.SupportConfig, rationale string, now time.Time) (float64, error) {
	for _, ev := range evidence {
		if err := q.InsertClaimEvidence(ctx, gen.InsertClaimEvidenceParams{ClaimID: cur.ID, EvidenceID: string(ev.ID), AreaID: cur.AreaID}); err != nil {
			return 0, err
		}
	}
	known, err := q.ListSupportRoots(ctx, cur.ID)
	if err != nil {
		return 0, err
	}
	seen := make(map[domain.ID]bool, len(known))
	for _, r := range known {
		seen[domain.ID(r)] = true
	}
	scope, err := scopeFromBytes(cur.Scope)
	if err != nil {
		return 0, err
	}
	for _, root := range domain.DistinctRoots(evidence) {
		if seen[root] {
			continue
		}
		e := domain.SupportEvent{
			Type: domain.SupportRootAdded, EvidenceRootIDs: []domain.ID{root}, Scope: scope, ActorUserID: domain.ID(caller.UserID),
			ActorAgentID: domain.ID(caller.AgentID), ConfigVersion: cfg.Version, Rationale: rationale, At: now,
		}
		if err := e.Validate(); err != nil {
			return 0, err
		}
		if err := q.InsertSupportEvent(ctx, gen.InsertSupportEventParams{
			RecordID: cur.ID, AreaID: cur.AreaID, EventType: string(e.Type), EvidenceRootIds: []string{string(root)}, Scope: scopeBytes(scope),
			ActorUserID: optional(caller.UserID), ActorAgentID: caller.agentID(), JobName: nil, ConfigVersion: cfg.Version, Rationale: rationale, At: now,
		}); err != nil {
			return 0, err
		}
	}
	rows, err := q.ListSupportEvents(ctx, cur.ID)
	if err != nil {
		return 0, err
	}
	events := make([]domain.SupportEvent, 0, len(rows))
	for _, r := range rows {
		ev, err := supportEventFromRow(r)
		if err != nil {
			return 0, err
		}
		events = append(events, ev)
	}
	score, err := domain.SupportScore(events, cfg)
	if err != nil {
		return 0, err
	}
	n, err := q.UpdateAssertionSupport(ctx, gen.UpdateAssertionSupportParams{
		ID: cur.ID, SupportScore: score, SupportConfigVersion: cfg.Version,
		NeedsReconsideration: !cfg.MeetsThreshold(domain.EpistemicKind(cur.EpistemicKind), score) && cur.NeedsReconsideration,
	})
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, ErrNotFound
	}
	return score, nil
}

func (p *Pool) currentSupportConfig(ctx context.Context, q *gen.Queries, areaID string) (domain.SupportConfig, error) {
	row, err := q.CurrentSupportConfig(ctx, areaID)
	if err != nil {
		return domain.SupportConfig{}, fmt.Errorf("store: reading support config: %w", mapErr(err))
	}
	return supportConfigFromRow(row.Version, row.EventWeights, row.ThresholdByKind, row.AllowedContributors, row.UseWithoutChallengeCap)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func isNoRows(err error) bool {
	return err != nil && (err == pgx.ErrNoRows || mapErr(err) == ErrNotFound)
}

func evidenceIDs(evidence []domain.Evidence) []string {
	out := make([]string, 0, len(evidence))
	for _, ev := range evidence {
		out = append(out, string(ev.ID))
	}
	return out
}
