package usecase

// TEC-1056 (RV-5.5 / 5.9): the seller fixes the lines of an accepted order
// before it ships — a wrong or missing product, a changed amount, the
// plexiglass and sign sets a new dealer gets on top of the products. The
// sales staff (orders.write) and the warehouse staff (orders.ship) edit
// while the order is submitted, approved, processing or preparing; a ready
// order is reopened first (ready -> preparing), a shipped one is closed.
//
// Unlike the buyer's draft edit (all lines replaced), the lines are matched
// by product: a kept line keeps its uuid, its locked unit price and its
// assignments; a new line is priced by the pricing module (no typed price,
// so the warehouse staff never enters one); a removed line releases its
// assignments and reservations. An amount below what is already assigned is
// refused (409 ORDER_LINE_BELOW_ASSIGNED): which unit to take off is the
// warehouse's call, made with the unassign action first.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5"
)

// ErrBelowAssigned: a line's new amount is below its assigned units.
var ErrBelowAssigned = errors.New("orders: the new amount is below the assigned units")

// sellerEditable are the statuses the seller edits the lines in.
var sellerEditable = map[string]bool{
	StatusSubmitted: true, StatusApproved: true, StatusProcessing: true, StatusPreparing: true,
}

// canEditItems reports whether the caller may change the lines of o now:
// the buyer its draft (orders.write), the seller before it is ready
// (orders.write or orders.ship).
func canEditItems(c Caller, o db.Order) bool {
	switch partyOf(c, o) {
	case PartyBuyer:
		return o.Status == StatusDraft && c.can(rbac.PermOrdersWrite)
	case PartySeller:
		return sellerEditable[o.Status] && (c.can(rbac.PermOrdersWrite) || c.can(rbac.PermOrdersShip))
	}
	return false
}

// LineChange is one line of an edit in the timeline: Before is the old
// amount (null: added), After the new one (null: removed); quantities as
// whole numbers, meters with two decimals.
type LineChange struct {
	Product string  `json:"product"`
	SKU     string  `json:"sku,omitempty"`
	Before  *string `json:"before"`
	After   *string `json:"after"`
	// PlotterCut: the line's plotter mark after the edit, set when it
	// changed (TEC-1058).
	PlotterCut *bool `json:"plotter_cut,omitempty"`
}

type lineSnap struct {
	name, sku string
	amount    string
	order     int64
	plotter   bool
}

// snapshotLines keeps the amount of every line by product.
func snapshotLines(ctx context.Context, q *db.Queries, o db.Order) (map[int64]lineSnap, error) {
	items, err := q.ListOrderItems(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("orders: lines: %w", err)
	}
	out := make(map[int64]lineSnap, len(items))
	for _, it := range items {
		p, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID})
		if err != nil {
			return nil, fmt.Errorf("orders: product: %w", err)
		}
		out[it.ProductID] = lineSnap{name: p.Name, sku: p.Sku, amount: amountText(it), order: it.ID, plotter: it.PlotterCut}
	}
	return out, nil
}

func amountText(it db.OrderItem) string {
	if it.Meters.Valid {
		return numericText(it.Meters, 2)
	}
	return fmt.Sprintf("%d", it.Quantity.Int32)
}

// diffLines lists the added, changed and removed lines between two
// snapshots (by line order, removed lines first in their old order).
func diffLines(before, after map[int64]lineSnap) []LineChange {
	type keyed struct {
		c   LineChange
		seq int64
	}
	var all []keyed
	for id, b := range before {
		a, ok := after[id]
		switch {
		case !ok:
			v := b.amount
			all = append(all, keyed{LineChange{Product: b.name, SKU: b.sku, Before: &v}, b.order})
		case a.amount != b.amount || a.plotter != b.plotter:
			bv, av := b.amount, a.amount
			c := LineChange{Product: b.name, SKU: b.sku, Before: &bv, After: &av}
			if a.plotter != b.plotter {
				pc := a.plotter
				c.PlotterCut = &pc
			}
			all = append(all, keyed{c, b.order})
		}
	}
	for id, a := range after {
		if _, ok := before[id]; !ok {
			v := a.amount
			c := LineChange{Product: a.name, SKU: a.sku, After: &v}
			if a.plotter {
				pc := true
				c.PlotterCut = &pc
			}
			all = append(all, keyed{c, a.order})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].seq < all[j].seq })
	out := make([]LineChange, 0, len(all))
	for _, k := range all {
		out = append(out, k.c)
	}
	return out
}

// editSellerLines applies the seller's edit to a locked order (see the file
// comment) and reports whether anything changed.
func (s *Service) editSellerLines(ctx context.Context, q *db.Queries, c Caller, o db.Order, items []ItemInput) (bool, error) {
	// TEC-1061: the seller may add a product closed to dealer orders (as
	// createForBuyer does), so no dealer rules here.
	lines, err := resolveLines(ctx, q, o.BrandID, items, lineRules{})
	if err != nil {
		return false, err
	}
	old, err := q.LockOrderItems(ctx, o.ID)
	if err != nil {
		return false, fmt.Errorf("orders: lines: %w", err)
	}
	byProduct := make(map[int64]db.OrderItem, len(old))
	for _, it := range old {
		byProduct[it.ProductID] = it
	}
	wanted := make(map[int64]bool, len(lines))
	changed := false
	var added []line
	var addedIdx []int
	for i, l := range lines {
		wanted[l.product.ID] = true
		it, ok := byProduct[l.product.ID]
		if !ok {
			added, addedIdx = append(added, l), append(addedIdx, i)
			continue
		}
		note := it.Note
		if items[i].Note != nil {
			note = l.note
		}
		same := ratEq(lineAmount(it), l.amount)
		if p := items[i].PlotterCut; p != nil && *p != it.PlotterCut {
			if _, err := q.SetOrderItemPlotterCut(ctx, db.SetOrderItemPlotterCutParams{ID: it.ID, PlotterCut: *p}); err != nil {
				return false, fmt.Errorf("orders: plotter mark: %w", err)
			}
			changed = true
		}
		if same && note == it.Note {
			continue
		}
		if !same {
			done, err := reservedOnLine(ctx, q, it.ID)
			if err != nil {
				return false, err
			}
			if done.Cmp(l.amount) > 0 {
				return false, ErrBelowAssigned
			}
		}
		total, err := lineTotal(numericText(it.UnitPrice, 4), l.amount)
		if err != nil {
			return false, err
		}
		lt, err := numeric(total)
		if err != nil {
			return false, err
		}
		if _, err := q.UpdateOrderItem(ctx, db.UpdateOrderItemParams{
			ID: it.ID, Quantity: l.qty, Meters: l.meters, UnitPrice: it.UnitPrice,
			PriceSource: it.PriceSource, RecommendedPriceSnapshot: it.RecommendedPriceSnapshot,
			LineTotal: lt, Note: note,
		}); err != nil {
			return false, fmt.Errorf("orders: update line: %w", err)
		}
		changed = true
	}
	for _, it := range old {
		if wanted[it.ProductID] {
			continue
		}
		if err := s.removeLine(ctx, q, c, o, it); err != nil {
			return false, err
		}
		changed = true
	}
	if len(added) > 0 {
		if err := s.insertEditedLines(ctx, q, o, added, addedIdx); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

// removeLine takes a line off an order before shipping: every assignment
// is undone (reservation released, unit_unassigned written), the released
// reservation history of the line goes with it.
func (s *Service) removeLine(ctx context.Context, q *db.Queries, c Caller, o db.Order, it db.OrderItem) error {
	units, err := q.ListOrderItemUnitsByItem(ctx, it.ID)
	if err != nil {
		return fmt.Errorf("orders: assigned units: %w", err)
	}
	for _, a := range units {
		if a.MovementID.Valid {
			// A unit with a movement has shipped; never the case before
			// shipping, refused rather than lost.
			return ErrNotEditable
		}
		if a.ReservationID.Valid {
			if _, err := q.ReleaseStockReservation(ctx, a.ReservationID.Int64); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("orders: release: %w", err)
			}
		}
		if _, err := q.DeleteOrderItemUnit(ctx, a.ID); err != nil {
			return fmt.Errorf("orders: unassign: %w", err)
		}
		u, err := q.GetUnit(ctx, a.UnitID)
		if err != nil {
			return fmt.Errorf("orders: unit: %w", err)
		}
		if err := s.recordUnassigned(ctx, q, c, o, it, u, a); err != nil {
			return err
		}
	}
	active, err := q.ListActiveReservationsByItem(ctx, it.ID)
	if err != nil {
		return fmt.Errorf("orders: line reservations: %w", err)
	}
	for _, r := range active {
		if _, err := q.ReleaseStockReservation(ctx, r.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("orders: release: %w", err)
		}
	}
	if _, err := q.DeleteReleasedReservationsByItem(ctx, it.ID); err != nil {
		return fmt.Errorf("orders: reservation history: %w", err)
	}
	if _, err := q.DeleteOrderItem(ctx, it.ID); err != nil {
		return fmt.Errorf("orders: delete line: %w", err)
	}
	return nil
}

// insertEditedLines prices and writes the lines the seller adds. An order
// the center opened for a dealer never blocks on a price (0, missing, as at
// creation, TEC-1024); any other order needs the buyer's purchase price
// (422 PRICE_NOT_FOUND, K8).
func (s *Service) insertEditedLines(ctx context.Context, q *db.Queries, o db.Order, lines []line, idx []int) error {
	buyer, err := q.GetOrganizationByID(ctx, o.BuyerOrgID)
	if err != nil {
		return fmt.Errorf("orders: buyer: %w", err)
	}
	if o.CreatedBySeller {
		return s.insertSellerLines(ctx, q, o, buyer, lines, nil)
	}
	// TEC-1064: an added line gets the buyer's default discount, like the
	// buyer's own lines (insertLines).
	d, err := buyerDiscount(ctx, q, buyer, o.SellerOrgID, o.ID)
	if err != nil {
		return err
	}
	ids := make([]int64, len(lines))
	amounts := make([]*big.Rat, len(lines))
	discounts := make([]*big.Rat, len(lines))
	fields := make([]string, len(lines))
	for i, l := range lines {
		ids[i], amounts[i], discounts[i], fields[i] = l.product.ID, l.amount, d.Pct, fmt.Sprintf("items[%d].product_uuid", idx[i])
	}
	priced, err := priceLines(ctx, q, buyer, o.Currency, ids, amounts, discounts, fields)
	if err != nil {
		return err
	}
	for i, l := range lines {
		if err := createLine(ctx, q, o, l, priced[i]); err != nil {
			return err
		}
	}
	return nil
}
