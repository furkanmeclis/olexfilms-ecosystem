package ledger

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Ledger writes stock movements. It holds no state of its own.
type Ledger struct {
	q   *db.Queries
	out outbox.Enqueuer
}

// New returns a Ledger over q, writing stock.* events to out.
func New(q *db.Queries, out outbox.Enqueuer) *Ledger {
	return &Ledger{q: q, out: out}
}

// Result is the outcome of Post.
type Result struct {
	Movement db.StockMovement
	// Replayed is true when the idempotency key already existed: nothing
	// was written and Movement is the earlier row.
	Replayed bool
}

// Post validates and appends one movement and updates every projection in
// tx. On error the caller rolls tx back (a failed statement aborts it).
func (l *Ledger) Post(ctx context.Context, tx pgx.Tx, m Movement) (Result, error) {
	if tx == nil {
		return Result{}, errors.New("ledger: transaction required")
	}
	if !slices.Contains(PostableTypes, m.Type) {
		return Result{}, fmt.Errorf("%w: type %q", ErrInvalidMovement, m.Type)
	}
	if m.UnitID <= 0 {
		return Result{}, fmt.Errorf("%w: unit id", ErrInvalidMovement)
	}
	q := l.q.WithTx(tx)

	// The unit row lock serialises every movement of the unit, including
	// the first one (no state row to lock yet) and fixed barcodes.
	unit, err := q.LockUnit(ctx, m.UnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrUnitNotFound
	}
	if err != nil {
		return Result{}, fmt.Errorf("ledger: lock unit: %w", err)
	}
	key, err := IdempotencyKey(m.Source, m.RefType, m.RefID, m.Type, unit.Barcode)
	if err != nil {
		return Result{}, err
	}
	if res, done, err := replay(ctx, q, key, unit, m); done || err != nil {
		return res, err
	}

	p := &post{l: l, q: q, tx: tx, m: m, unit: unit, key: key}
	if unit.UnitKind == KindFixed {
		return p.fixed(ctx)
	}
	return p.serial(ctx)
}

// replay returns the earlier movement written under key.
func replay(ctx context.Context, q *db.Queries, key string, unit db.Unit, m Movement) (Result, bool, error) {
	prev, err := q.GetStockMovementByIdempotencyKey(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, true, fmt.Errorf("ledger: idempotency lookup: %w", err)
	}
	if prev.UnitID != unit.ID || prev.Type != string(m.Type) {
		return Result{}, true, fmt.Errorf("%w: %s", ErrIdempotencyConflict, key)
	}
	return Result{Movement: prev, Replayed: true}, true, nil
}

type post struct {
	l    *Ledger
	q    *db.Queries
	tx   pgx.Tx
	m    Movement
	unit db.Unit
	key  string
}

func (p *post) serial(ctx context.Context) (Result, error) {
	cur := serialCurrent{status: Status(p.unit.Status), issuerOrg: p.unit.OrganizationID}
	if p.unit.InitialMeters.Valid {
		cur.isRoll = true
		var err error
		if cur.initial, err = numericToCm(p.unit.InitialMeters); err != nil {
			return Result{}, err
		}
		if cur.remaining, err = numericToCm(p.unit.RemainingMeters); err != nil {
			return Result{}, err
		}
	}
	state, err := p.q.LockUnitCurrentState(ctx, p.unit.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Result{}, fmt.Errorf("ledger: lock state: %w", err)
	default:
		cur.hasState = true
		cur.status = Status(state.Status)
		cur.owner = Owner{Type: OwnerType(state.OwnerType), ID: state.OwnerID, OrgID: state.HolderOrgID}
	}
	var prev *prevMovement
	if cur.hasState && state.LastMovementID.Valid {
		pm, err := p.q.GetStockMovement(ctx, state.LastMovementID.Int64)
		if err != nil {
			return Result{}, fmt.Errorf("ledger: last movement: %w", err)
		}
		prev = toPrev(pm)
		if prev.fromOwner != nil {
			// A movement is recorded on the holder before it, so the
			// source owner's holder is the movement's organization.
			prev.fromOwner.OrgID = pm.OrganizationID
		}
	}

	plan, err := planSerial(p.m, cur, prev)
	if err != nil {
		return Result{}, err
	}
	if plan.checkTarget && plan.ownerChanged {
		if err := p.checkOwner(ctx, plan.toOwner, plan.holder); err != nil {
			return Result{}, err
		}
	}

	arg, err := p.baseArgs()
	if err != nil {
		return Result{}, err
	}
	arg.QuantityDelta = plan.quantityDelta
	arg.MetersDelta = cmToNumeric(plan.metersDelta)
	arg.FromStatus = text(string(cur.status))
	arg.ToStatus = text(string(plan.toStatus))
	if cur.hasState {
		arg.FromOwnerType, arg.FromOwnerID = text(string(cur.owner.Type)), i8(cur.owner.ID)
	}
	arg.ToOwnerType, arg.ToOwnerID = text(string(plan.toOwner.Type)), i8(plan.toOwner.ID)
	arg.OrganizationID = cmp.Or(cur.owner.OrgID, plan.toOwner.OrgID)

	mv, replayed, err := p.insert(ctx, arg)
	if err != nil || replayed {
		return Result{Movement: mv, Replayed: replayed}, err
	}

	// unit_current_state: one owner per unit (PK).
	if cur.hasState {
		_, err = p.q.UpdateUnitCurrentState(ctx, db.UpdateUnitCurrentStateParams{
			UnitID: p.unit.ID, Version: state.Version,
			OwnerType: string(plan.toOwner.Type), OwnerID: plan.toOwner.ID,
			HolderOrgID: plan.toOwner.OrgID, Status: string(plan.toStatus),
			LastMovementID: i8(mv.ID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, ErrConcurrentUpdate
		}
	} else {
		_, err = p.q.InsertUnitCurrentState(ctx, db.InsertUnitCurrentStateParams{
			UnitID: p.unit.ID, BrandID: p.unit.BrandID,
			OwnerType: string(plan.toOwner.Type), OwnerID: plan.toOwner.ID,
			HolderOrgID: plan.toOwner.OrgID, Status: string(plan.toStatus),
			LastMovementID: i8(mv.ID),
		})
	}
	if err != nil {
		return Result{}, dbErr("unit state", err)
	}

	// units: status mirrors the state; remaining meters for rolls.
	after := cur.remaining + plan.metersDelta
	if plan.metersDelta != 0 {
		if _, err := p.q.UpdateUnitRemainingMeters(ctx, db.UpdateUnitRemainingMetersParams{
			ID: p.unit.ID, RemainingMeters: cmToNumeric(after),
		}); err != nil {
			return Result{}, dbErr("unit meters", err)
		}
	}
	if Status(p.unit.Status) != plan.toStatus {
		if _, err := p.q.UpdateUnitStatus(ctx, db.UpdateUnitStatusParams{
			ID: p.unit.ID, Status: string(plan.toStatus),
		}); err != nil {
			return Result{}, dbErr("unit status", err)
		}
	}

	// Product stock projections: remove the unit's old contribution and add
	// the new one.
	var d deltas
	if cur.hasState && counted(cur.status) {
		d.add(cur.owner, -1, -cur.remaining)
	}
	if counted(plan.toStatus) {
		d.add(plan.toOwner, 1, after)
	}
	if err := p.applyProjections(ctx, d); err != nil {
		return Result{}, err
	}
	return p.publish(ctx, mv, plan.toOwner.OrgID)
}

func (p *post) fixed(ctx context.Context) (Result, error) {
	plan, err := planFixed(p.m, Status(p.unit.Status))
	if err != nil {
		return Result{}, err
	}
	if plan.pairedOut != "" {
		if err := p.checkPairedFixed(ctx, plan); err != nil {
			return Result{}, err
		}
	}
	if plan.credit {
		if !plan.restore {
			if err := p.checkOwner(ctx, plan.owner, plan.holder); err != nil {
				return Result{}, err
			}
		}
	} else {
		h, err := p.q.LockFixedBarcodeHolding(ctx, db.LockFixedBarcodeHoldingParams{
			UnitID: p.unit.ID, OwnerType: string(plan.owner.Type), OwnerID: plan.owner.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && h.QuantityOnHand+plan.delta < 0) {
			return Result{}, fmt.Errorf("%w: %s %d holds %d, %d requested",
				ErrInsufficientStock, plan.owner.Type, plan.owner.ID, h.QuantityOnHand, -plan.delta)
		}
		if err != nil {
			return Result{}, fmt.Errorf("ledger: lock holding: %w", err)
		}
		plan.owner.OrgID = h.HolderOrgID
	}

	toStatus := cmp.Or(plan.setStatus, Status(p.unit.Status))
	arg, err := p.baseArgs()
	if err != nil {
		return Result{}, err
	}
	arg.QuantityDelta = plan.delta
	arg.MetersDelta = cmToNumeric(0)
	arg.FromStatus = text(p.unit.Status)
	arg.ToStatus = text(string(toStatus))
	if plan.credit {
		arg.ToOwnerType, arg.ToOwnerID = text(string(plan.owner.Type)), i8(plan.owner.ID)
	} else {
		arg.FromOwnerType, arg.FromOwnerID = text(string(plan.owner.Type)), i8(plan.owner.ID)
	}
	arg.OrganizationID = plan.owner.OrgID

	mv, replayed, err := p.insert(ctx, arg)
	if err != nil || replayed {
		return Result{Movement: mv, Replayed: replayed}, err
	}
	if err := p.q.EnsureFixedBarcodeHolding(ctx, db.EnsureFixedBarcodeHoldingParams{
		UnitID: p.unit.ID, BrandID: p.unit.BrandID,
		OwnerType: string(plan.owner.Type), OwnerID: plan.owner.ID, HolderOrgID: plan.owner.OrgID,
	}); err != nil {
		return Result{}, dbErr("ensure holding", err)
	}
	if _, err := p.q.AddFixedBarcodeHolding(ctx, db.AddFixedBarcodeHoldingParams{
		UnitID: p.unit.ID, OwnerType: string(plan.owner.Type), OwnerID: plan.owner.ID,
		QuantityDelta: plan.delta, LastMovementID: i8(mv.ID),
	}); err != nil {
		return Result{}, dbErr("holding", err)
	}
	if plan.setStatus != "" {
		if _, err := p.q.UpdateUnitStatus(ctx, db.UpdateUnitStatusParams{
			ID: p.unit.ID, Status: string(plan.setStatus),
		}); err != nil {
			return Result{}, dbErr("unit status", err)
		}
	}
	var d deltas
	d.add(plan.owner, plan.delta, 0)
	if err := p.applyProjections(ctx, d); err != nil {
		return Result{}, err
	}
	return p.publish(ctx, mv, plan.owner.OrgID)
}

// checkPairedFixed requires an *_out of the same reference for the unit
// with enough quantity left to bring in or restore; a restore goes back to
// that out's source owner.
func (p *post) checkPairedFixed(ctx context.Context, plan fixedPlan) error {
	rows, err := p.q.ListStockMovementsByReference(ctx, db.ListStockMovementsByReferenceParams{
		ReferenceType: text(p.m.RefType), ReferenceID: i8(p.m.RefID),
	})
	if err != nil {
		return fmt.Errorf("ledger: reference movements: %w", err)
	}
	var out, in int64
	source := false
	for _, r := range rows {
		if r.UnitID != p.unit.ID {
			continue
		}
		switch MovementType(r.Type) {
		case plan.pairedOut:
			out += int64(-r.QuantityDelta)
			if r.FromOwnerType.String == string(plan.owner.Type) && r.FromOwnerID.Int64 == plan.owner.ID {
				source = true
			}
		case TypeTransferIn, TypeTransferCancelRestore, TypeReceived, TypeOrderCancelRestore:
			if fixedRules[MovementType(r.Type)].pairedOut == plan.pairedOut {
				in += int64(r.QuantityDelta)
			}
		}
	}
	if in+int64(plan.delta) > out {
		return fmt.Errorf("%w: reference %s:%d sent %d, already back/in %d",
			ErrTransitionNotAllowed, p.m.RefType, p.m.RefID, out, in)
	}
	if plan.restore && !source {
		return fmt.Errorf("%w: restore goes back to the source owner", ErrOwnerMismatch)
	}
	return nil
}

// checkOwner validates a target owner: the location exists, is active and
// belongs to the holder; the holder is a live organization allowed to hold
// the unit's brand (its own brand, or any center: the warehouse is
// brand-independent, K20); entries go to a center (K14).
func (p *post) checkOwner(ctx context.Context, o Owner, rule holderRule) error {
	org, err := p.q.GetOrganizationByID(ctx, o.OrgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: organization %d", ErrOwnerNotAllowed, o.OrgID)
	}
	if err != nil {
		return fmt.Errorf("ledger: organization: %w", err)
	}
	if org.DeletedAt.Valid {
		return fmt.Errorf("%w: organization %d is deleted", ErrOwnerNotAllowed, o.OrgID)
	}
	if rule == holderCenter && org.Type != "center" {
		return fmt.Errorf("%w: stock enters at a center (K14)", ErrOwnerNotAllowed)
	}
	if org.Type != "center" && org.BrandID != p.unit.BrandID {
		return fmt.Errorf("%w: organization %d is not of the unit's brand", ErrOwnerNotAllowed, o.OrgID)
	}
	if o.Type == OwnerWarehouseLocation {
		loc, err := p.q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: o.ID, OrganizationID: o.OrgID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: location %d of organization %d", ErrOwnerNotAllowed, o.ID, o.OrgID)
		}
		if err != nil {
			return fmt.Errorf("ledger: location: %w", err)
		}
		if !loc.Active {
			return fmt.Errorf("%w: location %d is inactive", ErrOwnerNotAllowed, o.ID)
		}
	}
	return nil
}

func (p *post) baseArgs() (db.InsertStockMovementParams, error) {
	meta := p.m.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return db.InsertStockMovementParams{}, fmt.Errorf("%w: metadata: %v", ErrInvalidMovement, err)
	}
	arg := db.InsertStockMovementParams{
		BrandID: p.unit.BrandID, UnitID: p.unit.ID, ProductID: p.unit.ProductID,
		Type: string(p.m.Type), ReferenceType: text(p.m.RefType), ReferenceID: i8(p.m.RefID),
		Reason: text(p.m.Reason), Metadata: body, IdempotencyKey: p.key,
	}
	if p.m.ActorUserID != nil {
		arg.ActorUserID = i8(*p.m.ActorUserID)
	}
	return arg, nil
}

// insert appends the movement; a concurrent writer of the same key wins and
// its row is returned as a replay.
func (p *post) insert(ctx context.Context, arg db.InsertStockMovementParams) (db.StockMovement, bool, error) {
	mv, err := p.q.InsertStockMovement(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		res, _, err := replay(ctx, p.q, p.key, p.unit, p.m)
		return res.Movement, true, err
	}
	if err != nil {
		return db.StockMovement{}, false, dbErr("movement", err)
	}
	return mv, false, nil
}

// deltas accumulates projection changes per organization and location.
type deltas struct {
	org map[int64]stockDelta
	bin map[[2]int64]stockDelta // location, organization
}

type stockDelta struct {
	qty int32
	cm  int64
}

func (d *deltas) add(o Owner, qty int32, cm int64) {
	if o.Type != OwnerWarehouseLocation && o.Type != OwnerOrganization {
		return
	}
	if d.org == nil {
		d.org, d.bin = map[int64]stockDelta{}, map[[2]int64]stockDelta{}
	}
	od := d.org[o.OrgID]
	d.org[o.OrgID] = stockDelta{od.qty + qty, od.cm + cm}
	if o.Type == OwnerWarehouseLocation {
		k := [2]int64{o.ID, o.OrgID}
		bd := d.bin[k]
		d.bin[k] = stockDelta{bd.qty + qty, bd.cm + cm}
	}
}

// applyProjections applies the deltas in key order (a fixed lock order
// across concurrent posts), Ensure then Add; the CHECK (>= 0) rejects
// negative stock.
func (p *post) applyProjections(ctx context.Context, d deltas) error {
	return p.applyProjectionsFor(ctx, d, p.unit.ProductID)
}

// applyProjectionsFor applies the deltas to the rows of product (a
// reclassification touches the old and the new product).
func (p *post) applyProjectionsFor(ctx context.Context, d deltas, product int64) error {
	brand := p.unit.BrandID
	for _, k := range sortedKeys(d.bin, func(a, b [2]int64) int { return cmp.Compare(a[0], b[0]) }) {
		v := d.bin[k]
		if v.qty == 0 && v.cm == 0 {
			continue
		}
		if err := p.q.EnsureBinProductStock(ctx, db.EnsureBinProductStockParams{
			LocationID: k[0], OrganizationID: k[1], BrandID: brand, ProductID: product,
		}); err != nil {
			return dbErr("ensure bin stock", err)
		}
		if _, err := p.q.AddBinProductStock(ctx, db.AddBinProductStockParams{
			LocationID: k[0], ProductID: product, QuantityDelta: v.qty, MetersDelta: cmToNumeric(v.cm),
		}); err != nil {
			return dbErr("bin stock", err)
		}
	}
	for _, k := range sortedKeys(d.org, cmp.Compare[int64]) {
		v := d.org[k]
		if v.qty == 0 && v.cm == 0 {
			continue
		}
		if err := p.q.EnsureOrganizationProductStock(ctx, db.EnsureOrganizationProductStockParams{
			OrganizationID: k, ProductID: product, BrandID: brand,
		}); err != nil {
			return dbErr("ensure organization stock", err)
		}
		if _, err := p.q.AddOrganizationProductStock(ctx, db.AddOrganizationProductStockParams{
			OrganizationID: k, ProductID: product, QuantityDelta: v.qty, MetersDelta: cmToNumeric(v.cm),
		}); err != nil {
			return dbErr("organization stock", err)
		}
	}
	return nil
}

func sortedKeys[K comparable, V any](m map[K]V, less func(a, b K) int) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, less)
	return keys
}

// publish writes the stock.* event in the movement's transaction.
func (p *post) publish(ctx context.Context, mv db.StockMovement, holder int64) (Result, error) {
	if p.l.out != nil {
		id, uid := mv.ID, mv.Uuid
		payload := map[string]any{
			"movement_id":     mv.ID,
			"unit_id":         p.unit.ID,
			"unit_uuid":       p.unit.Uuid.String(),
			"barcode":         p.unit.Barcode,
			"brand_id":        mv.BrandID,
			"product_id":      mv.ProductID,
			"type":            mv.Type,
			"quantity_delta":  mv.QuantityDelta,
			"meters_delta":    FormatMeters(mustCm(mv.MetersDelta)),
			"from_owner_type": mv.FromOwnerType.String,
			"from_owner_id":   mv.FromOwnerID.Int64,
			"to_owner_type":   mv.ToOwnerType.String,
			"to_owner_id":     mv.ToOwnerID.Int64,
			"from_status":     mv.FromStatus.String,
			"to_status":       mv.ToStatus.String,
			"holder_org_id":   holder,
			"reference_type":  mv.ReferenceType.String,
			"reference_id":    mv.ReferenceID.Int64,
			"idempotency_key": mv.IdempotencyKey,
		}
		if MovementType(mv.Type) == TypeReclassification {
			payload["from_product_id"] = p.m.Metadata["from_product_id"]
			payload["to_product_id"] = mv.ProductID
		}
		if MovementType(mv.Type) == TypeSplit {
			for _, k := range []string{"split_role", "split_uuid", "counterpart_unit_id", "counterpart_barcode"} {
				payload[k] = p.m.Metadata[k]
			}
		}
		ev := events.New(EventName(MovementType(mv.Type))).
			WithTenant(mv.OrganizationID).
			WithEntity("stock_movement", &id, &uid).
			WithPayload(payload)
		if p.m.ActorUserID != nil {
			ev = ev.WithActor(*p.m.ActorUserID)
		}
		if err := p.l.out.Enqueue(ctx, p.tx, ev); err != nil {
			return Result{}, fmt.Errorf("ledger: outbox: %w", err)
		}
	}
	return Result{Movement: mv}, nil
}

var eventNames = map[MovementType]string{
	TypeEntry:                 events.StockEntry,
	TypePlacement:             events.StockPlacement,
	TypeTransferOut:           events.StockTransferOut,
	TypeTransferIn:            events.StockTransferIn,
	TypeTransferCancelRestore: events.StockTransferCancelRestore,
	TypeOrderOut:              events.StockOrderOut,
	TypeReceived:              events.StockReceived,
	TypeOrderCancelRestore:    events.StockOrderCancelRestore,
	TypeConsumption:           events.StockConsumption,
	TypePartialConsumption:    events.StockPartialConsumption,
	TypeReturn:                events.StockReturn,
	TypeSale:                  events.StockSale,
	TypeReclassification:      events.StockReclassification,
	TypeCountAdjustment:       events.StockCountAdjustment,
	TypeVoid:                  events.StockVoid,
	TypeExternalOutbound:      events.StockExternalOutbound,
	TypeSplit:                 events.StockSplit,
}

// EventName is the outbox event of a movement type.
func EventName(t MovementType) string { return eventNames[t] }

func toPrev(pm db.StockMovement) *prevMovement {
	prev := &prevMovement{
		typ: MovementType(pm.Type), refType: pm.ReferenceType.String, refID: pm.ReferenceID.Int64,
		fromStatus: Status(pm.FromStatus.String),
	}
	if pm.FromOwnerType.Valid {
		o := normalized(Owner{Type: OwnerType(pm.FromOwnerType.String), ID: pm.FromOwnerID.Int64})
		prev.fromOwner = &o
	}
	return prev
}

// dbErr maps CHECK violations (negative stock or meters) to
// ErrInsufficientStock.
func dbErr(what string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return fmt.Errorf("%w: %s: %s", ErrInsufficientStock, what, pgErr.ConstraintName)
	}
	return fmt.Errorf("ledger: %s: %w", what, err)
}

func mustCm(n pgtype.Numeric) int64 {
	v, _ := numericToCm(n)
	return v
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func i8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
