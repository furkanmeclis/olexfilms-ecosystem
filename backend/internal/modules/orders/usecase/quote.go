package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// seesDiscountTerms: the list price and the discount of the lines are the
// seller side's business (TEC-1064): not the buyer's, and only with
// pricing.sale.read.
func seesDiscountTerms(c Caller, o db.Order) bool {
	return partyOf(c, o) != PartyBuyer && c.can(rbac.PermPricingSaleRead)
}

// QuoteInput asks the prices of products for a new order: of the active
// organization to its seller, or — with BuyerOrgUUID — of the center for a
// dealer directly under it.
type QuoteInput struct {
	BuyerOrgUUID *string
	ProductUUIDs []string
}

// QuoteItem is one product's price on the new order. ListUnitPrice is the
// buyer's purchase price before the discount (seller side only), UnitPrice
// the discounted price; both nil when no price is found or the caller may
// not see prices.
type QuoteItem struct {
	ProductUUID   uuid.UUID `json:"product_uuid"`
	ListUnitPrice *string   `json:"list_unit_price"`
	UnitPrice     *string   `json:"unit_price"`
	PriceSource   string    `json:"price_source"`
}

// QuoteView is the price preview of a new order (TEC-1064): the order
// currency (the brand's), the current rate to TRY (nil without one; the
// order freezes its own at approval), whether this is the buyer's first
// order and — seller side with pricing.sale.read only — the discount that
// applies by default and the agreed rates.
type QuoteView struct {
	Currency              string      `json:"currency"`
	PricesVisible         bool        `json:"prices_visible"`
	FirstOrder            bool        `json:"first_order"`
	DiscountPct           *string     `json:"discount_pct"`
	StandardDiscountPct   *string     `json:"standard_discount_pct"`
	FirstOrderDiscountPct *string     `json:"first_order_discount_pct"`
	TryRate               *string     `json:"try_rate"`
	RateDate              *string     `json:"rate_date"`
	Items                 []QuoteItem `json:"items"`
}

// Quote previews the prices of a new order without writing anything.
func (s *Service) Quote(ctx context.Context, c Caller, in QuoteInput) (QuoteView, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermOrdersWrite) {
		return QuoteView{}, ErrForbidden
	}
	if len(in.ProductUUIDs) > MaxItems {
		return QuoteView{}, invalid("product_uuids", fmt.Sprintf("at most %d products", MaxItems))
	}
	q := s.q
	active, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return QuoteView{}, fmt.Errorf("orders: organization: %w", err)
	}
	var buyer, seller db.Organization
	sellerSide := in.BuyerOrgUUID != nil && strings.TrimSpace(*in.BuyerOrgUUID) != ""
	if sellerSide {
		if active.Type != OrgCenter {
			return QuoteView{}, ErrForbidden
		}
		id, err := uuid.Parse(strings.TrimSpace(*in.BuyerOrgUUID))
		if err != nil {
			return QuoteView{}, invalid("buyer_org_uuid", "must be a UUID")
		}
		buyer, err = q.GetOrganizationByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return QuoteView{}, errNotChildDealer()
		}
		if err != nil {
			return QuoteView{}, fmt.Errorf("orders: buyer: %w", err)
		}
		if buyer.Type != OrgDealer || buyer.BrandID != active.BrandID {
			return QuoteView{}, errNotChildDealer()
		}
		sup, err := s.supplier(ctx, q, buyer)
		if errors.Is(err, ErrNoSupplier) || (err == nil && sup.ID != active.ID) {
			return QuoteView{}, errNotChildDealer()
		}
		if err != nil {
			return QuoteView{}, err
		}
		seller = active
	} else {
		buyer = active
		if seller, err = s.supplier(ctx, q, buyer); err != nil {
			return QuoteView{}, err
		}
	}
	brand, err := q.GetBrandByID(ctx, buyer.BrandID)
	if err != nil {
		return QuoteView{}, fmt.Errorf("orders: brand: %w", err)
	}
	currency := strings.TrimSpace(brand.Currency)
	d, err := buyerDiscount(ctx, q, buyer, seller.ID, 0)
	if err != nil {
		return QuoteView{}, err
	}
	visible := c.can(rbac.PermPricingPurchaseRead)
	if sellerSide {
		visible = c.can(rbac.PermPricingSaleRead)
	}
	out := QuoteView{Currency: currency, PricesVisible: visible, FirstOrder: d.FirstOrder, Items: []QuoteItem{}}
	if visible && sellerSide {
		if d.Pct != nil {
			v := d.Pct.FloatString(2)
			out.DiscountPct = &v
		}
		out.StandardDiscountPct = numericTextPtr(d.Standard, 2)
		out.FirstOrderDiscountPct = numericTextPtr(d.FirstOnly, 2)
	}
	if visible {
		if snap, err := s.rates.ResolveRate(ctx, s.now().UTC(), currency, TryCurrency); err == nil {
			rate, day := snap.Rate, snap.RateDate
			out.TryRate, out.RateDate = &rate, &day
		} else if !errors.Is(err, fxrates.ErrRateNotFound) {
			return QuoteView{}, fmt.Errorf("orders: rate: %w", err)
		}
	}
	products := make([]db.Product, 0, len(in.ProductUUIDs))
	for i, raw := range in.ProductUUIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return QuoteView{}, invalid(fmt.Sprintf("product_uuids[%d]", i), "must be a UUID")
		}
		p, err := q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: id, BrandID: buyer.BrandID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !p.Active) {
			return QuoteView{}, invalid(fmt.Sprintf("product_uuids[%d]", i), "product not found")
		}
		if err != nil {
			return QuoteView{}, fmt.Errorf("orders: product: %w", err)
		}
		products = append(products, p)
	}
	prices := map[int64]pricingusecase.PurchasePrice{}
	if visible && len(products) > 0 {
		ids := make([]int64, len(products))
		for i, p := range products {
			ids[i] = p.ID
		}
		if prices, err = pricingusecase.New(q).BuyerPurchasePrices(ctx, buyer.ID, buyer.Type, buyer.BrandID, ids, currency); err != nil {
			return QuoteView{}, fmt.Errorf("orders: prices: %w", err)
		}
	}
	for _, p := range products {
		item := QuoteItem{ProductUUID: p.Uuid, PriceSource: PriceSourceMissing}
		if pr, ok := prices[p.ID]; ok {
			unit, err := discountedPrice(pr.Price, d.Pct)
			if err != nil {
				return QuoteView{}, err
			}
			item.UnitPrice, item.PriceSource = &unit, priceSource(pr.Source)
			if sellerSide {
				list := pr.Price
				item.ListUnitPrice = &list
			}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
