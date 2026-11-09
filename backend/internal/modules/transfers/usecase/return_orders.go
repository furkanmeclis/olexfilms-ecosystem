package usecase

// TEC-1085: returns opened by the parent for a child and returns linked to
// an order.
//
// The parent (the center or a distributor, with transfers.approve) picks one
// of its direct dealer / distributor children and opens the return in the
// child's name: the units still leave the child's stock and go to the
// parent, the accounting is the same reversal (PostStockReturnTx). Either
// side may name the order the parent sold the units on; the order's units
// are offered for picking and the order's line prices are frozen first.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MaxReturnOrders bounds the order picker of a return.
const MaxReturnOrders = 50

// returnChild resolves a direct dealer / distributor child of parent in
// its brand, else ErrNotChild.
func (s *Service) returnChild(ctx context.Context, q *db.Queries, parent db.Organization, id uuid.UUID) (db.Organization, error) {
	child, err := q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrNotChild
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("transfers: child: %w", err)
	}
	if child.BrandID != parent.BrandID || !child.ParentID.Valid || child.ParentID.Int64 != parent.ID ||
		(child.Type != OrgDealer && child.Type != OrgDistributor) || child.DeletedAt.Valid {
		return db.Organization{}, ErrNotChild
	}
	return child, nil
}

// returnOrder loads the order a return of buyer to seller refers to: sold
// by seller to buyer, not a draft or cancelled.
func returnOrder(ctx context.Context, q *db.Queries, brandID int64, id uuid.UUID, seller, buyer int64) (db.Order, error) {
	mismatch := &ValidationError{Field: "order_uuid", Code: CodeOrderMismatch,
		Message: "the order was not sold by the parent to the returning organization"}
	o, err := q.GetOrderByUUIDInBrand(ctx, db.GetOrderByUUIDInBrandParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Order{}, mismatch
	}
	if err != nil {
		return db.Order{}, fmt.Errorf("transfers: order: %w", err)
	}
	if o.SellerOrgID != seller || o.BuyerOrgID != buyer {
		return db.Order{}, mismatch
	}
	switch o.Status {
	case "draft", "cancelling", "cancelled":
		return db.Order{}, mismatch
	}
	return o, nil
}

// ReturnSources lists the organizations the active organization may open a
// return for: its direct dealer / distributor children (transfers.approve).
func (s *Service) ReturnSources(ctx context.Context, c Caller) ([]OrgRef, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermTransfersApprove) {
		return nil, ErrForbidden
	}
	rows, err := s.q.ListReturnSourceOrgs(ctx, db.ListReturnSourceOrgsParams{BrandID: c.Org.BrandID, ParentID: c.Org.InternalID})
	if err != nil {
		return nil, fmt.Errorf("transfers: return sources: %w", err)
	}
	out := make([]OrgRef, 0, len(rows))
	for _, o := range rows {
		out = append(out, OrgRef{UUID: o.Uuid, Name: o.Name, Type: o.Type})
	}
	return out, nil
}

// ReturnOrderView is an order a return may refer to.
type ReturnOrderView struct {
	UUID      uuid.UUID `json:"uuid"`
	OrderNo   string    `json:"order_no"`
	Status    string    `json:"status"`
	Currency  string    `json:"currency"`
	UnitCount int64     `json:"unit_count"`
	CreatedAt time.Time `json:"created_at"`
}

// returnParties resolves the giver and the receiver of a return the caller
// prepares: FromOrgUUID empty (or the active organization) is the active
// organization's own return to its parent (transfers.request); a child's
// uuid is a return the parent opens (transfers.approve).
func (s *Service) returnParties(ctx context.Context, c Caller, fromOrgUUID string) (db.Organization, db.Organization, error) {
	if c.Org.InternalID == 0 {
		return db.Organization{}, db.Organization{}, ErrForbidden
	}
	active, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return db.Organization{}, db.Organization{}, fmt.Errorf("transfers: organization: %w", err)
	}
	raw := strings.TrimSpace(fromOrgUUID)
	if raw == "" || raw == c.Org.UUID.String() {
		if !c.can(rbac.PermTransfersRequest) {
			return db.Organization{}, db.Organization{}, ErrForbidden
		}
		parent, ok, err := returnParent(ctx, s.q, active)
		if err != nil {
			return db.Organization{}, db.Organization{}, err
		}
		if !ok {
			return db.Organization{}, db.Organization{}, ErrNotParent
		}
		return active, parent, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return db.Organization{}, db.Organization{}, invalid("from_org_uuid", "must be a UUID")
	}
	if !c.can(rbac.PermTransfersApprove) {
		return db.Organization{}, db.Organization{}, ErrForbidden
	}
	child, err := s.returnChild(ctx, s.q, active, id)
	if err != nil {
		return db.Organization{}, db.Organization{}, err
	}
	return child, active, nil
}

// ReturnOrders lists the orders a return of the giver may refer to: what
// its parent sold to it, newest first (q matches the order number).
func (s *Service) ReturnOrders(ctx context.Context, c Caller, fromOrgUUID, q string) ([]ReturnOrderView, error) {
	giver, receiver, err := s.returnParties(ctx, c, fromOrgUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListReturnOrders(ctx, db.ListReturnOrdersParams{
		BrandID: giver.BrandID, SellerOrgID: receiver.ID, BuyerOrgID: giver.ID,
		Q: textOrNull(&q), RowLimit: MaxReturnOrders,
	})
	if err != nil {
		return nil, fmt.Errorf("transfers: return orders: %w", err)
	}
	out := make([]ReturnOrderView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReturnOrderView{
			UUID: r.Uuid, OrderNo: r.OrderNo, Status: r.Status, Currency: strings.TrimSpace(r.Currency),
			UnitCount: r.UnitCount, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ReturnUnitView is a unit of an order offered for a return. Returnable is
// what the buyer may still return of it: 1 for a serial unit it holds in
// stock, for a fixed barcode the smaller of the order quantity and what it
// holds, else 0.
type ReturnUnitView struct {
	UnitUUID   uuid.UUID  `json:"unit_uuid"`
	Barcode    string     `json:"barcode"`
	UnitKind   string     `json:"unit_kind"`
	Product    ProductRef `json:"product"`
	Quantity   *int32     `json:"quantity"`
	Meters     *string    `json:"meters"`
	Returnable int64      `json:"returnable"`
}

// ReturnOrderUnits lists the units of an order a return of the giver
// refers to (fromOrgUUID as in ReturnOrders).
func (s *Service) ReturnOrderUnits(ctx context.Context, c Caller, fromOrgUUID string, orderID uuid.UUID) ([]ReturnUnitView, error) {
	giver, receiver, err := s.returnParties(ctx, c, fromOrgUUID)
	if err != nil {
		return nil, err
	}
	o, err := returnOrder(ctx, s.q, giver.BrandID, orderID, receiver.ID, giver.ID)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.q.ListOrderReturnUnits(ctx, db.ListOrderReturnUnitsParams{BuyerOrgID: giver.ID, OrderID: o.ID})
	if err != nil {
		return nil, fmt.Errorf("transfers: order units: %w", err)
	}
	out := make([]ReturnUnitView, 0, len(rows))
	for _, r := range rows {
		v := ReturnUnitView{
			UnitUUID: r.UnitUuid, Barcode: r.Barcode, UnitKind: r.UnitKind,
			Product: ProductRef{UUID: r.ProductUuid, SKU: r.ProductSku, Name: r.ProductName, UnitType: r.ProductUnitType},
			Meters:  numericTextPtr(r.Meters, 2),
		}
		if r.Quantity.Valid {
			qv := r.Quantity.Int32
			v.Quantity = &qv
		}
		if r.UnitKind == ledger.KindFixed {
			v.Returnable = max(min(int64(r.Quantity.Int32), r.HeldQuantity), 0)
		} else if r.Held {
			v.Returnable = 1
		}
		out = append(out, v)
	}
	return out, nil
}
