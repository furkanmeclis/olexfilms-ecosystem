// Package pricing mounts the price list routes (TEC-146, K8). Prices belong
// to the catalog module: the center writes the list price and the
// distributor-specific prices, a distributor writes its price to dealers, and
// every reader gets one effective view masked by the pricing.* grants.
package pricing

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	pricinghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/handler"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// requireOrgType limits a route to organization types, before the step-up
// check so a wrong-level caller gets a plain 403.
func requireOrgType(types ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			org, ok := orgctx.ScopeFrom(r.Context())
			if ok {
				for _, t := range types {
					if org.OrgType == t {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			response.Forbidden(w, r, "This price is not writable by your organization")
		})
	}
}

// RegisterRoutes mounts pricing routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *pricinghandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	stepUp middleware.StepUpChecker,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleCatalog)
	read := func(fn http.HandlerFunc, slugs ...string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireAnyPermission(slugs...))
	}
	// pricing.sale.write is sensitive: every write needs a recent step-up.
	write := func(fn http.HandlerFunc, orgType string) http.Handler {
		return middleware.Chain(fn, authn, org, module,
			middleware.RequirePermission(rbac.PermPricingSaleWrite),
			requireOrgType(orgType),
			middleware.RequireStepUp(stepUp))
	}
	anyRead := []string{rbac.PermPricingPurchaseRead, rbac.PermPricingSaleRead, rbac.PermPricingRecommendedRead}

	mux.Handle("GET /v1/tenant/pricing/products", read(h.ListProducts, anyRead...))
	mux.Handle("GET /v1/tenant/pricing/products/{uuid}", read(h.GetProduct, anyRead...))

	center := pricingusecase.OrgCenter
	mux.Handle("PUT /v1/tenant/pricing/products/{uuid}/prices/{currency}", write(h.SetListPrice, center))
	mux.Handle("DELETE /v1/tenant/pricing/products/{uuid}/prices/{currency}", write(h.DeleteListPrice, center))
	mux.Handle("GET /v1/tenant/pricing/distributor-prices", middleware.Chain(http.HandlerFunc(h.ListDistributorOverrides),
		authn, org, module, middleware.RequirePermission(rbac.PermPricingSaleRead), requireOrgType(center)))
	mux.Handle("PUT /v1/tenant/pricing/products/{uuid}/distributor-prices/{distributor_uuid}/{currency}",
		write(h.SetDistributorOverride, center))
	mux.Handle("DELETE /v1/tenant/pricing/products/{uuid}/distributor-prices/{distributor_uuid}/{currency}",
		write(h.DeleteDistributorOverride, center))

	distributor := pricingusecase.OrgDistributor
	mux.Handle("PUT /v1/tenant/pricing/products/{uuid}/dealer-prices/{currency}", write(h.SetDealerPrice, distributor))
	mux.Handle("DELETE /v1/tenant/pricing/products/{uuid}/dealer-prices/{currency}", write(h.DeleteDealerPrice, distributor))
}

// RegisterRecommendedRoutes mounts the recommended price and price
// discipline routes (TEC-506, F5-09b). Publishing (API and import) is the
// brand center's, behind pricing.recommended.write and a recent step-up;
// the history is the center's, the price in force any organization's with
// pricing.recommended.read; the discipline reads follow the
// pricing.discipline.read scope (center: brand, distributor: subtree).
func RegisterRecommendedRoutes(
	mux *http.ServeMux,
	h *pricinghandler.RecommendedHandler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	stepUp middleware.StepUpChecker,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleCatalog)
	center := pricingusecase.OrgCenter
	publish := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module,
			middleware.RequirePermission(rbac.PermPricingRecommendedWrite),
			requireOrgType(center),
			middleware.RequireStepUp(stepUp))
	}
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequirePermission(rbac.PermPricingRecommendedRead))
	}
	discipline := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermPricingDisciplineRead))
	}

	mux.Handle("POST /v1/tenant/pricing/recommended/publish", publish(h.Publish))
	mux.Handle("POST /v1/tenant/pricing/recommended/import", publish(h.Import))
	mux.Handle("GET /v1/tenant/pricing/recommended/import/sample", middleware.Chain(http.HandlerFunc(h.ImportSample),
		authn, org, module, middleware.RequirePermission(rbac.PermPricingRecommendedWrite), requireOrgType(center)))
	mux.Handle("GET /v1/tenant/pricing/recommended/versions", middleware.Chain(http.HandlerFunc(h.Versions),
		authn, org, module, middleware.RequirePermission(rbac.PermPricingRecommendedRead), requireOrgType(center)))
	mux.Handle("GET /v1/tenant/pricing/recommended/current", read(h.Current))

	mux.Handle("GET /v1/pricing/discipline", discipline(h.Discipline))
	mux.Handle("GET /v1/pricing/discipline/summary", discipline(h.DisciplineSummary))
	mux.Handle("POST /v1/pricing/discipline/export", discipline(h.DisciplineExport))
}
