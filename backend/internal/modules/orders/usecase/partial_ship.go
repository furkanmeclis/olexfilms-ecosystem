package usecase

// Partial shipment and backorders (TEC-1227).
//
// The default rule stays: an order leaves when every line is fully picked
// (preparing -> ready needs checkFullyAssigned). When stock is short the
// seller's warehouse staff (orders.ship) may choose to send the picked part
// now: PartialShip splits a preparing order in one transaction.
//
//   - A line with every unit picked stays as it is.
//   - A line with part of its amount picked keeps the picked amount; the
//     rest becomes a new line of the backorder at the same unit price,
//     discount, gift and plotter marks. The line total is split so the two
//     add up to the original (the shipped part is unit price × amount,
//     the backorder gets the rest).
//   - A line nothing was picked for moves to the backorder whole.
//
// The backorder is a new order of the same seller, buyer, brand, currency
// and seller warehouse, linked by orders.backorder_of_order_id. It takes
// the frozen rate and approval of the main order (no new price approval)
// and starts approved, so it falls into the warehouse queue
// (orders.warehouse_new). It is cancelled like any order (its active
// reservations are released). The main order moves to ready and is shipped
// with the usual ship step.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Partial shipment errors; the handler maps them to 409.
var (
	// ErrNotPartiallyShippable: only a preparing order is split.
	ErrNotPartiallyShippable = errors.New("orders: only a preparing order can be shipped partially")
	// ErrNothingPicked: no unit is assigned yet; nothing would ship.
	ErrNothingPicked = errors.New("orders: nothing is picked yet")
	// ErrFullyPicked: every line is fully picked; ship the whole order.
	ErrFullyPicked = errors.New("orders: every line is fully picked; ship the whole order")
)

// Order activity actions of a partial shipment (timeline).
const (
	ActionBackorderCreated = "order.backorder_created"
	ActionBackorderOf      = "order.backorder_opened"
)

// PartialShipInput is the optional note of a partial shipment.
type PartialShipInput struct {
	Reason *string
}

// canPartialShip: the seller's warehouse staff split a preparing order.
func canPartialShip(c Caller, o db.Order) bool {
	return partyOf(c, o) == PartySeller && c.can(rbac.PermOrdersShip) && o.Status == StatusPreparing
}

// pickedSplit is one line of a partial shipment.
type pickedSplit struct {
	item   db.OrderItem
	picked *big.Rat
	rest   *big.Rat
}

// PartialShip ships the picked part of a preparing order: the unpicked
// rest moves to a new backorder and the order moves to ready.
func (s *Service) PartialShip(ctx context.Context, c Caller, orderUUID uuid.UUID, in PartialShipInput) (OrderView, error) {
	var result db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartySeller || !c.can(rbac.PermOrdersShip) {
			return ErrForbidden
		}
		if o.Status != StatusPreparing {
			return ErrNotPartiallyShippable
		}
		items, err := q.LockOrderItems(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("orders: lines: %w", err)
		}
		splits := make([]pickedSplit, 0, len(items))
		anyPicked, anyShort := false, false
		for _, it := range items {
			done, err := reservedOnLine(ctx, q, it.ID)
			if err != nil {
				return err
			}
			rest := new(big.Rat).Sub(lineAmount(it), done)
			if done.Sign() > 0 {
				anyPicked = true
			}
			if rest.Sign() > 0 {
				anyShort = true
			}
			splits = append(splits, pickedSplit{item: it, picked: done, rest: rest})
		}
		if !anyPicked {
			return ErrNothingPicked
		}
		if !anyShort {
			return ErrFullyPicked
		}
		b, err := q.CreateOrder(ctx, db.CreateOrderParams{
			SellerOrgID: o.SellerOrgID, BrandID: o.BrandID, BuyerOrgID: o.BuyerOrgID,
			Currency: strings.TrimSpace(o.Currency), Note: o.Note, CreatedByUserID: c.actor(),
			CreatedBySeller: o.CreatedBySeller,
		})
		if err != nil {
			return fmt.Errorf("orders: create backorder: %w", err)
		}
		changes := make([]LineChange, 0, len(splits))
		for _, sp := range splits {
			if sp.rest.Sign() <= 0 {
				continue
			}
			p, err := q.GetProduct(ctx, db.GetProductParams{ID: sp.item.ProductID, BrandID: o.BrandID})
			if err != nil {
				return fmt.Errorf("orders: product: %w", err)
			}
			before := splitAmountText(sp.item, lineAmount(sp.item))
			if sp.picked.Sign() == 0 {
				if _, err := q.MoveOrderItemToOrder(ctx, db.MoveOrderItemToOrderParams{ID: sp.item.ID, OrderID: b.ID}); err != nil {
					return fmt.Errorf("orders: move line: %w", err)
				}
				changes = append(changes, LineChange{Product: p.Name, SKU: p.Sku, Before: &before})
				continue
			}
			if err := splitLine(ctx, q, b, sp); err != nil {
				return err
			}
			after := splitAmountText(sp.item, sp.picked)
			changes = append(changes, LineChange{Product: p.Name, SKU: p.Sku, Before: &before, After: &after})
		}
		if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
			return fmt.Errorf("orders: totals: %w", err)
		}
		if b, err = q.RecalculateOrderTotals(ctx, b.ID); err != nil {
			return fmt.Errorf("orders: backorder totals: %w", err)
		}
		if b, err = q.MarkOrderBackorder(ctx, db.MarkOrderBackorderParams{ID: b.ID, SourceID: o.ID}); err != nil {
			return fmt.Errorf("orders: backorder approval: %w", err)
		}
		// The rest is on the backorder: the order is fully picked now.
		if err := checkFullyAssigned(ctx, q, o); err != nil {
			return err
		}
		from := o.Status
		if o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: StatusReady}); err != nil {
			return fmt.Errorf("orders: ready: %w", err)
		}
		meta := map[string]any{"partial": true, "backorder": b.OrderNo}
		if err := s.history(ctx, q, o, from, StatusReady, c, in.Reason, meta); err != nil {
			return err
		}
		if err := s.history(ctx, q, b, "", StatusApproved, c, nil, map[string]any{"backorder_of": o.OrderNo}); err != nil {
			return err
		}
		n := int64(len(changes))
		if err := s.recordActivity(ctx, q, c, o, ActionBackorderCreated, activityPayload{
			RelatedOrderNo: b.OrderNo, Lines: &n, Changes: changes,
		}); err != nil {
			return err
		}
		if err := s.recordActivity(ctx, q, c, b, ActionBackorderOf, activityPayload{RelatedOrderNo: o.OrderNo, Lines: &n}); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.OrdersReady, o, from, c); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.OrdersCreated, b, "", c); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.OrdersApproved, b, "", c); err != nil {
			return err
		}
		var w *db.Warehouse
		if b.SellerWarehouseID.Valid {
			wh, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: b.SellerWarehouseID.Int64, OrganizationID: b.SellerOrgID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("orders: warehouse: %w", err)
			}
			if err == nil {
				w = &wh
			}
		}
		result = o
		return s.emitWarehouseNew(ctx, q, tx, b, w, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

// splitLine keeps the picked amount on the line and opens the rest as a
// line of the backorder b with the same price terms. The totals add up to
// the original line total.
func splitLine(ctx context.Context, q *db.Queries, b db.Order, sp pickedSplit) error {
	it := sp.item
	total := numericRat(it.LineTotal)
	if total == nil {
		total = new(big.Rat)
	}
	keepText, err := lineTotal(numericText(it.UnitPrice, 4), sp.picked)
	if err != nil {
		return fmt.Errorf("orders: line total: %w", err)
	}
	if it.IsGift {
		keepText = "0.00"
	}
	keep, _ := parseRat(keepText)
	if keep.Cmp(total) > 0 {
		keep = total
	}
	restTotal := new(big.Rat).Sub(total, keep)
	keepNum, err := numeric(keep.FloatString(2))
	if err != nil {
		return err
	}
	restNum, err := numeric(restTotal.FloatString(2))
	if err != nil {
		return err
	}
	var keepQty, restQty pgtype.Int4
	var keepMeters, restMeters pgtype.Numeric
	if it.Meters.Valid {
		if keepMeters, err = numeric(sp.picked.FloatString(2)); err != nil {
			return err
		}
		if restMeters, err = numeric(sp.rest.FloatString(2)); err != nil {
			return err
		}
	} else {
		keepQty = pgtype.Int4{Int32: int32(sp.picked.Num().Int64()), Valid: true}
		restQty = pgtype.Int4{Int32: int32(sp.rest.Num().Int64()), Valid: true}
	}
	if _, err := q.SetOrderItemAmount(ctx, db.SetOrderItemAmountParams{
		ID: it.ID, Quantity: keepQty, Meters: keepMeters, LineTotal: keepNum,
	}); err != nil {
		return fmt.Errorf("orders: keep picked amount: %w", err)
	}
	if _, err := q.CreateOrderItem(ctx, db.CreateOrderItemParams{
		OrderID: b.ID, ProductID: it.ProductID, Quantity: restQty, Meters: restMeters,
		UnitPrice: it.UnitPrice, PriceSource: it.PriceSource, RecommendedPriceSnapshot: it.RecommendedPriceSnapshot,
		LineTotal: restNum, Note: it.Note, ListUnitPrice: it.ListUnitPrice, DiscountPct: it.DiscountPct,
		IsGift: pgtype.Bool{Bool: it.IsGift, Valid: true}, PlotterCut: it.PlotterCut, PlotterCredits: it.PlotterCredits,
		LineCredits: it.LineCredits,
	}); err != nil {
		return fmt.Errorf("orders: backorder line: %w", err)
	}
	return nil
}

// splitAmountText is a line amount for the timeline: whole pieces or meters.
func splitAmountText(it db.OrderItem, r *big.Rat) string {
	if it.Meters.Valid {
		return r.FloatString(2)
	}
	return r.FloatString(0)
}

// withBackorders fills the order a backorder was split from and the
// backorders of an order (detail only). Both share seller and buyer.
func (s *Service) withBackorders(ctx context.Context, q *db.Queries, c Caller, o db.Order, v *OrderView) error {
	v.Backorders = []OrderLink{}
	v.CanPartialShip = canPartialShip(c, o)
	if o.BackorderOfOrderID.Valid {
		p, err := q.GetOrder(ctx, db.GetOrderParams{ID: o.BackorderOfOrderID.Int64, BrandID: o.BrandID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("orders: backorder source: %w", err)
		}
		if err == nil && visible(c, p) {
			link := orderLink(ctx, p)
			v.BackorderOf = &link
		}
	}
	rows, err := q.ListOrderBackorders(ctx, db.ListOrderBackordersParams{
		BackorderOfOrderID: pgtype.Int8{Int64: o.ID, Valid: true}, BrandID: o.BrandID,
	})
	if err != nil {
		return fmt.Errorf("orders: backorders: %w", err)
	}
	for _, r := range rows {
		if visible(c, r) {
			v.Backorders = append(v.Backorders, orderLink(ctx, r))
		}
	}
	return nil
}
