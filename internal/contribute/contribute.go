package contribute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/wire"
)

// Errors a caller may match.
var (
	// ErrUnknownArea means no knowledge area has that key.
	ErrUnknownArea = errors.New("contribute: unknown knowledge area")
	// ErrNotContributor means the caller lacks the contributor role in the area.
	ErrNotContributor = errors.New("contribute: contributor role required in this area")
	// ErrInvalidInput means the change set is malformed; the message says where.
	ErrInvalidInput = errors.New("contribute: invalid change set")
)

// AuditContributionRefused is the audit event type of a refused contribution.
const AuditContributionRefused = "contribution_refused"

// ToolName is the tool idempotency keys are stored under.
const ToolName = "contribute"

// Service serves contribute calls.
type Service struct {
	pool   *store.Pool
	areas  Areas
	client string
	now    func() time.Time
	newID  func() string
}

// New builds the service. client names the calling application in
// contribution records (TR-64).
func New(pool *store.Pool, areas Areas, client string) *Service {
	return &Service{pool: pool, areas: areas, client: client, now: time.Now, newID: uuid.NewString}
}

// Contribute writes the change set for the caller, or replays the stored
// result of an earlier call with the same idempotency key.
func (s *Service) Contribute(ctx context.Context, ac *authz.Context, in wire.ContributeInput) (wire.ContributeOutput, error) {
	if ac == nil {
		return wire.ContributeOutput{}, authz.ErrNoContext
	}
	if err := validateInput(in); err != nil {
		return wire.ContributeOutput{}, err
	}
	caller, err := CallerFor(ctx, s.areas, ac)
	if err != nil {
		return wire.ContributeOutput{}, err
	}
	areaID, err := s.areas.AreaID(ctx, in.Area)
	if err != nil {
		return wire.ContributeOutput{}, err
	}
	if !ac.Allows(in.Area, authz.RoleContributor) {
		held := strings.Join(roleNames(ac.Grants[in.Area]), ", ")
		if held == "" {
			held = "none"
		}
		_ = s.pool.RecordAudit(ctx, caller, string(areaID), AuditContributionRefused, map[string]any{"area": in.Area, "roles_held": held, "reason": "contributor role required"})
		return wire.ContributeOutput{}, fmt.Errorf("%w: you hold %s in area %q", ErrNotContributor, held, in.Area)
	}

	hash := requestHash(in)
	if prior, err := s.pool.IdempotentResult(ctx, caller, in.IdempotencyKey); err == nil {
		return s.replay(ctx, caller, areaID, in, prior, hash)
	} else if !errors.Is(err, store.ErrNotFound) {
		return wire.ContributeOutput{}, err
	}

	c, refs, err := s.build(ctx, caller, ac, areaID, in)
	if err != nil {
		return wire.ContributeOutput{}, err
	}
	c.Idempotency = &store.Idempotency{Key: in.IdempotencyKey, Tool: ToolName, RequestHash: hash}
	res, err := s.pool.Contribute(ctx, caller, c)
	if errors.Is(err, store.ErrIdempotencyRace) {
		prior, perr := s.pool.IdempotentResult(ctx, caller, in.IdempotencyKey)
		if perr != nil {
			return wire.ContributeOutput{}, err
		}
		return s.replay(ctx, caller, areaID, in, prior, hash)
	}
	if err != nil {
		return wire.ContributeOutput{}, mapStoreErr(err)
	}
	_ = refs
	return s.output(ctx, caller, areaID, in.Area, res, false)
}

// replay returns the stored result of an earlier call with this key, or
// refuses when the request differs.
func (s *Service) replay(ctx context.Context, caller store.Caller, areaID domain.ID, in wire.ContributeInput, prior store.IdempotentResult, hash string) (wire.ContributeOutput, error) {
	if prior.Tool != ToolName || prior.RequestHash != hash {
		return wire.ContributeOutput{}, fmt.Errorf("%w: key %q", store.ErrIdempotencyMismatch, in.IdempotencyKey)
	}
	var res store.ContributionResult
	if err := json.Unmarshal(prior.Response, &res); err != nil {
		return wire.ContributeOutput{}, fmt.Errorf("contribute: decoding stored result: %w", err)
	}
	return s.output(ctx, caller, areaID, in.Area, &res, true)
}

// build translates the input into store records with fresh ids.
func (s *Service) build(ctx context.Context, caller store.Caller, ac *authz.Context, areaID domain.ID, in wire.ContributeInput) (store.Contribution, map[domain.ID]string, error) {
	now := s.now().UTC()
	refs := map[domain.ID]string{}
	ids := map[string]domain.ID{} // ref -> id
	c := store.Contribution{
		Contribution: domain.Contribution{
			ID: domain.ID(s.newID()), AreaID: areaID, ActorUserID: domain.ID(ac.UserID), ActorAgentID: domain.ID(ac.AgentID),
			Client: s.client, InvestigationRef: in.InvestigationRef, RecordedAt: now,
		},
		ClaimEvidence: map[domain.ID][]domain.ID{},
		Rationale:     in.Rationale,
		Refs:          refs,
	}
	for _, e := range in.Entities {
		typ, err := domain.ParseEntityType(e.Type)
		if err != nil {
			return store.Contribution{}, nil, fmt.Errorf("%w: entity %q: %v", ErrInvalidInput, e.Ref, err)
		}
		id := domain.ID(s.newID())
		ent, err := domain.NewEntity(id, areaID, typ, nameFromWire(e.Names[0]))
		if err != nil {
			return store.Contribution{}, nil, fmt.Errorf("%w: entity %q: %v", ErrInvalidInput, e.Ref, err)
		}
		for _, n := range e.Names[1:] {
			if err := ent.AddName(nameFromWire(n)); err != nil {
				return store.Contribution{}, nil, fmt.Errorf("%w: entity %q: %v", ErrInvalidInput, e.Ref, err)
			}
		}
		c.Entities = append(c.Entities, ent)
		ids[e.Ref] = id
		refs[id] = e.Ref
	}
	for _, ev := range in.Evidence {
		id := domain.ID(s.newID())
		rec := domain.Evidence{
			ID: id, AreaID: areaID, SourceKind: domain.SourceKind(ev.SourceKind), SourceURI: ev.SourceURI, SourceRevision: ev.SourceRevision,
			SourceLocator: ev.Locator, Excerpt: ev.Excerpt, AccessClass: domain.AccessClass(ev.AccessClass), ObservedAt: ev.ObservedAt, RecordedAt: now,
		}
		if ev.Root != "" {
			root, ok := ids[ev.Root]
			if !ok {
				root = domain.ID(ev.Root) // an existing evidence id in the area; the store checks it
			}
			rec.RootEvidenceID = root
		}
		if err := rec.Validate(); err != nil {
			return store.Contribution{}, nil, fmt.Errorf("%w: evidence %q: %v", ErrInvalidInput, ev.Ref, err)
		}
		c.Evidence = append(c.Evidence, rec)
		ids[ev.Ref] = id
		refs[id] = ev.Ref
	}
	claims := assertionsOf(in)
	for _, a := range claims {
		ids[a.Ref] = domain.ID(s.newID()) // assigned up front: derived_from may name a later ref
	}
	for _, ai := range claims {
		id := ids[ai.Ref]
		subject, ok := ids[ai.Subject]
		if !ok {
			subject = domain.ID(ai.Subject)
			if _, err := s.pool.GetEntity(ctx, caller, subject); err != nil {
				return store.Contribution{}, nil, fmt.Errorf("%w: %s %q: subject %q is neither a ref in this call nor an entity in the area", ErrInvalidInput, ai.Kind, ai.Ref, ai.Subject)
			}
		}
		scope := domain.Scope{}
		if ai.Scope != nil {
			scope = scopeFromWire(*ai.Scope)
		}
		a, err := domain.NewAssertion(domain.EpistemicKind(ai.Kind), id, areaID, subject, ai.BodyMarkdown, scope, now)
		if err != nil {
			return store.Contribution{}, nil, fmt.Errorf("%w: %s %q: %v", ErrInvalidInput, ai.Kind, ai.Ref, err)
		}
		a.ValidFrom, a.ValidTo = ai.ValidFrom, ai.ValidTo
		if err := a.Validate(); err != nil {
			return store.Contribution{}, nil, fmt.Errorf("%w: %s %q: %v", ErrInvalidInput, ai.Kind, ai.Ref, err)
		}
		for _, ref := range ai.Evidence {
			evID, ok := ids[ref]
			if !ok {
				evID = domain.ID(ref) // existing evidence in the area; the store resolves it
			}
			c.ClaimEvidence[id] = append(c.ClaimEvidence[id], evID)
		}
		linked := map[domain.ID]bool{}
		for _, ref := range ai.DerivedFrom {
			src, ok := ids[ref]
			if !ok {
				src = domain.ID(ref) // an existing assertion in the area; the store checks it
			}
			if linked[src] {
				continue
			}
			linked[src] = true
			c.Derivations = append(c.Derivations, domain.Derivation{DerivedID: id, SourceID: src, AreaID: areaID, Rationale: in.Rationale, RecordedAt: now})
		}
		c.Assertions = append(c.Assertions, a)
		refs[id] = ai.Ref
	}
	return c, refs, nil
}

// output shapes the result, reading the configuration the scores were
// projected under to report whether each claim meets its threshold.
func (s *Service) output(ctx context.Context, caller store.Caller, areaID domain.ID, areaKey string, res *store.ContributionResult, replayed bool) (wire.ContributeOutput, error) {
	cfg, err := s.pool.SupportConfig(ctx, caller, areaID, res.ConfigVersion)
	if err != nil {
		return wire.ContributeOutput{}, err
	}
	out := wire.ContributeOutput{ContributionID: string(res.ContributionID), Area: areaKey, ConfigVersion: res.ConfigVersion, Replayed: replayed}
	for _, w := range res.Written {
		rec := wire.Written{Ref: w.Ref, ID: string(w.ID), RecordType: w.RecordType, LifecycleState: store.LifecycleActive}
		if w.RecordType == store.RecordAssertion {
			kind := domain.EpistemicKind(w.Kind)
			if kind == "" {
				kind = domain.KindObservation // results stored before kinds were recorded held observations only
			}
			rec.Kind = string(kind)
			score := res.Scores[w.ID]
			rec.Support = &wire.Support{
				Score: score, ConfigVersion: res.ConfigVersion, Roots: res.Roots[w.ID], Events: res.Roots[w.ID],
				MeetsThreshold: cfg.MeetsThreshold(kind, score),
			}
		}
		if w.RecordType == store.RecordEvidence {
			rec.LifecycleState = ""
		}
		out.Records = append(out.Records, rec)
	}
	return out, nil
}

func validateInput(in wire.ContributeInput) error {
	if strings.TrimSpace(in.Area) == "" {
		return fmt.Errorf("%w: area is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return fmt.Errorf("%w: idempotency_key is required", ErrInvalidInput)
	}
	claims := assertionsOf(in)
	if len(in.Entities)+len(in.Evidence)+len(claims) == 0 {
		return fmt.Errorf("%w: nothing to contribute", ErrInvalidInput)
	}
	seen := map[string]bool{}
	ref := func(kind, r string) error {
		if strings.TrimSpace(r) == "" {
			return fmt.Errorf("%w: every %s needs a ref", ErrInvalidInput, kind)
		}
		if seen[r] {
			return fmt.Errorf("%w: ref %q is used twice", ErrInvalidInput, r)
		}
		seen[r] = true
		return nil
	}
	for _, e := range in.Entities {
		if err := ref("entity", e.Ref); err != nil {
			return err
		}
		if len(e.Names) == 0 {
			return fmt.Errorf("%w: entity %q has no names", ErrInvalidInput, e.Ref)
		}
	}
	for _, ev := range in.Evidence {
		if err := ref("evidence", ev.Ref); err != nil {
			return err
		}
	}
	otherRefs := make(map[string]bool, len(seen))
	for r := range seen {
		otherRefs[r] = true
	}
	for _, a := range claims {
		if err := ref("assertion", a.Ref); err != nil {
			return err
		}
	}
	ids := make([]domain.ID, 0, len(claims))
	var links []domain.Derivation
	for _, a := range claims {
		kind, err := domain.ParseEpistemicKind(a.Kind)
		if err != nil {
			return fmt.Errorf("%w: assertion %q: kind %q is not observation, decision or inference", ErrInvalidInput, a.Ref, a.Kind)
		}
		if strings.TrimSpace(a.Subject) == "" {
			return fmt.Errorf("%w: %s %q has no subject", ErrInvalidInput, kind, a.Ref)
		}
		switch err := kind.CheckGrounds(len(a.Evidence), len(a.DerivedFrom)); {
		case errors.Is(err, domain.ErrNoEvidence):
			return fmt.Errorf("%w: observation %q cites no evidence; every claim needs shareable evidence of its own (PR-43)", ErrInvalidInput, a.Ref)
		case errors.Is(err, domain.ErrObservationDerived):
			return fmt.Errorf("%w: observation %q names derived_from; an observation is what was seen, so record what was concluded as an inference (or a decision) instead", ErrInvalidInput, a.Ref)
		case errors.Is(err, domain.ErrNoGrounds):
			return fmt.Errorf("%w: %s %q cites no evidence and names no derived_from; cite evidence, name the assertions it was derived from, or both", ErrInvalidInput, kind, a.Ref)
		case err != nil:
			return fmt.Errorf("%w: %s %q: %v", ErrInvalidInput, kind, a.Ref, err)
		}
		ids = append(ids, domain.ID(a.Ref))
		for _, src := range a.DerivedFrom {
			if otherRefs[src] {
				return fmt.Errorf("%w: %s %q is derived from %q, which is an entity or evidence ref; derive from assertions and cite evidence under evidence", ErrInvalidInput, kind, a.Ref, src)
			}
			if seen[src] {
				links = append(links, domain.Derivation{DerivedID: domain.ID(a.Ref), SourceID: domain.ID(src)})
			}
		}
	}
	if _, err := domain.OrderByDerivation(ids, links); err != nil {
		return fmt.Errorf("%w: derived_from refs form a %v; a record cannot be derived, directly or through others, from itself", ErrInvalidInput, err)
	}
	return nil
}

// assertionsOf returns every assertion of the call: the slice 1
// observations as assertions of kind observation, then the assertions.
func assertionsOf(in wire.ContributeInput) []wire.AssertionInput {
	out := make([]wire.AssertionInput, 0, len(in.Observations)+len(in.Assertions))
	for _, o := range in.Observations {
		out = append(out, wire.AssertionInput{
			Ref: o.Ref, Kind: string(domain.KindObservation), Subject: o.Subject, BodyMarkdown: o.BodyMarkdown, Scope: o.Scope,
			ValidFrom: o.ValidFrom, ValidTo: o.ValidTo, Evidence: o.Evidence,
		})
	}
	return append(out, in.Assertions...)
}

// requestHash fingerprints the change set without its key, so the same key
// with a different request is detectable.
func requestHash(in wire.ContributeInput) string {
	in.IdempotencyKey = ""
	body, _ := json.Marshal(in)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func mapStoreErr(err error) error {
	switch {
	case errors.Is(err, store.ErrDuplicateEvidence):
		return fmt.Errorf("%w: the area already holds evidence for that source, revision and locator; cite its id instead of recording it again", ErrInvalidInput)
	case errors.Is(err, store.ErrCrossArea):
		return fmt.Errorf("%w: derived_from names an assertion in another knowledge area; derive only from assertions in the area you contribute to (%v)", ErrInvalidInput, err)
	case errors.Is(err, domain.ErrDerivationCycle):
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	case errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("%w: a referenced record was not found in the area (%v)", ErrInvalidInput, err)
	}
	return err
}

func roleNames(roles []authz.Role) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, string(r))
	}
	return out
}

func nameFromWire(n wire.Name) domain.EntityName {
	out := domain.EntityName{Name: n.Name, ValidFrom: n.ValidFrom, ValidTo: n.ValidTo}
	if n.Scope != nil {
		out.Scope = scopeFromWire(*n.Scope)
	}
	return out
}

func scopeFromWire(s wire.Scope) domain.Scope {
	return domain.Scope{Environment: s.Environment, Region: s.Region, VersionMin: s.VersionMin, VersionMax: s.VersionMax, Cluster: s.Cluster, Tenant: s.Tenant}
}
