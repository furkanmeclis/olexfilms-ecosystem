package usecase

// Stock side of a transfer request (TEC-197).
//
// A request line is one unit the giver holds: a serial piece, a whole roll
// (its remaining meters are kept as a snapshot) or a quantity of a fixed
// barcode. A unit is on at most one open (requested or approved) request
// and never on a request while an order line reserves it; the checks run
// under the unit row lock, the lock ledger.Post takes as well.
//
// approved -> shipped posts one transfer_out per line (key
// transfer:transfer_item:<id>:transfer_out:<barcode>): serial units go in
// transit owned by the receiver, fixed barcodes are debited from the giver's
// owner holding the quantity. shipped -> received posts the paired
// transfer_in into the receiver organization; shipped -> cancelled posts
// transfer_cancel_restore, which puts each unit back where it was. A
// rejected or cancelled-before-shipping request writes no movement.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func stocked(status string) bool {
	return status == string(ledger.StatusAvailable) || status == string(ledger.StatusPlaced)
}

// checkLine resolves and locks the unit of a request line and checks that
// the giver can hand it over. It returns the quantity (fixed barcodes) and
// the meters snapshot (rolls).
func (s *Service) checkLine(ctx context.Context, q *db.Queries, r db.StockTransferRequest, field string,
	it ItemInput) (db.Unit, pgtype.Int4, pgtype.Numeric, error) {
	var (
		qty    pgtype.Int4
		meters pgtype.Numeric
	)
	barcode := strings.TrimSpace(it.Barcode)
	if barcode == "" {
		return db.Unit{}, qty, meters, invalid(field+".barcode", "barcode is required")
	}
	u, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: r.BrandID, Barcode: barcode})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Unit{}, qty, meters, &ValidationError{Field: field + ".barcode", Code: CodeUnitNotFound, Message: "unit not found"}
	}
	if err != nil {
		return db.Unit{}, qty, meters, fmt.Errorf("transfers: unit: %w", err)
	}
	if u, err = q.LockUnit(ctx, u.ID); err != nil {
		return db.Unit{}, qty, meters, fmt.Errorf("transfers: lock unit: %w", err)
	}
	notAvailable := &ValidationError{Field: field + ".barcode", Code: CodeUnitNotAvailable,
		Message: "the unit is not in this organization's stock"}
	reserved := &ValidationError{Field: field + ".barcode", Code: CodeUnitReserved,
		Message: "the unit is reserved by an order or another transfer"}

	if u.UnitKind == ledger.KindFixed {
		if it.Quantity == nil || *it.Quantity <= 0 || *it.Quantity > MaxQuantity {
			return u, qty, meters, invalid(field+".quantity", fmt.Sprintf("must be between 1 and %d", MaxQuantity))
		}
		free, err := s.freeFixed(ctx, q, u, r.FromOrgID, r.ID)
		if err != nil {
			return u, qty, meters, err
		}
		if *it.Quantity > free {
			return u, qty, meters, &ValidationError{Field: field + ".quantity", Code: CodeInsufficientStock,
				Message: fmt.Sprintf("only %d available for this barcode", max(free, 0))}
		}
		return u, pgtype.Int4{Int32: int32(*it.Quantity), Valid: true}, meters, nil
	}

	if it.Quantity != nil && *it.Quantity != 1 {
		return u, qty, meters, invalid(field+".quantity", "a serial unit moves whole (quantity 1)")
	}
	st, err := q.GetUnitCurrentState(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, qty, meters, notAvailable
	}
	if err != nil {
		return u, qty, meters, fmt.Errorf("transfers: unit state: %w", err)
	}
	if st.HolderOrgID != r.FromOrgID || !stocked(st.Status) {
		return u, qty, meters, notAvailable
	}
	if err := s.checkSerialFree(ctx, q, u, r.ID); err != nil {
		if errors.Is(err, errReserved) {
			return u, qty, meters, reserved
		}
		return u, qty, meters, err
	}
	if u.RemainingMeters.Valid {
		meters = u.RemainingMeters
	}
	return u, qty, meters, nil
}

var errReserved = errors.New("transfers: unit reserved")

// checkSerialFree: no active order reservation and no other open request
// holds the serial unit.
func (s *Service) checkSerialFree(ctx context.Context, q *db.Queries, u db.Unit, requestID int64) error {
	active, err := q.LockActiveReservationsByUnit(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("transfers: reservations: %w", err)
	}
	if len(active) > 0 {
		return errReserved
	}
	n, err := q.CountOpenTransferItemsByUnit(ctx, db.CountOpenTransferItemsByUnitParams{UnitID: u.ID, ExcludeRequestID: requestID})
	if err != nil {
		return fmt.Errorf("transfers: open transfers: %w", err)
	}
	if n > 0 {
		return errReserved
	}
	return nil
}

// freeFixed is what the giver holds of a fixed barcode minus its active
// order reservations and its other open transfer requests.
func (s *Service) freeFixed(ctx context.Context, q *db.Queries, u db.Unit, from, requestID int64) (int64, error) {
	holdings, err := q.LockFixedBarcodeHoldingsByUnit(ctx, u.ID)
	if err != nil {
		return 0, fmt.Errorf("transfers: holdings: %w", err)
	}
	var onHand int64
	for _, h := range holdings {
		if h.HolderOrgID == from {
			onHand += int64(h.QuantityOnHand)
		}
	}
	reserved, err := q.SumActiveReservedQuantityByUnit(ctx, db.SumActiveReservedQuantityByUnitParams{
		UnitID: u.ID, OrganizationID: from,
	})
	if err != nil {
		return 0, fmt.Errorf("transfers: reserved quantity: %w", err)
	}
	open, err := q.SumOpenTransferQuantityByUnit(ctx, db.SumOpenTransferQuantityByUnitParams{
		UnitID: u.ID, FromOrgID: from, ExcludeRequestID: requestID,
	})
	if err != nil {
		return 0, fmt.Errorf("transfers: open quantity: %w", err)
	}
	return onHand - reserved - open, nil
}

// stockErr maps a ledger refusal to ErrStockUnavailable.
func stockErr(op, barcode string, err error) error {
	if errors.Is(err, ledger.ErrInsufficientStock) || errors.Is(err, ledger.ErrTransitionNotAllowed) ||
		errors.Is(err, ledger.ErrOwnerMismatch) || errors.Is(err, ledger.ErrOwnerNotAllowed) {
		return fmt.Errorf("%w: %s: %v", ErrStockUnavailable, barcode, err)
	}
	return fmt.Errorf("transfers: %s %s: %w", op, barcode, err)
}

func (s *Service) items(ctx context.Context, q *db.Queries, r db.StockTransferRequest) ([]db.ListTransferRequestItemsRow, error) {
	rows, err := q.ListTransferRequestItems(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("transfers: items: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: the request has no unit", ErrStockUnavailable)
	}
	return rows, nil
}

func movementMeta(r db.StockTransferRequest) map[string]any {
	return map[string]any{"transfer_uuid": r.Uuid.String(), "transfer_no": r.TransferNo}
}

// ship posts a transfer_out for every line.
func (s *Service) ship(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, r db.StockTransferRequest) error {
	rows, err := s.items(ctx, q, r)
	if err != nil {
		return err
	}
	receiver := ledger.Owner{Type: ledger.OwnerOrganization, ID: r.ToOrgID, OrgID: r.ToOrgID}
	for _, it := range rows {
		m := ledger.Movement{
			Type: ledger.TypeTransferOut, UnitID: it.UnitID,
			Source: LedgerSource, RefType: LedgerRefType, RefID: it.ID, ActorUserID: c.actorPtr(),
			Metadata: movementMeta(r),
		}
		if it.UnitKind == ledger.KindFixed {
			from, err := fixedSource(ctx, q, it.UnitID, r.FromOrgID, it.Quantity.Int32)
			if err != nil {
				return err
			}
			m.From, m.Quantity = &from, it.Quantity.Int32
		} else if !it.OutMovementID.Valid {
			// A unit reserved by an order after the request was opened
			// stays with that order.
			u, err := q.LockUnit(ctx, it.UnitID)
			if err != nil {
				return fmt.Errorf("transfers: lock unit: %w", err)
			}
			if err := s.checkSerialFree(ctx, q, u, r.ID); err != nil {
				if errors.Is(err, errReserved) {
					return fmt.Errorf("%w: unit %s is reserved by an order", ErrStockUnavailable, it.Barcode)
				}
				return err
			}
			st, err := q.GetUnitCurrentState(ctx, it.UnitID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("transfers: unit state: %w", err)
			}
			if err != nil || st.HolderOrgID != r.FromOrgID {
				return fmt.Errorf("%w: unit %s is not held by the sender", ErrStockUnavailable, it.Barcode)
			}
			m.To = &receiver
		} else {
			m.To = &receiver
		}
		res, err := s.ledger.Post(ctx, tx, m)
		if err != nil {
			return stockErr("transfer_out", it.Barcode, err)
		}
		if err := q.SetTransferItemOutMovement(ctx, db.SetTransferItemOutMovementParams{
			ID: it.ID, MovementID: pgtype.Int8{Int64: res.Movement.ID, Valid: true},
		}); err != nil {
			return fmt.Errorf("transfers: item movement: %w", err)
		}
	}
	return nil
}

// receive posts the paired transfer_in of every shipped line into the
// receiver organization.
func (s *Service) receive(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, r db.StockTransferRequest) error {
	rows, err := s.items(ctx, q, r)
	if err != nil {
		return err
	}
	receiver := ledger.Owner{Type: ledger.OwnerOrganization, ID: r.ToOrgID, OrgID: r.ToOrgID}
	for _, it := range rows {
		if !it.OutMovementID.Valid {
			return fmt.Errorf("%w: unit %s was not shipped", ErrStockUnavailable, it.Barcode)
		}
		m := ledger.Movement{
			Type: ledger.TypeTransferIn, UnitID: it.UnitID, To: &receiver,
			Source: LedgerSource, RefType: LedgerRefType, RefID: it.ID, ActorUserID: c.actorPtr(),
			Metadata: movementMeta(r),
		}
		if it.UnitKind == ledger.KindFixed {
			m.Quantity = it.Quantity.Int32
		}
		res, err := s.ledger.Post(ctx, tx, m)
		if err != nil {
			return stockErr("transfer_in", it.Barcode, err)
		}
		if err := q.SetTransferItemInMovement(ctx, db.SetTransferItemInMovementParams{
			ID: it.ID, MovementID: pgtype.Int8{Int64: res.Movement.ID, Valid: true},
		}); err != nil {
			return fmt.Errorf("transfers: item movement: %w", err)
		}
	}
	return nil
}

// restore posts a transfer_cancel_restore for every shipped line: serial
// units go back to the transfer_out's source owner and status, fixed
// barcodes are credited back to the owner the transfer_out debited.
func (s *Service) restore(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, r db.StockTransferRequest) error {
	rows, err := s.items(ctx, q, r)
	if err != nil {
		return err
	}
	for _, it := range rows {
		if !it.OutMovementID.Valid {
			return fmt.Errorf("%w: unit %s was not shipped", ErrStockUnavailable, it.Barcode)
		}
		m := ledger.Movement{
			Type: ledger.TypeTransferCancelRestore, UnitID: it.UnitID,
			Source: LedgerSource, RefType: LedgerRefType, RefID: it.ID, ActorUserID: c.actorPtr(),
			Reason: "transfer cancelled after shipping", Metadata: movementMeta(r),
		}
		if it.UnitKind == ledger.KindFixed {
			out, err := q.GetStockMovement(ctx, it.OutMovementID.Int64)
			if err != nil {
				return fmt.Errorf("transfers: transfer_out of %s: %w", it.Barcode, err)
			}
			if !out.FromOwnerType.Valid || !out.FromOwnerID.Valid {
				return fmt.Errorf("%w: transfer_out of %s has no source", ErrStockUnavailable, it.Barcode)
			}
			src := ledger.Owner{Type: ledger.OwnerType(out.FromOwnerType.String), ID: out.FromOwnerID.Int64, OrgID: out.OrganizationID}
			m.To, m.Quantity = &src, it.Quantity.Int32
		}
		res, err := s.ledger.Post(ctx, tx, m)
		if err != nil {
			return stockErr("transfer_cancel_restore", it.Barcode, err)
		}
		if err := q.SetTransferItemRestoreMovement(ctx, db.SetTransferItemRestoreMovementParams{
			ID: it.ID, MovementID: pgtype.Int8{Int64: res.Movement.ID, Valid: true},
		}); err != nil {
			return fmt.Errorf("transfers: item movement: %w", err)
		}
	}
	return nil
}

// fixedSource picks the giver's owner (location or the organization) that
// holds the whole quantity, the largest first: one line is one movement.
func fixedSource(ctx context.Context, q *db.Queries, unitID, from int64, qty int32) (ledger.Owner, error) {
	holdings, err := q.ListFixedBarcodeHoldingsByUnit(ctx, unitID)
	if err != nil {
		return ledger.Owner{}, fmt.Errorf("transfers: holdings: %w", err)
	}
	var best *db.FixedBarcodeHolding
	for i := range holdings {
		h := &holdings[i]
		if h.HolderOrgID != from || h.QuantityOnHand < qty {
			continue
		}
		if best == nil || h.QuantityOnHand > best.QuantityOnHand {
			best = h
		}
	}
	if best == nil {
		return ledger.Owner{}, fmt.Errorf("%w: no single owner of the sender holds %d", ErrStockUnavailable, qty)
	}
	return ledger.Owner{Type: ledger.OwnerType(best.OwnerType), ID: best.OwnerID, OrgID: best.HolderOrgID}, nil
}

// freezePrices locks the giver's purchase price into every line (K13: the
// transfer price is A's purchase price) and returns the request total. A
// line without a price stays unpriced and the total is then NULL.
func (s *Service) freezePrices(ctx context.Context, q *db.Queries, r db.StockTransferRequest) (pgtype.Numeric, error) {
	rows, err := s.items(ctx, q, r)
	if err != nil {
		return pgtype.Numeric{}, err
	}
	from, err := q.GetOrganizationByID(ctx, r.FromOrgID)
	if err != nil {
		return pgtype.Numeric{}, fmt.Errorf("transfers: sender: %w", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, it := range rows {
		ids = append(ids, it.ProductID)
	}
	prices, err := pricingusecase.New(q).BuyerPurchasePrices(ctx, from.ID, from.Type, r.BrandID, ids, strings.TrimSpace(r.Currency))
	if err != nil {
		return pgtype.Numeric{}, fmt.Errorf("transfers: prices: %w", err)
	}
	total, complete := new(big.Rat), true
	for _, it := range rows {
		p, ok := prices[it.ProductID]
		if !ok {
			complete = false
			continue
		}
		price, err := parseRat(p.Price)
		if err != nil {
			return pgtype.Numeric{}, err
		}
		line := new(big.Rat).Mul(price, lineAmount(it))
		up, err := numeric(price.FloatString(4))
		if err != nil {
			return pgtype.Numeric{}, err
		}
		lt, err := numeric(line.FloatString(2))
		if err != nil {
			return pgtype.Numeric{}, err
		}
		if err := q.SetTransferItemPrice(ctx, db.SetTransferItemPriceParams{ID: it.ID, UnitPrice: up, LineTotal: lt}); err != nil {
			return pgtype.Numeric{}, fmt.Errorf("transfers: item price: %w", err)
		}
		total.Add(total, line)
	}
	if !complete {
		return pgtype.Numeric{}, nil
	}
	return numeric(total.FloatString(2))
}

// lineAmount is the priced amount of a line: the quantity of a fixed
// barcode, the meters of a roll of a per-meter product, else one piece.
func lineAmount(it db.ListTransferRequestItemsRow) *big.Rat {
	switch {
	case it.Quantity.Valid:
		return new(big.Rat).SetInt64(int64(it.Quantity.Int32))
	case it.Meters.Valid && it.ProductUnitType == "roll_meter":
		if r := numericRat(it.Meters); r != nil {
			return r
		}
	}
	return big.NewRat(1, 1)
}
