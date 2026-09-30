package authctx

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const (
	keyPrincipal ctxKey = iota + 1
)

// Principal is the authenticated identity attached to a request context.
type Principal struct {
	UserID             uuid.UUID
	UserInternal       int64
	Email              string
	Roles              []string
	Permissions        []string
	IsSuperAdmin       bool
	ImpersonatorUserID *uuid.UUID
	SessionID          uuid.UUID
	OrganizationUUID   *uuid.UUID
}

// WithPrincipal stores the principal on the context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, keyPrincipal, p)
}

// PrincipalFrom returns the principal if present.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(keyPrincipal).(Principal)
	return p, ok
}

// MustPrincipal returns the principal or panics (for handlers behind Authenticate).
func MustPrincipal(ctx context.Context) Principal {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		panic("authctx: principal missing from context")
	}
	return p
}

// HasPermission reports whether the principal includes the given permission slug.
func (p Principal) HasPermission(slug string) bool {
	if p.IsSuperAdmin {
		return true
	}
	for _, perm := range p.Permissions {
		if perm == slug {
			return true
		}
	}
	return false
}

// HasRole reports whether the principal includes the given role slug.
func (p Principal) HasRole(slug string) bool {
	for _, role := range p.Roles {
		if role == slug {
			return true
		}
	}
	return false
}
