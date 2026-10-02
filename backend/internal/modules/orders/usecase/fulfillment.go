package usecase

// Barcode assignment, readiness and shipping (TEC-167, F1-04c).
//
// The seller assigns units it holds to the lines of a preparing order (by
// scanning the barcode or picking the unit). Every assignment opens an
// active stock_reservations row and an order_item_units row in one
// transaction:
//
//   - serial pieces: one unit = quantity 1; a second active reservation of
//     the unit is refused (checked here and by uq_stock_reservations_active_serial).
//   - rolls (roll_meter lines): the whole roll (its remaining meters), or
//     fewer meters (TEC-184): the ledger first splits them off the roll as a
//     new unit with its own barcode (ledger.Split, idempotency key
//     order_item:<line uuid>:<client key or random>) and the new unit is
//     assigned; the roll stays with the seller. More meters than the roll
//     has is refused (ROLL_METERS_EXCEED_REMAINING).
//   - fixed barcodes: a quantity; the seller's active reservations of the
//     barcode never exceed what the seller holds. The check runs under the
//     unit row lock (the lock ledger.Post takes as well).
//
// preparing -> ready needs every line fully assigned. ready -> shipped posts
// one order_out per assigned unit (idempotency key
// order:order_item_unit:<id>:order_out:<barcode>), consumes the
// reservations and writes orders.shipped, all in one transaction.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Fulfillment errors; the handler maps them to 404/409.
var (
	// ErrNotAssignable: units are assigned only while the order is preparing.
	ErrNotAssignable = errors.New("orders: units can be assigned only while preparing")
	// ErrItemNotFound: the line is not part of the order.
	ErrItemNotFound = errors.New("orders: order line not found")
	// ErrAssignmentNotFound: the unit is not assigned to the line.
	ErrAssignmentNotFound = errors.New("orders: unit is not assigned to this line")
	// ErrUnitAlreadyAssigned: the unit is already on this line.
	ErrUnitAlreadyAssigned = errors.New("orders: unit already assigned to this line")
	// ErrUnitReserved: a serial unit is reserved by another order line.
	ErrUnitReserved = errors.New("orders: unit is reserved by another order")
	// ErrUnitNotAvailable: the seller does not hold the unit in stock.
	ErrUnitNotAvailable = errors.New("orders: unit is not in the seller's stock")
	// ErrInsufficientStock: a fixed barcode quantity above what the seller
	// holds minus its active reservations.
	ErrInsufficientStock = errors.New("orders: not enough stock for this barcode")
	// ErrOverAssigned: the assignment exceeds the line amount.
	ErrOverAssigned = errors.New("orders: assignment exceeds the line amount")
	// ErrNotFullyAssigned: preparing -> ready with an incompletely assigned line.
	ErrNotFullyAssigned = errors.New("orders: every line must be fully assigned")
	// ErrRollMetersExceed: more meters than the roll has left (TEC-184).
	ErrRollMetersExceed = errors.New("orders: the roll has fewer meters left")
	// ErrStockUnavailable: the ledger refused an order_out at shipping.
	ErrStockUnavailable = errors.New("orders: stock movement refused")
)

// Ledger reference of an order shipment: one movement per assigned unit.
const (
	LedgerSource  = "order"
	LedgerRefType = "order_item_unit"
)

// Validation codes of an assignment.
const (
	CodeUnitNotFound = "UNIT_NOT_FOUND"
)

// AssignInput names the unit (barcode or unit UUID) and the amount: a
// quantity for fixed barcodes (pieces are always 1), meters for rolls
// (optional: the whole roll; fewer meters split the roll). IdempotencyKey
// optionally makes a split assignment retry-safe.
type AssignInput struct {
	Barcode        string
	UnitUUID       string
	Quantity       *int64
	Meters         *string
	IdempotencyKey string
}

// MaxAssignKeyLen bounds AssignInput.IdempotencyKey.
const MaxAssignKeyLen = 64

// SplitRefType is the ledger reference of a split made for an order line.
const SplitRefType = "order_item"

func ratEq(a, b *big.Rat) bool { return a.Cmp(b) == 0 }

func lineAmount(it db.OrderItem) *big.Rat {
	if it.Meters.Valid {
		return numericRat(it.Meters)
	}
	return new(big.Rat).SetInt64(int64(it.Quantity.Int32))
}

func reservationAmount(r db.StockReservation) *big.Rat {
	if r.Meters.Valid {
		return numericRat(r.Meters)
	}
	return new(big.Rat).SetInt64(int64(r.Quantity.Int32))
}

// reservedOnLine sums the active reservations of a line.
func reservedOnLine(ctx context.Context, q *db.Queries, itemID int64) (*big.Rat, error) {
	rows, err := q.ListActiveReservationsByItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("orders: line reservations: %w", err)
	}
	sum := new(big.Rat)
	for _, r := range rows {
		sum.Add(sum, reservationAmount(r))
	}
	return sum, nil
}

// lockAssignable locks the order and its line for an assignment change by
// the seller.
func (s *Service) lockAssignable(ctx context.Context, q *db.Queries, c Caller, orderUUID, itemUUID uuid.UUID) (db.Order, db.OrderItem, error) {
	o, err := s.lockVisible(ctx, q, c, orderUUID)
	if err != nil {
		return o, db.OrderItem{}, err
	}
	if partyOf(c, o) != PartySeller || !c.can(rbac.PermOrdersShip) {
		return o, db.OrderItem{}, ErrForbidden
	}
	if o.Status != StatusPreparing {
		return o, db.OrderItem{}, ErrNotAssignable
	}
	it, err := q.GetOrderItemByUUID(ctx, db.GetOrderItemByUUIDParams{Uuid: itemUUID, BrandID: o.BrandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && it.OrderID != o.ID) {
		return o, db.OrderItem{}, ErrItemNotFound
	}
	if err != nil {
		return o, db.OrderItem{}, fmt.Errorf("orders: line: %w", err)
	}
	if it, err = q.LockOrderItem(ctx, db.LockOrderItemParams{ID: it.ID, BrandID: o.BrandID}); err != nil {
		return o, db.OrderItem{}, fmt.Errorf("orders: lock line: %w", err)
	}
	return o, it, nil
}

// findUnit resolves the unit by barcode or UUID inside the order's brand.
func findUnit(ctx context.Context, q *db.Queries, brandID int64, in AssignInput) (db.Unit, error) {
	barcode, rawUUID := strings.TrimSpace(in.Barcode), strings.TrimSpace(in.UnitUUID)
	notFound := &ValidationError{Field: "barcode", Code: CodeUnitNotFound, Message: "unit not found"}
	var (
		u   db.Unit
		err error
	)
	switch {
	case barcode != "" && rawUUID != "":
		return db.Unit{}, invalid("barcode", "give either barcode or unit_uuid")
	case barcode != "":
		u, err = q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: brandID, Barcode: barcode})
	case rawUUID != "":
		id, perr := uuid.Parse(rawUUID)
		if perr != nil {
			return db.Unit{}, invalid("unit_uuid", "must be a UUID")
		}
		notFound.Field = "unit_uuid"
		u, err = q.GetUnitByUUID(ctx, id)
		if err == nil && u.BrandID != brandID {
			return db.Unit{}, notFound
		}
	default:
		return db.Unit{}, invalid("barcode", "barcode or unit_uuid is required")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Unit{}, notFound
	}
	if err != nil {
		return db.Unit{}, fmt.Errorf("orders: unit: %w", err)
	}
	return u, nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// AssignUnit assigns a unit the seller holds to an order line and reserves it.
func (s *Service) AssignUnit(ctx context.Context, c Caller, orderUUID, itemUUID uuid.UUID, in AssignInput) (OrderView, error) {
	var result db.Order
	var split *SplitView
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if len(in.IdempotencyKey) > MaxAssignKeyLen {
		return OrderView{}, invalid("idempotency_key", fmt.Sprintf("at most %d characters", MaxAssignKeyLen))
	}
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, it, err := s.lockAssignable(ctx, q, c, orderUUID, itemUUID)
		if err != nil {
			return err
		}
		u, err := findUnit(ctx, q, o.BrandID, in)
		if err != nil {
			return err
		}
		// The unit row lock serialises reservations and ledger writes of the
		// unit across orders.
		if u, err = q.LockUnit(ctx, u.ID); err != nil {
			return fmt.Errorf("orders: lock unit: %w", err)
		}
		if u.ProductID != it.ProductID {
			return invalid("barcode", "the unit is not of the line's product")
		}
		if _, err := q.GetOrderItemUnit(ctx, db.GetOrderItemUnitParams{OrderItemID: it.ID, UnitID: u.ID}); err == nil {
			return ErrUnitAlreadyAssigned
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("orders: assignment: %w", err)
		}

		splitKey, err := assignSplitKey(it, in.IdempotencyKey)
		if err != nil {
			return err
		}
		// A retried split assignment: the split already happened.
		if in.IdempotencyKey != "" && u.RemainingMeters.Valid {
			prev, err := ledger.FindSplit(ctx, q, o.SellerOrgID, splitKey)
			if err != nil {
				return err
			}
			if prev != nil {
				if prev.Split.SourceUnitID != u.ID {
					return invalid("idempotency_key", "the key was used for another roll")
				}
				split = splitView(*prev)
				if _, err := q.GetOrderItemUnit(ctx, db.GetOrderItemUnitParams{OrderItemID: it.ID, UnitID: prev.New.ID}); err == nil {
					result = o
					return nil
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("orders: assignment: %w", err)
				}
				if u, err = q.LockUnit(ctx, prev.New.ID); err != nil {
					return fmt.Errorf("orders: lock unit: %w", err)
				}
				in.Meters = nil
			}
		}

		var (
			qty    pgtype.Int4
			meters pgtype.Numeric
			amount *big.Rat
		)
		if u.UnitKind == ledger.KindFixed {
			if in.Meters != nil {
				return invalid("meters", "a fixed barcode is assigned by quantity")
			}
			if in.Quantity == nil || *in.Quantity <= 0 || *in.Quantity > MaxQuantity {
				return invalid("quantity", fmt.Sprintf("must be between 1 and %d", MaxQuantity))
			}
			if err := s.checkFixedStock(ctx, q, u, o.SellerOrgID, *in.Quantity); err != nil {
				return err
			}
			qty = pgtype.Int4{Int32: int32(*in.Quantity), Valid: true}
			amount = new(big.Rat).SetInt64(*in.Quantity)
		} else {
			if err := checkSerialStock(ctx, q, u, o.SellerOrgID); err != nil {
				return err
			}
			if u.RemainingMeters.Valid {
				if in.Quantity != nil {
					return invalid("quantity", "a roll is assigned in meters")
				}
				rest := numericRat(u.RemainingMeters)
				if rest == nil || rest.Sign() <= 0 {
					return ErrUnitNotAvailable
				}
				want := rest
				if in.Meters != nil {
					m, ok := normalizeMeters(*in.Meters)
					if !ok {
						return invalid("meters", "meters must be a positive number with at most two decimals")
					}
					want, _ = parseRat(m)
					if want.Cmp(rest) > 0 {
						return fmt.Errorf("%w: %s m left", ErrRollMetersExceed, rest.FloatString(2))
					}
				}
				if want.Cmp(rest) < 0 {
					// Check the line first: an over-assignment never splits.
					done, err := reservedOnLine(ctx, q, it.ID)
					if err != nil {
						return err
					}
					if new(big.Rat).Add(done, want).Cmp(lineAmount(it)) > 0 {
						return ErrOverAssigned
					}
					cm, err := ledger.ParseMeters(want.FloatString(2))
					if err != nil {
						return invalid("meters", "invalid meters")
					}
					res, err := s.ledger.Split(ctx, tx, ledger.SplitCommand{
						SourceUnitID: u.ID, Centimeters: cm, HolderOrgID: o.SellerOrgID,
						IdempotencyKey: splitKey, RefType: SplitRefType, RefID: it.ID, ActorUserID: actorOf(c),
						Metadata: map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo},
					})
					if err != nil {
						return splitErr(u.Barcode, err)
					}
					split = splitView(res)
					if u, err = q.LockUnit(ctx, res.New.ID); err != nil {
						return fmt.Errorf("orders: lock unit: %w", err)
					}
				}
				meters, amount = u.RemainingMeters, numericRat(u.RemainingMeters)
			} else {
				if in.Meters != nil || (in.Quantity != nil && *in.Quantity != 1) {
					return invalid("quantity", "a serial piece is assigned as quantity 1")
				}
				qty, amount = pgtype.Int4{Int32: 1, Valid: true}, big.NewRat(1, 1)
			}
		}
		if it.Meters.Valid != meters.Valid {
			return invalid("barcode", "the unit does not use the line's measure")
		}
		done, err := reservedOnLine(ctx, q, it.ID)
		if err != nil {
			return err
		}
		if new(big.Rat).Add(done, amount).Cmp(lineAmount(it)) > 0 {
			return ErrOverAssigned
		}
		res, err := q.CreateStockReservation(ctx, db.CreateStockReservationParams{
			Quantity: qty, Meters: meters, CreatedByUserID: c.actor(), UnitID: u.ID, OrderItemID: it.ID,
		})
		if isUniqueViolation(err) {
			return ErrUnitReserved
		}
		if err != nil {
			return fmt.Errorf("orders: reserve: %w", err)
		}
		if _, err := q.CreateOrderItemUnit(ctx, db.CreateOrderItemUnitParams{
			UnitID: u.ID, Quantity: qty, Meters: meters, OrderItemID: it.ID,
			ReservationID: pgtype.Int8{Int64: res.ID, Valid: true}, AssignedByUserID: c.actor(),
		}); err != nil {
			if isUniqueViolation(err) {
				return ErrUnitAlreadyAssigned
			}
			return fmt.Errorf("orders: assign: %w", err)
		}
		result = o
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	v, err := s.view(ctx, s.q, c, result)
	if err != nil {
		return OrderView{}, err
	}
	v.Split = split
	return v, nil
}

// assignSplitKey is the ledger split key of a split assignment: the
// client's key (a retry finds the split) or a random one.
func assignSplitKey(it db.OrderItem, clientKey string) (string, error) {
	k := clientKey
	if k == "" {
		k = uuid.NewString()
	}
	if strings.ContainsAny(k, " \t\r\n") {
		return "", invalid("idempotency_key", "must not contain whitespace")
	}
	return SplitRefType + ":" + it.Uuid.String() + ":" + k, nil
}

func splitView(r ledger.SplitResult) *SplitView {
	return &SplitView{
		UUID: r.Split.Uuid, Meters: ratString(numericRat(r.Split.Meters)),
		SourceUnitUUID: r.Source.Uuid, SourceBarcode: r.Source.Barcode,
		SourceRemainingMeters: ratString(numericRat(r.Source.RemainingMeters)),
		NewUnitUUID:           r.New.Uuid, NewBarcode: r.New.Barcode, Replayed: r.Replayed,
	}
}

func ratString(r *big.Rat) string {
	if r == nil {
		return ""
	}
	return r.FloatString(2)
}

// splitErr maps a split refusal of the order flow.
func splitErr(barcode string, err error) error {
	switch {
	case errors.Is(err, ledger.ErrInsufficientMeters):
		return fmt.Errorf("%w: %v", ErrRollMetersExceed, err)
	case errors.Is(err, ledger.ErrSplitUnitElsewhere), errors.Is(err, ledger.ErrSplitNotRoll):
		return ErrUnitNotAvailable
	case errors.Is(err, ledger.ErrIdempotencyConflict):
		return invalid("idempotency_key", "the key was used for another roll")
	}
	return fmt.Errorf("orders: split %s: %w", barcode, err)
}

// checkSerialStock: the seller holds the serial unit in stock and no other
// line reserves it.
func checkSerialStock(ctx context.Context, q *db.Queries, u db.Unit, seller int64) error {
	st, err := q.GetUnitCurrentState(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnitNotAvailable
	}
	if err != nil {
		return fmt.Errorf("orders: unit state: %w", err)
	}
	if st.HolderOrgID != seller ||
		(st.Status != string(ledger.StatusAvailable) && st.Status != string(ledger.StatusPlaced)) {
		return ErrUnitNotAvailable
	}
	active, err := q.LockActiveReservationsByUnit(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("orders: unit reservations: %w", err)
	}
	if len(active) > 0 {
		return ErrUnitReserved
	}
	return nil
}

// checkFixedStock: the seller's active reservations of the barcode plus qty
// do not exceed what the seller holds (all its owners). The caller holds
// the unit row lock.
func (s *Service) checkFixedStock(ctx context.Context, q *db.Queries, u db.Unit, seller, qty int64) error {
	holdings, err := q.LockFixedBarcodeHoldingsByUnit(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("orders: holdings: %w", err)
	}
	var onHand int64
	for _, h := range holdings {
		if h.HolderOrgID == seller {
			onHand += int64(h.QuantityOnHand)
		}
	}
	reserved, err := q.SumActiveReservedQuantityByUnit(ctx, db.SumActiveReservedQuantityByUnitParams{
		UnitID: u.ID, OrganizationID: seller,
	})
	if err != nil {
		return fmt.Errorf("orders: reserved quantity: %w", err)
	}
	if reserved+qty > onHand {
		return ErrInsufficientStock
	}
	return nil
}

// UnassignUnit removes a unit from a line; its reservation is released.
func (s *Service) UnassignUnit(ctx context.Context, c Caller, orderUUID, itemUUID, unitUUID uuid.UUID) (OrderView, error) {
	var result db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, it, err := s.lockAssignable(ctx, q, c, orderUUID, itemUUID)
		if err != nil {
			return err
		}
		u, err := q.GetUnitByUUID(ctx, unitUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && u.BrandID != o.BrandID) {
			return ErrAssignmentNotFound
		}
		if err != nil {
			return fmt.Errorf("orders: unit: %w", err)
		}
		a, err := q.GetOrderItemUnit(ctx, db.GetOrderItemUnitParams{OrderItemID: it.ID, UnitID: u.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAssignmentNotFound
		}
		if err != nil {
			return fmt.Errorf("orders: assignment: %w", err)
		}
		if a.ReservationID.Valid {
			if _, err := q.ReleaseStockReservation(ctx, a.ReservationID.Int64); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("orders: release: %w", err)
			}
		}
		// The released reservation stays as history; the assignment row goes
		// so the unit can be assigned to the line again.
		if _, err := q.DeleteOrderItemUnit(ctx, a.ID); err != nil {
			return fmt.Errorf("orders: unassign: %w", err)
		}
		result = o
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

// checkFullyAssigned: every line's active reservations add up to its amount.
func checkFullyAssigned(ctx context.Context, q *db.Queries, o db.Order) error {
	items, err := q.LockOrderItems(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("orders: lines: %w", err)
	}
	if len(items) == 0 {
		return ErrNotFullyAssigned
	}
	for _, it := range items {
		done, err := reservedOnLine(ctx, q, it.ID)
		if err != nil {
			return err
		}
		if !ratEq(done, lineAmount(it)) {
			return ErrNotFullyAssigned
		}
	}
	return nil
}

// ship posts an order_out for every assigned unit, consumes the
// reservations and records the movement on the assignment. Serial units go
// in transit owned by the buyer; fixed barcodes are debited from the
// seller's owner holding the quantity.
func (s *Service) ship(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, o db.Order) (map[string]any, error) {
	units, err := q.ListOrderItemUnitsByOrder(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("orders: assigned units: %w", err)
	}
	if len(units) == 0 {
		return nil, ErrNotFullyAssigned
	}
	actor := actorOf(c)
	buyer := ledger.Owner{Type: ledger.OwnerOrganization, ID: o.BuyerOrgID, OrgID: o.BuyerOrgID}
	movements := 0
	for _, a := range units {
		if !a.ReservationID.Valid {
			return nil, ErrNotFullyAssigned
		}
		res, err := q.LockStockReservation(ctx, a.ReservationID.Int64)
		if err != nil {
			return nil, fmt.Errorf("orders: reservation: %w", err)
		}
		m := ledger.Movement{
			Type: ledger.TypeOrderOut, UnitID: a.UnitID,
			Source: LedgerSource, RefType: LedgerRefType, RefID: a.ID, ActorUserID: actor,
			Metadata: map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo},
		}
		if a.UnitKind == ledger.KindFixed {
			from, err := fixedSource(ctx, q, a.UnitID, o.SellerOrgID, a.Quantity.Int32)
			if err != nil {
				return nil, err
			}
			m.From, m.Quantity = &from, a.Quantity.Int32
		} else {
			st, err := q.GetUnitCurrentState(ctx, a.UnitID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("orders: unit state: %w", err)
			}
			if err != nil || st.HolderOrgID != o.SellerOrgID {
				return nil, fmt.Errorf("%w: unit %s is not held by the seller", ErrStockUnavailable, a.Barcode)
			}
			m.To = &buyer
		}
		r, err := s.ledger.Post(ctx, tx, m)
		if err != nil {
			return nil, stockErr("order_out", a.Barcode, err)
		}
		if !r.Replayed {
			movements++
		}
		if !a.MovementID.Valid {
			if _, err := q.SetOrderItemUnitMovement(ctx, db.SetOrderItemUnitMovementParams{
				ID: a.ID, MovementID: pgtype.Int8{Int64: r.Movement.ID, Valid: true},
			}); err != nil {
				return nil, fmt.Errorf("orders: assignment movement: %w", err)
			}
		}
		if res.Status == "active" {
			if _, err := q.ConsumeStockReservation(ctx, res.ID); err != nil {
				return nil, fmt.Errorf("orders: consume reservation: %w", err)
			}
		}
	}
	return map[string]any{"units": len(units), "movements": movements}, nil
}

// fixedSource picks the seller's owner (location or the organization) that
// holds the whole quantity, the largest first. One assignment is one
// movement (one idempotency key), so the quantity is not split across
// owners.
func fixedSource(ctx context.Context, q *db.Queries, unitID, seller int64, qty int32) (ledger.Owner, error) {
	holdings, err := q.ListFixedBarcodeHoldingsByUnit(ctx, unitID)
	if err != nil {
		return ledger.Owner{}, fmt.Errorf("orders: holdings: %w", err)
	}
	var best *db.FixedBarcodeHolding
	for i := range holdings {
		h := &holdings[i]
		if h.HolderOrgID != seller || h.QuantityOnHand < qty {
			continue
		}
		if best == nil || h.QuantityOnHand > best.QuantityOnHand {
			best = h
		}
	}
	if best == nil {
		return ledger.Owner{}, fmt.Errorf("%w: no single owner of the seller holds %d", ErrStockUnavailable, qty)
	}
	return ledger.Owner{Type: ledger.OwnerType(best.OwnerType), ID: best.OwnerID, OrgID: best.HolderOrgID}, nil
}
