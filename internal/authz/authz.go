package authz

import (
	"context"
	"fmt"
	"sort"
)

// Role is what a caller may do within a knowledge area.
type Role string

// Roles, from least to most permissive. Admin is for operators only and does
// not include the others.
const (
	RoleReader      Role = "reader"
	RoleContributor Role = "contributor"
	RoleReviewer    Role = "reviewer"
	RoleAdmin       Role = "admin"
)

// ParseRole validates a role name.
func ParseRole(s string) (Role, error) {
	switch Role(s) {
	case RoleReader, RoleContributor, RoleReviewer, RoleAdmin:
		return Role(s), nil
	}
	return "", fmt.Errorf("authz: unknown role %q", s)
}

// rank orders the implied roles; admin is outside the chain.
func rank(r Role) int {
	switch r {
	case RoleReader:
		return 1
	case RoleContributor:
		return 2
	case RoleReviewer:
		return 3
	}
	return 0
}

// Principal is an authenticated identity as a transport reports it: a stable
// subject (the user id) and optional claims such as group memberships. It
// says nothing about how the caller was authenticated.
type Principal struct {
	Subject string
	Claims  map[string][]string
}

// Context is the immutable authorisation context of one request.
type Context struct {
	// UserID is the human caller's stable identifier.
	UserID string
	// DisplayName is the caller's name for display and audit.
	DisplayName string
	// AgentID identifies the agent acting for the caller, or is empty.
	AgentID string
	// Grants maps a knowledge area id to the roles held there.
	Grants map[string][]Role
}

// Builder turns an authenticated Principal into a Context, or reports that
// the principal is unknown.
type Builder interface {
	Build(ctx context.Context, p Principal) (*Context, error)
}

// Allows reports whether the context holds role in area, counting implied
// roles: reviewer allows contributor and reader, contributor allows reader.
// Admin is only allowed by an explicit admin grant.
func (c *Context) Allows(area string, role Role) bool {
	if c == nil {
		return false
	}
	for _, held := range c.Grants[area] {
		if held == role {
			return true
		}
		if role != RoleAdmin && held != RoleAdmin && rank(held) >= rank(role) {
			return true
		}
	}
	return false
}

// Areas lists the areas with at least one grant, sorted.
func (c *Context) Areas() []string {
	if c == nil {
		return nil
	}
	areas := make([]string, 0, len(c.Grants))
	for a := range c.Grants {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	return areas
}

type ctxKey struct{}

// WithContext attaches the authorisation context to a request context.
func WithContext(ctx context.Context, ac *Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, ac)
}

// FromContext returns the attached authorisation context, or nil.
func FromContext(ctx context.Context) *Context {
	ac, _ := ctx.Value(ctxKey{}).(*Context)
	return ac
}
