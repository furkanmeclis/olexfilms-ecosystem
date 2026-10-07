package dealeraccounting

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealeraccounting/handler"
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
	accountingModule := middleware.RequireFeature(checker, features.ModuleAccounting)
	dealerModule := middleware.RequireFeatureForOrgType(checker, "dealer", features.ModuleDealerAccounting)
	route := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, accountingModule, dealerModule, middleware.RequireScope(q, perm))
	}

	mux.Handle("GET /v1/dealer-prices", route(h.ListDealerPrices, rbac.PermDealerPricingWrite))
	mux.Handle("PUT /v1/dealer-prices", route(h.SetDealerPrice, rbac.PermDealerPricingWrite))
	mux.Handle("POST /v1/product-sales", route(h.CreateProductSale, rbac.PermProductSalesWrite))
	mux.Handle("POST /v1/product-sales/{uuid}/void", route(h.VoidProductSale, rbac.PermProductSalesWrite))
	mux.Handle("GET /v1/suppliers", route(h.ListSuppliers, rbac.PermSuppliersManage))
	mux.Handle("POST /v1/suppliers", route(h.CreateSupplier, rbac.PermSuppliersManage))
	mux.Handle("PATCH /v1/suppliers/{uuid}", route(h.UpdateSupplier, rbac.PermSuppliersManage))
	mux.Handle("POST /v1/purchases", route(h.CreatePurchase, rbac.PermPurchasesWrite))
	// TEC-348 (F3-07h): list reads of the dealer sales screens.
	mux.Handle("GET /v1/dealer-prices/catalog", route(h.ListPriceCatalog, rbac.PermDealerPricingWrite))
	mux.Handle("GET /v1/product-sales", route(h.ListProductSales, rbac.PermProductSalesWrite))
	mux.Handle("GET /v1/product-sales/lookup", route(h.LookupSaleItem, rbac.PermProductSalesWrite))
	mux.Handle("GET /v1/purchases", route(h.ListPurchases, rbac.PermPurchasesWrite))
}
