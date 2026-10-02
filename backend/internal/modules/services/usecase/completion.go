package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Ledger idempotency key parts of a service item consumption
// (service:service_item:<item id>:<type>:<barcode>, TEC-97 decision 6).
const (
	ledgerSource  = "service"
	ledgerRefType = "service_item"
)

// complete consumes the stock of every item and moves the locked service
// to completed, all in tx (TEC-180, TEC-97 decision 6):
//
//  1. one ledger.Post per item: consumption for a whole unit (serial piece,
//     whole roll, n pieces of a fixed barcode), partial_consumption for a
//     cut of meters (a cut equal to the rest of the roll is a consumption,
//     the ledger refuses a partial that empties the roll);
//  2. service_items.stock_movement_id is written before the status flips
//     (the trigger locks the items once the service is completed);
//  3. the status becomes completed, a status log is appended;
//  4. a service.completed outbox event carries the service, its
//     organization, brand, vehicle, customer and items (warranty listener,
//     TEC-186).
//
// Any ledger refusal (unit no longer held, not enough meters or pieces)
// rolls the whole transaction back and answers ErrUnitNotAvailable.
func (s *Service) complete(ctx context.Context, q *db.Queries, tx pgx.Tx, svc db.Service, c Caller,
	note *string) (db.Service, error) {
	items, err := q.LockServiceItems(ctx, svc.ID)
	if err != nil {
		return db.Service{}, fmt.Errorf("services: lock items: %w", err)
	}
	if len(items) == 0 {
		return db.Service{}, invalid("items", "a service needs at least one item to be completed")
	}
	l := ledger.New(s.q, s.out)
	payloadItems := make([]map[string]any, 0, len(items))
	itemIDs := make([]int64, 0, len(items))
	for _, item := range items {
		mv, err := s.consume(ctx, q, tx, l, svc, item, c)
		if err != nil {
			return db.Service{}, err
		}
		switch {
		case item.StockMovementID.Valid && item.StockMovementID.Int64 == mv.ID:
		case item.StockMovementID.Valid:
			return db.Service{}, fmt.Errorf("services: item %d already points to movement %d, not %d",
				item.ID, item.StockMovementID.Int64, mv.ID)
		default:
			if _, err := q.SetServiceItemMovement(ctx, db.SetServiceItemMovementParams{
				ID: item.ID, StockMovementID: pgtype.Int8{Int64: mv.ID, Valid: true},
			}); err != nil {
				return db.Service{}, fmt.Errorf("services: item movement: %w", err)
			}
		}
		itemIDs = append(itemIDs, item.ID)
		entry := map[string]any{
			"id": item.ID, "uuid": item.Uuid.String(), "unit_id": item.UnitID, "product_id": item.ProductID,
			"kind": item.Kind, "stock_movement_id": mv.ID, "movement_type": mv.Type,
		}
		if item.Quantity.Valid {
			entry["quantity"] = item.Quantity.Int32
		}
		if m := numericTextPtr(item.Meters); m != nil {
			entry["meters"] = *m
		}
		payloadItems = append(payloadItems, entry)
	}

	from := svc.Status
	done, err := q.CompleteService(ctx, db.CompleteServiceParams{ID: svc.ID, ActorUserID: c.actor()})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, ErrInvalidTransition
	}
	if err != nil {
		return db.Service{}, fmt.Errorf("services: complete: %w", err)
	}
	if err := s.log(ctx, q, done, from, StatusCompleted, c, note, map[string]any{"item_ids": itemIDs}); err != nil {
		return db.Service{}, err
	}
	extra := map[string]any{
		"service_id": done.ID,
		"item_ids":   itemIDs,
		"items":      payloadItems,
	}
	if done.CompletedAt.Valid {
		extra["completed_at"] = done.CompletedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	if err := s.emit(ctx, tx, events.ServiceCompleted, done, from, c, extra); err != nil {
		return db.Service{}, err
	}
	return done, nil
}

// consume posts the ledger movement of one item.
func (s *Service) consume(ctx context.Context, q *db.Queries, tx pgx.Tx, l *ledger.Ledger, svc db.Service,
	item db.ServiceItem, c Caller) (db.StockMovement, error) {
	// Lock the unit first so the remaining meters read here are the ones
	// the ledger sees (Post locks the same row again).
	unit, err := q.LockUnit(ctx, item.UnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.StockMovement{}, fmt.Errorf("%w: unit %d", ErrUnitNotAvailable, item.UnitID)
	}
	if err != nil {
		return db.StockMovement{}, fmt.Errorf("services: lock unit: %w", err)
	}
	m := ledger.Movement{
		UnitID: unit.ID, Source: ledgerSource, RefType: ledgerRefType, RefID: item.ID,
		Reason: "service " + svc.ServiceNo,
		Metadata: map[string]any{
			"service_uuid": svc.Uuid.String(), "service_no": svc.ServiceNo, "item_uuid": item.Uuid.String(),
		},
	}
	if c.Principal.UserInternal != 0 {
		actor := c.Principal.UserInternal
		m.ActorUserID = &actor
	}
	serviceOwner := ledger.Owner{Type: ledger.OwnerService, ID: svc.ID, OrgID: svc.OrganizationID}

	if unit.UnitKind == ledger.KindFixed {
		if !item.Quantity.Valid || item.Quantity.Int32 <= 0 {
			return db.StockMovement{}, fmt.Errorf("services: fixed item %d without quantity", item.ID)
		}
		from, err := fixedSource(ctx, q, unit.ID, svc.OrganizationID, item.Quantity.Int32)
		if err != nil {
			return db.StockMovement{}, err
		}
		m.Type, m.From, m.Quantity = ledger.TypeConsumption, &from, item.Quantity.Int32
		return post(ctx, l, tx, m, item)
	}

	// Serial: the service organization must hold the unit (the partial
	// consumption rule keeps the owner and does not check it).
	if err := requireSerialHeld(ctx, q, unit.ID, svc.OrganizationID); err != nil {
		return db.StockMovement{}, fmt.Errorf("%w: item %s", err, item.Uuid)
	}
	m.Type, m.To = ledger.TypeConsumption, &serviceOwner
	if item.Kind == KindPartial {
		cm, err := centimeters(item.Meters)
		if err != nil {
			return db.StockMovement{}, err
		}
		remaining, err := centimeters(unit.RemainingMeters)
		if err != nil {
			return db.StockMovement{}, err
		}
		switch {
		case cm > remaining:
			return db.StockMovement{}, fmt.Errorf("%w: item %s needs %s m, the roll has %s m",
				ErrUnitNotAvailable, item.Uuid, ledger.FormatMeters(cm), ledger.FormatMeters(remaining))
		case cm < remaining:
			m.Type, m.To, m.Centimeters = ledger.TypePartialConsumption, nil, cm
		}
		// cm == remaining: the cut takes the rest of the roll, a consumption.
	}
	return post(ctx, l, tx, m, item)
}

func post(ctx context.Context, l *ledger.Ledger, tx pgx.Tx, m ledger.Movement, item db.ServiceItem) (db.StockMovement, error) {
	res, err := l.Post(ctx, tx, m)
	if err == nil {
		return res.Movement, nil
	}
	if errors.Is(err, ledger.ErrUnitNotFound) || errors.Is(err, ledger.ErrTransitionNotAllowed) ||
		errors.Is(err, ledger.ErrOwnerMismatch) || errors.Is(err, ledger.ErrOwnerNotAllowed) ||
		errors.Is(err, ledger.ErrInsufficientStock) || errors.Is(err, ledger.ErrInsufficientMeters) ||
		errors.Is(err, ledger.ErrConcurrentUpdate) {
		return db.StockMovement{}, fmt.Errorf("%w: item %s: %v", ErrUnitNotAvailable, item.Uuid, err)
	}
	return db.StockMovement{}, fmt.Errorf("services: consume item %d: %w", item.ID, err)
}

// fixedSource picks the one owner of the organization the pieces are taken
// from: a ledger movement touches one owner and the item has one
// idempotency key, so the whole quantity must sit at one owner. The
// organization owner comes first, then locations in holding order.
func fixedSource(ctx context.Context, q *db.Queries, unitID, orgID int64, qty int32) (ledger.Owner, error) {
	rows, err := q.ListFixedBarcodeHoldingsByUnit(ctx, unitID)
	if err != nil {
		return ledger.Owner{}, fmt.Errorf("services: holdings: %w", err)
	}
	var best *db.FixedBarcodeHolding
	for i := range rows {
		h := &rows[i]
		if h.HolderOrgID != orgID || h.QuantityOnHand < qty ||
			(h.OwnerType != string(ledger.OwnerOrganization) && h.OwnerType != string(ledger.OwnerWarehouseLocation)) {
			continue
		}
		if best == nil || (h.OwnerType == string(ledger.OwnerOrganization) && best.OwnerType != h.OwnerType) {
			best = h
		}
	}
	if best == nil {
		return ledger.Owner{}, fmt.Errorf("%w: %d pieces of unit %d at one owner", ErrUnitNotAvailable, qty, unitID)
	}
	return ledger.Owner{Type: ledger.OwnerType(best.OwnerType), ID: best.OwnerID, OrgID: best.HolderOrgID}, nil
}

// centimeters converts a NUMERIC(10,2) meter amount to hundredths.
func centimeters(n pgtype.Numeric) (int64, error) {
	r := numericRat(n)
	if r == nil {
		return 0, errors.New("services: meters missing")
	}
	r.Mul(r, big.NewRat(100, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, fmt.Errorf("services: meters %s are not whole centimeters", r.FloatString(4))
	}
	return r.Num().Int64(), nil
}
