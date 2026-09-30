package access

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	accesshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/access/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts access policy routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *accesshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/access/settings", middleware.Chain(
		http.HandlerFunc(h.GetSettings), authn, require(rbac.PermPlatformAccessRead),
	))
	mux.Handle("PATCH /v1/platform/access/settings", middleware.Chain(
		http.HandlerFunc(h.PatchSettings), authn, require(rbac.PermPlatformAccessWrite),
	))
}
