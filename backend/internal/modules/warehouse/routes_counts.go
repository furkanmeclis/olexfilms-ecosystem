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

// RegisterCountRoutes mounts the TEC-206 stock counts. Reads need
// warehouse.read, every change warehouse.write; approving additionally
// needs stock.adjust (checked in the use case). Scans resolve through the
// TEC-203 resolver (units follow the caller's stock.read reach).
func RegisterCountRoutes(
	mux *http.ServeMux,
	h *whhandler.Counts,
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

	mux.Handle("GET /v1/warehouse/stock-counts", read(h.List))
	mux.Handle("POST /v1/warehouse/stock-counts", write(h.Create))
	mux.Handle("GET /v1/warehouse/stock-counts/{uuid}", read(h.Get))
	mux.Handle("POST /v1/warehouse/stock-counts/{uuid}/approve-start", write(h.ApproveStart))
	mux.Handle("POST /v1/warehouse/stock-counts/{uuid}/start", write(h.Start))
	mux.Handle("GET /v1/warehouse/stock-counts/{uuid}/scans", read(h.Scans))
	mux.Handle("POST /v1/warehouse/stock-counts/{uuid}/scans", write(h.Scan))
	mux.Handle("DELETE /v1/warehouse/stock-counts/{uuid}/scans/{scan_uuid}", write(h.DeleteScan))
	mux.Handle("POST /v1/warehouse/stock-counts/{uuid}/complete", write(h.Complete))
	mux.Handle("GET /v1/warehouse/stock-counts/{uuid}/report", read(h.Report))
	mux.Handle("GET /v1/warehouse/stock-counts/{uuid}/export", read(h.Export))
	mux.Handle("POST /v1/warehouse/stock-counts/{uuid}/approve", write(h.Approve))
	mux.Handle("POST /v1/warehouse/stock-counts/{uuid}/cancel", write(h.Cancel))
}
