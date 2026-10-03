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

// RegisterEODRoutes mounts the TEC-207 end-of-day reports. Reads and the
// PDF need warehouse.read; a manual run (POST) needs warehouse.write. The
// use case limits them to the active organization (center / distributor).
func RegisterEODRoutes(
	mux *http.ServeMux,
	h *whhandler.EOD,
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

	mux.Handle("GET /v1/warehouse/eod-reports", read(h.List))
	mux.Handle("POST /v1/warehouse/eod-reports", write(h.Generate))
	mux.Handle("GET /v1/warehouse/eod-reports/{uuid}", read(h.Get))
	mux.Handle("POST /v1/warehouse/eod-reports/{uuid}/pdf", read(h.RequestPDF))
	mux.Handle("GET /v1/warehouse/eod-report-pdfs/{uuid}", read(h.GetPDF))
	mux.Handle("GET /v1/warehouse/eod-report-pdfs/{uuid}/download", read(h.DownloadPDF))
}
