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
	rh *stockhandler.Reclassify,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	stepUp middleware.StepUpChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleStock)
	scoped := func(fn http.HandlerFunc, slug string, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, module, middleware.RequireScope(q, slug)}, extra...)
		return middleware.Chain(fn, mws...)
	}
	read := func(fn http.HandlerFunc) http.Handler { return scoped(fn, rbac.PermStockRead) }

	mux.Handle("GET /v1/stock/units/by-barcode/{barcode}", read(h.UnitHistory))
	mux.Handle("GET /v1/stock/organizations/{uuid}/products", read(h.OrganizationStock))
	// TEC-216: unit list; the distributor reaches its dealers (subtree).
	mux.Handle("GET /v1/stock/organizations/{uuid}/units", read(h.OrganizationUnits))
	mux.Handle("GET /v1/stock/locations/{uuid}/products", read(h.LocationStock))

	// TEC-157 reclassification: the holding organization requests
	// (stock.write), the center approves (stock.reclassify, sensitive:
	// step-up) and the approval applies it through the ledger.
	mux.Handle("GET /v1/stock/reclassifications", read(rh.List))
	mux.Handle("POST /v1/stock/reclassifications", scoped(rh.Create, rbac.PermStockWrite))
	mux.Handle("GET /v1/stock/reclassifications/{uuid}", read(rh.Get))
	mux.Handle("POST /v1/stock/reclassifications/{uuid}/approve",
		scoped(rh.Approve, rbac.PermStockReclassify, middleware.RequireStepUp(stepUp)))
	mux.Handle("POST /v1/stock/reclassifications/{uuid}/reject", scoped(rh.Reject, rbac.PermStockReclassify))
	mux.Handle("POST /v1/stock/reclassifications/{uuid}/cancel", scoped(rh.Cancel, rbac.PermStockWrite))
}

// RegisterSplitRoutes mounts the TEC-184 roll split (stock.write): the
// holding organization cuts meters off a roll as a new unit.
func RegisterSplitRoutes(
	mux *http.ServeMux,
	h *stockhandler.Split,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	mux.Handle("POST /v1/stock/splits", middleware.Chain(http.HandlerFunc(h.Create),
		middleware.Authenticate(tokens, loader),
		middleware.RequireOrganization(tokens, q),
		middleware.RequireFeature(checker, features.ModuleStock),
		middleware.RequireScope(q, rbac.PermStockWrite),
	))
}

// RegisterImportRoutes mounts the TEC-158 stock import upload (stock.import,
// center only). Preview, confirm and undo run on /v1/tenant/imports/{uuid}.
func RegisterImportRoutes(
	mux *http.ServeMux,
	h *stockhandler.Import,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	chain := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(tokens, q),
			middleware.RequireFeature(checker, features.ModuleStock),
			middleware.RequireScope(q, rbac.PermStockImport),
		)
	}
	mux.Handle("POST /v1/stock/import", chain(h.Upload))
	mux.Handle("GET /v1/stock/import/sample", chain(h.Sample))
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

// RegisterLabelRoutes mounts the TEC-202 barcode batches (center only,
// stock.write), the label templates (stock.read / stock.write) and the label
// print endpoints. Location labels sit under /v1/warehouse and need
// warehouse.read with the warehouse module on.
func RegisterLabelRoutes(
	mux *http.ServeMux,
	h *stockhandler.Labels,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	scoped := func(fn http.HandlerFunc, module, slug string) http.Handler {
		return middleware.Chain(fn, authn, org, middleware.RequireFeature(checker, module), middleware.RequireScope(q, slug))
	}
	read := func(fn http.HandlerFunc) http.Handler { return scoped(fn, features.ModuleStock, rbac.PermStockRead) }
	write := func(fn http.HandlerFunc) http.Handler { return scoped(fn, features.ModuleStock, rbac.PermStockWrite) }

	mux.Handle("GET /v1/stock/barcodes", read(h.ListBatches))
	mux.Handle("POST /v1/stock/barcodes", write(h.CreateBatch))
	mux.Handle("GET /v1/stock/barcodes/{uuid}", read(h.GetBatch))
	mux.Handle("GET /v1/stock/barcodes/{uuid}/labels.pdf", read(h.PrintBatch))

	mux.Handle("GET /v1/stock/label-templates", read(h.ListTemplates))
	mux.Handle("POST /v1/stock/label-templates", write(h.CreateTemplate))
	mux.Handle("GET /v1/stock/label-templates/{uuid}", read(h.GetTemplate))
	mux.Handle("PUT /v1/stock/label-templates/{uuid}", write(h.UpdateTemplate))
	mux.Handle("DELETE /v1/stock/label-templates/{uuid}", write(h.DeleteTemplate))

	mux.Handle("GET /v1/stock/labels/units.pdf", read(h.PrintUnits))
	mux.Handle("GET /v1/warehouse/labels/locations.pdf", scoped(h.PrintLocations, features.ModuleWarehouse, rbac.PermWarehouseRead))
}
