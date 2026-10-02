// Package transfers mounts the stock transfer request routes (TEC-197,
// F1-04f2, K13): sibling dealers or sibling distributors hand units over to
// each other; the receiver or the common parent decides, the stock moves
// through the ledger (transfer_out / transfer_in / transfer_cancel_restore).
package transfers

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	transfershandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/transfers/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/stock-transfers behind the dealer_transfers
// module. Every route needs transfers.request or transfers.approve; a
// request is visible only to its parties (giver, receiver, common parent),
// others get 404. The use case checks the permission of each action for the
// caller's side of the request.
func RegisterRoutes(
	mux *http.ServeMux,
	h *transfershandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleDealerTransfers)
	anyPerm := middleware.RequireAnyPermission(rbac.PermTransfersRequest, rbac.PermTransfersApprove)
	route := func(fn http.HandlerFunc, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, module, anyPerm}, extra...)
		return middleware.Chain(fn, mws...)
	}
	request := middleware.RequirePermission(rbac.PermTransfersRequest)

	mux.Handle("GET /v1/stock-transfers", route(h.List))
	mux.Handle("POST /v1/stock-transfers", route(h.Create, request))
	mux.Handle("GET /v1/stock-transfers/targets", route(h.Targets, request))
	mux.Handle("GET /v1/stock-transfers/{uuid}", route(h.Get))
	mux.Handle("POST /v1/stock-transfers/{uuid}/transitions", route(h.Transition))
}
