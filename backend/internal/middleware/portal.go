package middleware

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// PortalReadOnly reports whether a portal principal is a read-only fleet
// account (TEC-245; design.md roles table "Filo: salt okunur"): it holds
// the fleet role and not the customer role. A user who is both a customer
// and a fleet account keeps the customer's writes on their own records.
func PortalReadOnly(p authctx.Principal) bool {
	return p.HasRole(rbac.RoleFleet) && !p.HasRole(rbac.RoleCustomer)
}

// DenyPortalReadOnly refuses portal write endpoints (vehicle transfer and
// the like) to a fleet session with 403 PORTAL_READ_ONLY; the reads stay
// open. It runs after Authenticate. No migration: the fleet role keeps
// vehicles.read, the check is on the role itself.
func DenyPortalReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := authctx.PrincipalFrom(r.Context())
		if !ok {
			response.Unauthorized(w, r, "Authentication is required")
			return
		}
		if PortalReadOnly(p) {
			response.Error(w, r, http.StatusForbidden, response.CodePortalReadOnly,
				"This account is read only")
			return
		}
		next.ServeHTTP(w, r)
	})
}
