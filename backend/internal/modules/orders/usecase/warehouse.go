package usecase

// The seller warehouse of an order (TEC-1028).
//
// An approved order falls into one warehouse of the seller: the warehouse
// the center picked when it opened the order for a dealer, else the
// seller's active primary warehouse (TEC-1006). An organization without a
// primary warehouse leaves it empty and picking runs over the whole
// organization as before. Picking suggestions and pick locations are
// limited to the order's warehouse (legacy locations outside any warehouse
// stay in), and the warehouse staff (members of the
// warehouse holding orders.ship, or holders without any warehouse
// membership) get orders.warehouse_new.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WarehouseRef names the seller warehouse of an order.
type WarehouseRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	IsPrimary bool      `json:"is_primary"`
}

func warehouseRef(w db.Warehouse) *WarehouseRef {
	return &WarehouseRef{UUID: w.Uuid, Code: w.Code, Name: w.Name, IsPrimary: w.IsPrimary}
}

// chosenWarehouse resolves a warehouse picked by the seller: an active
// warehouse of the seller organization. Empty input picks none.
func chosenWarehouse(ctx context.Context, q *db.Queries, sellerOrgID int64, raw *string) (*db.Warehouse, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return nil, invalid("seller_warehouse_uuid", "must be a UUID")
	}
	w, err := q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: id, OrganizationID: sellerOrgID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !w.Active) {
		return nil, invalid("seller_warehouse_uuid", "must be an active warehouse of the seller")
	}
	if err != nil {
		return nil, fmt.Errorf("orders: warehouse: %w", err)
	}
	return &w, nil
}

// assignWarehouse puts an approved order into a seller warehouse: chosen,
// else the one already set, else the seller's active primary warehouse.
// It returns the order and its warehouse (nil: none).
func assignWarehouse(ctx context.Context, q *db.Queries, o db.Order, chosen *db.Warehouse) (db.Order, *db.Warehouse, error) {
	if chosen == nil && o.SellerWarehouseID.Valid {
		w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: o.SellerWarehouseID.Int64, OrganizationID: o.SellerOrgID})
		if err != nil {
			return o, nil, fmt.Errorf("orders: warehouse: %w", err)
		}
		return o, &w, nil
	}
	w := chosen
	if w == nil {
		p, err := q.GetPrimaryWarehouse(ctx, o.SellerOrgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return o, nil, nil
		}
		if err != nil {
			return o, nil, fmt.Errorf("orders: primary warehouse: %w", err)
		}
		w = &p
	}
	o, err := q.SetOrderSellerWarehouse(ctx, db.SetOrderSellerWarehouseParams{
		ID: o.ID, SellerWarehouseID: pgtype.Int8{Int64: w.ID, Valid: true},
	})
	if err != nil {
		return o, nil, fmt.Errorf("orders: set warehouse: %w", err)
	}
	return o, w, nil
}

// warehouseFilter is the picking warehouse of an order (null: the whole
// seller organization).
func warehouseFilter(o db.Order) pgtype.Int8 {
	return o.SellerWarehouseID
}

// emitWarehouseNew writes orders.warehouse_new with the warehouse staff
// holding orders.ship (minus the actor) as recipients. Without a warehouse
// every holder of orders.ship in the seller organization is told.
func (s *Service) emitWarehouseNew(ctx context.Context, q *db.Queries, tx pgx.Tx, o db.Order, w *db.Warehouse, c Caller) error {
	ids, err := q.ListOrderWarehouseNotifyUserIDs(ctx, db.ListOrderWarehouseNotifyUserIDsParams{
		OrganizationID: o.SellerOrgID, PermissionSlug: rbac.PermOrdersShip, WarehouseID: o.SellerWarehouseID,
	})
	if err != nil {
		return fmt.Errorf("orders: warehouse recipients: %w", err)
	}
	notify := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != c.Principal.UserInternal {
			notify = append(notify, id)
		}
	}
	buyer, err := q.GetOrganizationByID(ctx, o.BuyerOrgID)
	if err != nil {
		return fmt.Errorf("orders: buyer: %w", err)
	}
	payload := map[string]any{
		"order_uuid": o.Uuid.String(), "order_no": o.OrderNo, "status": o.Status,
		"seller_org_id": o.SellerOrgID, "buyer_org_id": o.BuyerOrgID, "brand_id": o.BrandID,
		"buyer_name": buyer.Name, "notify_user_ids": notify,
	}
	if w != nil {
		payload["warehouse_uuid"], payload["warehouse_name"] = w.Uuid.String(), w.Name
	} else {
		seller, err := q.GetOrganizationByID(ctx, o.SellerOrgID)
		if err != nil {
			return fmt.Errorf("orders: seller: %w", err)
		}
		payload["warehouse_name"] = seller.Name
	}
	id, uid := o.ID, o.Uuid
	ev := events.New(events.OrdersWarehouseNew).WithTenant(o.OrganizationID).WithEntity("order", &id, &uid).
		WithPayload(payload)
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("orders: outbox: %w", err)
	}
	return nil
}
