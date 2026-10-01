// Package orders mounts the order routes (TEC-166, F1-04b): draft orders up
// the tree (K6), server-side prices (K8), the rate frozen at seller approval
// (K7) and the stock-free status transitions.
package orders

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	ordershandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/orders. Every route resolves the orders.read
// scope (an order outside it reads as 404); writes additionally need the
// permission of the action, checked by the use case for the caller's side
// of the order (orders.write, orders.approve, orders.ship, orders.cancel).
func RegisterRoutes(
	mux *http.ServeMux,
	h *ordershandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleOrders)
	read := middleware.RequireScope(q, rbac.PermOrdersRead)
	route := func(fn http.HandlerFunc, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, module, read}, extra...)
		return middleware.Chain(fn, mws...)
	}
	write := middleware.RequirePermission(rbac.PermOrdersWrite)
	act := middleware.RequireAnyPermission(rbac.PermOrdersWrite, rbac.PermOrdersApprove,
		rbac.PermOrdersShip, rbac.PermOrdersReceive, rbac.PermOrdersCancel)

	mux.Handle("GET /v1/orders", route(h.List))
	mux.Handle("POST /v1/orders", route(h.Create, write))
	mux.Handle("GET /v1/orders/{uuid}", route(h.Get))
	mux.Handle("PUT /v1/orders/{uuid}/items", route(h.ReplaceItems, write))
	mux.Handle("POST /v1/orders/{uuid}/transitions", route(h.Transition, act))
}
