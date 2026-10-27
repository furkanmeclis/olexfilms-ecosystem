package usecase

import (
	"context"
	"errors"
	"fmt"
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

// errNotChildDealer: the buyer is not an active dealer directly under the
// caller's center.
func errNotChildDealer() error {
	return invalid("buyer_org_uuid", "must be an active dealer directly under this center")
}

// createForBuyer opens an order of the center (the seller) for a dealer
// directly under it (TEC-1024). The order starts approved, exactly as the
// seller's approval leaves it (prices locked, rate frozen), so the warehouse
// flow (preparing, unit assignment, ready, shipped, received) runs
// unchanged. Prices never block: a typed unit price wins, otherwise the
// dealer's purchase price, otherwise 0 marked missing.
func (s *Service) createForBuyer(ctx context.Context, c Caller, in CreateInput) (OrderView, error) {
	buyerUUID, err := uuid.Parse(strings.TrimSpace(*in.BuyerOrgUUID))
	if err != nil {
		return OrderView{}, invalid("buyer_org_uuid", "must be a UUID")
	}
	manual := make([]*string, len(in.Items))
	for i, it := range in.Items {
		if it.UnitPrice == nil || strings.TrimSpace(*it.UnitPrice) == "" {
			continue
		}
		v, ok := normalizeUnitPrice(*it.UnitPrice)
		if !ok {
			return OrderView{}, invalid(fmt.Sprintf("items[%d].unit_price", i),
				"must be a non-negative decimal with at most 4 fractional digits")
		}
		manual[i] = &v
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
		if buyer.Type != OrgDealer || buyer.Status != "active" || buyer.BrandID != seller.BrandID {
			return errNotChildDealer()
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
		lines, err := resolveLines(ctx, q, seller.BrandID, in.Items)
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
		if err := s.insertSellerLines(ctx, q, o, buyer, lines, manual); err != nil {
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
		return s.emitWarehouseNew(ctx, q, tx, o, warehouse, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, created)
}

// insertSellerLines writes the lines of a center-opened order: the typed
// price, else the dealer's purchase price, else 0 (missing).
func (s *Service) insertSellerLines(ctx context.Context, q *db.Queries, o db.Order, buyer db.Organization,
	lines []line, manual []*string) error {
	ids := make([]int64, len(lines))
	for i, l := range lines {
		ids[i] = l.product.ID
	}
	prices, err := pricingusecase.New(q).BuyerPurchasePrices(ctx, buyer.ID, buyer.Type, buyer.BrandID, ids, o.Currency)
	if err != nil {
		return fmt.Errorf("orders: prices: %w", err)
	}
	for i, l := range lines {
		price, source := "0", PriceSourceMissing
		switch {
		case i < len(manual) && manual[i] != nil:
			price, source = *manual[i], PriceSourceManual
		default:
			if p, ok := prices[l.product.ID]; ok {
				price, source = p.Price, priceSource(p.Source)
			}
		}
		total, err := lineTotal(price, l.amount)
		if err != nil {
			return err
		}
		up, err := numeric(price)
		if err != nil {
			return err
		}
		lt, err := numeric(total)
		if err != nil {
			return err
		}
		if _, err := q.CreateOrderItem(ctx, db.CreateOrderItemParams{
			OrderID: o.ID, ProductID: l.product.ID, Quantity: l.qty, Meters: l.meters,
			UnitPrice: up, PriceSource: source, LineTotal: lt, Note: l.note,
		}); err != nil {
			return fmt.Errorf("orders: create line: %w", err)
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
