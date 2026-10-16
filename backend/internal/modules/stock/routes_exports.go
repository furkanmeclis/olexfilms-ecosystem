package stock

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	stockhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterExportRoutes mounts the TEC-761 stock exports of the warehouse
// screens. Each needs stock.read with the stock module on, like the list it
// exports; the barcode batch list stays center only (use case).
func RegisterExportRoutes(
	mux *http.ServeMux,
	h *stockhandler.StockExports,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(tokens, q),
			middleware.RequireFeature(checker, features.ModuleStock),
			middleware.RequireScope(q, rbac.PermStockRead),
		)
	}
	mux.Handle("POST /v1/stock/organizations/{uuid}/products/export", read(h.ProductStock))
	mux.Handle("POST /v1/stock/locations/{uuid}/products/export", read(h.LocationStock))
	mux.Handle("POST /v1/stock/units/by-barcode/{barcode}/export", read(h.UnitHistory))
	mux.Handle("POST /v1/stock/barcodes/export", read(h.Batches))
}
