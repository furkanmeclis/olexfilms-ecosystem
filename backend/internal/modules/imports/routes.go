package imports

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	importhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
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
	mux.Handle("GET /v1/platform/imports/{uuid}/error-report", middleware.Chain(
		http.HandlerFunc(h.ErrorReport), authn, require(rbac.PermPlatformImportsRead),
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
	// A record-scoped warehouse barcode job belongs to warehouse.write.
	// Other tenant import jobs continue to require tenant.imports.read.
	tenantRecordJob := func(handler http.HandlerFunc) http.Handler {
		check := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := authctx.MustPrincipal(r.Context())
				org := orgctx.MustScope(r.Context())
				id, err := uuid.Parse(r.PathValue("uuid"))
				if err != nil {
					response.NotFound(w, r, "Import job not found")
					return
				}
				job, err := q.GetImportJobByUUID(r.Context(), id)
				if err != nil || !job.OrganizationID.Valid || job.OrganizationID.Int64 != org.InternalID {
					response.NotFound(w, r, "Import job not found")
					return
				}
				if job.Resource == "tenant.warehouse.entry_barcodes" {
					middleware.RequireScope(q, rbac.PermWarehouseWrite)(next).ServeHTTP(w, r)
					return
				}
				if !p.HasPermission(rbac.PermTenantImportsRead) {
					response.Forbidden(w, r, "Import job permission required")
					return
				}
				next.ServeHTTP(w, r)
			})
		}
		return middleware.Chain(http.HandlerFunc(handler), authn, requireOrg, check)
	}
	mux.Handle("GET /v1/tenant/imports", tenantJobs(h.ListTenant))
	mux.Handle("GET /v1/tenant/imports/{uuid}", tenantRecordJob(h.GetTenant))
	mux.Handle("PATCH /v1/tenant/imports/{uuid}/mapping", tenantRecordJob(h.UpdateMappingTenant))
	mux.Handle("GET /v1/tenant/imports/{uuid}/error-report", tenantRecordJob(h.ErrorReportTenant))
	mux.Handle("POST /v1/tenant/imports/{uuid}/preview", tenantRecordJob(h.PreviewTenant))
	mux.Handle("POST /v1/tenant/imports/{uuid}/confirm", tenantRecordJob(h.ConfirmTenant))
	mux.Handle("POST /v1/tenant/imports/{uuid}/rollback", tenantRecordJob(h.RollbackTenant))
}
