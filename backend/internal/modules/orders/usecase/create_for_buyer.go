package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Price sources of a line the center prices itself (TEC-1024).
const (
	// PriceSourceManual: the unit price typed by the center.
	PriceSourceManual = "manual"
	// PriceSourceMissing: no purchase price and none typed; the line is 0.
	PriceSourceMissing = "missing"
)

// unitPriceRe: order_items.unit_price is NUMERIC(14,4), never negative.
var unitPriceRe = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,4})?$`)

// normalizeUnitPrice validates a typed unit price (>= 0, at most 4
// fractional digits) and returns it with four decimals.
func normalizeUnitPrice(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	if !unitPriceRe.MatchString(v) {
		return "", false
	}
	r, err := parseRat(v)
	if err != nil || r.Sign() < 0 {
		return "", false
	}
	return r.FloatString(4), true
}

// errNotChildDealer: the buyer is not an active dealer (or a waiting pool
// recipient, TEC-1062) directly under the caller's center.
func errNotChildDealer() error {
	return invalid("buyer_org_uuid", "must be an active dealer directly under this center")
}

// pendingRecipientOf reports whether buyer is a recipient of the center's
// waiting pool (TEC-1062): an organization in status pending, kept in the
// pool or as a retail point, that the center opens orders for.
func pendingRecipientOf(ctx context.Context, q *db.Queries, centerID int64, buyer db.Organization) (bool, error) {
	if buyer.Status != "pending" {
		return false, nil
	}
	row, err := q.GetPendingRecipientByOrg(ctx, buyer.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("orders: pending recipient: %w", err)
	}
	return row.OrganizationID == centerID && row.State != "converted", nil
}

// createForBuyer opens an order of the center (the seller) for a dealer
// directly under it (TEC-1024). The order starts approved, exactly as the
// seller's approval leaves it (prices locked, rate frozen), so the warehouse
// flow (preparing, unit assignment, ready, shipped, received) runs
// unchanged. Prices never block: a gift line is 0, a typed unit price wins,
// otherwise the dealer's purchase price less the typed or default discount
// (TEC-1064), otherwise 0 marked missing. The warehouse staff
// get orders.warehouse_new unless the creator prepares it himself
// (SelfPrepare, TEC-1052).
func (s *Service) createForBuyer(ctx context.Context, c Caller, in CreateInput) (OrderView, error) {
	if in.SelfPrepare && !c.can(rbac.PermOrdersShip) {
		return OrderView{}, invalid("self_prepare", "needs orders.ship: only a user who prepares orders can prepare it himself")
	}
	buyerUUID, err := uuid.Parse(strings.TrimSpace(*in.BuyerOrgUUID))
	if err != nil {
		return OrderView{}, invalid("buyer_org_uuid", "must be a UUID")
	}
	terms, err := sellerTerms(in.Items)
	if err != nil {
		return OrderView{}, err
	}
	var created db.Order
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		seller, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
		if err != nil {
			return fmt.Errorf("orders: seller: %w", err)
		}
		if seller.Type != OrgCenter {
			return ErrForbidden
		}
		buyer, err := q.GetOrganizationByUUID(ctx, buyerUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotChildDealer()
		}
		if err != nil {
			return fmt.Errorf("orders: buyer: %w", err)
		}
		if buyer.Type != OrgDealer || buyer.BrandID != seller.BrandID {
			return errNotChildDealer()
		}
		if buyer.Status != "active" {
			ok, err := pendingRecipientOf(ctx, q, seller.ID, buyer)
			if err != nil {
				return err
			}
			if !ok {
				return errNotChildDealer()
			}
		}
		sup, err := s.supplier(ctx, q, buyer)
		if errors.Is(err, ErrNoSupplier) || (err == nil && sup.ID != seller.ID) {
			return errNotChildDealer()
		}
		if err != nil {
			return err
		}
		brand, err := q.GetBrandByID(ctx, seller.BrandID)
		if err != nil {
			return fmt.Errorf("orders: brand: %w", err)
		}
		// TEC-1061: the center may add products closed to dealer orders
		// (plexi set gifts, samples).
		lines, err := resolveLines(ctx, q, seller.BrandID, in.Items, lineRules{})
		if err != nil {
			return err
		}
		chosen, err := chosenWarehouse(ctx, q, seller.ID, in.SellerWarehouseUUID)
		if err != nil {
			return err
		}
		o, err := q.CreateOrder(ctx, db.CreateOrderParams{
			SellerOrgID: seller.ID, BrandID: seller.BrandID, BuyerOrgID: buyer.ID,
			Currency: strings.TrimSpace(brand.Currency), Note: textOrNull(in.Note), CreatedByUserID: c.actor(),
			CreatedBySeller: true,
		})
		if err != nil {
			return fmt.Errorf("orders: create: %w", err)
		}
		if err := s.insertSellerLines(ctx, q, o, buyer, lines, terms); err != nil {
			return err
		}
		if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
			return fmt.Errorf("orders: totals: %w", err)
		}
		o, meta, err := s.freezeApproval(ctx, q, c, o, buyer)
		if err != nil {
			return err
		}
		o, warehouse, err := assignWarehouse(ctx, q, o, chosen)
		if err != nil {
			return err
		}
		meta["created_by_seller"] = true
		if warehouse != nil {
			meta["seller_warehouse"] = warehouse.Code
		}
		if in.SelfPrepare {
			meta["self_prepare"] = true
		}
		if err := s.history(ctx, q, o, "", StatusApproved, c, nil, meta); err != nil {
			return err
		}
		created = o
		if err := s.emit(ctx, tx, events.OrdersCreated, o, "", c); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.OrdersApproved, o, "", c); err != nil {
			return err
		}
		if err := s.emitCreatedBySeller(ctx, q, tx, o, seller, buyer, c); err != nil {
			return err
		}
		// TEC-1052: the creator prepares it himself; no warehouse staff
		// (of this warehouse or another) is told.
		if in.SelfPrepare {
			return nil
		}
		return s.emitWarehouseNew(ctx, q, tx, o, warehouse, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, created)
}

// lineTerms is what the center typed for one line (TEC-1024, TEC-1064).
type lineTerms struct {
	// manual: a typed unit price, charged as is.
	manual *string
	// discount: a typed discount replacing the dealer's default one.
	discount *big.Rat
	gift     bool
}

// sellerTerms validates the typed unit prices, discounts and gift flags of
// a center order for a dealer.
func sellerTerms(items []ItemInput) ([]lineTerms, error) {
	out := make([]lineTerms, len(items))
	for i, it := range items {
		out[i].gift = it.IsGift
		if it.UnitPrice != nil && strings.TrimSpace(*it.UnitPrice) != "" {
			v, ok := normalizeUnitPrice(*it.UnitPrice)
			if !ok {
				return nil, invalid(fmt.Sprintf("items[%d].unit_price", i),
					"must be a non-negative decimal with at most 4 fractional digits")
			}
			out[i].manual = &v
		}
		if it.DiscountPct != nil && strings.TrimSpace(*it.DiscountPct) != "" {
			v, ok := pricingusecase.NormalizeDiscount(*it.DiscountPct)
			if !ok {
				return nil, invalid(fmt.Sprintf("items[%d].discount_pct", i),
					"must be a percentage between 0 and 100 with at most 2 fractional digits")
			}
			out[i].discount, _ = parseRat(v)
		}
	}
	return out, nil
}

// insertSellerLines writes the lines of a center-opened order: a gift line
// is 0, a typed price is charged as is, otherwise the dealer's purchase
// price less the typed or default discount (TEC-1064), otherwise 0
// (missing).
func (s *Service) insertSellerLines(ctx context.Context, q *db.Queries, o db.Order, buyer db.Organization,
	lines []line, terms []lineTerms) error {
	d, err := buyerDiscount(ctx, q, buyer, o.SellerOrgID, o.ID)
	if err != nil {
		return err
	}
	ids := make([]int64, len(lines))
	for i, l := range lines {
		ids[i] = l.product.ID
	}
	prices, err := pricingusecase.New(q).BuyerPurchasePrices(ctx, buyer.ID, buyer.Type, buyer.BrandID, ids, o.Currency)
	if err != nil {
		return fmt.Errorf("orders: prices: %w", err)
	}
	for i, l := range lines {
		in := chargeInput{amount: l.amount, discount: d.Pct, source: PriceSourceMissing}
		if p, ok := prices[l.product.ID]; ok {
			list := p.Price
			in.list, in.source = &list, priceSource(p.Source)
		}
		if i < len(terms) {
			t := terms[i]
			in.manual, in.gift = t.manual, t.gift
			if t.discount != nil {
				in.discount = t.discount
			}
		}
		c, err := charge(in)
		if err != nil {
			return err
		}
		if err := createLine(ctx, q, o, l, c); err != nil {
			return err
		}
	}
	return nil
}

// emitCreatedBySeller writes orders.created_by_seller with the dealer's
// members holding orders.read as recipients (notifications bus).
func (s *Service) emitCreatedBySeller(ctx context.Context, q *db.Queries, tx pgx.Tx, o db.Order,
	seller, buyer db.Organization, c Caller) error {
	ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{
		OrganizationID: buyer.ID, PermissionSlug: rbac.PermOrdersRead,
	})
	if err != nil {
		return fmt.Errorf("orders: recipients: %w", err)
	}
	notify := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != c.Principal.UserInternal {
			notify = append(notify, id)
		}
	}
	id, uid := o.ID, o.Uuid
	ev := events.New(events.OrdersCreatedBySeller).WithTenant(o.OrganizationID).WithEntity("order", &id, &uid).
		WithPayload(map[string]any{
			"order_uuid": o.Uuid.String(), "order_no": o.OrderNo, "status": o.Status,
			"seller_org_id": o.SellerOrgID, "buyer_org_id": o.BuyerOrgID, "brand_id": o.BrandID,
			"seller_name": seller.Name, "buyer_name": buyer.Name,
			"currency": o.Currency, "total": numericText(o.Total, 2), "notify_user_ids": notify,
		})
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("orders: outbox: %w", err)
	}
	return nil
}
