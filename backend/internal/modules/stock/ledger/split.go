package ledger

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// TypeSplit cuts meters off a roll as a new unit with its own barcode
// (TEC-184). It is not postable: only Ledger.Split writes it, always as a
// pair in one transaction:
//
//   - on the source roll: meters_delta = -N, owner kept, status kept (used
//     when nothing is left), metadata.split_role = "source";
//   - the first movement of the new unit: the source's owner and status,
//     meters_delta = 0 (the new unit's initial = remaining = N),
//     metadata.split_role = "target".
//
// Both reference the stock_splits row (split:stock_split:<id>:split:<barcode>),
// so the meters the roll lost equal the meters of the new unit and the
// product stock totals are unchanged.
const TypeSplit MovementType = "split"

// Idempotency key parts of a split movement.
const (
	SplitSource  = "split"
	SplitRefType = "stock_split"
)

// Split roles in the movement metadata.
const (
	SplitRoleSource = "source"
	SplitRoleTarget = "target"
)

// maxBarcodeLen is units.barcode VARCHAR(64).
const maxBarcodeLen = 64

// MaxSplitKeyLen is stock_splits.idempotency_key VARCHAR(128).
const MaxSplitKeyLen = 128

// Split refusals.
var (
	// ErrSplitNotRoll: only rolls (serial units with meters) are split;
	// pieces and fixed barcodes are counted, not measured.
	ErrSplitNotRoll = errors.New("ledger: only rolls can be split")
	// ErrSplitUnitElsewhere: the roll is not on hand at the organization.
	ErrSplitUnitElsewhere = errors.New("ledger: roll is not on hand at the organization")
)

// SplitCommand is one split of a roll held on hand by HolderOrgID.
type SplitCommand struct {
	SourceUnitID int64
	// Centimeters cut off (> 0, at most the remaining meters).
	Centimeters int64
	HolderOrgID int64
	// IdempotencyKey: one split per key and organization; a retry with the
	// same key returns the earlier split and writes nothing.
	IdempotencyKey string
	// RefType / RefID optionally name the document the split serves (an
	// order line).
	RefType     string
	RefID       int64
	ActorUserID *int64
	Reason      string
	Metadata    map[string]any
}

// SplitResult is the outcome of Split.
type SplitResult struct {
	Split db.StockSplit
	// Source is the roll after the split; New is the new unit.
	Source db.Unit
	New    db.Unit
	// Replayed: the key already existed, nothing was written.
	Replayed bool
}

// FindSplit returns the split recorded under (org, key), if any.
func FindSplit(ctx context.Context, q *db.Queries, orgID int64, key string) (*SplitResult, error) {
	s, err := q.GetStockSplitByKey(ctx, db.GetStockSplitByKeyParams{OrganizationID: orgID, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ledger: split lookup: %w", err)
	}
	src, err := q.GetUnit(ctx, s.SourceUnitID)
	if err != nil {
		return nil, fmt.Errorf("ledger: split source: %w", err)
	}
	nu, err := q.GetUnit(ctx, s.NewUnitID)
	if err != nil {
		return nil, fmt.Errorf("ledger: split unit: %w", err)
	}
	return &SplitResult{Split: s, Source: src, New: nu, Replayed: true}, nil
}

// SplitBarcode derives the barcode of the n-th split of a roll:
// <source barcode>-S<n>, the source part shortened to fit VARCHAR(64).
func SplitBarcode(source string, n int64) string {
	suffix := "-S" + strconv.FormatInt(n, 10)
	if len(source)+len(suffix) > maxBarcodeLen {
		source = source[:maxBarcodeLen-len(suffix)]
	}
	return source + suffix
}

// Split cuts c.Centimeters off a roll in tx: the roll row and its state are
// locked, the key is looked up (a retry returns the earlier split), the
// roll must be on hand at the holder, a new unit of the same product is
// created with the next derived barcode, the stock_splits row is saved and
// the two split movements are appended with their projections and
// stock.split events. A roll cut to 0 m is used up.
func (l *Ledger) Split(ctx context.Context, tx pgx.Tx, c SplitCommand) (SplitResult, error) {
	if tx == nil {
		return SplitResult{}, errors.New("ledger: transaction required")
	}
	key := strings.TrimSpace(c.IdempotencyKey)
	switch {
	case c.SourceUnitID <= 0 || c.HolderOrgID <= 0:
		return SplitResult{}, fmt.Errorf("%w: split", ErrInvalidMovement)
	case key == "" || len(key) > MaxSplitKeyLen:
		return SplitResult{}, fmt.Errorf("%w: split idempotency key", ErrInvalidMovement)
	case c.Centimeters <= 0:
		return SplitResult{}, fmt.Errorf("%w: split needs meters > 0", ErrInvalidMovement)
	}
	q := l.q.WithTx(tx)
	unit, err := q.LockUnit(ctx, c.SourceUnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SplitResult{}, ErrUnitNotFound
	}
	if err != nil {
		return SplitResult{}, fmt.Errorf("ledger: lock unit: %w", err)
	}
	// The unit lock serialises splits of the roll, so a retry of the same
	// key sees the committed split here.
	if prev, err := FindSplit(ctx, q, c.HolderOrgID, key); err != nil || prev != nil {
		if err != nil {
			return SplitResult{}, err
		}
		if prev.Split.SourceUnitID != unit.ID || mustCm(prev.Split.Meters) != c.Centimeters {
			return SplitResult{}, fmt.Errorf("%w: split key %s", ErrIdempotencyConflict, key)
		}
		return *prev, nil
	}
	if unit.UnitKind == KindFixed || !unit.InitialMeters.Valid {
		return SplitResult{}, ErrSplitNotRoll
	}
	remaining, err := numericToCm(unit.RemainingMeters)
	if err != nil {
		return SplitResult{}, err
	}
	state, err := q.GetUnitCurrentState(ctx, unit.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SplitResult{}, ErrSplitUnitElsewhere
	}
	if err != nil {
		return SplitResult{}, fmt.Errorf("ledger: unit state: %w", err)
	}
	owner := Owner{Type: OwnerType(state.OwnerType), ID: state.OwnerID, OrgID: state.HolderOrgID}
	status := Status(state.Status)
	if !counted(status) || state.HolderOrgID != c.HolderOrgID {
		return SplitResult{}, ErrSplitUnitElsewhere
	}
	if c.Centimeters > remaining {
		return SplitResult{}, fmt.Errorf("%w: %s m left, %s m requested",
			ErrInsufficientMeters, FormatMeters(remaining), FormatMeters(c.Centimeters))
	}

	nu, err := l.createSplitUnit(ctx, q, unit, status, c.Centimeters)
	if err != nil {
		return SplitResult{}, err
	}
	arg := db.CreateStockSplitParams{
		OrganizationID: c.HolderOrgID, BrandID: unit.BrandID, ProductID: unit.ProductID,
		SourceUnitID: unit.ID, NewUnitID: nu.ID, Meters: cmToNumeric(c.Centimeters),
		IdempotencyKey: key, ReferenceType: text(c.RefType),
	}
	if c.RefType != "" {
		arg.ReferenceID = i8(c.RefID)
	}
	if c.ActorUserID != nil {
		arg.CreatedByUserID = i8(*c.ActorUserID)
	}
	split, err := q.CreateStockSplit(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		// The same key was used concurrently for another roll.
		return SplitResult{}, fmt.Errorf("%w: split key %s", ErrIdempotencyConflict, key)
	}
	if err != nil {
		return SplitResult{}, fmt.Errorf("ledger: split: %w", err)
	}

	meta := func(role string, other db.Unit) map[string]any {
		m := maps.Clone(c.Metadata)
		if m == nil {
			m = map[string]any{}
		}
		m["split_role"] = role
		m["split_uuid"] = split.Uuid.String()
		m["meters"] = FormatMeters(c.Centimeters)
		m["counterpart_unit_id"] = other.ID
		m["counterpart_barcode"] = other.Barcode
		return m
	}

	// Source: through the serial transition table (targetKeep, meters).
	src := Movement{
		Type: TypeSplit, UnitID: unit.ID, Centimeters: c.Centimeters,
		Source: SplitSource, RefType: SplitRefType, RefID: split.ID,
		ActorUserID: c.ActorUserID, Reason: c.Reason, Metadata: meta(SplitRoleSource, nu),
	}
	srcKey, err := IdempotencyKey(src.Source, src.RefType, src.RefID, src.Type, unit.Barcode)
	if err != nil {
		return SplitResult{}, err
	}
	sp := &post{l: l, q: q, tx: tx, m: src, unit: unit, key: srcKey}
	if _, err := sp.serial(ctx); err != nil {
		return SplitResult{}, err
	}

	// Target: the new unit's first movement, at the source's owner.
	tgt := Movement{
		Type: TypeSplit, UnitID: nu.ID, Source: SplitSource, RefType: SplitRefType, RefID: split.ID,
		ActorUserID: c.ActorUserID, Reason: c.Reason, Metadata: meta(SplitRoleTarget, unit),
	}
	tgtKey, err := IdempotencyKey(tgt.Source, tgt.RefType, tgt.RefID, tgt.Type, nu.Barcode)
	if err != nil {
		return SplitResult{}, err
	}
	tp := &post{l: l, q: q, tx: tx, m: tgt, unit: nu, key: tgtKey}
	if err := tp.splitTarget(ctx, owner, status, c.Centimeters); err != nil {
		return SplitResult{}, err
	}

	after, err := q.GetUnit(ctx, unit.ID)
	if err != nil {
		return SplitResult{}, fmt.Errorf("ledger: split source: %w", err)
	}
	return SplitResult{Split: split, Source: after, New: nu}, nil
}

// createSplitUnit opens the new unit: same issuer, brand and product as the
// roll, the next free derived barcode, initial = remaining = cm, the roll's
// status (its first movement puts it at the roll's owner).
func (l *Ledger) createSplitUnit(ctx context.Context, q *db.Queries, src db.Unit, status Status, cm int64) (db.Unit, error) {
	n, err := q.CountStockSplitsBySource(ctx, src.ID)
	if err != nil {
		return db.Unit{}, fmt.Errorf("ledger: split count: %w", err)
	}
	for i := int64(1); i <= 100; i++ {
		barcode := SplitBarcode(src.Barcode, n+i)
		_, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: src.BrandID, Barcode: barcode})
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.Unit{}, fmt.Errorf("ledger: barcode lookup: %w", err)
		}
		u, err := q.CreateUnit(ctx, db.CreateUnitParams{
			OrganizationID: src.OrganizationID, BrandID: src.BrandID, ProductID: src.ProductID,
			Barcode: barcode, UnitKind: KindSerial, Source: "generated", Status: string(status),
			InitialMeters: cmToNumeric(cm), RemainingMeters: cmToNumeric(cm),
		})
		if err != nil {
			return db.Unit{}, dbErr("split unit", err)
		}
		return u, nil
	}
	return db.Unit{}, fmt.Errorf("ledger: no free split barcode for %s", src.Barcode)
}

// splitTarget writes the first movement of a split's new unit and its
// projections.
func (p *post) splitTarget(ctx context.Context, owner Owner, status Status, cm int64) error {
	arg, err := p.baseArgs()
	if err != nil {
		return err
	}
	arg.QuantityDelta = b2i(counted(status))
	arg.MetersDelta = cmToNumeric(0)
	arg.ToStatus = text(string(status))
	arg.ToOwnerType, arg.ToOwnerID = text(string(owner.Type)), i8(owner.ID)
	arg.OrganizationID = owner.OrgID
	mv, replayed, err := p.insert(ctx, arg)
	if err != nil {
		return err
	}
	if replayed {
		return fmt.Errorf("%w: split target %s", ErrIdempotencyConflict, p.key)
	}
	if _, err := p.q.InsertUnitCurrentState(ctx, db.InsertUnitCurrentStateParams{
		UnitID: p.unit.ID, BrandID: p.unit.BrandID,
		OwnerType: string(owner.Type), OwnerID: owner.ID, HolderOrgID: owner.OrgID,
		Status: string(status), LastMovementID: i8(mv.ID),
	}); err != nil {
		return dbErr("unit state", err)
	}
	var d deltas
	if counted(status) {
		d.add(owner, 1, cm)
	}
	if err := p.applyProjections(ctx, d); err != nil {
		return err
	}
	_, err = p.publish(ctx, mv, owner.OrgID)
	return err
}

// splitPair collects both sides of one split while replaying.
type splitPair struct {
	sourceCm, targetCm   int64
	hasSource, hasTarget bool
}

// trackSplits checks the split movements of one replayed unit and records
// their meters per split (reference id) for SplitAnomalies.
func (p *Projection) trackSplits(u db.Unit, up *UnitProjection, mvs []db.StockMovement) {
	for i := range mvs {
		mv := &mvs[i]
		if MovementType(mv.Type) != TypeSplit {
			continue
		}
		if mv.ReferenceType.String != SplitRefType || !mv.ReferenceID.Valid {
			up.anomaly("movement %d (%s): split without its stock_split reference", mv.ID, mv.Type)
			continue
		}
		if p.splits == nil {
			p.splits = map[int64]*splitPair{}
		}
		pair := p.splits[mv.ReferenceID.Int64]
		if pair == nil {
			pair = &splitPair{}
			p.splits[mv.ReferenceID.Int64] = pair
		}
		cm := mustCm(mv.MetersDelta)
		switch splitRole(mv.Metadata) {
		case SplitRoleSource:
			if cm >= 0 {
				up.anomaly("movement %d (%s): split source without meters", mv.ID, mv.Type)
			}
			if mv.FromOwnerType != mv.ToOwnerType || mv.FromOwnerID != mv.ToOwnerID {
				up.anomaly("movement %d (%s): split moves the source roll", mv.ID, mv.Type)
			}
			if mv.FromStatus != mv.ToStatus && Status(mv.ToStatus.String) != StatusUsed {
				up.anomaly("movement %d (%s): split source status %s -> %s", mv.ID, mv.Type, mv.FromStatus.String, mv.ToStatus.String)
			}
			pair.sourceCm, pair.hasSource = -cm, true
		case SplitRoleTarget:
			if i != 0 {
				up.anomaly("movement %d (%s): split target is not the unit's first movement", mv.ID, mv.Type)
			}
			if cm != 0 || mv.FromOwnerType.Valid {
				up.anomaly("movement %d (%s): split target with meters or a source owner", mv.ID, mv.Type)
			}
			initial, err := numericToCm(u.InitialMeters)
			if err != nil || !u.InitialMeters.Valid {
				up.anomaly("movement %d (%s): split target is not a roll", mv.ID, mv.Type)
			}
			pair.targetCm, pair.hasTarget = initial, true
		default:
			up.anomaly("movement %d (%s): split without split_role", mv.ID, mv.Type)
		}
	}
}

// SplitAnomalies reports splits whose source lost other meters than the new
// unit holds. A split with only one side in the replayed scope is skipped.
func (p *Projection) SplitAnomalies() []string {
	var out []string
	for _, id := range sortedKeys(p.splits, cmp.Compare[int64]) {
		pair := p.splits[id]
		if pair.hasSource && pair.hasTarget && pair.sourceCm != pair.targetCm {
			out = append(out, fmt.Sprintf("split %d: source lost %s m, new unit holds %s m",
				id, FormatMeters(pair.sourceCm), FormatMeters(pair.targetCm)))
		}
	}
	return out
}

// splitRole reads metadata.split_role of a split movement.
func splitRole(meta []byte) string {
	var m struct {
		Role string `json:"split_role"`
	}
	_ = json.Unmarshal(meta, &m)
	return m.Role
}
