package migrator

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
)

// LedgerStep opens the stock ledger from the legacy movement history and
// rebuilds the stock projections from it (TEC-258, design §7).
//
// Hub and warehouse stock_movements of the units TEC-257 migrated are
// appended to stock_movements through ledger.Import (source legacy: the
// idempotency key starts with "legacy:", the legacy timestamp is kept, the
// movement uuid is the migration_map target, so a legacy row is imported
// once). Legacy history is translated, not validated against the
// transition table:
//
//	hub imported              -> entry, center organization, available
//	hub received              -> received, center organization, available
//	hub transferred_to_dealer -> transfer_in, the item's dealer, available
//	hub used_in_service       -> consumption, used (service owner unknown until F2-01h)
//	hub external_outbound     -> external_outbound, holder's trash, used
//	wh  placement / *_in / *_restore / count_adjustment(+)
//	                          -> warehouse location (center), placed
//	wh  transfer_out / order_out -> center organization, in_transit
//	wh  void / count_adjustment(-) -> center trash, void
//	wh  external_outbound     -> center trash, used
//
// Units TEC-257 left without an owner (used in a service, warehouse
// in_transit / used / reserved / printed) get their history without owners
// so the ledger gives them no owner either (reported, not guessed); fixed
// barcode units likewise, their quantity per owner comes from the opening
// correction.
//
// When the replayed history does not end at the ownership TEC-257 wrote,
// one opening correction movement (count_adjustment, reason "legacy opening
// correction") takes the unit there; a unit without legacy history gets
// its opening the same way. Then the projections (unit_current_state,
// fixed_barcode_holdings, units.status, bin and organization product
// stocks) are rebuilt from the ledger (rebuild.ApplyTx) for every
// organization involved, and the result is checked against that
// ownership. Nothing is written to the outbox: the import triggers no
// accounting or notification.
//
// Units that already have movements this application wrote are the
// ledger's: their legacy history is skipped and reported.
type LedgerStep struct {
	System   string
	WHSystem string
}

// Name implements Step.
func (LedgerStep) Name() string { return "ledger" }

func (s LedgerStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

func (s LedgerStep) whSystem() string {
	if s.WHSystem == "" {
		return SourceWH
	}
	return s.WHSystem
}

// hubMovementsQuery keeps movements whose stock item the legacy hub deleted
// (si.id NULL): the step skips and reports them instead of losing them.
const hubMovementsQuery = `SELECT m.id, m.stock_item_id, m.user_id, m.action, m.description, m.created_at, si.dealer_id, si.id IS NOT NULL
FROM stock_movements m
LEFT JOIN stock_items si ON si.id = m.stock_item_id
ORDER BY m.id`

const whMovementsQuery = `SELECT CAST(m.id AS CHAR(36)), CAST(m.product_barcode_id AS CHAR(36)), CAST(m.warehouse_location_id AS CHAR(36)),
	m.quantity_delta, m.movement_type, m.reference_type, CAST(m.reference_id AS CHAR(36)), CAST(m.user_id AS CHAR(36)),
	m.notes, m.created_at, COALESCE(b.name, '')
FROM stock_movements m
JOIN products p ON p.id = m.product_id
LEFT JOIN brands b ON b.id = p.brand_id
ORDER BY m.created_at, m.id`

// Opening correction (TEC-258).
const (
	openingReason  = "legacy opening correction"
	openingKeyPart = "opening"
)

type hubMovement struct {
	ID, StockItemID int64
	UserID          sql.NullInt64
	Action          string
	Description     sql.NullString
	CreatedAt       sql.NullTime
	DealerID        sql.NullInt64
	UnitExists      bool
}

type whMovement struct {
	ID                   string
	BarcodeID            sql.NullString
	LocationID           sql.NullString
	Quantity             int32
	Type                 string
	RefType, RefID, User sql.NullString
	Notes                sql.NullString
	CreatedAt            sql.NullTime
	Brand                string
}

// ownerIntent says how a legacy movement changes the owner.
type ownerIntent int

const (
	intentKeep        ownerIntent = iota // owner and status stay
	intentTo                             // to the given owner
	intentTrashHolder                    // to the trash of the current holder
	intentUnknown                        // the owner is not known (status only)
)

// legacyMove is one legacy movement translated for a unit.
type legacyMove struct {
	Key    Key
	Sum    string
	Type   ledger.MovementType
	Intent ownerIntent
	To     ledger.Owner
	Status ledger.Status
	At     time.Time
	order  int // source order (hub before warehouse at the same time)
	Reason string
	Meta   map[string]any
}

// hubMove translates a hub movement; dealerOrg is the item's dealer (0:
// none or not mapped).
func hubMove(action string, dealerOrg, centerID int64) (legacyMove, bool) {
	org := func(id int64) ledger.Owner {
		return ledger.Owner{Type: ledger.OwnerOrganization, ID: id, OrgID: id}
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "imported":
		return legacyMove{Type: ledger.TypeEntry, Intent: intentTo, To: org(centerID), Status: ledger.StatusAvailable}, true
	case "received":
		return legacyMove{Type: ledger.TypeReceived, Intent: intentTo, To: org(centerID), Status: ledger.StatusAvailable}, true
	case "transferred_to_dealer":
		if dealerOrg == 0 {
			return legacyMove{Type: ledger.TypeTransferIn, Intent: intentUnknown, Status: ledger.StatusAvailable}, true
		}
		return legacyMove{Type: ledger.TypeTransferIn, Intent: intentTo, To: org(dealerOrg), Status: ledger.StatusAvailable}, true
	case "used_in_service":
		return legacyMove{Type: ledger.TypeConsumption, Intent: intentUnknown, Status: ledger.StatusUsed}, true
	case "external_outbound":
		return legacyMove{Type: ledger.TypeExternalOutbound, Intent: intentTrashHolder, Status: ledger.StatusUsed}, true
	}
	return legacyMove{Type: ledger.TypeCountAdjustment, Intent: intentKeep}, false
}

// whMove translates a warehouse movement; locationID is its mapped
// location (0: none or not mapped).
func whMove(typ string, quantity int32, locationID, centerID int64) (legacyMove, bool) {
	center := ledger.Owner{Type: ledger.OwnerOrganization, ID: centerID, OrgID: centerID}
	trash := ledger.Owner{Type: ledger.OwnerTrash, ID: centerID, OrgID: centerID}
	stock := func(t ledger.MovementType) legacyMove {
		if locationID == 0 {
			return legacyMove{Type: t, Intent: intentTo, To: center, Status: ledger.StatusAvailable}
		}
		return legacyMove{Type: t, Intent: intentTo, Status: ledger.StatusPlaced,
			To: ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: locationID, OrgID: centerID}}
	}
	switch t := ledger.MovementType(strings.ToLower(strings.TrimSpace(typ))); t {
	case ledger.TypePlacement, ledger.TypeTransferIn, ledger.TypeTransferCancelRestore, ledger.TypeOrderCancelRestore:
		return stock(t), true
	case ledger.TypeTransferOut, ledger.TypeOrderOut:
		return legacyMove{Type: t, Intent: intentTo, To: center, Status: ledger.StatusInTransit}, true
	case ledger.TypeVoid:
		return legacyMove{Type: t, Intent: intentTo, To: trash, Status: ledger.StatusVoid}, true
	case ledger.TypeExternalOutbound:
		return legacyMove{Type: t, Intent: intentTo, To: trash, Status: ledger.StatusUsed}, true
	case ledger.TypeCountAdjustment:
		switch {
		case quantity > 0:
			return stock(t), true
		case quantity < 0:
			return legacyMove{Type: t, Intent: intentTo, To: trash, Status: ledger.StatusVoid}, true
		}
		return legacyMove{Type: t, Intent: intentKeep}, true
	}
	return legacyMove{Type: ledger.TypeCountAdjustment, Intent: intentKeep}, false
}

// openingTarget is the ownership TEC-257 wrote for a unit: the serial state
// (nil: no owner) or the fixed holdings.
type openingTarget struct {
	State    *ledger.UnitState
	Holdings []ledger.FixedHolding
}

func (t openingTarget) owned(kind string) bool {
	if kind == ledger.KindFixed {
		return len(t.Holdings) > 0
	}
	return t.State != nil
}

// unitPlan is what the step writes for one unit.
type unitPlan struct {
	Records     []ledger.Recorded
	Legacy      int // the first Legacy records are legacy movements, the rest corrections
	Ownerless   bool
	Unreachable bool
}

// planOpening translates the unit's new legacy movements (sorted) on top
// of its existing movements and adds the corrections that make the ledger
// end at target. It is pure: the ledger's own Replay decides the result,
// exactly as the rebuild will.
func planOpening(u db.Unit, existing []db.StockMovement, moves []legacyMove, target openingTarget) (unitPlan, error) {
	plan := unitPlan{Ownerless: u.UnitKind == ledger.KindFixed || !target.owned(u.UnitKind)}
	sim := slices.Clone(existing)
	var lastID int64
	var lastAt time.Time
	for _, mv := range existing {
		lastID = max(lastID, mv.ID)
		if mv.CreatedAt.Time.After(lastAt) {
			lastAt = mv.CreatedAt.Time
		}
	}
	replay := func() (*ledger.UnitProjection, error) {
		return ledger.NewProjection().Replay(u, sim)
	}
	add := func(r ledger.Recorded) {
		plan.Records = append(plan.Records, r)
		lastID++
		sim = append(sim, recordedMovement(u, r, lastID))
		if r.CreatedAt.After(lastAt) {
			lastAt = r.CreatedAt
		}
	}

	for _, mv := range moves {
		up, err := replay()
		if err != nil {
			return plan, err
		}
		r := ledger.Recorded{
			UnitID: u.ID, Type: mv.Type, CreatedAt: mv.At, Reason: mv.Reason, Metadata: mv.Meta,
			Key: legacyKey(mv.Key),
		}
		cur := up.State
		var curStatus ledger.Status
		if cur != nil {
			curStatus = cur.Status
		}
		toStatus := cmp.Or(mv.Status, curStatus)
		if mv.Intent == intentKeep {
			toStatus = curStatus
		}
		r.FromStatus, r.ToStatus = curStatus, toStatus
		r.OrganizationID = u.OrganizationID
		if plan.Ownerless {
			if cur != nil {
				r.OrganizationID = cur.Owner.OrgID
			}
			add(r)
			continue
		}
		var to *ledger.Owner
		switch mv.Intent {
		case intentTo:
			o := mv.To
			to = &o
		case intentTrashHolder:
			holder := u.OrganizationID
			if cur != nil {
				holder = cur.Owner.OrgID
			}
			to = &ledger.Owner{Type: ledger.OwnerTrash, ID: holder, OrgID: holder}
		case intentKeep:
			if cur != nil {
				o := cur.Owner
				to = &o
			}
		}
		if cur != nil {
			from := cur.Owner
			r.From = &from
			r.OrganizationID = cur.Owner.OrgID
		} else if to != nil {
			r.OrganizationID = to.OrgID
		}
		r.To = to
		r.QuantityDelta = serialDelta(cur, toStatus)
		add(r)
	}
	plan.Legacy = len(plan.Records)

	corrAt := lastAt
	if corrAt.IsZero() && u.CreatedAt.Valid {
		corrAt = u.CreatedAt.Time
	}
	correction := func() ledger.Recorded {
		return ledger.Recorded{
			UnitID: u.ID, Type: ledger.TypeCountAdjustment, Reason: openingReason, CreatedAt: corrAt,
			Key:      fmt.Sprintf("%s%s:%s:%d", ledger.ImportKeyPrefix, openingKeyPart, u.Uuid, len(sim)+1),
			Metadata: map[string]any{"opening_correction": true},
		}
	}
	up, err := replay()
	if err != nil {
		return plan, err
	}
	if u.UnitKind == ledger.KindFixed {
		for _, c := range fixedCorrections(up.Holdings, target.Holdings) {
			r := correction()
			r.QuantityDelta = int32(c.Quantity)
			r.OrganizationID = c.Owner.OrgID
			o := c.Owner
			if c.Quantity > 0 {
				r.To = &o
			} else {
				r.From = &o
			}
			r.FromStatus, r.ToStatus = ledger.Status(u.Status), ledger.StatusAvailable
			add(r)
		}
		up, err = replay()
		if err != nil {
			return plan, err
		}
		plan.Unreachable = len(fixedCorrections(up.Holdings, target.Holdings)) > 0
		return plan, nil
	}

	want := target.State
	if !sameState(up.State, want) && want != nil {
		cur := up.State
		if cur != nil && want.Owner.Type == ledger.OwnerWarehouseLocation && cur.Owner.OrgID != want.Owner.OrgID {
			// The rebuild keeps the holder when a unit moves into a
			// location: first to the location's organization.
			r := correction()
			from := cur.Owner
			holder := ledger.Owner{Type: ledger.OwnerOrganization, ID: want.Owner.OrgID, OrgID: want.Owner.OrgID}
			r.From, r.To, r.OrganizationID = &from, &holder, from.OrgID
			r.FromStatus, r.ToStatus = cur.Status, ledger.StatusAvailable
			r.QuantityDelta = serialDelta(cur, ledger.StatusAvailable)
			add(r)
			if up, err = replay(); err != nil {
				return plan, err
			}
			cur = up.State
		}
		r := correction()
		to := want.Owner
		r.To, r.ToStatus, r.OrganizationID = &to, want.Status, to.OrgID
		if cur != nil {
			from := cur.Owner
			r.From, r.FromStatus, r.OrganizationID = &from, cur.Status, from.OrgID
		}
		r.QuantityDelta = serialDelta(cur, want.Status)
		add(r)
		if up, err = replay(); err != nil {
			return plan, err
		}
	}
	plan.Unreachable = !sameState(up.State, want)
	return plan, nil
}

// serialDelta is the counted stock change of a serial movement, as Post
// records it.
func serialDelta(cur *ledger.UnitState, to ledger.Status) int32 {
	var d int32
	if to == ledger.StatusAvailable || to == ledger.StatusPlaced {
		d++
	}
	if cur != nil && (cur.Status == ledger.StatusAvailable || cur.Status == ledger.StatusPlaced) {
		d--
	}
	return d
}

func sameState(a, b *ledger.UnitState) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Owner.Type == b.Owner.Type && a.Owner.ID == b.Owner.ID && a.Owner.OrgID == b.Owner.OrgID && a.Status == b.Status
}

// fixedCorrections returns the signed quantity per owner that takes the
// replayed holdings to the target ones (owner order of the target, then
// the rest).
func fixedCorrections(have, want []ledger.FixedHolding) []ledger.FixedHolding {
	type key struct {
		t  ledger.OwnerType
		id int64
	}
	got := map[key]ledger.FixedHolding{}
	for _, h := range have {
		got[key{h.Owner.Type, h.Owner.ID}] = h
	}
	var out []ledger.FixedHolding
	for _, w := range want {
		k := key{w.Owner.Type, w.Owner.ID}
		h := got[k]
		delete(got, k)
		if d := w.Quantity - h.Quantity; d != 0 {
			out = append(out, ledger.FixedHolding{Owner: w.Owner, Quantity: d})
		}
	}
	rest := make([]ledger.FixedHolding, 0, len(got))
	for _, h := range got {
		if h.Quantity != 0 {
			rest = append(rest, ledger.FixedHolding{Owner: h.Owner, Quantity: -h.Quantity})
		}
	}
	slices.SortFunc(rest, func(a, b ledger.FixedHolding) int {
		return cmp.Or(cmp.Compare(a.Owner.Type, b.Owner.Type), cmp.Compare(a.Owner.ID, b.Owner.ID))
	})
	return append(out, rest...)
}

// recordedMovement is the stock_movements row Import writes for r (for the
// in-memory replay).
func recordedMovement(u db.Unit, r ledger.Recorded, id int64) db.StockMovement {
	mv := db.StockMovement{
		ID: id, OrganizationID: r.OrganizationID, BrandID: u.BrandID, UnitID: u.ID, ProductID: u.ProductID,
		Type: string(r.Type), QuantityDelta: r.QuantityDelta, MetersDelta: ledger.CentimetersToNumeric(0),
		FromStatus: pgText(string(r.FromStatus)), ToStatus: pgText(string(r.ToStatus)),
		IdempotencyKey: r.Key,
	}
	if r.From != nil {
		mv.FromOwnerType, mv.FromOwnerID = pgText(string(r.From.Type)), pgInt8(r.From.ID)
	}
	if r.To != nil {
		mv.ToOwnerType, mv.ToOwnerID = pgText(string(r.To.Type)), pgInt8(r.To.ID)
	}
	mv.CreatedAt.Time, mv.CreatedAt.Valid = r.CreatedAt, !r.CreatedAt.IsZero()
	return mv
}

// legacyKey is the idempotency key of an imported legacy movement.
func legacyKey(k Key) string {
	return ledger.ImportKeyPrefix + k.System + ":" + k.Table + ":" + k.ID
}

// ledgerUnit collects the new legacy movements of one unit.
type ledgerUnit struct {
	uuid  uuid.UUID
	moves []legacyMove
}

// Run implements Step.
func (s LedgerStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	wh, err := src.Get(SourceWH)
	if err != nil {
		return StepResult{}, err
	}
	brand, err := dst.Q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return StepResult{}, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	center, err := dst.Q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		return StepResult{}, fmt.Errorf("olex center (run the organizations step first): %w", err)
	}
	hubRows, err := readHubMovements(ctx, hub)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	whRows, err := readWHMovements(ctx, wh)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	u := &unitCtx{
		q: dst.Q, m: m, brandID: brand.ID, centerID: center.ID,
		products: map[uuid.UUID]db.MigratorProductForUnitRow{}, dealers: map[int64]int64{}, locs: map[string]int64{},
	}

	var watermark time.Time
	byUnit := map[uuid.UUID]*ledgerUnit{}
	collect := func(unit uuid.UUID, mv legacyMove) {
		lu := byUnit[unit]
		if lu == nil {
			lu = &ledgerUnit{uuid: unit}
			byUnit[unit] = lu
		}
		lu.moves = append(lu.moves, mv)
	}

	for i, r := range hubRows {
		c.inc("hub_read")
		if t := latest(r.CreatedAt); t.After(watermark) {
			watermark = t
		}
		mv, unit, ok, err := s.hubLegacy(ctx, u, r, c)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("hub stock movement %d: %w", r.ID, err)
		}
		if ok {
			mv.order = i
			collect(unit, mv)
		}
	}
	for i, r := range whRows {
		c.inc("wh_read")
		if t := latest(r.CreatedAt); t.After(watermark) {
			watermark = t
		}
		mv, unit, ok, err := s.whLegacy(ctx, u, r, c)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("warehouse stock movement %s: %w", r.ID, err)
		}
		if ok {
			mv.order = len(hubRows) + i
			collect(unit, mv)
		}
	}

	// The units to open: those with new legacy movements and those TEC-257
	// gave an owner without any movement.
	moves := map[int64][]legacyMove{}
	var ids []int64
	for unitUUID, lu := range byUnit {
		row, err := dst.Q.MigratorUnitByUUID(ctx, db.MigratorUnitByUUIDParams{Uuid: unitUUID, BrandID: brand.ID})
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("unit %s: %w", unitUUID, err)
		}
		moves[row.ID] = lu.moves
		ids = append(ids, row.ID)
	}
	bare, err := dst.Q.MigratorUnitsWithoutMovements(ctx, brand.ID)
	if err != nil {
		return StepResult{Counts: c}, fmt.Errorf("units without movements: %w", err)
	}
	for _, id := range bare {
		if _, ok := moves[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	led := ledger.New(dst.Q, nil) // no outbox: imports trigger nothing
	orgs := map[int64]bool{}      // organizations whose projections are rebuilt
	targets := map[int64]openingTarget{}
	var opened []db.Unit
	for batch := range slices.Chunk(ids, 500) {
		units, err := dst.Q.ListUnitsByIDs(ctx, batch)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("units: %w", err)
		}
		existing, err := dst.Q.ListStockMovementsByUnitIDs(ctx, batch)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("unit movements: %w", err)
		}
		tg, err := readOpeningTargets(ctx, dst.Q, batch)
		if err != nil {
			return StepResult{Counts: c}, err
		}
		byUnitMv := map[int64][]db.StockMovement{}
		for _, mv := range existing {
			byUnitMv[mv.UnitID] = append(byUnitMv[mv.UnitID], mv)
		}
		for _, unit := range units {
			if unit.BrandID != brand.ID {
				continue
			}
			unitMoves := moves[unit.ID]
			slices.SortStableFunc(unitMoves, func(a, b legacyMove) int {
				return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.order, b.order))
			})
			own, err := dst.Q.MigratorUnitHasLedgerMovements(ctx, unit.ID)
			if err != nil {
				return StepResult{Counts: c}, fmt.Errorf("unit %d movements: %w", unit.ID, err)
			}
			if own {
				c.inc("units_skipped_ledger")
				c.add("movements_skipped_ledger", int64(len(unitMoves)))
				continue
			}
			target := tg[unit.ID]
			plan, err := planOpening(unit, byUnitMv[unit.ID], unitMoves, target)
			if err != nil {
				return StepResult{Counts: c}, fmt.Errorf("unit %s: %w", unit.Barcode, err)
			}
			if plan.Unreachable {
				c.inc("opening_unreachable:" + unit.Barcode)
			}
			if err := s.writePlan(ctx, dst, m, led, unit, unitMoves, plan, c); err != nil {
				return StepResult{Counts: c}, fmt.Errorf("unit %s: %w", unit.Barcode, err)
			}
			targets[unit.ID] = target
			opened = append(opened, unit)
			for _, r := range plan.Records {
				orgs[r.OrganizationID] = true
				for _, o := range []*ledger.Owner{r.From, r.To} {
					if o != nil && o.OrgID > 0 {
						orgs[o.OrgID] = true
					}
				}
			}
			for _, h := range target.Holdings {
				orgs[h.Owner.OrgID] = true
			}
			if target.State != nil {
				orgs[target.State.Owner.OrgID] = true
			}
		}
	}

	// Rebuild the projections from the ledger, per organization involved.
	orgIDs := make([]int64, 0, len(orgs))
	for id := range orgs {
		orgIDs = append(orgIDs, id)
	}
	slices.Sort(orgIDs)
	for _, id := range orgIDs {
		rep, err := rebuild.ApplyTx(ctx, dst.Tx, rebuild.Options{OrganizationID: id, Source: "migrator"})
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("rebuild organization %d: %w", id, err)
		}
		c.inc("rebuild_organizations")
		c.add("rebuild_diffs", int64(rep.DiffCount))
		c.add("rebuild_anomalies", int64(len(rep.Anomalies)))
	}
	if err := checkOpening(ctx, dst.Q, opened, targets, c); err != nil {
		return StepResult{Counts: c}, err
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

// writePlan maps and imports the unit's movements.
func (s LedgerStep) writePlan(ctx context.Context, dst *Target, m *Mapper, led *ledger.Ledger, unit db.Unit,
	moves []legacyMove, plan unitPlan, c counts) error {
	for i, r := range plan.Records {
		if i < plan.Legacy {
			res, err := m.Upsert(ctx, moves[i].Key, moves[i].Sum)
			if err != nil {
				return err
			}
			r.UUID = res.UUID
		}
		got, err := led.Import(ctx, dst.Tx, r)
		if err != nil {
			return fmt.Errorf("import %s: %w", r.Key, err)
		}
		switch {
		case got.Replayed:
			c.inc("movements_replayed")
		case i < plan.Legacy:
			c.inc("movements_created")
			c.inc("movements_created:" + moves[i].Key.System)
			if r.From == nil && r.To == nil {
				c.inc("movements_without_owner")
			}
		default:
			c.inc("opening_corrections")
		}
	}
	if len(plan.Records) > plan.Legacy {
		c.inc("units_corrected")
	}
	return nil
}

// readOpeningTargets reads the ownership rows of the units.
func readOpeningTargets(ctx context.Context, q *db.Queries, ids []int64) (map[int64]openingTarget, error) {
	out := map[int64]openingTarget{}
	states, err := q.ListUnitCurrentStatesByUnitIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("unit states: %w", err)
	}
	for _, st := range states {
		out[st.UnitID] = openingTarget{State: &ledger.UnitState{
			Owner:  ledger.Owner{Type: ledger.OwnerType(st.OwnerType), ID: st.OwnerID, OrgID: st.HolderOrgID},
			Status: ledger.Status(st.Status),
		}}
	}
	holdings, err := q.ListFixedBarcodeHoldingsByUnitIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("fixed holdings: %w", err)
	}
	for _, h := range holdings {
		t := out[h.UnitID]
		t.Holdings = append(t.Holdings, ledger.FixedHolding{
			Owner:    ledger.Owner{Type: ledger.OwnerType(h.OwnerType), ID: h.OwnerID, OrgID: h.HolderOrgID},
			Quantity: int64(h.QuantityOnHand),
		})
		out[h.UnitID] = t
	}
	return out, nil
}

// checkOpening compares the rebuilt ownership with the opening targets and
// reports the units without an owner by status.
func checkOpening(ctx context.Context, q *db.Queries, units []db.Unit, targets map[int64]openingTarget, c counts) error {
	for batch := range slices.Chunk(units, 500) {
		ids := make([]int64, len(batch))
		for i, u := range batch {
			ids[i] = u.ID
		}
		got, err := readOpeningTargets(ctx, q, ids)
		if err != nil {
			return err
		}
		for _, u := range batch {
			want, have := targets[u.ID], got[u.ID]
			if !want.owned(u.UnitKind) {
				c.inc("units_without_owner:" + u.Status)
			} else {
				c.inc("units_opened")
			}
			same := sameState(want.State, have.State)
			if u.UnitKind == ledger.KindFixed {
				same = len(fixedCorrections(have.Holdings, want.Holdings)) == 0
			}
			if !same {
				c.inc("ownership_mismatch:" + u.Barcode)
			}
		}
	}
	return nil
}

func (s LedgerStep) hubLegacy(ctx context.Context, u *unitCtx, r hubMovement, c counts) (legacyMove, uuid.UUID, bool, error) {
	id := strconv.FormatInt(r.ID, 10)
	key := Key{System: s.system(), Table: "stock_movements", ID: id, TargetTable: "stock_movements"}
	if _, mapped, err := u.m.Lookup(ctx, key.System, key.Table, key.ID); err != nil || mapped {
		if mapped {
			c.inc("movements_unchanged")
		}
		return legacyMove{}, uuid.Nil, false, err
	}
	if !r.UnitExists {
		c.inc("hub_skipped_unit_deleted:" + id)
		return legacyMove{}, uuid.Nil, false, nil
	}
	unit, ok, err := u.m.Lookup(ctx, s.system(), "stock_items", strconv.FormatInt(r.StockItemID, 10))
	if err != nil {
		return legacyMove{}, uuid.Nil, false, err
	}
	if !ok {
		c.inc("hub_skipped_unit_unmapped:" + id)
		return legacyMove{}, uuid.Nil, false, nil
	}
	var dealerOrg int64
	if r.DealerID.Valid {
		if dealerOrg, err = u.dealerOrg(ctx, s.system(), r.DealerID.Int64); err != nil {
			return legacyMove{}, uuid.Nil, false, err
		}
	}
	mv, known := hubMove(r.Action, dealerOrg, u.centerID)
	if !known {
		c.inc("hub_action_unknown:" + r.Action)
	}
	mv.Key = key
	mv.Sum = Checksum(r.StockItemID, r.UserID.Int64, r.Action, r.Description.String, r.CreatedAt.Time)
	mv.At = r.CreatedAt.Time
	mv.Reason = strings.TrimSpace(r.Description.String)
	mv.Meta = map[string]any{
		"legacy_system": s.system(), "legacy_table": "stock_movements", "legacy_id": r.ID,
		"legacy_action": r.Action, "legacy_stock_item_id": r.StockItemID,
	}
	if r.UserID.Valid {
		mv.Meta["legacy_user_id"] = r.UserID.Int64
	}
	return mv, unit, true, nil
}

func (s LedgerStep) whLegacy(ctx context.Context, u *unitCtx, r whMovement, c counts) (legacyMove, uuid.UUID, bool, error) {
	switch whBrand(r.Brand) {
	case "glorian":
		c.inc("wh_glorian_skipped")
		return legacyMove{}, uuid.Nil, false, nil
	case "":
		c.inc("wh_brand_unknown:" + strings.TrimSpace(r.Brand))
		return legacyMove{}, uuid.Nil, false, nil
	}
	key := Key{System: s.whSystem(), Table: "stock_movements", ID: r.ID, TargetTable: "stock_movements"}
	if _, mapped, err := u.m.Lookup(ctx, key.System, key.Table, key.ID); err != nil || mapped {
		if mapped {
			c.inc("movements_unchanged")
		}
		return legacyMove{}, uuid.Nil, false, err
	}
	if !r.BarcodeID.Valid || r.BarcodeID.String == "" {
		c.inc("wh_skipped_no_barcode")
		return legacyMove{}, uuid.Nil, false, nil
	}
	unit, ok, err := u.m.Lookup(ctx, s.whSystem(), "product_barcodes", r.BarcodeID.String)
	if err != nil {
		return legacyMove{}, uuid.Nil, false, err
	}
	if !ok {
		c.inc("wh_skipped_unit_unmapped")
		return legacyMove{}, uuid.Nil, false, nil
	}
	var locationID int64
	if r.LocationID.Valid && r.LocationID.String != "" {
		if locationID, err = u.location(ctx, s.whSystem(), r.LocationID.String); err != nil {
			return legacyMove{}, uuid.Nil, false, err
		}
	}
	mv, known := whMove(r.Type, r.Quantity, locationID, u.centerID)
	if !known {
		c.inc("wh_type_unknown:" + r.Type)
	}
	mv.Key = key
	mv.Sum = Checksum(r.BarcodeID.String, r.LocationID.String, r.Quantity, r.Type, r.RefType.String, r.RefID.String,
		r.Notes.String, r.CreatedAt.Time)
	mv.At = r.CreatedAt.Time
	mv.Reason = strings.TrimSpace(r.Notes.String)
	mv.Meta = map[string]any{
		"legacy_system": s.whSystem(), "legacy_table": "stock_movements", "legacy_id": r.ID,
		"legacy_type": r.Type, "legacy_quantity_delta": r.Quantity, "legacy_product_barcode_id": r.BarcodeID.String,
	}
	for k, v := range map[string]sql.NullString{
		"legacy_location_id": r.LocationID, "legacy_reference_type": r.RefType,
		"legacy_reference_id": r.RefID, "legacy_user_id": r.User,
	} {
		if v.Valid && v.String != "" {
			mv.Meta[k] = v.String
		}
	}
	return mv, unit, true, nil
}

func readHubMovements(ctx context.Context, hub source.LegacySource) ([]hubMovement, error) {
	rows, err := hub.Query(ctx, hubMovementsQuery)
	if err != nil {
		return nil, err
	}
	var out []hubMovement
	for rows.Next() {
		var r hubMovement
		if err := rows.Scan(&r.ID, &r.StockItemID, &r.UserID, &r.Action, &r.Description, &r.CreatedAt, &r.DealerID, &r.UnitExists); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan hub stock movement: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read hub stock movements: %w", err)
	}
	return out, nil
}

func readWHMovements(ctx context.Context, wh source.LegacySource) ([]whMovement, error) {
	rows, err := wh.Query(ctx, whMovementsQuery)
	if err != nil {
		return nil, err
	}
	var out []whMovement
	for rows.Next() {
		var r whMovement
		if err := rows.Scan(&r.ID, &r.BarcodeID, &r.LocationID, &r.Quantity, &r.Type, &r.RefType, &r.RefID, &r.User,
			&r.Notes, &r.CreatedAt, &r.Brand); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan warehouse stock movement: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read warehouse stock movements: %w", err)
	}
	return out, nil
}
