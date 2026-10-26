// Package stockbulk mounts the bulk barcode operations (TEC-1008).
package stockbulk

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockbulk/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// RegisterRoutes mounts /v1/platform/stock-bulk: super_admin
// (stock.bulk) in the brand center; run and undo need a recent step-up.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	stepUp middleware.StepUpChecker,
) {
	base := []func(http.Handler) http.Handler{
		middleware.Authenticate(tokens, loader),
		middleware.RequireSuperAdmin,
		middleware.RequireOrganization(tokens, q),
		middleware.RequirePermission(rbac.PermStockBulk),
		requireCenter,
	}
	route := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, base...) }
	stepped := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, append(base, middleware.RequireStepUp(stepUp))...)
	}
	mux.Handle("GET /v1/platform/stock-bulk", route(h.List))
	mux.Handle("POST /v1/platform/stock-bulk", route(h.Preview))
	mux.Handle("GET /v1/platform/stock-bulk/{uuid}", route(h.Get))
	mux.Handle("GET /v1/platform/stock-bulk/{uuid}/items", route(h.Items))
	mux.Handle("DELETE /v1/platform/stock-bulk/{uuid}", route(h.Discard))
	mux.Handle("POST /v1/platform/stock-bulk/{uuid}/run", stepped(h.Run))
	mux.Handle("POST /v1/platform/stock-bulk/{uuid}/undo", stepped(h.Undo))
}

// requireCenter keeps the jobs on the brand center (the units' issuer).
func requireCenter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if org, ok := orgctx.ScopeFrom(r.Context()); ok && org.OrgType == "center" {
			next.ServeHTTP(w, r)
			return
		}
		response.Forbidden(w, r, "Bulk barcode operations run in the brand center")
	})
}
