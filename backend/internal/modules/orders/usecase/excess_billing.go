package usecase

// Excess meter billing (TEC-1250, K7, 19.11 kapsam ekleme).
//
// A piece that completes a meter line takes the whole roll (or box) when
// what would stay is shorter than the minimum cut; the pieces then carry
// more meters than the line. Whether the buyer pays those extra meters is
// the seller organization's setting (organizations.excess_meter_billing):
//
//   - free (default): the line keeps the ordered meters; invoices, the
//     account, returns and the order total use them.
//   - charge: the line's meters are raised to the real meters of its
//     pieces, its line_total and the order total follow; ordered_meters
//     keeps what the buyer ordered. Invoices, e-invoices, the account,
//     returns and the dealer cost read the line, so they bill every meter.
//
// The mode is frozen on the line (order_items.excess_billing) when the
// extra meters are decided; a later change of the setting never touches
// it. When the extra meters go away (the whole piece is unassigned) the
// line goes back to its ordered meters and the mode is cleared. Every
// change of the billed meters is written to the order history.

import (
	"context"
	"fmt"
	"math/big"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// Excess billing modes (organizations.excess_meter_billing,
// order_items.excess_billing).
const (
	ExcessBillingFree   = "free"
	ExcessBillingCharge = "charge"
)

// Order history steps of a charged line (TEC-1250).
const (
	// ActionExcessCharged: the extra meters of a whole roll were added to
	// the line's billed meters.
	ActionExcessCharged = "order.excess_charged"
	// ActionExcessUncharged: the line went back to its ordered meters.
	ActionExcessUncharged = "order.excess_uncharged"
)

// excessBillingOf is the mode that applies to a line's extra meters: the
// frozen one on the line, else the seller's current setting.
func excessBillingOf(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem) (string, error) {
	if it.ExcessBilling.Valid {
		return it.ExcessBilling.String, nil
	}
	mode, err := q.GetOrganizationExcessMeterBilling(ctx, o.SellerOrgID)
	if err != nil {
		return "", fmt.Errorf("orders: excess billing setting: %w", err)
	}
	if mode != ExcessBillingCharge {
		return ExcessBillingFree, nil
	}
	return mode, nil
}

// orderedMeters is what the buyer ordered on a meter line: ordered_meters
// on a charged line, else the line's meters.
func orderedMeters(it db.OrderItem) *big.Rat {
	if it.OrderedMeters.Valid {
		return numericRat(it.OrderedMeters)
	}
	return numericRat(it.Meters)
}

// settleExcess reconciles a meter line with the real meters of its active
// pieces after an assignment or an unassignment: it freezes the excess
// billing mode when the pieces exceed the ordered meters, raises (charge)
// or restores the billed meters, the line total and the order total, and
// writes the history step. The caller holds the line lock.
func (s *Service) settleExcess(ctx context.Context, q *db.Queries, c Caller, o db.Order, itemID int64) error {
	it, err := q.GetOrderItem(ctx, db.GetOrderItemParams{ID: itemID, BrandID: o.BrandID})
	if err != nil {
		return fmt.Errorf("orders: line: %w", err)
	}
	if !it.Meters.Valid {
		return nil
	}
	ordered := orderedMeters(it)
	if ordered == nil {
		return nil
	}
	pieces, err := reservedOnLineRaw(ctx, q, it.ID)
	if err != nil {
		return err
	}
	excess := new(big.Rat).Sub(pieces, ordered)
	if excess.Sign() <= 0 {
		if !it.ExcessBilling.Valid {
			return nil
		}
		// The extra meters went away: back to the ordered meters.
		return s.setExcessBilling(ctx, q, c, o, it, pgtype.Text{}, ordered, nil)
	}
	mode, err := excessBillingOf(ctx, q, o, it)
	if err != nil {
		return err
	}
	billed := ordered
	if mode == ExcessBillingCharge {
		billed = pieces
	}
	return s.setExcessBilling(ctx, q, c, o, it, pgtype.Text{String: mode, Valid: true}, billed, excess)
}

// setExcessBilling writes the line's mode and billed meters; a change of
// the billed meters recomputes the totals and lands in the history.
func (s *Service) setExcessBilling(ctx context.Context, q *db.Queries, c Caller, o db.Order, it db.OrderItem,
	mode pgtype.Text, billed, excess *big.Rat) error {
	ordered := orderedMeters(it)
	current := numericRat(it.Meters)
	if it.ExcessBilling == mode && current.Cmp(billed) == 0 {
		return nil
	}
	meters, err := numeric(billed.FloatString(2))
	if err != nil {
		return err
	}
	var orderedCol pgtype.Numeric
	if billed.Cmp(ordered) != 0 {
		if orderedCol, err = numeric(ordered.FloatString(2)); err != nil {
			return err
		}
	}
	total := it.LineTotal
	if current.Cmp(billed) != 0 {
		price := numericRat(it.UnitPrice)
		if price == nil {
			price = new(big.Rat)
		}
		if total, err = numeric(new(big.Rat).Mul(price, billed).FloatString(2)); err != nil {
			return err
		}
	}
	if _, err := q.SetOrderItemExcessBilling(ctx, db.SetOrderItemExcessBillingParams{
		ID: it.ID, ExcessBilling: mode, OrderedMeters: orderedCol, Meters: meters, LineTotal: total,
	}); err != nil {
		return fmt.Errorf("orders: excess billing: %w", err)
	}
	if current.Cmp(billed) == 0 {
		return nil
	}
	if _, err := q.RecalculateOrderTotals(ctx, o.ID); err != nil {
		return fmt.Errorf("orders: totals: %w", err)
	}
	p := activityPayload{ItemUUID: it.Uuid.String()}
	if pr, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID}); err == nil {
		p.Product = pr.Name
	}
	action := ActionExcessUncharged
	if billed.Cmp(ordered) > 0 && excess != nil {
		action = ActionExcessCharged
		v := excess.FloatString(2)
		p.Meters = &v
	} else {
		v := ordered.FloatString(2)
		p.Meters = &v
	}
	return s.recordActivity(ctx, q, c, o, action, p)
}
