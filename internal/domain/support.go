package domain

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

// SupportEventType names one of the append-only events that earn or cost a
// claim support (PR-15, TR-29). Retrieval, repetition and model confidence
// are not events: nothing here can be appended by reading a claim.
type SupportEventType string

// The nine event types of TR-29.
const (
	SupportRootAdded           SupportEventType = "root_added"            // a distinct evidence root linked
	SupportVerified            SupportEventType = "verified"              // fresh evidence checked on a date, in a scope
	SupportOutcomeSucceeded    SupportEventType = "outcome_succeeded"     // an evidenced use outcome that worked
	SupportOutcomeFailed       SupportEventType = "outcome_failed"        // an evidenced use outcome that did not
	SupportUseWithoutChallenge SupportEventType = "use_without_challenge" // a bounded periodic credit while in use and unchallenged
	SupportSourceChanged       SupportEventType = "source_changed"        // a cited source changed or vanished
	SupportContradicted        SupportEventType = "contradicted"          // a conflicting record overlaps in scope and time
	SupportFlagged             SupportEventType = "flagged"               // a reviewer flagged the claim for reconsideration
	SupportSupportRetracted    SupportEventType = "support_retracted"     // a supporting record was retracted or superseded
)

// SupportEventTypes lists the event types in a stable order.
func SupportEventTypes() []SupportEventType {
	return []SupportEventType{
		SupportRootAdded, SupportVerified, SupportOutcomeSucceeded, SupportOutcomeFailed, SupportUseWithoutChallenge,
		SupportSourceChanged, SupportContradicted, SupportFlagged, SupportSupportRetracted,
	}
}

// Valid reports whether t is one of the nine event types.
func (t SupportEventType) Valid() bool {
	for _, k := range SupportEventTypes() {
		if k == t {
			return true
		}
	}
	return false
}

// Raises reports whether events of this type may only add support. The
// others may only remove it; quiet use raises, within the cap.
func (t SupportEventType) Raises() bool {
	switch t {
	case SupportRootAdded, SupportVerified, SupportOutcomeSucceeded, SupportUseWithoutChallenge:
		return true
	}
	return false
}

// ParseSupportEventType returns the type named by s, or ErrUnknownSupportEventType.
func ParseSupportEventType(s string) (SupportEventType, error) {
	t := SupportEventType(s)
	if !t.Valid() {
		return "", ErrUnknownSupportEventType
	}
	return t, nil
}

// SupportEvent is one entry in a claim's append-only support history
// (TR-29): what happened, which evidence roots it concerns, where it applies,
// who or which job appended it, the configuration version in force, why, and
// when. It never carries a score: the score is projected from the events.
// A root_added event also carries its root's source kind, which the
// configuration weighs; it is empty on events that predate it.
type SupportEvent struct {
	Type            SupportEventType
	EvidenceRootIDs []ID
	SourceKind      SourceKind // the root's source kind, on a root_added event
	Scope           Scope
	ActorUserID     ID     // the person, when a person or their agent appended it
	ActorAgentID    ID     // the agent acting for them, if any
	JobName         string // the dreamer job, when a job appended it
	ConfigVersion   string // the support configuration version in force
	Rationale       string
	At              time.Time
}

// Validate checks the type, that an actor or a job is named, the
// configuration version and the time, and that a root_added event names
// exactly one root.
func (e SupportEvent) Validate() error {
	if !e.Type.Valid() {
		return fieldErr("support event type", ErrUnknownSupportEventType)
	}
	if e.ActorUserID == "" && strings.TrimSpace(e.JobName) == "" {
		return fieldErr("support event actor", ErrNoSupportActor)
	}
	if strings.TrimSpace(e.ConfigVersion) == "" {
		return fieldErr("support config version", ErrEmptyName)
	}
	if e.At.IsZero() {
		return fieldErr("support event at", ErrNoRecordedAt)
	}
	if e.Type == SupportRootAdded && len(e.EvidenceRootIDs) != 1 {
		return fieldErr("support event roots", ErrNoEvidenceRoot)
	}
	return nil
}

// DefaultSupportConfigVersion is the version of the configuration the
// service ships; an area without its own configuration uses it.
const DefaultSupportConfigVersion = "service-1"

// SupportConfig is an area's versioned support configuration (TR-43): the
// weight of each event type, the weight of a root by its source kind (a
// root_added event earns its event weight times its root's weight, so a
// person's statement can count less than an observed source), the cap on
// quiet-use credit, the threshold per kind below which answers carry
// insufficient_evidence, and who may write.
type SupportConfig struct {
	Version                string
	EventWeights           map[SupportEventType]float64
	RootWeightBySourceKind map[SourceKind]float64
	UseWithoutChallengeCap float64
	ThresholdByKind        map[EpistemicKind]float64
	AllowedContributors    AllowedContributors
}

// AllowedContributors names the contributor classes and source kinds that
// may write to an area (PR-41).
type AllowedContributors struct {
	Classes     []string
	SourceKinds []SourceKind
}

// DefaultStatementRootWeight is the service default weight of a root that is
// a person's statement: half an observed source, so one statement alone does
// not meet an observation's threshold.
const DefaultStatementRootWeight = 0.5

// DefaultSupportConfig returns the configuration the service ships. The
// migrations seed the same values; an area can version its own within these
// rules. Every observed source kind weighs 1, the weight every root had
// before root weights existed.
func DefaultSupportConfig() SupportConfig {
	roots := make(map[SourceKind]float64, len(SourceKinds()))
	for _, k := range SourceKinds() {
		roots[k] = 1
	}
	roots[SourceStatement] = DefaultStatementRootWeight
	return SupportConfig{
		Version: DefaultSupportConfigVersion,
		EventWeights: map[SupportEventType]float64{
			SupportRootAdded:           1,
			SupportVerified:            1,
			SupportOutcomeSucceeded:    1.5,
			SupportOutcomeFailed:       -1.5,
			SupportUseWithoutChallenge: 0.1,
			SupportSourceChanged:       -1,
			SupportContradicted:        -1,
			SupportFlagged:             -1,
			SupportSupportRetracted:    -1,
		},
		RootWeightBySourceKind: roots,
		UseWithoutChallengeCap: 0.5,
		ThresholdByKind: map[EpistemicKind]float64{
			KindObservation: 1,
			KindDecision:    1,
			KindInference:   2,
		},
		AllowedContributors: AllowedContributors{
			Classes:     []string{"person", "agent"},
			SourceKinds: SourceKinds(),
		},
	}
}

// Validate checks the configuration against the service bounds: a version,
// a finite weight for every event type with the sign its type allows, a root
// weight between 0 and 1 for every source kind (one root never counts as
// more than one), a non-negative cap below the inference threshold (quiet use alone never
// carries an inference past it, PR-15), and a non-negative threshold for
// every epistemic kind.
func (c SupportConfig) Validate() error {
	if strings.TrimSpace(c.Version) == "" {
		return fieldErr("support config version", ErrEmptyName)
	}
	for t := range c.EventWeights {
		if !t.Valid() {
			return fieldErr("support config weight "+string(t), ErrInvalidSupportConfig)
		}
	}
	for _, t := range SupportEventTypes() {
		w, ok := c.EventWeights[t]
		if !ok || math.IsNaN(w) || math.IsInf(w, 0) {
			return fieldErr("support config weight "+string(t), ErrInvalidSupportConfig)
		}
		if t.Raises() && w < 0 || !t.Raises() && w > 0 {
			return fieldErr("support config weight "+string(t), ErrInvalidSupportConfig)
		}
	}
	for k := range c.RootWeightBySourceKind {
		if !k.Valid() {
			return fieldErr("support config root weight "+string(k), ErrInvalidSupportConfig)
		}
	}
	for _, k := range SourceKinds() {
		w, ok := c.RootWeightBySourceKind[k]
		if !ok || math.IsNaN(w) || w < 0 || w > 1 {
			return fieldErr("support config root weight "+string(k), ErrInvalidSupportConfig)
		}
	}
	for _, k := range EpistemicKinds() {
		th, ok := c.ThresholdByKind[k]
		if !ok || math.IsNaN(th) || th < 0 {
			return fieldErr("support config threshold "+string(k), ErrInvalidSupportConfig)
		}
	}
	if math.IsNaN(c.UseWithoutChallengeCap) || c.UseWithoutChallengeCap < 0 || c.UseWithoutChallengeCap >= c.ThresholdByKind[KindInference] {
		return fieldErr("support config use_without_challenge_cap", ErrInvalidSupportConfig)
	}
	return nil
}

// MeetsThreshold reports whether score reaches the threshold for kind. A
// kind without a threshold never meets it.
func (c SupportConfig) MeetsThreshold(kind EpistemicKind, score float64) bool {
	th, ok := c.ThresholdByKind[kind]
	return ok && score >= th
}

// RootWeight is the weight of a root of source kind k. A root whose kind
// was not recorded (events that predate root weights) weighs 1, as every
// root did then.
func (c SupportConfig) RootWeight(k SourceKind) float64 {
	if w, ok := c.RootWeightBySourceKind[k]; ok {
		return w
	}
	return 1
}

func (c SupportConfig) clone() SupportConfig {
	out := c
	out.EventWeights = make(map[SupportEventType]float64, len(c.EventWeights))
	for k, v := range c.EventWeights {
		out.EventWeights[k] = v
	}
	out.RootWeightBySourceKind = make(map[SourceKind]float64, len(c.RootWeightBySourceKind))
	for k, v := range c.RootWeightBySourceKind {
		out.RootWeightBySourceKind[k] = v
	}
	out.ThresholdByKind = make(map[EpistemicKind]float64, len(c.ThresholdByKind))
	for k, v := range c.ThresholdByKind {
		out.ThresholdByKind[k] = v
	}
	return out
}

// SupportScore projects a claim's support events to its score under cfg
// (TR-29, TR-43): the sum of the weights of every event, a root_added event
// weighted again by its root's source kind, with the quiet-use
// credit capped at cfg.UseWithoutChallengeCap and the result floored at
// zero. It is deterministic in the events and the configuration, so a stored
// score can be recomputed and explained, and it depends on nothing else: not
// on the order of events, not on how often the claim was retrieved. The
// result is rounded to four decimal places.
func SupportScore(events []SupportEvent, cfg SupportConfig) (float64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	var score, quiet float64
	for _, e := range events {
		w, ok := cfg.EventWeights[e.Type]
		if !ok {
			return 0, fieldErr("support event type "+string(e.Type), ErrUnknownSupportEventType)
		}
		switch e.Type {
		case SupportUseWithoutChallenge:
			quiet += w
			continue
		case SupportRootAdded:
			w *= cfg.RootWeight(e.SourceKind)
		}
		score += w
	}
	score += math.Min(quiet, cfg.UseWithoutChallengeCap)
	if score < 0 {
		score = 0
	}
	return roundScore(score), nil
}

func roundScore(s float64) float64 {
	return math.Round(s*10000) / 10000
}

// RootKindSupport is how many of a claim's roots are of one source kind and
// what each weighs under the configuration.
type RootKindSupport struct {
	Kind   SourceKind
	Roots  int
	Weight float64
}

// RootsBySourceKind summarises a claim's root_added events by source kind,
// in SourceKinds order with roots of no recorded kind last; nil when there
// are none.
func RootsBySourceKind(events []SupportEvent, cfg SupportConfig) []RootKindSupport {
	counts := map[SourceKind]int{}
	for _, e := range events {
		if e.Type == SupportRootAdded {
			counts[e.SourceKind]++
		}
	}
	return SummariseRoots(counts, cfg)
}

// SummariseRoots turns counts of roots per source kind into the summary
// RootsBySourceKind returns, with each kind's weight under cfg.
func SummariseRoots(counts map[SourceKind]int, cfg SupportConfig) []RootKindSupport {
	var out []RootKindSupport
	for _, k := range append(SourceKinds(), "") {
		if n := counts[k]; n > 0 {
			out = append(out, RootKindSupport{Kind: k, Roots: n, Weight: cfg.RootWeight(k)})
		}
	}
	return out
}

// DistinctRoots returns the distinct evidence roots among the given evidence
// in sorted order, resolved as Corroboration resolves them. The writer
// appends one root_added event per entry that the claim has not seen before.
func DistinctRoots(evidence []Evidence) []ID {
	if len(evidence) == 0 {
		return nil
	}
	byID := make(map[ID]Evidence, len(evidence))
	for _, e := range evidence {
		byID[e.ID] = e
	}
	set := make(map[ID]struct{}, len(evidence))
	for _, e := range evidence {
		set[resolveRoot(byID, e)] = struct{}{}
	}
	roots := make([]ID, 0, len(set))
	for r := range set {
		roots = append(roots, r)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	return roots
}

// Sentinel errors of the support model.
var (
	// ErrUnknownSupportEventType means the event type is not one of the nine of TR-29.
	ErrUnknownSupportEventType = errors.New("domain: unknown support event type")
	// ErrNoSupportActor means a support event names neither a person nor a job.
	ErrNoSupportActor = errors.New("domain: support event has no actor or job")
	// ErrNoEvidenceRoot means a root_added event does not name exactly one root.
	ErrNoEvidenceRoot = errors.New("domain: root_added event must name exactly one evidence root")
	// ErrInvalidSupportConfig means a support configuration is outside the service bounds.
	ErrInvalidSupportConfig = errors.New("domain: support configuration is outside the service bounds")
)
