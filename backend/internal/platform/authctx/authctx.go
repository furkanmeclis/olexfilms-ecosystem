package authctx

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type ctxKey int

const (
	keyPrincipal ctxKey = iota + 1
)

// Principal is the authenticated identity attached to a request context.
type Principal struct {
	UserID       uuid.UUID
	UserInternal int64
	Email        string
	// Roles are the global roles plus the roles of the active organization.
	Roles       []string
	Permissions []string
	// PermissionScopes maps each held permission to the broadest scope
	// granted by the user's global roles and the active organization roles.
	PermissionScopes   map[string]rbac.Scope
	IsSuperAdmin       bool
	ImpersonatorUserID *uuid.UUID
	SessionID          uuid.UUID
	OrganizationUUID   *uuid.UUID
	// Realm is the session realm of the access token (jwt aud): "panel" or
	// "portal" (TEC-90).
	Realm string
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

// HasPermission reports whether the principal includes the given permission
// slug. Super admins hold every permission; super_admin-only permissions
// (impersonation) are never satisfied through another role.
func (p Principal) HasPermission(slug string) bool {
	_, ok := p.ScopeFor(slug)
	return ok
}

// ScopeFor returns the broadest scope the principal holds for a permission.
// Super admins hold every permission at its broadest catalog scope. A
// permission listed without a recorded scope counts as its broadest scope.
func (p Principal) ScopeFor(slug string) (rbac.Scope, bool) {
	if p.IsSuperAdmin {
		if def, ok := rbac.PermissionBySlug(slug); ok {
			return rbac.Broadest(def.Scopes), true
		}
		return rbac.ScopeAll, true
	}
	if rbac.SuperAdminOnly(slug) {
		return "", false
	}
	if scope, ok := p.PermissionScopes[slug]; ok {
		return scope, true
	}
	for _, perm := range p.Permissions {
		if perm == slug {
			if def, ok := rbac.PermissionBySlug(slug); ok {
				return rbac.Broadest(def.Scopes), true
			}
			return rbac.ScopeAll, true
		}
	}
	return "", false
}

// Can reports whether the principal holds a permission with a scope that
// covers need (e.g. a subtree grant covers a managed check).
func (p Principal) Can(slug string, need rbac.Scope) bool {
	scope, ok := p.ScopeFor(slug)
	return ok && scope.Covers(need)
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
