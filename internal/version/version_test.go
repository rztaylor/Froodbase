package version

import (
	"strings"
	"testing"
)

func TestStringIncludesAllFields(t *testing.T) {
	got := String()
	for _, want := range []string{Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Fatalf("String() = %q, missing %q", got, want)
		}
	}
}

func TestCompatible(t *testing.T) {
	tests := []struct {
		client string
		want   bool
	}{
		{"dev", true},
		{"edd3d86-dirty", true},
		{"", true},
		{"v0.0.0", true},
		{"v0.1.0", true},
		{"v1.2.3-rc1", true},
		{"v1.2.3+build", true},
	}
	for _, tt := range tests {
		if got := Compatible(tt.client); got != tt.want {
			t.Errorf("Compatible(%q) = %v, want %v", tt.client, got, tt.want)
		}
	}
}

func TestParseSemver(t *testing.T) {
	good := map[string][3]int{"v0.0.0": {0, 0, 0}, "v1.2.3": {1, 2, 3}, "v10.20.30-rc1": {10, 20, 30}, "v1.0.0+meta": {1, 0, 0}}
	for in, want := range good {
		got, ok := parseSemver(in)
		if !ok || got != want {
			t.Errorf("parseSemver(%q) = %v, %v", in, got, ok)
		}
	}
	for _, in := range []string{"1.2.3", "v1.2", "v1.2.3.4", "v1..2", "vx.y.z", "v", "v1.2.", "dev"} {
		if _, ok := parseSemver(in); ok {
			t.Errorf("parseSemver(%q) accepted", in)
		}
	}
	// Ordering below the minimum is only observable if the minimum moves;
	// check the comparison directly.
	if c, _ := parseSemver("v0.0.1"); c[2] <= 0 {
		t.Error("patch not parsed")
	}
}
