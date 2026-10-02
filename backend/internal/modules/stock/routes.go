// Package stock mounts the TEC-155 stock read routes. Every route needs
// stock.read; the resolved scope filter narrows the records on the holding
// organization (TEC-94 decision 4). Writes go through ledger.Post only.
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

// RegisterRoutes mounts /v1/stock routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *stockhandler.Handler,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleStock)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermStockRead))
	}

	mux.Handle("GET /v1/stock/units/by-barcode/{barcode}", read(h.UnitHistory))
	mux.Handle("GET /v1/stock/organizations/{uuid}/products", read(h.OrganizationStock))
	mux.Handle("GET /v1/stock/locations/{uuid}/products", read(h.LocationStock))
}

// RegisterPlatformRoutes mounts the super_admin stock maintenance routes
// (TEC-156): the projection drift check is a dry run.
func RegisterPlatformRoutes(
	mux *http.ServeMux,
	h *stockhandler.Rebuild,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	mux.Handle("POST /v1/platform/stock/rebuild-check", middleware.Chain(
		http.HandlerFunc(h.Check), authn, middleware.RequireSuperAdmin,
	))
}
