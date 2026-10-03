// Package warehouse mounts the TEC-201 warehouse tree routes. Reads need
// warehouse.read, writes warehouse.write; the warehouse feature module must
// be on. The use case additionally limits the module to center and
// distributor organizations (K12) and to the active organization's own
// warehouses; the tree is brand-independent (K20).
package warehouse

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	whhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/warehouse routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *whhandler.Handler,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleWarehouse)
	scoped := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, slug))
	}
	read := func(fn http.HandlerFunc) http.Handler { return scoped(fn, rbac.PermWarehouseRead) }
	write := func(fn http.HandlerFunc) http.Handler { return scoped(fn, rbac.PermWarehouseWrite) }

	mux.Handle("GET /v1/warehouse/warehouses", read(h.ListWarehouses))
	mux.Handle("POST /v1/warehouse/warehouses", write(h.CreateWarehouse))
	mux.Handle("POST /v1/warehouse/warehouses/reorder", write(h.ReorderWarehouses))
	mux.Handle("GET /v1/warehouse/warehouses/{uuid}", read(h.GetWarehouse))
	mux.Handle("PATCH /v1/warehouse/warehouses/{uuid}", write(h.UpdateWarehouse))
	mux.Handle("DELETE /v1/warehouse/warehouses/{uuid}", write(h.DeleteWarehouse))
	mux.Handle("GET /v1/warehouse/warehouses/{uuid}/rooms", read(h.ListRooms))
	mux.Handle("POST /v1/warehouse/warehouses/{uuid}/rooms", write(h.CreateRoom))

	mux.Handle("POST /v1/warehouse/rooms/reorder", write(h.ReorderRooms))
	mux.Handle("GET /v1/warehouse/rooms/{uuid}", read(h.GetRoom))
	mux.Handle("PATCH /v1/warehouse/rooms/{uuid}", write(h.UpdateRoom))
	mux.Handle("DELETE /v1/warehouse/rooms/{uuid}", write(h.DeleteRoom))
	mux.Handle("GET /v1/warehouse/rooms/{uuid}/locations", read(h.ListRoomLocations))

	mux.Handle("POST /v1/warehouse/locations", write(h.CreateLocation))
	mux.Handle("POST /v1/warehouse/locations/reorder", write(h.ReorderLocations))
	mux.Handle("POST /v1/warehouse/locations/generate", write(h.GenerateLocations))
	mux.Handle("GET /v1/warehouse/locations/{uuid}", read(h.GetLocation))
	mux.Handle("PATCH /v1/warehouse/locations/{uuid}", write(h.UpdateLocation))
	mux.Handle("DELETE /v1/warehouse/locations/{uuid}", write(h.DeleteLocation))
}

// RegisterScanRoutes mounts the TEC-203 universal scan resolver. It needs
// warehouse.read (center and distributor, K12); units additionally follow
// the caller's stock.read reach inside the use case.
func RegisterScanRoutes(
	mux *http.ServeMux,
	h *whhandler.ScanHandler,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleWarehouse)
	mux.Handle("POST /v1/warehouse/scan",
		middleware.Chain(http.HandlerFunc(h.Scan), authn, org, module, middleware.RequireScope(q, rbac.PermWarehouseRead)))
}
