package authz

import (
	"context"
	"testing"
)

func TestAllows(t *testing.T) {
	ac := &Context{Grants: map[string][]Role{
		"support": {RoleReviewer},
		"arch":    {RoleReader},
		"ops":     {RoleAdmin},
	}}
	tests := []struct {
		area string
		role Role
		want bool
	}{
		{"support", RoleReader, true},
		{"support", RoleContributor, true},
		{"support", RoleReviewer, true},
		{"support", RoleAdmin, false},
		{"arch", RoleReader, true},
		{"arch", RoleContributor, false},
		{"arch", RoleReviewer, false},
		{"ops", RoleAdmin, true},
		{"ops", RoleReader, false},
		{"missing", RoleReader, false},
	}
	for _, tt := range tests {
		if got := ac.Allows(tt.area, tt.role); got != tt.want {
			t.Errorf("Allows(%q, %q) = %v, want %v", tt.area, tt.role, got, tt.want)
		}
	}
	var nilCtx *Context
	if nilCtx.Allows("support", RoleReader) {
		t.Error("nil context allows")
	}
}

func TestParseRole(t *testing.T) {
	for _, s := range []string{"reader", "contributor", "reviewer", "admin"} {
		if _, err := ParseRole(s); err != nil {
			t.Errorf("ParseRole(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "Reader", "owner"} {
		if _, err := ParseRole(s); err == nil {
			t.Errorf("ParseRole(%q) accepted", s)
		}
	}
}

func TestAreasAndContext(t *testing.T) {
	ac := &Context{UserID: "u1", Grants: map[string][]Role{"b": {RoleReader}, "a": {RoleReader}}}
	if got := ac.Areas(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Areas() = %v", got)
	}
	ctx := WithContext(context.Background(), ac)
	if FromContext(ctx) != ac {
		t.Error("FromContext did not return the attached context")
	}
	if FromContext(context.Background()) != nil {
		t.Error("FromContext on a bare context is not nil")
	}
}
