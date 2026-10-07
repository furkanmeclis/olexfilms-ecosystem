package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	ordersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// OrdersReader is the orders use case surface the tools use.
type OrdersReader interface {
	List(ctx context.Context, c ordersuc.Caller, f ordersuc.ListFilter) ([]ordersuc.OrderView, int64, error)
	Get(ctx context.Context, c ordersuc.Caller, id uuid.UUID) (ordersuc.OrderView, error)
}

type ordersBase struct {
	orders OrdersReader
	tree   scopefilter.TreeReader
}

func (b ordersBase) caller(ctx context.Context, p Principal) (ordersuc.Caller, error) {
	f, err := resolveScope(ctx, b.tree, p, rbac.PermOrdersRead)
	if err != nil {
		return ordersuc.Caller{}, err
	}
	return ordersuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, nil
}

func (ordersBase) errs(tool string) errCases {
	return errCases{tool: tool, what: "order", notFound: []error{ordersuc.ErrNotFound},
		forbidden: []error{ordersuc.ErrForbidden}, invalid: asError[*ordersuc.ValidationError]}
}

// orderAmountsVisible: order prices are the buyer's purchase price and the
// seller's sale price, so the buyer needs pricing.purchase.read, the seller
// pricing.sale.read and an observer (a parent above both) either.
func orderAmountsVisible(p Principal, role string) bool {
	purchase := p.Auth.HasPermission(rbac.PermPricingPurchaseRead)
	sale := p.Auth.HasPermission(rbac.PermPricingSaleRead)
	switch role {
	case string(ordersuc.PartyBuyer):
		return purchase
	case string(ordersuc.PartySeller):
		return sale
	}
	return purchase || sale
}

type orderRow struct {
	UUID      uuid.UUID  `json:"uuid"`
	OrderNo   string     `json:"order_no"`
	Status    string     `json:"status"`
	Role      string     `json:"your_role"`
	Seller    string     `json:"seller"`
	Buyer     string     `json:"buyer"`
	Currency  string     `json:"currency,omitempty"`
	Total     string     `json:"total,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ShippedAt *time.Time `json:"shipped_at,omitempty"`
}

func toOrderRow(p Principal, v ordersuc.OrderView) orderRow {
	r := orderRow{
		UUID: v.UUID, OrderNo: v.OrderNo, Status: v.Status, Role: v.Role,
		Seller: dataText(v.Seller.Name, maxNameChars), Buyer: dataText(v.Buyer.Name, maxNameChars),
		CreatedAt: v.CreatedAt, ShippedAt: v.ShippedAt,
	}
	if orderAmountsVisible(p, v.Role) {
		r.Currency, r.Total = v.Currency, v.Total
	}
	return r
}

var orderStatuses = []string{
	ordersuc.StatusDraft, ordersuc.StatusSubmitted, ordersuc.StatusApproved, ordersuc.StatusPreparing,
	ordersuc.StatusReady, ordersuc.StatusProcessing, ordersuc.StatusShipped, ordersuc.StatusDelivered,
	ordersuc.StatusReceived, ordersuc.StatusCancelling, ordersuc.StatusCancelled,
}

// ListOrders: sipariş listesi.
type ListOrders struct{ ordersBase }

// Spec implements Tool.
func (ListOrders) Spec() Spec {
	return Spec{
		Name: "list_orders",
		Description: "List product orders of your organization (and of organizations within your access): placed " +
			"to your supplier (side=buyer) or received from your dealers (side=seller). Newest first. " +
			"Totals are included only when the user may read the prices.",
		InputSchema: object(map[string]any{
			"query":  str("Optional order number, tracking number or party name.", 100),
			"side":   enum("Optional: buyer (orders you placed) or seller (orders you received).", ordersuc.SideBuyer, ordersuc.SideSeller),
			"status": enum("Optional status filter.", orderStatuses...),
			"limit":  limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleOrders,
		Permissions: []string{rbac.PermOrdersRead},
	}
}

// Run implements Tool.
func (t ListOrders) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query  string `json:"query"`
		Side   string `json:"side"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	f := ordersuc.ListFilter{Side: in.Side, Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit)}
	if in.Status != "" {
		f.Statuses = []string{in.Status}
	}
	rows, total, err := t.orders.List(ctx, c, f)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	out := make([]orderRow, 0, len(rows))
	for _, v := range rows {
		out = append(out, toOrderRow(env.Principal, v))
	}
	return JSONResult(NewList(out, total))
}

// GetOrder: sipariş detayı.
type GetOrder struct{ ordersBase }

// Spec implements Tool.
func (GetOrder) Spec() Spec {
	return Spec{
		Name: "get_order",
		Description: "Get one order with its lines and status history, by uuid or order number. Prices are " +
			"included only when the user may read them.",
		InputSchema: object(map[string]any{
			"order": strMin("Order uuid or order number.", 2, 64),
		}, "order"),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleOrders,
		Permissions: []string{rbac.PermOrdersRead},
	}
}

type orderLine struct {
	Product   string  `json:"product"`
	SKU       string  `json:"sku"`
	Quantity  *int32  `json:"quantity,omitempty"`
	Meters    *string `json:"meters,omitempty"`
	Assigned  string  `json:"assigned"`
	UnitPrice string  `json:"unit_price,omitempty"`
	LineTotal string  `json:"line_total,omitempty"`
}

type orderStatusLog struct {
	From   *string   `json:"from,omitempty"`
	To     string    `json:"to"`
	Reason *string   `json:"reason,omitempty"`
	At     time.Time `json:"at"`
}

type orderDetail struct {
	orderRow
	Subtotal     string           `json:"subtotal,omitempty"`
	TaxTotal     string           `json:"tax_total,omitempty"`
	Note         *string          `json:"note,omitempty"`
	CancelReason *string          `json:"cancel_reason,omitempty"`
	Lines        []orderLine      `json:"lines"`
	History      []orderStatusLog `json:"history"`
}

// Run implements Tool.
func (t GetOrder) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Order string `json:"order"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	id, ok := parseID(in.Order)
	if !ok {
		no := strings.TrimSpace(in.Order)
		rows, _, err := t.orders.List(ctx, c, ordersuc.ListFilter{Q: no, Limit: 5})
		if err != nil {
			return t.errs(t.Spec().Name).result(err)
		}
		for _, r := range rows {
			if strings.EqualFold(r.OrderNo, no) {
				id, ok = r.UUID, true
				break
			}
		}
		if !ok {
			return notFound("order"), nil
		}
	}
	v, err := t.orders.Get(ctx, c, id)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	prices := orderAmountsVisible(env.Principal, v.Role)
	d := orderDetail{
		orderRow: toOrderRow(env.Principal, v), Note: textPtr(v.Note, maxTextChars),
		CancelReason: textPtr(v.CancelReason, maxTextChars), Lines: []orderLine{}, History: []orderStatusLog{},
	}
	if prices {
		d.Subtotal, d.TaxTotal = v.Subtotal, v.TaxTotal
	}
	for _, it := range v.Items {
		l := orderLine{Product: dataText(it.Product.Name, maxNameChars), SKU: it.Product.SKU,
			Quantity: it.Quantity, Meters: it.Meters, Assigned: it.Assigned}
		if prices {
			l.UnitPrice, l.LineTotal = it.UnitPrice, it.LineTotal
		}
		d.Lines = append(d.Lines, l)
	}
	for _, h := range v.History {
		d.History = append(d.History, orderStatusLog{From: h.FromStatus, To: h.ToStatus,
			Reason: textPtr(h.Reason, maxTextChars), At: h.CreatedAt})
	}
	return JSONResult(d)
}
