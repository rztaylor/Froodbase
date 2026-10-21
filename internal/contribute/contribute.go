package contribute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	in, err := inferInput(in)
	if err != nil {
		return wire.ContributeOutput{}, err
	}
	if err := validateInput(in); err != nil {
		return wire.ContributeOutput{}, err
	}
	if err := attributeStatements(&in, ac); err != nil {
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
		}.WithDefaults()
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
	c.Links = linksOf(in.Links, ids)
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
			scope = ScopeFromWire(*ai.Scope)
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
			rec.Support = supportOf(res, w.ID, kind, cfg)
		}
		if w.RecordType == store.RecordEvidence {
			rec.LifecycleState = ""
		}
		out.Records = append(out.Records, rec)
	}
	for _, l := range res.Linked {
		evidence := make([]string, len(l.EvidenceIDs))
		for i, id := range l.EvidenceIDs {
			evidence[i] = string(id)
		}
		out.Linked = append(out.Linked, wire.Linked{
			ID: string(l.ID), Kind: l.Kind, Revision: l.Revision, Evidence: evidence, RootsAdded: len(l.RootsAdded),
			Support: supportOf(res, l.ID, domain.EpistemicKind(l.Kind), cfg),
		})
	}
	return out, nil
}

// supportOf is a claim's support as the result reports it. Every event a
// contribution writes is a root_added, so the events are the roots.
func supportOf(res *store.ContributionResult, id domain.ID, kind domain.EpistemicKind, cfg domain.SupportConfig) *wire.Support {
	score := res.Scores[id]
	return &wire.Support{
		Score: score, ConfigVersion: res.ConfigVersion, Roots: res.Roots[id], Events: res.Roots[id],
		RootsBySourceKind: RootWeightsToWire(domain.SummariseRoots(res.RootKinds[id], cfg)), MeetsThreshold: cfg.MeetsThreshold(kind, score),
	}
}

// linksOf translates evidence for existing claims into the store's links:
// one per claim, in the order first named, evidence refs from this call
// resolved to their new ids and each evidence id once.
func linksOf(in []wire.LinkInput, ids map[string]domain.ID) []store.EvidenceLink {
	var out []store.EvidenceLink
	at := map[domain.ID]int{}
	for _, l := range in {
		claim := domain.ID(strings.TrimSpace(l.Claim))
		i, ok := at[claim]
		if !ok {
			i = len(out)
			at[claim] = i
			out = append(out, store.EvidenceLink{ClaimID: claim})
		}
		for _, ref := range l.Evidence {
			id, ok := ids[ref]
			if !ok {
				id = domain.ID(strings.TrimSpace(ref)) // existing evidence in the area; the store resolves it
			}
			if !slices.Contains(out[i].EvidenceIDs, id) {
				out[i].EvidenceIDs = append(out[i].EvidenceIDs, id)
			}
		}
	}
	return out
}

// RootWeightsToWire is the wire form of a claim's roots by source kind.
func RootWeightsToWire(roots []domain.RootKindSupport) []wire.RootWeight {
	if len(roots) == 0 {
		return nil
	}
	out := make([]wire.RootWeight, len(roots))
	for i, r := range roots {
		out[i] = wire.RootWeight{SourceKind: string(r.Kind), Roots: r.Roots, Weight: r.Weight}
	}
	return out
}

func validateInput(in wire.ContributeInput) error {
	if strings.TrimSpace(in.Area) == "" {
		return fmt.Errorf("%w: area is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return fmt.Errorf("%w: idempotency_key is required", ErrInvalidInput)
	}
	claims := assertionsOf(in)
	if len(in.Entities)+len(in.Evidence)+len(claims)+len(in.Links) == 0 {
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
		for _, n := range e.Names {
			if err := checkTemporal(n.Scope, n.ValidFrom, n.ValidTo); err != nil {
				return fmt.Errorf("%w: entity %q: name %q: %v", ErrInvalidInput, e.Ref, n.Name, err)
			}
		}
	}
	evidenceRefs := map[string]bool{}
	for _, ev := range in.Evidence {
		if err := ref("evidence", ev.Ref); err != nil {
			return err
		}
		evidenceRefs[ev.Ref] = true
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
		if err := checkTemporal(a.Scope, a.ValidFrom, a.ValidTo); err != nil {
			return fmt.Errorf("%w: %s %q: %v", ErrInvalidInput, kind, a.Ref, err)
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
			} else if _, err := uuid.Parse(src); err != nil {
				return fmt.Errorf("%w: %s %q is derived from %q, which is neither the ref of an assertion in this call nor an assertion id", ErrInvalidInput, kind, a.Ref, src)
			}
		}
	}
	if _, err := domain.OrderByDerivation(ids, links); err != nil {
		return fmt.Errorf("%w: derived_from refs loop (%v); a record cannot be derived, directly or through others, from itself", ErrInvalidInput, err)
	}
	return validateLinks(in.Links, seen, evidenceRefs)
}

// checkTemporal checks the scope and validity a contributor gave a claim or
// a name (TR-26, TR-30): version bounds must be versions and in order, and
// a validity window must end after it starts. Both are optional and nothing
// is filled in when they are left out: dates nobody gave stay unknown, and
// are never taken from the recording time (TR-32). Stored scopes are not
// re-checked when read, so records written before this check still load.
func checkTemporal(scope *wire.Scope, from, to *time.Time) error {
	if scope != nil {
		if err := ScopeFromWire(*scope).Validate(); err != nil {
			return fmt.Errorf("scope: version_min and version_max must be versions such as 2.3 or v2.4.1, with version_min not above version_max (got %q to %q)", scope.VersionMin, scope.VersionMax)
		}
	}
	if from != nil && to != nil && !from.Before(*to) {
		return fmt.Errorf("valid_to (%s) must be after valid_from (%s); leave out a date you do not know", to.Format(time.RFC3339), from.Format(time.RFC3339))
	}
	return nil
}

// validateLinks checks evidence for existing claims: each names an
// assertion id (not a ref in this call, whose evidence belongs in its own
// list) and cites evidence refs from this call or evidence ids.
func validateLinks(links []wire.LinkInput, refs, evidenceRefs map[string]bool) error {
	for i, l := range links {
		claim := strings.TrimSpace(l.Claim)
		switch {
		case claim == "":
			return fmt.Errorf("%w: link %d names no claim; claim is the id of the recorded assertion to add evidence to", ErrInvalidInput, i+1)
		case refs[claim]:
			return fmt.Errorf("%w: link %d names claim %q, which is a ref in this call; links add evidence to claims already recorded, so cite the evidence in that assertion's own evidence list instead", ErrInvalidInput, i+1, claim)
		}
		if _, err := uuid.Parse(claim); err != nil {
			return fmt.Errorf("%w: link %d names claim %q, which is not the id of an assertion; search for the recorded claim and use its id", ErrInvalidInput, i+1, claim)
		}
		if len(l.Evidence) == 0 {
			return fmt.Errorf("%w: link %d to claim %s cites no evidence", ErrInvalidInput, i+1, claim)
		}
		for _, ev := range l.Evidence {
			if evidenceRefs[ev] {
				continue
			}
			if _, err := uuid.Parse(strings.TrimSpace(ev)); refs[ev] || err != nil {
				return fmt.Errorf("%w: link %d to claim %s cites %q, which is neither the ref of evidence in this call nor an evidence id", ErrInvalidInput, i+1, claim, ev)
			}
		}
	}
	return nil
}

// attributeStatements checks that every statement in the call is the
// caller's own: a person can only record what they themself said, so its
// source is person://<the caller's user id>, filled in when left empty. A
// call with no person behind it (an agent acting alone) cannot record a
// statement at all.
func attributeStatements(in *wire.ContributeInput, ac *authz.Context) error {
	in.Evidence = append([]wire.EvidenceInput(nil), in.Evidence...) // never write into the caller's slice
	for i, ev := range in.Evidence {
		if ev.SourceKind != string(domain.SourceStatement) {
			continue
		}
		if strings.TrimSpace(ac.UserID) == "" {
			return fmt.Errorf("%w: statement %q: there is no person behind this call; a statement records what a named person said, so an agent acting alone cannot record one", ErrInvalidInput, ev.Ref)
		}
		if strings.TrimSpace(ev.SourceURI) == "" {
			in.Evidence[i].SourceURI = domain.StatementURI(ac.UserID)
			continue
		}
		person, err := domain.StatementPerson(ev.SourceURI)
		if err != nil {
			return fmt.Errorf("%w: statement %q: source_uri must be person://<your user id> (or empty to use yours)", ErrInvalidInput, ev.Ref)
		}
		if string(person) != ac.UserID {
			return fmt.Errorf("%w: statement %q is attributed to %q, but you are %q: a person can only record their own statements; cite where %s wrote it down (a ticket or document) instead", ErrInvalidInput, ev.Ref, person, ac.UserID, person)
		}
		in.Evidence[i].SourceURI = domain.StatementURI(ac.UserID) // one spelling per person
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
		return fmt.Errorf("%w: the call names a record in another knowledge area; derive from, cite and add evidence to records in the area you contribute to only (%v)", ErrInvalidInput, err)
	case errors.Is(err, store.ErrInvalidLink):
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
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
		out.Scope = ScopeFromWire(*n.Scope)
	}
	return out
}

// ScopeFromWire is the domain form of a wire scope.
func ScopeFromWire(s wire.Scope) domain.Scope {
	return domain.Scope{Environment: s.Environment, Region: s.Region, VersionMin: s.VersionMin, VersionMax: s.VersionMax, Cluster: s.Cluster, Tenant: s.Tenant}
}
