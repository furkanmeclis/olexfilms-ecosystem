// Package reports mounts the /v1/reports contract (TEC-495, F5-05f): the
// redesigned mobile reports, also read by the panel widgets. Every route
// needs an authenticated member with an active organization; the module
// and permission of each report are checked by the usecase (403
// FEATURE_DISABLED / FORBIDDEN), so the catalog can hide what is closed.
package reports

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/reports/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	route := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, authn, org) }

	mux.Handle("GET /v1/reports/catalog", route(h.Catalog))
	mux.Handle("GET /v1/reports/overview", route(h.Overview))
	mux.Handle("GET /v1/reports/layout", route(h.GetLayout))
	mux.Handle("PUT /v1/reports/layout", route(h.PutLayout))
	mux.Handle("GET /v1/reports/{report}", route(h.Report))
}
