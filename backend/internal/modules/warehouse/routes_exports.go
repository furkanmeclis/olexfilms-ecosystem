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

// RegisterExportRoutes mounts the TEC-761 warehouse export jobs. Each needs
// warehouse.read like the list it exports; the use case limits them to the
// active organization (center / distributor, K12).
func RegisterExportRoutes(
	mux *http.ServeMux,
	h *whhandler.Exports,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleWarehouse)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermWarehouseRead))
	}

	mux.Handle("POST /v1/warehouse/stock-entries/export", read(h.Entries))
	mux.Handle("POST /v1/warehouse/stock-counts/export", read(h.Counts))
	mux.Handle("POST /v1/warehouse/stock-counts/{uuid}/export", read(h.CountReport))
	mux.Handle("POST /v1/warehouse/transfers/export", read(h.Transfers))
	mux.Handle("POST /v1/warehouse/transfers/summary/export", read(h.TransferSummary))
	mux.Handle("POST /v1/warehouse/locations/export", read(h.Locations))
	mux.Handle("POST /v1/warehouse/eod-reports/export", read(h.EODReports))
}
