package domain_test

import (
	"errors"
	"testing"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/domain/domaintest"
)

// The shared overlap table, in both orders: overlap is symmetric.
func TestScopeOverlaps(t *testing.T) {
	for _, tc := range domaintest.OverlapCases() {
		t.Run(tc.Name, func(t *testing.T) {
			if got := tc.A.Overlaps(tc.B); got != tc.Want {
				t.Errorf("%+v.Overlaps(%+v) = %v, want %v", tc.A, tc.B, got, tc.Want)
			}
			if got := tc.B.Overlaps(tc.A); got != tc.Want {
				t.Errorf("%+v.Overlaps(%+v) = %v, want %v (reversed)", tc.B, tc.A, got, tc.Want)
			}
		})
	}
}

// Every key has a case for each combination of specified and unspecified.
func TestOverlapCasesCoverEveryKey(t *testing.T) {
	cases := domaintest.OverlapCases()
	for _, k := range domain.ScopeKeys() {
		patterns := map[[2]bool]bool{}
		for _, tc := range cases {
			patterns[[2]bool{tc.A.Specified(k), tc.B.Specified(k)}] = true
		}
		if len(patterns) != 4 {
			t.Errorf("key %s: overlap cases cover %d of the 4 specified/unspecified combinations", k, len(patterns))
		}
	}
}

func TestScopeKeys(t *testing.T) {
	s := domain.Scope{Environment: "e", Region: "r", VersionMin: "1", VersionMax: "2", Cluster: "c", Tenant: "t"}
	want := map[domain.ScopeKey]string{
		domain.ScopeEnvironment: "e", domain.ScopeRegion: "r", domain.ScopeVersionMin: "1",
		domain.ScopeVersionMax: "2", domain.ScopeCluster: "c", domain.ScopeTenant: "t",
	}
	keys := domain.ScopeKeys()
	if len(keys) != 6 {
		t.Fatalf("ScopeKeys() = %v, want six keys", keys)
	}
	for _, k := range keys {
		if got := s.Value(k); got != want[k] {
			t.Errorf("Value(%s) = %q, want %q", k, got, want[k])
		}
		if !s.Specified(k) || (domain.Scope{}).Specified(k) {
			t.Errorf("Specified(%s) wrong", k)
		}
	}
	if (domain.Scope{}).Value("colour") != "" {
		t.Error("an unknown key should have no value")
	}
}

func TestScopeIsUnspecifiedTreatsBlankAsUnspecified(t *testing.T) {
	if !(domain.Scope{Environment: "  ", Tenant: "\t"}).IsUnspecified() {
		t.Error("a scope of blank values should be unspecified")
	}
}

func TestScopeValidate(t *testing.T) {
	tests := []struct {
		name string
		s    domain.Scope
		want error
	}{
		{"unspecified", domain.Scope{}, nil},
		{"names only", domain.Scope{Environment: "prod", Region: "uksouth", Cluster: "c1", Tenant: "t1"}, nil},
		{"one version", domain.Scope{VersionMin: "2.3", VersionMax: "2.3"}, nil},
		{"open range", domain.Scope{VersionMin: "v2.3.1"}, nil},
		{"range", domain.Scope{VersionMin: "2.9", VersionMax: "2.10"}, nil},
		{"equal with trailing zero", domain.Scope{VersionMin: "2.4.0", VersionMax: "2.4"}, nil},
		{"inverted range", domain.Scope{VersionMin: "2.10", VersionMax: "2.9"}, domain.ErrInvalidScopeVersion},
		{"pre-release suffix", domain.Scope{VersionMin: "2.4-beta"}, domain.ErrInvalidScopeVersion},
		{"word", domain.Scope{VersionMax: "latest"}, domain.ErrInvalidScopeVersion},
		{"empty segment", domain.Scope{VersionMin: "2..4"}, domain.ErrInvalidScopeVersion},
		{"bare v", domain.Scope{VersionMin: "v"}, domain.ErrInvalidScopeVersion},
		{"two prefixes", domain.Scope{VersionMin: "vV2.3"}, domain.ErrInvalidScopeVersion},
		{"capital V", domain.Scope{VersionMin: "V2.3"}, nil},
		{"spaces around", domain.Scope{VersionMin: " 2.3 "}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.s.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
