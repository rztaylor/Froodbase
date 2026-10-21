package domain

import (
	"strings"
)

// Scope is where a fact applies: the fixed dimensions of the architecture's
// scope object (TR-26). Each dimension is either specified or explicitly
// unspecified, written as an empty (or blank) string, which matches
// anything. VersionMin and VersionMax bound an inclusive version range; a
// missing bound is open, so a fact about one version gives both.
type Scope struct {
	Environment string
	Region      string
	VersionMin  string
	VersionMax  string
	Cluster     string
	Tenant      string
}

// ScopeKey names one scope dimension. The keys are the fixed keys of the
// stored scope object; a missing key means unspecified.
type ScopeKey string

// The six scope keys.
const (
	ScopeEnvironment ScopeKey = "environment"
	ScopeRegion      ScopeKey = "region"
	ScopeVersionMin  ScopeKey = "version_min"
	ScopeVersionMax  ScopeKey = "version_max"
	ScopeCluster     ScopeKey = "cluster"
	ScopeTenant      ScopeKey = "tenant"
)

// ScopeKeys lists the keys in a stable order.
func ScopeKeys() []ScopeKey {
	return []ScopeKey{ScopeEnvironment, ScopeRegion, ScopeVersionMin, ScopeVersionMax, ScopeCluster, ScopeTenant}
}

// Value returns the dimension's value as given, or "" for an unknown key.
func (s Scope) Value(k ScopeKey) string {
	switch k {
	case ScopeEnvironment:
		return s.Environment
	case ScopeRegion:
		return s.Region
	case ScopeVersionMin:
		return s.VersionMin
	case ScopeVersionMax:
		return s.VersionMax
	case ScopeCluster:
		return s.Cluster
	case ScopeTenant:
		return s.Tenant
	}
	return ""
}

// Specified reports whether the dimension has a value; blank is unspecified.
func (s Scope) Specified(k ScopeKey) bool {
	return strings.TrimSpace(s.Value(k)) != ""
}

// IsUnspecified reports whether no dimension is specified.
func (s Scope) IsUnspecified() bool {
	for _, k := range ScopeKeys() {
		if s.Specified(k) {
			return false
		}
	}
	return true
}

// Validate checks that each version bound is a version (an optional "v"
// followed by dot-separated whole numbers, such as 2.3 or v2.4.1) and that
// the range is not inverted.
func (s Scope) Validate() error {
	lo, loOK := versionBound(s.VersionMin)
	hi, hiOK := versionBound(s.VersionMax)
	if (s.Specified(ScopeVersionMin) && !loOK) || (s.Specified(ScopeVersionMax) && !hiOK) {
		return ErrInvalidScopeVersion
	}
	if loOK && hiOK && compareVersions(lo, hi) > 0 {
		return ErrInvalidScopeVersion
	}
	return nil
}

// Overlaps reports whether two scopes can describe the same place (TR-26).
// It is symmetric and is the one definition the conflict classifier and
// the mode filter share (D3). Every dimension must overlap: an unspecified
// name matches anything and two specified names must be equal, ignoring case
// and surrounding spaces; version ranges overlap when neither lies wholly
// above the other. A bound that is not a version cannot rule anything out,
// so it is treated as open.
func (s Scope) Overlaps(o Scope) bool {
	for _, k := range []ScopeKey{ScopeEnvironment, ScopeRegion, ScopeCluster, ScopeTenant} {
		if s.Specified(k) && o.Specified(k) && !strings.EqualFold(strings.TrimSpace(s.Value(k)), strings.TrimSpace(o.Value(k))) {
			return false
		}
	}
	return !above(s.VersionMin, o.VersionMax) && !above(o.VersionMin, s.VersionMax)
}

// above reports whether the lower bound lo lies above the upper bound hi,
// both readable.
func above(lo, hi string) bool {
	l, lok := versionBound(lo)
	h, hok := versionBound(hi)
	return lok && hok && compareVersions(l, h) > 0
}

// versionBound parses a version: an optional leading "v" or "V", then one
// or more dot-separated runs of digits. It returns the segments without
// leading zeros, and false when s is blank or not a version.
func versionBound(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		s = s[1:]
	}
	if s == "" {
		return nil, false
	}
	segs := strings.Split(s, ".")
	for i, seg := range segs {
		if seg == "" {
			return nil, false
		}
		for _, r := range seg {
			if r < '0' || r > '9' {
				return nil, false
			}
		}
		if seg = strings.TrimLeft(seg, "0"); seg == "" {
			seg = "0"
		}
		segs[i] = seg
	}
	return segs, true
}

// compareVersions orders two parsed versions segment by segment, numerically;
// missing trailing segments count as zero, so 2.4 equals 2.4.0.
func compareVersions(a, b []string) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := "0", "0"
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if len(x) != len(y) {
			if len(x) < len(y) {
				return -1
			}
			return 1
		}
		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}
