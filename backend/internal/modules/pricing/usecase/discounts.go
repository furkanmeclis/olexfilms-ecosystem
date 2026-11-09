package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1064 (RV-5.3): the discount a seller (center or distributor) agreed
// with an organization directly under it. standard_discount_pct applies to
// every order of that buyer; first_order_discount_pct replaces it on the
// buyer's first order. The orders module applies them; here they are read
// and written by the seller holding pricing.sale.write.

// ErrBuyerNotFound: the organization is not directly under the caller.
var ErrBuyerNotFound = errors.New("pricing: organization not found under the caller")

// discountRe: NUMERIC(5,2) between 0 and 100.
var discountRe = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,2})?$`)

// NormalizeDiscount validates a discount percentage (0..100, at most two
// fractional digits, decimal comma accepted) and returns it with two
// decimals.
func NormalizeDiscount(raw string) (string, bool) {
	v := strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	if !discountRe.MatchString(v) {
		return "", false
	}
	r, ok := new(big.Rat).SetString(v)
	if !ok || r.Sign() < 0 || r.Cmp(big.NewRat(100, 1)) > 0 {
		return "", false
	}
	return r.FloatString(2), true
}

// DiscountOrg names the buyer organization.
type DiscountOrg struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// DiscountView is the agreed discount of one buyer organization. FirstOrder
// is true while the buyer has placed no order with the caller (the
// first-order discount then applies).
type DiscountView struct {
	Organization          DiscountOrg `json:"organization"`
	StandardDiscountPct   *string     `json:"standard_discount_pct"`
	FirstOrderDiscountPct *string     `json:"first_order_discount_pct"`
	FirstOrder            bool        `json:"first_order"`
}

// DiscountInput replaces both discounts; nil or "" clears one.
type DiscountInput struct {
	StandardDiscountPct   *string
	FirstOrderDiscountPct *string
}

// Discounts reads and writes the buyer discounts.
type Discounts struct {
	q *db.Queries
}

// NewDiscounts creates the discount service.
func NewDiscounts(q *db.Queries) *Discounts { return &Discounts{q: q} }

// child loads an organization directly under the caller's organization.
func (d *Discounts) child(ctx context.Context, v Viewer, orgUUID uuid.UUID) (db.Organization, error) {
	if !v.SaleWrite || (v.OrgType != OrgCenter && v.OrgType != OrgDistributor) {
		return db.Organization{}, ErrForbidden
	}
	o, err := d.q.GetOrganizationByUUID(ctx, orgUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrBuyerNotFound
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("pricing: organization: %w", err)
	}
	if o.DeletedAt.Valid || o.BrandID != v.BrandID || !o.ParentID.Valid || o.ParentID.Int64 != v.OrgID {
		return db.Organization{}, ErrBuyerNotFound
	}
	return o, nil
}

func (d *Discounts) view(ctx context.Context, v Viewer, o db.Organization) (DiscountView, error) {
	n, err := d.q.CountPlacedBuyerOrders(ctx, db.CountPlacedBuyerOrdersParams{BuyerOrgID: o.ID, SellerOrgID: v.OrgID})
	if err != nil {
		return DiscountView{}, fmt.Errorf("pricing: placed orders: %w", err)
	}
	return DiscountView{
		Organization:          DiscountOrg{UUID: o.Uuid, Name: o.Name, Type: o.Type},
		StandardDiscountPct:   pctText(o.StandardDiscountPct),
		FirstOrderDiscountPct: pctText(o.FirstOrderDiscountPct),
		FirstOrder:            n == 0,
	}, nil
}

// Get returns the discounts of an organization directly under the caller.
func (d *Discounts) Get(ctx context.Context, v Viewer, orgUUID uuid.UUID) (DiscountView, error) {
	o, err := d.child(ctx, v, orgUUID)
	if err != nil {
		return DiscountView{}, err
	}
	return d.view(ctx, v, o)
}

// Set replaces the discounts of an organization directly under the caller.
func (d *Discounts) Set(ctx context.Context, v Viewer, orgUUID uuid.UUID, in DiscountInput) (DiscountView, error) {
	std, err := pctInput("standard_discount_pct", in.StandardDiscountPct)
	if err != nil {
		return DiscountView{}, err
	}
	first, err := pctInput("first_order_discount_pct", in.FirstOrderDiscountPct)
	if err != nil {
		return DiscountView{}, err
	}
	o, err := d.child(ctx, v, orgUUID)
	if err != nil {
		return DiscountView{}, err
	}
	o, err = d.q.SetOrganizationDiscounts(ctx, db.SetOrganizationDiscountsParams{
		ID: o.ID, StandardDiscountPct: std, FirstOrderDiscountPct: first,
	})
	if err != nil {
		return DiscountView{}, fmt.Errorf("pricing: set discounts: %w", err)
	}
	return d.view(ctx, v, o)
}

func pctInput(field string, raw *string) (pgtype.Numeric, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return pgtype.Numeric{}, nil
	}
	v, ok := NormalizeDiscount(*raw)
	if !ok {
		return pgtype.Numeric{}, invalid(field, "must be a percentage between 0 and 100 with at most 2 fractional digits")
	}
	var n pgtype.Numeric
	if err := n.Scan(v); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("pricing: numeric: %w", err)
	}
	return n, nil
}

// pctText formats a NUMERIC(5,2) with two decimals (nil when NULL).
func pctText(n pgtype.Numeric) *string {
	if !n.Valid || n.Int == nil {
		return nil
	}
	r := new(big.Rat).SetInt(n.Int)
	e := n.Exp
	if e != 0 {
		abs := e
		if abs < 0 {
			abs = -abs
		}
		p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs)), nil))
		if e > 0 {
			r.Mul(r, p)
		} else {
			r.Quo(r, p)
		}
	}
	s := r.FloatString(2)
	return &s
}
