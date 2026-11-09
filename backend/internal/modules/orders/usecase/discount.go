package usecase

import (
	"context"
	"fmt"
	"math/big"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1064 (RV-5.3): a buyer's discount is agreed by its seller and kept on
// the buyer organization (standard_discount_pct, first_order_discount_pct).
// A line keeps its list price (list_unit_price), the applied discount
// (discount_pct) and the charged price (unit_price); a gift line is
// charged 0.

var hundred = big.NewRat(100, 1)

// discountedPrice is list × (100 − pct) / 100 with four decimals (the
// unit_price scale); a nil or zero pct leaves the list price.
func discountedPrice(list string, pct *big.Rat) (string, error) {
	p, err := parseRat(list)
	if err != nil {
		return "", err
	}
	if pct == nil || pct.Sign() == 0 {
		return p.FloatString(4), nil
	}
	f := new(big.Rat).Sub(hundred, pct)
	f.Quo(f, hundred)
	return new(big.Rat).Mul(p, f).FloatString(4), nil
}

// BuyerDiscount is the discount a seller applies to a buyer's next order.
type BuyerDiscount struct {
	// Pct is the applied percentage (nil: none).
	Pct *big.Rat
	// FirstOrder: the buyer has placed no order with the seller yet.
	FirstOrder bool
	Standard   pgtype.Numeric
	FirstOnly  pgtype.Numeric
}

// buyerDiscount resolves the default discount of the buyer's order to the
// seller: the first-order discount on the first placed order (when it is
// set), otherwise the standard one. excludeOrderID is the order being
// priced (0: none yet).
func buyerDiscount(ctx context.Context, q *db.Queries, buyer db.Organization, sellerID, excludeOrderID int64) (BuyerDiscount, error) {
	n, err := q.CountPlacedBuyerOrders(ctx, db.CountPlacedBuyerOrdersParams{
		BuyerOrgID: buyer.ID, SellerOrgID: sellerID, ExcludeOrderID: excludeOrderID,
	})
	if err != nil {
		return BuyerDiscount{}, fmt.Errorf("orders: placed orders: %w", err)
	}
	d := BuyerDiscount{FirstOrder: n == 0, Standard: buyer.StandardDiscountPct, FirstOnly: buyer.FirstOrderDiscountPct}
	switch {
	case d.FirstOrder && buyer.FirstOrderDiscountPct.Valid:
		d.Pct = numericRat(buyer.FirstOrderDiscountPct)
	case buyer.StandardDiscountPct.Valid:
		d.Pct = numericRat(buyer.StandardDiscountPct)
	}
	return d, nil
}

// lineCharge is what one line stores.
type lineCharge struct {
	listPrice pgtype.Numeric
	discount  pgtype.Numeric
	unitPrice pgtype.Numeric
	lineTotal pgtype.Numeric
	source    string
	gift      bool
}

// chargeInput describes how a line is charged.
type chargeInput struct {
	// list is the buyer's purchase price (nil: none found).
	list   *string
	source string
	amount *big.Rat
	// discount applies to the list price (nil: none).
	discount *big.Rat
	// manual is a unit price typed by the center; it is charged as is.
	manual *string
	gift   bool
}

func optNumeric(s *string) (pgtype.Numeric, error) {
	if s == nil {
		return pgtype.Numeric{}, nil
	}
	return numeric(*s)
}

// charge prices one line: a gift is 0 (no discount), a typed price is
// charged as is, a list price gets the discount, no price at all is 0
// (missing).
func charge(in chargeInput) (lineCharge, error) {
	list, err := optNumeric(in.list)
	if err != nil {
		return lineCharge{}, err
	}
	out := lineCharge{listPrice: list, source: in.source, gift: in.gift}
	price := "0"
	switch {
	case in.gift:
		if in.list == nil {
			out.source = PriceSourceMissing
		}
	case in.manual != nil:
		price, out.source = *in.manual, PriceSourceManual
	case in.list != nil:
		if price, err = discountedPrice(*in.list, in.discount); err != nil {
			return lineCharge{}, err
		}
		if in.discount != nil && in.discount.Sign() > 0 {
			if out.discount, err = numeric(in.discount.FloatString(2)); err != nil {
				return lineCharge{}, err
			}
		}
	default:
		out.source = PriceSourceMissing
	}
	total, err := lineTotal(price, in.amount)
	if err != nil {
		return lineCharge{}, err
	}
	if out.unitPrice, err = numeric(price); err != nil {
		return lineCharge{}, err
	}
	if out.lineTotal, err = numeric(total); err != nil {
		return lineCharge{}, err
	}
	return out, nil
}
