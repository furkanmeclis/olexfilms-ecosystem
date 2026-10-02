package ledger

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// TypeReclassification changes the product of a unit while its barcode,
// owner and status stay the same (TEC-157). It is not postable: only
// Ledger.Reclassify writes it, for an approved stock_reclassifications row.
const TypeReclassification MovementType = "reclassification"

// Idempotency key parts of a reclassification movement:
// reclassification:stock_reclassification:<request id>:reclassification:<barcode>.
const (
	ReclassificationSource  = "reclassification"
	ReclassificationRefType = "stock_reclassification"
)

// Reclassification refusals (wrong routing). Each one has its own API code.
var (
	// ErrReclassifyFixedBarcode: a fixed barcode stands for many pieces
	// held by many owners; it is not reclassified.
	ErrReclassifyFixedBarcode = errors.New("ledger: fixed barcodes cannot be reclassified")
	// ErrReclassifyProductChanged: the unit no longer carries the product
	// the request was made for.
	ErrReclassifyProductChanged = errors.New("ledger: unit product changed since the request")
	// ErrReclassifySameProduct: the target is the unit's current product.
	ErrReclassifySameProduct = errors.New("ledger: unit already has the target product")
	// ErrReclassifyBrandMismatch: the target product is of another brand (K1).
	ErrReclassifyBrandMismatch = errors.New("ledger: target product is of another brand")
	// ErrReclassifyUnitTypeMismatch: roll <-> piece, or fixed <-> serial.
	ErrReclassifyUnitTypeMismatch = errors.New("ledger: target product has another unit type")
	// ErrReclassifyProductInactive: the target product is passive.
	ErrReclassifyProductInactive = errors.New("ledger: target product is inactive")
	// ErrReclassifyUnitConsumed: the unit is used or void.
	ErrReclassifyUnitConsumed = errors.New("ledger: unit is consumed")
	// ErrReclassifyUnitElsewhere: the unit is not on hand at the expected
	// organization (another holder, in transit, in a service or not in
	// stock yet).
	ErrReclassifyUnitElsewhere = errors.New("ledger: unit is not on hand at the organization")
)

// Reclassification is one approved reclassification request.
type Reclassification struct {
	// RequestID is stock_reclassifications.id (reference and key).
	RequestID int64
	UnitID    int64
	// FromProductID is the product the request was made for; the unit must
	// still carry it.
	FromProductID int64
	ToProductID   int64
	// HolderOrgID is the organization that must hold the unit on hand.
	HolderOrgID int64
	ActorUserID *int64
	Reason      string
	Metadata    map[string]any
}

// CheckReclassification validates a reclassification of unit (current
// state st, nil when it has none) from product from to product to for the
// holder organization. It is pure; Reclassify runs it under the row locks
// and the request flow runs it before saving a request.
func CheckReclassification(unit db.Unit, st *db.UnitCurrentState, from, to db.Product, holderOrgID int64) error {
	if unit.UnitKind == KindFixed {
		return ErrReclassifyFixedBarcode
	}
	if unit.ProductID != from.ID {
		return fmt.Errorf("%w: unit %d carries product %d, not %d", ErrReclassifyProductChanged, unit.ID, unit.ProductID, from.ID)
	}
	if to.ID == from.ID {
		return ErrReclassifySameProduct
	}
	if to.BrandID != unit.BrandID {
		return ErrReclassifyBrandMismatch
	}
	if to.UnitType != from.UnitType || to.UsesFixedBarcode != from.UsesFixedBarcode {
		return fmt.Errorf("%w: %s -> %s", ErrReclassifyUnitTypeMismatch, from.UnitType, to.UnitType)
	}
	if !to.Active {
		return ErrReclassifyProductInactive
	}
	status := Status(unit.Status)
	if st != nil {
		status = Status(st.Status)
	}
	if status == StatusUsed || status == StatusVoid {
		return ErrReclassifyUnitConsumed
	}
	if st == nil || !counted(status) || st.HolderOrgID != holderOrgID {
		return ErrReclassifyUnitElsewhere
	}
	return nil
}

// Reclassify writes the reclassification movement of an approved request
// in tx: the unit row and its state are locked, the request is checked
// again, a reclassification movement (same owner and status, product_id =
// the new product, metadata from/to product) is appended under its
// idempotency key, units.product_id changes, unit_current_state points to
// the movement, the unit's stock moves from the old to the new product in
// bin_product_stocks and organization_product_stocks, and a
// stock.reclassification event is written to the outbox. A retry of the
// same request returns the earlier movement (Replayed) and writes nothing.
func (l *Ledger) Reclassify(ctx context.Context, tx pgx.Tx, r Reclassification) (Result, error) {
	if tx == nil {
		return Result{}, errors.New("ledger: transaction required")
	}
	if r.UnitID <= 0 || r.RequestID <= 0 || r.HolderOrgID <= 0 {
		return Result{}, fmt.Errorf("%w: reclassification", ErrInvalidMovement)
	}
	q := l.q.WithTx(tx)
	unit, err := q.LockUnit(ctx, r.UnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrUnitNotFound
	}
	if err != nil {
		return Result{}, fmt.Errorf("ledger: lock unit: %w", err)
	}
	m := Movement{
		Type: TypeReclassification, UnitID: unit.ID, Source: ReclassificationSource,
		RefType: ReclassificationRefType, RefID: r.RequestID, ActorUserID: r.ActorUserID, Reason: r.Reason,
	}
	key, err := IdempotencyKey(m.Source, m.RefType, m.RefID, m.Type, unit.Barcode)
	if err != nil {
		return Result{}, err
	}
	if res, done, err := replay(ctx, q, key, unit, m); done || err != nil {
		return res, err
	}

	from, err := q.GetProduct(ctx, db.GetProductParams{ID: r.FromProductID, BrandID: unit.BrandID})
	if err != nil {
		return Result{}, fmt.Errorf("ledger: source product: %w", err)
	}
	to, err := q.GetProductByIDAnyBrand(ctx, r.ToProductID)
	if err != nil {
		return Result{}, fmt.Errorf("ledger: target product: %w", err)
	}
	var st *db.UnitCurrentState
	state, err := q.LockUnitCurrentState(ctx, unit.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Result{}, fmt.Errorf("ledger: lock state: %w", err)
	default:
		st = &state
	}
	if err := CheckReclassification(unit, st, from, to, r.HolderOrgID); err != nil {
		return Result{}, err
	}
	owner := Owner{Type: OwnerType(state.OwnerType), ID: state.OwnerID, OrgID: state.HolderOrgID}
	remaining := int64(0)
	if unit.InitialMeters.Valid {
		if remaining, err = numericToCm(unit.RemainingMeters); err != nil {
			return Result{}, err
		}
	}

	m.Metadata = maps.Clone(r.Metadata)
	if m.Metadata == nil {
		m.Metadata = map[string]any{}
	}
	m.Metadata["from_product_id"] = from.ID
	m.Metadata["to_product_id"] = to.ID
	p := &post{l: l, q: q, tx: tx, m: m, unit: unit, key: key}
	arg, err := p.baseArgs()
	if err != nil {
		return Result{}, err
	}
	arg.ProductID = to.ID
	arg.MetersDelta = cmToNumeric(0)
	arg.FromStatus, arg.ToStatus = text(state.Status), text(state.Status)
	arg.FromOwnerType, arg.FromOwnerID = text(string(owner.Type)), i8(owner.ID)
	arg.ToOwnerType, arg.ToOwnerID = text(string(owner.Type)), i8(owner.ID)
	arg.OrganizationID = owner.OrgID
	mv, replayed, err := p.insert(ctx, arg)
	if err != nil || replayed {
		return Result{Movement: mv, Replayed: replayed}, err
	}

	if _, err := q.UpdateUnitProduct(ctx, db.UpdateUnitProductParams{ID: unit.ID, ProductID: to.ID}); err != nil {
		return Result{}, dbErr("unit product", err)
	}
	if _, err := q.UpdateUnitCurrentState(ctx, db.UpdateUnitCurrentStateParams{
		UnitID: unit.ID, Version: state.Version,
		OwnerType: state.OwnerType, OwnerID: state.OwnerID, HolderOrgID: state.HolderOrgID,
		Status: state.Status, LastMovementID: i8(mv.ID),
	}); errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrConcurrentUpdate
	} else if err != nil {
		return Result{}, dbErr("unit state", err)
	}

	// The unit is on hand (counted): its stock leaves the old product and
	// joins the new one at the same owner.
	var out, in deltas
	out.add(owner, -1, -remaining)
	in.add(owner, 1, remaining)
	if err := p.applyProjectionsFor(ctx, out, from.ID); err != nil {
		return Result{}, err
	}
	if err := p.applyProjectionsFor(ctx, in, to.ID); err != nil {
		return Result{}, err
	}
	p.unit.ProductID = to.ID
	return p.publish(ctx, mv, owner.OrgID)
}
