// Package warranty_claims mounts warranty claim API routes.
package warranty_claims

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleWarrantyClaims)
	tenant := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, perm))
	}
	mux.Handle("GET /v1/warranty-claims", tenant(h.List, rbac.PermWarrantyClaimsRead))
	mux.Handle("GET /v1/warranty-claims/reports/failure-rate", tenant(h.FailureRateReport, rbac.PermWarrantyClaimsRead))
	mux.Handle("POST /v1/warranty-claims/reports/failure-rate/export", tenant(h.ExportFailureRateReport, rbac.PermWarrantyClaimsRead))
	mux.Handle("GET /v1/warranty-claims/reports/by-dealer", tenant(h.ByDealerReport, rbac.PermWarrantyClaimsRead))
	mux.Handle("POST /v1/warranty-claims/reports/by-dealer/export", tenant(h.ExportByDealerReport, rbac.PermWarrantyClaimsRead))
	mux.Handle("GET /v1/warranty-claims/reports/parts", tenant(h.PartsReport, rbac.PermWarrantyClaimsRead))
	mux.Handle("POST /v1/warranty-claims/reports/parts/export", tenant(h.ExportPartsReport, rbac.PermWarrantyClaimsRead))
	mux.Handle("POST /v1/warranty-claims", tenant(h.Create, rbac.PermWarrantyClaimsWrite))
	mux.Handle("POST /v1/warranty-claims/{uuid}/photos", tenant(h.AddPhoto, rbac.PermWarrantyClaimsWrite))
	mux.Handle("POST /v1/warranty-claims/{uuid}/status", middleware.Chain(
		http.HandlerFunc(h.Transition), authn, org, module,
		middleware.RequireAnyPermission(rbac.PermWarrantyClaimsWrite, rbac.PermWarrantyClaimsReview, rbac.PermWarrantyClaimsDecide),
		middleware.RequireScope(q, rbac.PermWarrantyClaimsRead),
	))
	mux.Handle("POST /v1/warranty-claims/{uuid}/reapply-service", middleware.Chain(
		http.HandlerFunc(h.ReapplyService), authn, org, module,
		middleware.RequireScope(q, rbac.PermWarrantyClaimsWrite),
		middleware.RequireScope(q, rbac.PermWarrantyClaimsRead),
	))
	// TEC-337: claim detail with the center's cost summary.
	mux.Handle("GET /v1/warranty-claims/{uuid}", tenant(h.Get, rbac.PermWarrantyClaimsRead))
	mux.Handle("GET /v1/portal/warranty-claims", middleware.Chain(
		http.HandlerFunc(h.PortalList), authn, middleware.RequirePermission(rbac.PermWarrantiesRead),
	))
}
