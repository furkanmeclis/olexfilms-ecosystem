// Package orders mounts the order routes (TEC-166, F1-04b; TEC-167,
// F1-04c): draft orders up the tree (K6), server-side prices (K8), the rate
// frozen at seller approval (K7), barcode assignment with stock
// reservations and the status transitions up to shipping.
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
	// TEC-373: list export with the list filters (orders.read scope).
	mux.Handle("POST /v1/orders/export", route(h.RequestListExport))
	mux.Handle("POST /v1/orders", route(h.Create, write))
	// TEC-1064: price preview of a new order (list price, discount, TRY).
	mux.Handle("POST /v1/orders/quote", route(h.Quote, write))
	mux.Handle("GET /v1/orders/{uuid}", route(h.Get))
	mux.Handle("GET /v1/orders/{uuid}/order-slip.pdf", route(h.OrderSlip))
	// TEC-1056: the seller's sales (orders.write) and warehouse
	// (orders.ship) staff edit the lines before the order is ready.
	editItems := middleware.RequireAnyPermission(rbac.PermOrdersWrite, rbac.PermOrdersShip)
	mux.Handle("PUT /v1/orders/{uuid}/items", route(h.ReplaceItems, editItems))
	// TEC-1222: a supplementary order of a shipped or delivered order.
	mux.Handle("POST /v1/orders/{uuid}/supplements", route(h.CreateSupplement, editItems))
	// TEC-1058: the plotter mark of one line, also after shipping.
	mux.Handle("PATCH /v1/orders/{uuid}/items/{item_uuid}", route(h.PatchItem, editItems))
	mux.Handle("POST /v1/orders/{uuid}/transitions", route(h.Transition, act))
	// TEC-1228: the warehouse work queue (priority / due date, taking a job).
	mux.Handle("PUT /v1/orders/{uuid}/schedule", route(h.SetSchedule,
		middleware.RequireAnyPermission(rbac.PermOrdersWrite, rbac.PermOrdersApprove)))
	claim := middleware.RequireAnyPermission(rbac.PermOrdersShip, rbac.PermOrdersWrite, rbac.PermOrdersApprove)
	mux.Handle("POST /v1/orders/{uuid}/claim", route(h.Claim, claim))
	mux.Handle("DELETE /v1/orders/{uuid}/claim", route(h.ReleaseClaim, claim))
	ship := middleware.RequirePermission(rbac.PermOrdersShip)
	mux.Handle("POST /v1/orders/{uuid}/items/{item_uuid}/units", route(h.AssignUnit, ship))
	mux.Handle("DELETE /v1/orders/{uuid}/items/{item_uuid}/units/{unit_uuid}", route(h.UnassignUnit, ship))
	// TEC-768: barcode suggestions, pick locations, pick list, shipping
	// documents and the delivery method catalog.
	mux.Handle("GET /v1/orders/{uuid}/items/{item_uuid}/suggestions", route(h.Suggest, ship))
	mux.Handle("POST /v1/orders/{uuid}/items/{item_uuid}/suggestions/apply", route(h.ApplySuggestions, ship))
	mux.Handle("GET /v1/orders/{uuid}/items/{item_uuid}/pick-locations", route(h.PickLocations, ship))
	mux.Handle("GET /v1/orders/{uuid}/pick-list", route(h.PickList, ship))
	mux.Handle("POST /v1/orders/{uuid}/shipping-document", route(h.RegenerateShippingDocument, ship))
	mux.Handle("GET /v1/orders/{uuid}/documents/{kind}", route(h.Document))
	receive := middleware.RequirePermission(rbac.PermOrdersReceive)
	mux.Handle("POST /v1/orders/{uuid}/receipt-document", route(h.AttachReceipt, receive))
	// TEC-1227: partial shipment (the rest becomes a backorder) and the
	// receipt discrepancy records (missing / damaged units).
	mux.Handle("POST /v1/orders/{uuid}/partial-ship", route(h.PartialShip, ship))
	resolve := middleware.RequireAnyPermission(rbac.PermOrdersReceive, rbac.PermOrdersShip)
	mux.Handle("POST /v1/orders/{uuid}/discrepancies/{discrepancy_uuid}/resolve", route(h.ResolveDiscrepancy, resolve))
	mux.Handle("POST /v1/orders/{uuid}/discrepancies/{discrepancy_uuid}/photos", route(h.AddDiscrepancyPhoto, receive))
	mux.Handle("GET /v1/orders/{uuid}/discrepancies/{discrepancy_uuid}/photos/{index}", route(h.DiscrepancyPhoto))
	// TEC-1030: manual "invoiced" marks by the seller's accounting
	// (einvoice.manage, the center accounting grant).
	invoice := middleware.RequirePermission(rbac.PermEinvoiceManage)
	mux.Handle("POST /v1/orders/{uuid}/invoices", route(h.MarkInvoiced, invoice))
	mux.Handle("DELETE /v1/orders/{uuid}/invoices/{invoice_uuid}", route(h.DeleteInvoice, invoice))
	mux.Handle("GET /v1/delivery-methods", route(h.ListDeliveryMethods))
	mux.Handle("POST /v1/delivery-methods", route(h.CreateDeliveryMethod, ship))
	mux.Handle("PUT /v1/delivery-methods/{uuid}", route(h.UpdateDeliveryMethod, ship))
	mux.Handle("DELETE /v1/delivery-methods/{uuid}", route(h.DeleteDeliveryMethod, ship))
}
