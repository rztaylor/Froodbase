package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Derivation links a deduction (a decision or an inference) to one assertion
// it was derived from (PR-13). The deduction stays labelled derived and
// starts with the distinct evidence roots of its sources (PR-12, PR-42). A
// derivation is written with the deduction and never changes; both records
// live in the same area.
type Derivation struct {
	DerivedID  ID
	SourceID   ID
	AreaID     ID
	Rationale  string
	RecordedAt time.Time
}

// Validate checks the identifiers, that a record is not derived from itself
// and the recording time.
func (d Derivation) Validate() error {
	if err := d.DerivedID.Validate(); err != nil {
		return fieldErr("derived id", err)
	}
	if err := d.SourceID.Validate(); err != nil {
		return fieldErr("source id", err)
	}
	if err := d.AreaID.Validate(); err != nil {
		return fieldErr("area id", err)
	}
	if d.DerivedID == d.SourceID {
		return fmt.Errorf("%s derived from itself: %w", d.DerivedID, ErrDerivationCycle)
	}
	if d.RecordedAt.IsZero() {
		return fieldErr("recorded_at", ErrNoRecordedAt)
	}
	return nil
}

// OrderByDerivation orders records written together so that every source
// among them comes before each record derived from it, keeping the given
// order otherwise. Sources outside ids (records that already exist) impose
// no order. It returns ErrDerivationCycle when records among ids derive from
// each other in a loop, and ErrUnknownDerived when a link's derived record is
// not in ids.
func OrderByDerivation(ids []ID, links []Derivation) ([]ID, error) {
	pending := make(map[ID]bool, len(ids))
	for _, id := range ids {
		pending[id] = true
	}
	sources := make(map[ID][]ID, len(links))
	for _, l := range links {
		if !pending[l.DerivedID] {
			return nil, fmt.Errorf("%s: %w", l.DerivedID, ErrUnknownDerived)
		}
		sources[l.DerivedID] = append(sources[l.DerivedID], l.SourceID)
	}
	ready := func(id ID) bool {
		for _, s := range sources[id] {
			if pending[s] {
				return false
			}
		}
		return true
	}
	out := make([]ID, 0, len(ids))
	for len(out) < len(ids) {
		progressed := false
		for _, id := range ids {
			if pending[id] && ready(id) {
				out = append(out, id)
				pending[id] = false
				progressed = true
			}
		}
		if !progressed {
			var stuck []string
			for _, id := range ids {
				if pending[id] {
					stuck = append(stuck, string(id))
				}
			}
			return nil, fmt.Errorf("%w involving %s", ErrDerivationCycle, strings.Join(stuck, ", "))
		}
	}
	return out, nil
}

// RootOrigin is one distinct evidence root of a claim and where it came
// from: cited directly by the claim's own evidence, inherited from the
// sources it was derived from, or both.
type RootOrigin struct {
	Root    ID
	Direct  bool
	Sources []ID // the sources it was inherited from, sorted
}

// Rationale is the reason recorded on the root's root_added event: the
// contribution's rationale for a root cited directly, otherwise the sources
// it was inherited from.
func (o RootOrigin) Rationale(contribution string) string {
	if o.Direct || len(o.Sources) == 0 {
		return contribution
	}
	names := make([]string, len(o.Sources))
	for i, s := range o.Sources {
		names[i] = string(s)
	}
	return "inherited from " + strings.Join(names, ", ")
}

// DerivedRoots returns the distinct evidence roots of a claim, sorted: the
// union of the roots of its direct evidence (resolved as DistinctRoots
// resolves them) and the roots of each source it was derived from
// (sourceRoots maps a source id to its distinct roots). A root shared by
// several sources, or both inherited and cited, counts once (PR-12, TR-25).
func DerivedRoots(direct []Evidence, sourceRoots map[ID][]ID) []RootOrigin {
	byRoot := map[ID]*RootOrigin{}
	origin := func(root ID) *RootOrigin {
		o, ok := byRoot[root]
		if !ok {
			o = &RootOrigin{Root: root}
			byRoot[root] = o
		}
		return o
	}
	for _, r := range DistinctRoots(direct) {
		origin(r).Direct = true
	}
	for src, roots := range sourceRoots {
		for _, r := range roots {
			o := origin(r)
			if len(o.Sources) == 0 || o.Sources[len(o.Sources)-1] != src {
				o.Sources = append(o.Sources, src)
			}
		}
	}
	if len(byRoot) == 0 {
		return nil
	}
	out := make([]RootOrigin, 0, len(byRoot))
	for _, o := range byRoot {
		sort.Slice(o.Sources, func(i, j int) bool { return o.Sources[i] < o.Sources[j] })
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out
}
