package settings

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	settingshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *settingshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/settings", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformSettingsRead),
	))
	mux.Handle("PATCH /v1/platform/settings", middleware.Chain(
		http.HandlerFunc(h.Patch), authn, require(rbac.PermPlatformSettingsWrite),
	))
	mux.Handle("PUT /v1/platform/settings/logo", middleware.Chain(
		http.HandlerFunc(h.UploadLogo), authn, require(rbac.PermPlatformSettingsWrite),
	))
	mux.Handle("DELETE /v1/platform/settings/logo", middleware.Chain(
		http.HandlerFunc(h.DeleteLogo), authn, require(rbac.PermPlatformSettingsWrite),
	))
	mux.Handle("GET /v1/platform/settings/logo", middleware.Chain(
		http.HandlerFunc(h.StreamLogo), authn,
	))
}
