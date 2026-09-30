package exports

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	exporthandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
)

// RegisterRoutes mounts export routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *exporthandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	stepUp *stepup.Service,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}
	var requireStepUp func(http.Handler) http.Handler
	if stepUp != nil {
		requireStepUp = middleware.RequireStepUp(stepUp)
	} else {
		requireStepUp = func(next http.Handler) http.Handler { return next }
	}

	mux.Handle("GET /v1/platform/exports", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformExportsRead),
	))
	mux.Handle("GET /v1/platform/exports/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformExportsRead),
	))
	mux.Handle("GET /v1/platform/exports/{uuid}/download", middleware.Chain(
		http.HandlerFunc(h.Download), authn, require(rbac.PermPlatformExportsRead),
	))

	mux.Handle("POST /v1/platform/users/export", middleware.Chain(
		http.HandlerFunc(h.RequestUsersExport), authn, require(rbac.PermPlatformUsersExport), requireStepUp,
	))
	mux.Handle("POST /v1/platform/roles/export", middleware.Chain(
		http.HandlerFunc(h.RequestRolesExport), authn, require(rbac.PermPlatformRolesExport), requireStepUp,
	))
	mux.Handle("POST /v1/platform/notifications/export", middleware.Chain(
		http.HandlerFunc(h.RequestNotificationsExport), authn, require(rbac.PermPlatformNotificationsExport), requireStepUp,
	))
	mux.Handle("POST /v1/platform/activity/export", middleware.Chain(
		http.HandlerFunc(h.RequestActivityExport), authn, require(rbac.PermPlatformActivityRead), requireStepUp,
	))

	// Tenant export jobs: business modules add their own
	// POST /v1/tenant/<resource>/export routes next to their permissions.
	requireOrg := middleware.RequireOrganization(tokens, q)
	requireOwner := middleware.RequireOrgRole("owner")
	tenantExport := func(handler http.HandlerFunc) http.Handler {
		return middleware.Chain(http.HandlerFunc(handler), authn, requireOrg, requireOwner)
	}
	mux.Handle("GET /v1/tenant/exports", tenantExport(h.ListTenant))
	mux.Handle("GET /v1/tenant/exports/{uuid}", tenantExport(h.GetTenant))
	mux.Handle("GET /v1/tenant/exports/{uuid}/download", tenantExport(h.DownloadTenant))
}
