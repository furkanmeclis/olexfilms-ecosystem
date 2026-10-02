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
	sys *settingshandler.SystemHandler,
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

	// TEC-215: global system settings store (catalog-validated JSON values).
	mux.Handle("GET /v1/platform/system-settings", middleware.Chain(
		http.HandlerFunc(sys.List), authn, require(rbac.PermPlatformSettingsRead),
	))
	mux.Handle("GET /v1/platform/system-settings/{key}", middleware.Chain(
		http.HandlerFunc(sys.Get), authn, require(rbac.PermPlatformSettingsRead),
	))
	mux.Handle("PUT /v1/platform/system-settings/{key}", middleware.Chain(
		http.HandlerFunc(sys.Put), authn, require(rbac.PermPlatformSettingsWrite),
	))
	mux.Handle("DELETE /v1/platform/system-settings/{key}", middleware.Chain(
		http.HandlerFunc(sys.Reset), authn, require(rbac.PermPlatformSettingsWrite),
	))
}
