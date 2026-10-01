package middleware

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// RequirePermissionScope allows the request when the principal holds slug
// with a scope that covers need (e.g. a subtree grant covers managed).
func RequirePermissionScope(slug string, need rbac.Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			if !p.Can(slug, need) {
				response.Forbidden(w, r, "Missing permission: "+slug)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireScope checks slug and stores its resolved scopefilter.Filter on the
// context for the handler and repository (scopefilter.From). Place it after
// RequireOrganization on tenant routes.
func RequireScope(tree scopefilter.TreeReader, slug string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			var org *orgctx.Scope
			if s, ok := orgctx.ScopeFrom(r.Context()); ok {
				org = &s
			}
			f, err := scopefilter.Resolve(r.Context(), tree, p, org, slug)
			switch {
			case errors.Is(err, scopefilter.ErrForbidden):
				response.Forbidden(w, r, "Missing permission: "+slug)
				return
			case errors.Is(err, scopefilter.ErrOrganizationRequired):
				response.Error(w, r, http.StatusForbidden, CodeOrganizationContextRequired,
					"Organization context is required")
				return
			case err != nil:
				response.InternalErr(w, r, err, "failed to resolve permission scope")
				return
			}
			next.ServeHTTP(w, r.WithContext(scopefilter.With(r.Context(), f)))
		})
	}
}
