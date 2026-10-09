package bulk

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	bulkhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/handler"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	performanceusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	whatsappusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *bulkhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/bulk", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformBulkRead),
	))
	mux.Handle("GET /v1/platform/bulk/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformBulkRead),
	))
	mux.Handle("POST /v1/platform/bulk/{uuid}/rollback", middleware.Chain(
		http.HandlerFunc(h.Rollback), authn, require(rbac.PermPlatformBulkRead),
	))

	mux.Handle("POST /v1/platform/users/bulk", middleware.Chain(
		http.HandlerFunc(h.ExecuteUsers), authn, require(rbac.PermPlatformUsersRead),
	))
	mux.Handle("POST /v1/platform/roles/bulk", middleware.Chain(
		http.HandlerFunc(h.ExecuteRoles), authn, require(rbac.PermPlatformRolesRead),
	))
	// TEC-365: platform organizations (status change, extend access).
	mux.Handle("POST /v1/platform/organizations/bulk", middleware.Chain(
		http.HandlerFunc(h.ExecuteOrganizations), authn, require(rbac.PermPlatformOrganizationsRead),
	))
	// TEC-369: vehicle catalog activate / deactivate (global reference data).
	mux.Handle("POST /v1/platform/vehicle-catalog/brands/bulk", middleware.Chain(
		h.ExecutePlatform(adapters.ResourceVehicleBrands), authn, require(rbac.PermVehicleCatalogWrite),
	))
	mux.Handle("POST /v1/platform/vehicle-catalog/models/bulk", middleware.Chain(
		h.ExecutePlatform(adapters.ResourceVehicleModels), authn, require(rbac.PermVehicleCatalogWrite),
	))
	// TEC-398: WhatsApp conversations (close, assign, AI mode); platform
	// admins only (S2).
	mux.Handle("POST /v1/conversations/bulk", middleware.Chain(
		h.ExecutePlatform(whatsappusecase.ResourceBulk), authn, require(rbac.PermConversationsManage),
	))
	// TEC-212: undo of a platform operation (users / roles).
	mux.Handle("POST /v1/platform/bulk-operations/{uuid}/undo", middleware.Chain(
		http.HandlerFunc(h.UndoPlatform), authn, require(rbac.PermPlatformBulkRead),
	))
}

// RegisterTenantRoutes mounts the organization-scoped bulk routes
// (TEC-212): bulk actions of tenant resources and the undo log.
func RegisterTenantRoutes(
	mux *http.ServeMux,
	h *bulkhandler.Handler,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	route := func(fn http.HandlerFunc, slug string, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org}, extra...)
		mws = append(mws, middleware.RequireScope(q, slug))
		return middleware.Chain(fn, mws...)
	}

	mux.Handle("POST /v1/catalog/products/bulk", route(
		h.ExecuteTenant(adapters.ResourceCatalogProducts), rbac.PermCatalogWrite,
		middleware.RequireFeature(checker, features.ModuleCatalog),
	))
	// TEC-369: catalog categories activate / deactivate / delete.
	mux.Handle("POST /v1/catalog/categories/bulk", route(
		h.ExecuteTenant(adapters.ResourceCatalogCategories), rbac.PermCatalogWrite,
		middleware.RequireFeature(checker, features.ModuleCatalog),
	))
	mux.Handle("POST /v1/tasks/bulk", route(h.ExecuteTenant(adapters.ResourceTasks), rbac.PermTasksWrite))
	// TEC-371: leads (assign, set status) inside the caller's leads.write scope.
	mux.Handle("POST /v1/leads/bulk", route(
		h.ExecuteTenantScoped(leadsusecase.ResourceBulk), rbac.PermLeadsWrite,
		middleware.RequireFeature(checker, features.ModuleLeads),
	))
	// TEC-497: bulk approval of the dealer's calculated bonus accruals.
	mux.Handle("POST /v1/performance/bonuses/bulk", route(
		h.ExecuteTenantScoped(performanceusecase.ResourceBonusBulk), rbac.PermPerformanceBonusManage,
		middleware.RequireFeature(checker, features.ModulePerformance),
		middleware.RequireFeature(checker, features.ModuleDealerAccounting),
	))

	// The undo log of the organization; undo itself checks the action's
	// permission on the operation row.
	mux.Handle("GET /v1/tenant/bulk-operations", middleware.Chain(
		http.HandlerFunc(h.ListTenantOperations), authn, org,
	))
	mux.Handle("POST /v1/tenant/bulk-operations/{uuid}/undo", middleware.Chain(
		http.HandlerFunc(h.UndoTenant), authn, org,
	))
}
