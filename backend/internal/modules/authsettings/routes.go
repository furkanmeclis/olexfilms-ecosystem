package authsettings

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	authsettingshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/authsettings/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts auth settings routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *authsettingshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}
	superAdmin := middleware.RequireSuperAdmin

	mux.Handle("GET /v1/platform/auth/settings", middleware.Chain(
		http.HandlerFunc(h.GetSettings), authn, require(rbac.PermPlatformAuthSettingsRead), superAdmin,
	))
	mux.Handle("PATCH /v1/platform/auth/settings", middleware.Chain(
		http.HandlerFunc(h.PatchSettings), authn, require(rbac.PermPlatformAuthSettingsWrite), superAdmin,
	))
}
