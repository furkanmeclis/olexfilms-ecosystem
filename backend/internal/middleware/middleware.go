package middleware

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// IdentityLoader hydrates principal fields (internal ids, permissions) after JWT parse.
type IdentityLoader interface {
	LoadPrincipal(r *http.Request, claims jwt.Claims) (authctx.Principal, error)
}

// Authenticate requires a Bearer access JWT and attaches Principal to context.
func Authenticate(tokens *jwt.Manager, loader IdentityLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				response.Unauthorized(w, r, "Bearer access token is required")
				return
			}
			claims, err := tokens.ParseAccess(raw)
			if err != nil {
				response.Unauthorized(w, r, "Invalid or expired access token")
				return
			}
			principal, err := loader.LoadPrincipal(r, claims)
			if err != nil {
				response.Unauthorized(w, r, "Session is no longer valid")
				return
			}
			next.ServeHTTP(w, r.WithContext(authctx.WithPrincipal(r.Context(), principal)))
		})
	}
}

// RequireSuperAdmin allows only platform super admins.
func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := authctx.PrincipalFrom(r.Context())
		if !ok || !p.IsSuperAdmin {
			response.Forbidden(w, r, "Platform admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireTenant is deprecated; kept as no-op for compatibility during migration cleanup.
func RequireTenant(_ bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return next
	}
}

// RequirePermission checks role_permissions (super_admin bypasses).
func RequirePermission(slug string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			if !p.HasPermission(slug) {
				response.Forbidden(w, r, "Missing permission: "+slug)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission allows the request when the principal has at least one of the slugs.
func RequireAnyPermission(slugs ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			for _, slug := range slugs {
				if p.HasPermission(slug) {
					next.ServeHTTP(w, r)
					return
				}
			}
			response.Forbidden(w, r, "Missing required permission")
		})
	}
}

// Chain applies middleware in order (first wraps outermost).
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// ParseUUID is a small helper for path params.
func ParseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}
