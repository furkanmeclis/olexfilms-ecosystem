package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrNotSupplementable: a supplementary order is opened only on a shipped,
// delivered or received order (TEC-1222).
var ErrNotSupplementable = errors.New("orders: only a shipped or delivered order gets a supplementary order")

// OrderLink is a linked order of a detail (TEC-1222): the main order of a
// supplement or a supplement of a main order.
type OrderLink struct {
	UUID        uuid.UUID `json:"uuid"`
	OrderNo     string    `json:"order_no"`
	Status      string    `json:"status"`
	StatusLabel string    `json:"status_label"`
}

// supplementable reports whether status allows a supplementary order: the
// goods of the main order are on the way or there.
func supplementable(status string) bool {
	switch status {
	case StatusShipped, StatusDelivered, StatusReceived:
		return true
	}
	return false
}

// canSupplement: the seller's sales (orders.write) or warehouse
// (orders.ship) staff open supplementary orders (TEC-1222).
func canSupplement(c Caller) bool {
	return c.can(rbac.PermOrdersWrite) || c.can(rbac.PermOrdersShip)
}

// CreateSupplement opens a supplementary order of a shipped or delivered
// order (TEC-1222, revize 5.9): plexi and sign sets sent after the goods.
// Seller, buyer, brand and seller warehouse come from the main order; the
// caller sends the lines (gift lines too). Like an order the seller opens
// for the dealer (TEC-1024) it starts approved and then runs its own flow
// (preparing, shipping, delivery, books, invoice). A supplement of a
// supplement hangs on the same main order. A caller who may not see prices
// (warehouse staff) cannot type prices or discounts: the lines take the
// dealer's purchase price.
func (s *Service) CreateSupplement(ctx context.Context, c Caller, parentUUID uuid.UUID, in CreateInput) (OrderView, error) {
	if c.Org.InternalID == 0 || !canSupplement(c) {
		return OrderView{}, ErrForbidden
	}
	if in.BuyerOrgUUID != nil && strings.TrimSpace(*in.BuyerOrgUUID) != "" {
		return OrderView{}, invalid("buyer_org_uuid", "is taken from the main order")
	}
	if in.SelfPrepare && !c.can(rbac.PermOrdersShip) {
		return OrderView{}, invalid("self_prepare", "needs orders.ship: only a user who prepares orders can prepare it himself")
	}
	if !c.SeesPrices() {
		for i := range in.Items {
			in.Items[i].UnitPrice, in.Items[i].DiscountPct = nil, nil
		}
	}
	terms, err := sellerTerms(in.Items)
	if err != nil {
		return OrderView{}, err
	}
	var created db.Order
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		parent, err := q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: parentUUID, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("orders: main order: %w", err)
		}
		if !visible(c, parent) {
			return ErrNotFound
		}
		// Only the seller sends more goods.
		if partyOf(c, parent) != PartySeller {
			return ErrForbidden
		}
		if !supplementable(parent.Status) {
			return ErrNotSupplementable
		}
		main := parent
		if parent.ParentOrderID.Valid {
			if main, err = q.GetOrder(ctx, db.GetOrderParams{ID: parent.ParentOrderID.Int64, BrandID: parent.BrandID}); err != nil {
				return fmt.Errorf("orders: main order: %w", err)
			}
		}
		seller, err := q.GetOrganizationByID(ctx, parent.SellerOrgID)
		if err != nil {
			return fmt.Errorf("orders: seller: %w", err)
		}
		buyer, err := q.GetOrganizationByID(ctx, parent.BuyerOrgID)
		if err != nil {
			return fmt.Errorf("orders: buyer: %w", err)
		}
		lines, err := resolveLines(ctx, q, parent.BrandID, in.Items, lineRules{})
		if err != nil {
			return err
		}
		chosen, err := chosenWarehouse(ctx, q, seller.ID, in.SellerWarehouseUUID)
		if err != nil {
			return err
		}
		if chosen == nil && parent.SellerWarehouseID.Valid {
			w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: parent.SellerWarehouseID.Int64, OrganizationID: seller.ID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("orders: warehouse: %w", err)
			}
			if err == nil && w.Active {
				chosen = &w
			}
		}
		o, err := q.CreateOrder(ctx, db.CreateOrderParams{
			SellerOrgID: seller.ID, BrandID: parent.BrandID, BuyerOrgID: buyer.ID,
			Currency: strings.TrimSpace(parent.Currency), Note: textOrNull(in.Note), CreatedByUserID: c.actor(),
			CreatedBySeller: true, ParentOrderID: pgtype.Int8{Int64: main.ID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("orders: create: %w", err)
		}
		if o, err = createSchedule(ctx, q, o, in); err != nil {
			return err
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
		meta["parent_order"] = main.OrderNo
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

func orderLink(ctx context.Context, o db.Order) OrderLink {
	return OrderLink{
		UUID: o.Uuid, OrderNo: o.OrderNo, Status: o.Status,
		StatusLabel: i18n.Translate(i18n.FromContext(ctx).Locale, StatusLabelKey(o.Status)),
	}
}

// withSupplements fills the main order of a supplement and the supplements
// of a main order (detail only, TEC-1222). Both share seller and buyer, so
// whoever sees one sees the other.
func (s *Service) withSupplements(ctx context.Context, q *db.Queries, c Caller, o db.Order, v *OrderView) error {
	v.Supplements = []OrderLink{}
	if o.ParentOrderID.Valid {
		p, err := q.GetOrder(ctx, db.GetOrderParams{ID: o.ParentOrderID.Int64, BrandID: o.BrandID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("orders: main order: %w", err)
		}
		if err == nil && visible(c, p) {
			link := orderLink(ctx, p)
			v.ParentOrder = &link
		}
		return nil
	}
	rows, err := q.ListOrderSupplements(ctx, db.ListOrderSupplementsParams{ParentOrderID: pgtype.Int8{Int64: o.ID, Valid: true}, BrandID: o.BrandID})
	if err != nil {
		return fmt.Errorf("orders: supplements: %w", err)
	}
	for _, r := range rows {
		if visible(c, r) {
			v.Supplements = append(v.Supplements, orderLink(ctx, r))
		}
	}
	v.CanAddSupplement = partyOf(c, o) == PartySeller && canSupplement(c) && supplementable(o.Status)
	return nil
}
