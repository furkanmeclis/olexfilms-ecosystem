package ledger

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// TypeMeasureConversion turns a piece unit into a roll of a given length,
// or a roll that was never cut back into a piece (TEC-1008). Owner, status
// and product stay; the movement carries no meters delta (the projection
// counts the unit's remaining meters from the conversion on) and records
// the measure before and after in its metadata. It is not postable: only
// Ledger.ConvertMeasure writes it.
const TypeMeasureConversion MovementType = "measure_conversion"

// Conversion refusals.
var (
	// ErrConvertFixedBarcode: a fixed barcode is many pieces.
	ErrConvertFixedBarcode = errors.New("ledger: fixed barcodes cannot be converted")
	// ErrConvertAlreadyRoll: the unit already carries meters.
	ErrConvertAlreadyRoll = errors.New("ledger: unit is already a roll")
	// ErrConvertNotRoll: back to piece needs a roll.
	ErrConvertNotRoll = errors.New("ledger: unit is not a roll")
	// ErrConvertRollUsed: a roll with meters used cannot go back to piece.
	ErrConvertRollUsed = errors.New("ledger: roll has been cut")
	// ErrConvertNotOnHand: the unit must be on hand (available or placed)
	// at the holder.
	ErrConvertNotOnHand = errors.New("ledger: unit is not on hand at the organization")
)

// MeasureConversion is one conversion of a serial unit.
type MeasureConversion struct {
	UnitID int64
	// Centimeters is the roll length (> 0); 0 converts a roll back to a
	// piece.
	Centimeters int64
	// HolderOrgID is the organization that must hold the unit on hand.
	HolderOrgID int64
	// Idempotency key parts: {source}:{ref_type}:{ref_id}:measure_conversion:{barcode}.
	Source, RefType string
	RefID           int64
	ActorUserID     *int64
	Reason          string
	Metadata        map[string]any
}

// ConvertMeasure records the conversion and updates the unit, its state
// version and the product stock meters.
func (l *Ledger) ConvertMeasure(ctx context.Context, tx pgx.Tx, c MeasureConversion) (Result, error) {
	if tx == nil {
		return Result{}, errors.New("ledger: transaction required")
	}
	if c.UnitID <= 0 || c.HolderOrgID <= 0 || c.Centimeters < 0 {
		return Result{}, fmt.Errorf("%w: measure conversion", ErrInvalidMovement)
	}
	q := l.q.WithTx(tx)
	unit, err := q.LockUnit(ctx, c.UnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrUnitNotFound
	}
	if err != nil {
		return Result{}, fmt.Errorf("ledger: lock unit: %w", err)
	}
	m := Movement{
		Type: TypeMeasureConversion, UnitID: unit.ID, Source: c.Source, RefType: c.RefType, RefID: c.RefID,
		ActorUserID: c.ActorUserID, Reason: c.Reason,
	}
	key, err := IdempotencyKey(m.Source, m.RefType, m.RefID, m.Type, unit.Barcode)
	if err != nil {
		return Result{}, err
	}
	if res, done, err := replay(ctx, q, key, unit, m); done || err != nil {
		return res, err
	}
	if unit.UnitKind != KindSerial {
		return Result{}, ErrConvertFixedBarcode
	}
	state, err := q.LockUnitCurrentState(ctx, unit.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrConvertNotOnHand
	}
	if err != nil {
		return Result{}, fmt.Errorf("ledger: lock state: %w", err)
	}
	if state.HolderOrgID != c.HolderOrgID || !counted(Status(state.Status)) {
		return Result{}, ErrConvertNotOnHand
	}

	var before, after pgtype.Numeric
	var delta int64
	if c.Centimeters > 0 {
		if unit.InitialMeters.Valid {
			return Result{}, ErrConvertAlreadyRoll
		}
		after, delta = cmToNumeric(c.Centimeters), c.Centimeters
	} else {
		if !unit.InitialMeters.Valid {
			return Result{}, ErrConvertNotRoll
		}
		initial, err := numericToCm(unit.InitialMeters)
		if err != nil {
			return Result{}, err
		}
		remaining, err := numericToCm(unit.RemainingMeters)
		if err != nil {
			return Result{}, err
		}
		if remaining != initial {
			return Result{}, ErrConvertRollUsed
		}
		before, delta = unit.InitialMeters, -initial
	}

	owner := Owner{Type: OwnerType(state.OwnerType), ID: state.OwnerID, OrgID: state.HolderOrgID}
	m.Metadata = maps.Clone(c.Metadata)
	if m.Metadata == nil {
		m.Metadata = map[string]any{}
	}
	m.Metadata["from_meters"] = numericMeters(unit.InitialMeters, before)
	m.Metadata["to_meters"] = numericMeters(after, pgtype.Numeric{})
	p := &post{l: l, q: q, tx: tx, m: m, unit: unit, key: key}
	arg, err := p.baseArgs()
	if err != nil {
		return Result{}, err
	}
	arg.MetersDelta = cmToNumeric(0)
	arg.FromStatus, arg.ToStatus = text(state.Status), text(state.Status)
	arg.FromOwnerType, arg.FromOwnerID = text(string(owner.Type)), i8(owner.ID)
	arg.ToOwnerType, arg.ToOwnerID = text(string(owner.Type)), i8(owner.ID)
	arg.OrganizationID = owner.OrgID
	mv, replayed, err := p.insert(ctx, arg)
	if err != nil || replayed {
		return Result{Movement: mv, Replayed: replayed}, err
	}
	if err := q.SetUnitMeasure(ctx, db.SetUnitMeasureParams{ID: unit.ID, Meters: after}); err != nil {
		return Result{}, dbErr("unit measure", err)
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
	// The on-hand unit now counts its meters at the same owner.
	var d deltas
	d.add(owner, 0, delta)
	if err := p.applyProjections(ctx, d); err != nil {
		return Result{}, err
	}
	return p.publish(ctx, mv, owner.OrgID)
}

// numericMeters renders the first valid numeric as meters (nil: none).
func numericMeters(ns ...pgtype.Numeric) any {
	for _, n := range ns {
		if n.Valid {
			cm, err := numericToCm(n)
			if err == nil {
				return FormatMeters(cm)
			}
		}
	}
	return nil
}
