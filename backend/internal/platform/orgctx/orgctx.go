package orgctx

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const keyScope ctxKey = 1

// Scope is the active organization context for tenant-scoped routes.
type Scope struct {
	InternalID int64
	UUID       uuid.UUID
	Slug       string
	Name       string
	MemberRole string
}

// WithScope stores organization scope on the context.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, keyScope, s)
}

// ScopeFrom returns organization scope when present.
func ScopeFrom(ctx context.Context) (Scope, bool) {
	s, ok := ctx.Value(keyScope).(Scope)
	return s, ok
}

// MustScope returns scope or panics (handlers behind RequireOrganization).
func MustScope(ctx context.Context) Scope {
	s, ok := ScopeFrom(ctx)
	if !ok {
		panic("orgctx: organization scope missing from context")
	}
	return s
}
