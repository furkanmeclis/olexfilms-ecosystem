// Package catalog mounts the TEC-145 product catalog routes. Reads need
// catalog.read, writes catalog.write; on top of the permission the use case
// lets only the brand center write (K4). Every route is filtered by the
// brand of the active organization, which RequireOrganization already ties
// to the request domain (K1/K20).
package catalog

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	cataloghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/catalog routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *cataloghandler.Handler,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleCatalog)
	route := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, slug))
	}
	read := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermCatalogRead) }
	write := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermCatalogWrite) }

	mux.Handle("GET /v1/catalog/categories", read(h.ListCategories))
	mux.Handle("POST /v1/catalog/categories", write(h.CreateCategory))
	mux.Handle("GET /v1/catalog/categories/{uuid}", read(h.GetCategory))
	mux.Handle("PATCH /v1/catalog/categories/{uuid}", write(h.UpdateCategory))
	mux.Handle("DELETE /v1/catalog/categories/{uuid}", write(h.DeleteCategory))

	mux.Handle("GET /v1/catalog/products", read(h.ListProducts))
	mux.Handle("POST /v1/catalog/products", write(h.CreateProduct))
	mux.Handle("POST /v1/catalog/products/bulk-active", write(h.BulkActive))
	mux.Handle("POST /v1/catalog/products/export", read(h.ExportProducts))
	mux.Handle("POST /v1/catalog/products/import", write(h.ImportProducts))
	mux.Handle("GET /v1/catalog/products/import/sample", write(h.ImportSample))
	mux.Handle("GET /v1/catalog/products/{uuid}", read(h.GetProduct))
	mux.Handle("PATCH /v1/catalog/products/{uuid}", write(h.UpdateProduct))
	mux.Handle("DELETE /v1/catalog/products/{uuid}", write(h.DeleteProduct))
}
