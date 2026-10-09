// Package features mounts the module (feature flag) routes of TEC-86.
package features

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	featurehandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/features/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts module routes. The Özellikler routes belong to the
// core "organizations" module, so RequireFeature never closes them.
func RegisterRoutes(
	mux *http.ServeMux,
	h *featurehandler.Handler,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	core := middleware.RequireFeature(checker, features.ModuleOrganizations)
	tenant := func(fn http.HandlerFunc, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, core}, extra...)
		return middleware.Chain(fn, mws...)
	}
	platform := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(slug))
	}
	readScope := middleware.RequireScope(q, rbac.PermModulesRead)
	manageScope := middleware.RequireScope(q, rbac.PermModulesManage)

	// Every member reads its organization's modules (menus and guards).
	mux.Handle("GET /v1/features", tenant(h.List))
	mux.Handle("POST /v1/features/{key}/request", tenant(h.Request, middleware.RequirePermission(rbac.PermModulesRead)))
	// TEC-508: the requester withdraws its pending request.
	mux.Handle("DELETE /v1/features/{key}/request", tenant(h.CancelRequest, middleware.RequirePermission(rbac.PermModulesRead)))

	mux.Handle("GET /v1/tenant/modules/dealers", tenant(h.Dealers, readScope))
	mux.Handle("POST /v1/tenant/modules/dealers/bulk", tenant(h.BulkDealers, manageScope))
	mux.Handle("PUT /v1/tenant/modules/dealers/{uuid}/{key}", tenant(h.SetDealer, manageScope))
	mux.Handle("DELETE /v1/tenant/modules/dealers/{uuid}/{key}", tenant(h.ClearDealer, manageScope))
	mux.Handle("GET /v1/tenant/modules/dealer-standard", tenant(h.DealerStandard, readScope))
	mux.Handle("PUT /v1/tenant/modules/dealer-standard/{key}", tenant(h.SetDealerStandard, manageScope))
	mux.Handle("DELETE /v1/tenant/modules/dealer-standard/{key}", tenant(h.ClearDealerStandard, manageScope))
	// TEC-508: the distributor decides its dealers' module requests.
	mux.Handle("GET /v1/tenant/modules/requests", tenant(h.TenantRequests, manageScope))
	mux.Handle("POST /v1/tenant/modules/requests/{uuid}/approve", tenant(h.TenantApproveRequest, manageScope))
	mux.Handle("POST /v1/tenant/modules/requests/{uuid}/reject", tenant(h.TenantRejectRequest, manageScope))

	mux.Handle("GET /v1/platform/modules", platform(h.PlatformList, rbac.PermPlatformModulesRead))
	mux.Handle("PATCH /v1/platform/modules/{key}", platform(h.PlatformPatch, rbac.PermPlatformModulesWrite))
	mux.Handle("GET /v1/platform/organizations/{uuid}/modules",
		platform(h.PlatformOrgModules, rbac.PermPlatformModulesRead))
	mux.Handle("PUT /v1/platform/organizations/{uuid}/modules/{key}",
		platform(h.PlatformSetOrgModule, rbac.PermPlatformModulesWrite))
	mux.Handle("DELETE /v1/platform/organizations/{uuid}/modules/{key}",
		platform(h.PlatformClearOrgModule, rbac.PermPlatformModulesWrite))
	// TEC-508: the center decides the requests of distributors and of
	// dealers without a distributor.
	mux.Handle("GET /v1/platform/modules/requests", platform(h.PlatformRequests, rbac.PermPlatformModulesRead))
	mux.Handle("POST /v1/platform/modules/requests/{uuid}/approve",
		platform(h.PlatformApproveRequest, rbac.PermPlatformModulesWrite))
	mux.Handle("POST /v1/platform/modules/requests/{uuid}/reject",
		platform(h.PlatformRejectRequest, rbac.PermPlatformModulesWrite))
}
