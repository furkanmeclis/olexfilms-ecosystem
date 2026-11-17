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

// RegisterAdjustmentRoutes mounts the TEC-1226 stock adjustment documents
// and the return inspection queue. Reads need warehouse.read, changes
// warehouse.write; approving an adjustment, the thresholds and a supplier
// return additionally need stock.adjust (checked in the use case). The
// mobile app reaches the same routes through the /v1/mobile panel mirror
// ("warehouse" prefix), with the same permissions.
func RegisterAdjustmentRoutes(
	mux *http.ServeMux,
	h *whhandler.Adjustments,
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

	const p = "/v1/warehouse/adjustments"
	mux.Handle("GET "+p, read(h.List))
	mux.Handle("POST "+p, write(h.Create))
	mux.Handle("GET "+p+"/{uuid}", read(h.Get))
	mux.Handle("PATCH "+p+"/{uuid}", write(h.Update))
	mux.Handle("POST "+p+"/{uuid}/lines", write(h.AddLine))
	mux.Handle("DELETE "+p+"/{uuid}/lines/{line_uuid}", write(h.DeleteLine))
	mux.Handle("POST "+p+"/{uuid}/photos", write(h.AddPhoto))
	mux.Handle("GET "+p+"/{uuid}/photos/{photo_uuid}", read(h.Photo))
	mux.Handle("DELETE "+p+"/{uuid}/photos/{photo_uuid}", write(h.DeletePhoto))
	mux.Handle("POST "+p+"/{uuid}/submit", write(h.Submit))
	mux.Handle("POST "+p+"/{uuid}/approve", write(h.Approve))
	mux.Handle("POST "+p+"/{uuid}/cancel", write(h.Cancel))
	mux.Handle("GET /v1/warehouse/adjustment-settings", read(h.Settings))
	mux.Handle("PUT /v1/warehouse/adjustment-settings", write(h.UpdateSettings))

	mux.Handle("GET /v1/warehouse/return-inspections", read(h.Quarantine))
	mux.Handle("GET /v1/warehouse/return-inspections/summary", read(h.QuarantineSummary))
	mux.Handle("POST /v1/warehouse/return-inspections", write(h.Inspect))
}
