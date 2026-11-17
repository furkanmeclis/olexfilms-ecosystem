// Package brandcurrency mounts the brand currency routes (TEC-1214).
package brandcurrency

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/brandcurrency/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the routes: listing needs
// platform.organizations.read; a change or a price conversion (dry run
// included) is for platform admins only.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	mux.Handle("GET /v1/platform/brands", middleware.Chain(http.HandlerFunc(h.List), authn,
		middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)))
	admin := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequireSuperAdmin)
	}
	mux.Handle("PUT /v1/platform/brands/{uuid}/currency", admin(h.ChangeCurrency))
	mux.Handle("POST /v1/platform/brands/{uuid}/price-conversion", admin(h.ConvertPrices))
}
