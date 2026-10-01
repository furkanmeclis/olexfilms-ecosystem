package imports

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	importhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *importhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/imports", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("GET /v1/platform/imports/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("PATCH /v1/platform/imports/{uuid}/mapping", middleware.Chain(
		http.HandlerFunc(h.UpdateMapping), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("POST /v1/platform/imports/{uuid}/preview", middleware.Chain(
		http.HandlerFunc(h.Preview), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("POST /v1/platform/imports/{uuid}/confirm", middleware.Chain(
		http.HandlerFunc(h.Confirm), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("POST /v1/platform/imports/{uuid}/rollback", middleware.Chain(
		http.HandlerFunc(h.Rollback), authn, require(rbac.PermPlatformImportsRead),
	))

	mux.Handle("POST /v1/platform/users/import", middleware.Chain(
		http.HandlerFunc(h.UploadUsers), authn, require(rbac.PermPlatformUsersImport),
	))
	mux.Handle("GET /v1/platform/users/import/sample", middleware.Chain(
		http.HandlerFunc(h.SampleUsers), authn, require(rbac.PermPlatformUsersImport),
	))
	mux.Handle("POST /v1/platform/roles/import", middleware.Chain(
		http.HandlerFunc(h.UploadRoles), authn, require(rbac.PermPlatformRolesImport),
	))
	mux.Handle("GET /v1/platform/roles/import/sample", middleware.Chain(
		http.HandlerFunc(h.SampleRoles), authn, require(rbac.PermPlatformRolesImport),
	))

	requireOrg := middleware.RequireOrganization(tokens, q)
	tenantJobs := func(handler http.HandlerFunc) http.Handler {
		return middleware.Chain(http.HandlerFunc(handler), authn, requireOrg, require(rbac.PermTenantImportsRead))
	}
	mux.Handle("GET /v1/tenant/imports", tenantJobs(h.ListTenant))
	mux.Handle("GET /v1/tenant/imports/{uuid}", tenantJobs(h.GetTenant))
	mux.Handle("PATCH /v1/tenant/imports/{uuid}/mapping", tenantJobs(h.UpdateMappingTenant))
	mux.Handle("POST /v1/tenant/imports/{uuid}/preview", tenantJobs(h.PreviewTenant))
	mux.Handle("POST /v1/tenant/imports/{uuid}/confirm", tenantJobs(h.ConfirmTenant))
	mux.Handle("POST /v1/tenant/imports/{uuid}/rollback", tenantJobs(h.RollbackTenant))
}
