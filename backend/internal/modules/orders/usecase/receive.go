package usecase

// Receipt and cancel after shipping (TEC-168, F1-04d).
//
// shipped -> received: the buyer takes the whole shipment (no partial
// receipt). Every shipped unit gets one ledger received movement (key
// order:order_item_unit:<id>:received:<barcode>) paired with its order_out;
// the unit becomes available at the buyer organization. The status
// history, orders.received and the accounting hook run in the same
// transaction.
//
// shipped -> cancelling -> cancelled: a cancel after shipping first waits
// for the goods to come back; when the seller takes them, every shipped
// unit gets one order_cancel_restore (key
// order:order_item_unit:<id>:order_cancel_restore:<barcode>), which puts it
// back to the order_out's source owner and status. The ledger pairs a
// restore with the *_out of the same reference, so an order shipment is
// restored with order_cancel_restore (transfer_cancel_restore belongs to
// transfer_out).

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/jackc/pgx/v5"
)

// ReceiptHook runs inside the received transaction after the stock has
// moved and the order is received. The accounting bridge (TEC-169) books
// the seller income/receivable and the buyer expense/payable here; an
// error rolls the receipt back. actor is the receiving user (nil: system).
type ReceiptHook interface {
	OrderReceived(ctx context.Context, tx pgx.Tx, q *db.Queries, o db.Order, actor *int64) error
}

// NoopReceiptHook books nothing (tests, accounting not wired).
type NoopReceiptHook struct{}

// OrderReceived does nothing.
func (NoopReceiptHook) OrderReceived(context.Context, pgx.Tx, *db.Queries, db.Order, *int64) error {
	return nil
}

func actorOf(c Caller) *int64 {
	if c.Principal.UserInternal == 0 {
		return nil
	}
	v := c.Principal.UserInternal
	return &v
}

// stockErr maps a ledger refusal to ErrStockUnavailable.
func stockErr(op, barcode string, err error) error {
	if errors.Is(err, ledger.ErrInsufficientStock) || errors.Is(err, ledger.ErrTransitionNotAllowed) ||
		errors.Is(err, ledger.ErrOwnerMismatch) || errors.Is(err, ledger.ErrOwnerNotAllowed) {
		return fmt.Errorf("%w: %s: %v", ErrStockUnavailable, barcode, err)
	}
	return fmt.Errorf("orders: %s %s: %w", op, barcode, err)
}

// shippedUnits lists the order's assignments; each must carry its order_out.
func shippedUnits(ctx context.Context, q *db.Queries, o db.Order) ([]db.ListOrderItemUnitsByOrderRow, error) {
	units, err := q.ListOrderItemUnitsByOrder(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("orders: shipped units: %w", err)
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("%w: the order has no shipped unit", ErrStockUnavailable)
	}
	for _, a := range units {
		if !a.MovementID.Valid {
			return nil, fmt.Errorf("%w: unit %s was not shipped", ErrStockUnavailable, a.Barcode)
		}
	}
	return units, nil
}

// receive posts a received movement for every shipped unit into the buyer
// organization.
func (s *Service) receive(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, o db.Order) (map[string]any, error) {
	units, err := shippedUnits(ctx, q, o)
	if err != nil {
		return nil, err
	}
	buyer := ledger.Owner{Type: ledger.OwnerOrganization, ID: o.BuyerOrgID, OrgID: o.BuyerOrgID}
	movements := 0
	for _, a := range units {
		m := ledger.Movement{
			Type: ledger.TypeReceived, UnitID: a.UnitID, To: &buyer,
			Source: LedgerSource, RefType: LedgerRefType, RefID: a.ID, ActorUserID: actorOf(c),
			Metadata: map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo},
		}
		if a.UnitKind == ledger.KindFixed {
			m.Quantity = a.Quantity.Int32
		}
		r, err := s.ledger.Post(ctx, tx, m)
		if err != nil {
			return nil, stockErr("received", a.Barcode, err)
		}
		if !r.Replayed {
			movements++
		}
	}
	return map[string]any{"units": len(units), "movements": movements}, nil
}

// restore posts an order_cancel_restore for every shipped unit: serial
// units go back to the order_out's source owner and status, fixed barcodes
// are credited back to the owner the order_out debited.
func (s *Service) restore(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, o db.Order) (map[string]any, error) {
	units, err := shippedUnits(ctx, q, o)
	if err != nil {
		return nil, err
	}
	movements := 0
	for _, a := range units {
		m := ledger.Movement{
			Type: ledger.TypeOrderCancelRestore, UnitID: a.UnitID,
			Source: LedgerSource, RefType: LedgerRefType, RefID: a.ID, ActorUserID: actorOf(c),
			Reason:   "order cancelled after shipping",
			Metadata: map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo},
		}
		if a.UnitKind == ledger.KindFixed {
			out, err := q.GetStockMovement(ctx, a.MovementID.Int64)
			if err != nil {
				return nil, fmt.Errorf("orders: order_out of %s: %w", a.Barcode, err)
			}
			if !out.FromOwnerType.Valid || !out.FromOwnerID.Valid {
				return nil, fmt.Errorf("%w: order_out of %s has no source", ErrStockUnavailable, a.Barcode)
			}
			src := ledger.Owner{Type: ledger.OwnerType(out.FromOwnerType.String), ID: out.FromOwnerID.Int64, OrgID: out.OrganizationID}
			m.To, m.Quantity = &src, a.Quantity.Int32
		}
		r, err := s.ledger.Post(ctx, tx, m)
		if err != nil {
			return nil, stockErr("order_cancel_restore", a.Barcode, err)
		}
		if !r.Replayed {
			movements++
		}
	}
	return map[string]any{"units": len(units), "movements": movements}, nil
}
