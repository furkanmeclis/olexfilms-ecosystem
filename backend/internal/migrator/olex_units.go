package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// UnitsStep imports the stock units of both legacy systems into one units
// table and writes their initial ownership (TEC-257, design §7).
//
// Hub stock_items and warehouse product_barcodes are merged by barcode: a
// barcode present in both is one unit and both legacy rows map to it. When
// the two disagree, the more recently updated row decides the product,
// status and owner (a tie goes to the warehouse) and the conflict is
// counted.
//
// Ownership (no ledger here; F2-01g opens it and rebuilds the projections):
//
//	hub   center / available           -> center organization, available
//	hub   dealer / available|reserved  -> dealer organization, available
//	hub   trash  / used                -> trash of the dealer (else center), used
//	hub   service / used               -> no owner yet: the service arrives with F2-01h
//	hub   dealer|center / used         -> same as service / used (the legacy hub
//	                                      leaves a consumed unit where it was)
//	wh    placed + location            -> that warehouse location (center), placed
//	wh    reserved | printed           -> not in stock yet: no owner (like a new label)
//	wh    void                         -> center trash, void
//	wh    in_transit | used            -> no owner yet (reported)
//
// Serial units get a unit_current_state row and fixed barcode units a
// fixed_barcode_holdings row, both without a movement. A unit that already
// has movements is the ledger's: a rerun never rewrites it. Glorian units
// (warehouse products of the Glorian brand) are skipped and counted (K2).
// Finally the legacy bin_product_stocks of mapped locations and products are
// written, unless the location already has ledger movements; a bin whose
// legacy quantity differs from the units placed there is reported.
//
// The step always reads every legacy row: merging needs both sides of a
// barcode. Reruns are idempotent through migration_map checksums.
type UnitsStep struct {
	// System and WHSystem are the migration_map source systems of the hub
	// and warehouse rows; empty means SourceHub / SourceWH.
	System   string
	WHSystem string
}

// Name implements Step.
func (UnitsStep) Name() string { return "units" }

func (s UnitsStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

func (s UnitsStep) whSystem() string {
	if s.WHSystem == "" {
		return SourceWH
	}
	return s.WHSystem
}

const hubStockItemsQuery = `SELECT id, product_id, dealer_id, barcode, location, status, created_at, updated_at
FROM stock_items ORDER BY id`

const whBarcodesQuery = `SELECT CAST(pb.id AS CHAR(36)), CAST(pb.product_id AS CHAR(36)), COALESCE(b.name, ''), pb.code,
	pb.status, pb.quantity_on_hand, CAST(pb.warehouse_location_id AS CHAR(36)), pb.deleted_at, pb.created_at, pb.updated_at
FROM product_barcodes pb
JOIN products p ON p.id = pb.product_id
LEFT JOIN brands b ON b.id = p.brand_id
ORDER BY pb.created_at, pb.id`

const whBinStocksQuery = `SELECT CAST(s.id AS CHAR(36)), CAST(s.warehouse_location_id AS CHAR(36)), CAST(s.product_id AS CHAR(36)),
	COALESCE(b.name, ''), s.quantity_on_hand, s.created_at, s.updated_at
FROM bin_product_stocks s
JOIN products p ON p.id = s.product_id
LEFT JOIN brands b ON b.id = p.brand_id
ORDER BY s.warehouse_location_id, s.product_id`

// unitBarcodeMax is units.barcode VARCHAR(64).
const unitBarcodeMax = 64

// Owner types and unit statuses of the stock ledger (000046).
const (
	ownerLocation = "warehouse_location"
	ownerOrg      = "organization"
	ownerTrash    = "trash"

	unitAvailable = "available"
	unitReserved  = "reserved"
	unitPrinted   = "printed"
	unitPlaced    = "placed"
	unitInTransit = "in_transit"
	unitUsed      = "used"
	unitVoid      = "void"
)

type hubStockItem struct {
	ID, ProductID        int64
	DealerID             sql.NullInt64
	Barcode              string
	Location, Status     string
	CreatedAt, UpdatedAt sql.NullTime
}

type whBarcode struct {
	ID, ProductID, Brand, Code, Status string
	Quantity                           int32
	LocationID                         sql.NullString
	DeletedAt, CreatedAt, UpdatedAt    sql.NullTime
}

type whBinStock struct {
	ID, LocationID, ProductID, Brand string
	Quantity                         int32
	CreatedAt, UpdatedAt             sql.NullTime
}

// ownership is the initial owner and status of a unit. OwnerType "" means
// no owner row: Pending names why (reported), empty Pending means the unit
// is not in stock yet (reserved / printed label).
type ownership struct {
	Status    string
	OwnerType string
	OwnerID   int64
	Holder    int64
	Quantity  int32 // fixed barcode units
	Pending   string
}

// unitSide is one legacy row of a barcode, resolved against this database.
type unitSide struct {
	Key       Key
	Sum       string
	Product   uuid.UUID
	Own       ownership
	CreatedAt sql.NullTime
	UpdatedAt time.Time
}

type unitCandidate struct {
	Barcode string
	Hub, WH *unitSide
}

// unitCtx is what the unit import needs from this database.
type unitCtx struct {
	q        *db.Queries
	m        *Mapper
	brandID  int64
	centerID int64
	products map[uuid.UUID]db.MigratorProductForUnitRow
	dealers  map[int64]int64
	locs     map[string]int64
}

// Run implements Step.
func (s UnitsStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
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
	hubItems, err := readHubStockItems(ctx, hub)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	whRows, err := readWHBarcodes(ctx, wh)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	bins, err := readWHBinStocks(ctx, wh)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	u := &unitCtx{
		q: dst.Q, m: m, brandID: brand.ID, centerID: center.ID,
		products: map[uuid.UUID]db.MigratorProductForUnitRow{}, dealers: map[int64]int64{}, locs: map[string]int64{},
	}
	var watermark time.Time
	seen := func(ts ...sql.NullTime) {
		if t := latest(ts...); t.After(watermark) {
			watermark = t
		}
	}

	byCode := map[string]*unitCandidate{}
	var order []*unitCandidate
	candidate := func(code string) *unitCandidate {
		if cand, ok := byCode[code]; ok {
			return cand
		}
		cand := &unitCandidate{Barcode: code}
		byCode[code] = cand
		order = append(order, cand)
		return cand
	}

	for _, it := range hubItems {
		c.inc("hub_read")
		seen(it.CreatedAt, it.UpdatedAt)
		side, code, err := s.hubSide(ctx, u, it, c)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("stock item %d: %w", it.ID, err)
		}
		if side != nil {
			candidate(code).Hub = side
		}
	}
	for _, b := range whRows {
		c.inc("wh_read")
		seen(b.CreatedAt, b.UpdatedAt, b.DeletedAt)
		side, code, err := s.whSide(ctx, u, b, c)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("product barcode %s: %w", b.ID, err)
		}
		if side != nil {
			candidate(code).WH = side
		}
	}

	for _, cand := range order {
		if cand.Hub != nil && cand.WH != nil {
			c.inc("barcodes_merged")
		}
		if err := s.importUnit(ctx, u, cand, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("barcode %s: %w", cand.Barcode, err)
		}
	}

	for _, b := range bins {
		c.inc("bin_stocks_read")
		seen(b.CreatedAt, b.UpdatedAt)
		if err := s.importBinStock(ctx, u, b, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("bin product stock %s: %w", b.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func readHubStockItems(ctx context.Context, hub source.LegacySource) ([]hubStockItem, error) {
	rows, err := hub.Query(ctx, hubStockItemsQuery)
	if err != nil {
		return nil, err
	}
	var out []hubStockItem
	for rows.Next() {
		var it hubStockItem
		if err := rows.Scan(&it.ID, &it.ProductID, &it.DealerID, &it.Barcode, &it.Location, &it.Status,
			&it.CreatedAt, &it.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan stock item: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read stock items: %w", err)
	}
	return out, nil
}

func readWHBarcodes(ctx context.Context, wh source.LegacySource) ([]whBarcode, error) {
	rows, err := wh.Query(ctx, whBarcodesQuery)
	if err != nil {
		return nil, err
	}
	var out []whBarcode
	for rows.Next() {
		var b whBarcode
		if err := rows.Scan(&b.ID, &b.ProductID, &b.Brand, &b.Code, &b.Status, &b.Quantity, &b.LocationID,
			&b.DeletedAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan product barcode: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read product barcodes: %w", err)
	}
	return out, nil
}

func readWHBinStocks(ctx context.Context, wh source.LegacySource) ([]whBinStock, error) {
	rows, err := wh.Query(ctx, whBinStocksQuery)
	if err != nil {
		return nil, err
	}
	var out []whBinStock
	for rows.Next() {
		var b whBinStock
		if err := rows.Scan(&b.ID, &b.LocationID, &b.ProductID, &b.Brand, &b.Quantity, &b.CreatedAt, &b.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan bin product stock: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read bin product stocks: %w", err)
	}
	return out, nil
}

// unitBarcode trims a legacy barcode; "" when it is empty or too long for
// units.barcode (a cut barcode could collide: report, do not guess).
func unitBarcode(raw string) string {
	code := strings.TrimSpace(raw)
	if code == "" || len([]rune(code)) > unitBarcodeMax {
		return ""
	}
	return code
}

// hubOwnership maps a hub stock item's location and status (see UnitsStep).
// ok false means the pair is unknown.
func hubOwnership(location, status string, dealerOrg, centerID int64) (ownership, bool) {
	loc := strings.ToLower(strings.TrimSpace(location))
	st := strings.ToLower(strings.TrimSpace(status))
	switch {
	case loc == "center" && (st == "available" || st == "reserved"):
		return ownership{Status: unitAvailable, OwnerType: ownerOrg, OwnerID: centerID, Holder: centerID, Quantity: 1}, true
	case loc == "dealer" && (st == "available" || st == "reserved"):
		if dealerOrg == 0 {
			return ownership{Status: unitAvailable, Pending: "dealer_unmapped"}, true
		}
		return ownership{Status: unitAvailable, OwnerType: ownerOrg, OwnerID: dealerOrg, Holder: dealerOrg, Quantity: 1}, true
	case loc == "trash" && st == "used":
		org := centerID
		if dealerOrg != 0 {
			org = dealerOrg
		}
		return ownership{Status: unitUsed, OwnerType: ownerTrash, OwnerID: org, Holder: org, Quantity: 1}, true
	case (loc == "service" || loc == "dealer" || loc == "center") && st == "used":
		return ownership{Status: unitUsed, Pending: "service"}, true
	}
	return ownership{}, false
}

// whOwnership maps a warehouse barcode's status and location (see
// UnitsStep). ok false means the status is unknown.
func whOwnership(status string, locationID, centerID int64, quantity int32) (ownership, bool) {
	switch st := strings.ToLower(strings.TrimSpace(status)); st {
	case unitReserved, unitPrinted:
		return ownership{Status: st}, true
	case unitPlaced:
		if locationID == 0 {
			return ownership{Status: unitAvailable, OwnerType: ownerOrg, OwnerID: centerID, Holder: centerID,
				Quantity: quantity, Pending: "placed_without_location"}, true
		}
		return ownership{Status: unitPlaced, OwnerType: ownerLocation, OwnerID: locationID, Holder: centerID, Quantity: quantity}, true
	case unitVoid:
		return ownership{Status: unitVoid, OwnerType: ownerTrash, OwnerID: centerID, Holder: centerID, Quantity: quantity}, true
	case unitInTransit, unitUsed:
		return ownership{Status: st, Pending: "wh_" + st}, true
	}
	return ownership{}, false
}

func (s UnitsStep) hubSide(ctx context.Context, u *unitCtx, it hubStockItem, c counts) (*unitSide, string, error) {
	id := strconv.FormatInt(it.ID, 10)
	code := unitBarcode(it.Barcode)
	if code == "" {
		c.inc("hub_skipped_barcode:" + id)
		return nil, "", nil
	}
	product, ok, err := u.m.Lookup(ctx, s.system(), "products", strconv.FormatInt(it.ProductID, 10))
	if err != nil {
		return nil, "", err
	}
	if !ok {
		c.inc("hub_skipped_product_unmapped:" + id)
		return nil, "", nil
	}
	var dealerOrg int64
	if it.DealerID.Valid {
		if dealerOrg, err = u.dealerOrg(ctx, s.system(), it.DealerID.Int64); err != nil {
			return nil, "", err
		}
	}
	own, ok := hubOwnership(it.Location, it.Status, dealerOrg, u.centerID)
	if !ok {
		c.inc("hub_state_unknown:" + it.Location + "/" + it.Status)
		c.inc("hub_skipped_state:" + id)
		return nil, "", nil
	}
	if strings.EqualFold(strings.TrimSpace(it.Status), "reserved") {
		c.inc("hub_reserved_as_available")
	}
	return &unitSide{
		Key:       Key{System: s.system(), Table: "stock_items", ID: id, TargetTable: "units"},
		Sum:       Checksum(it.ProductID, it.DealerID.Int64, it.DealerID.Valid, it.Barcode, it.Location, it.Status),
		Product:   product,
		Own:       own,
		CreatedAt: it.CreatedAt,
		UpdatedAt: latest(it.CreatedAt, it.UpdatedAt),
	}, code, nil
}

func (s UnitsStep) whSide(ctx context.Context, u *unitCtx, b whBarcode, c counts) (*unitSide, string, error) {
	switch whBrand(b.Brand) {
	case "glorian":
		c.inc("wh_glorian_skipped")
		return nil, "", nil
	case "":
		c.inc("wh_brand_unknown:" + strings.TrimSpace(b.Brand))
		c.inc("wh_skipped_brand_unknown:" + b.ID)
		return nil, "", nil
	}
	if b.DeletedAt.Valid {
		c.inc("wh_deleted_skipped")
		return nil, "", nil
	}
	code := unitBarcode(b.Code)
	if code == "" {
		c.inc("wh_skipped_barcode:" + b.ID)
		return nil, "", nil
	}
	product, ok, err := u.m.Lookup(ctx, s.whSystem(), "products", b.ProductID)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		c.inc("wh_skipped_product_unmapped:" + b.ID)
		return nil, "", nil
	}
	var locationID int64
	if b.LocationID.Valid && b.LocationID.String != "" {
		if locationID, err = u.location(ctx, s.whSystem(), b.LocationID.String); err != nil {
			return nil, "", err
		}
	}
	own, ok := whOwnership(b.Status, locationID, u.centerID, b.Quantity)
	if !ok {
		c.inc("wh_status_unknown:" + b.Status)
		c.inc("wh_skipped_status:" + b.ID)
		return nil, "", nil
	}
	return &unitSide{
		Key:       Key{System: s.whSystem(), Table: "product_barcodes", ID: b.ID, TargetTable: "units"},
		Sum:       Checksum(b.ProductID, b.Code, b.Status, b.Quantity, b.LocationID.String),
		Product:   product,
		Own:       own,
		CreatedAt: b.CreatedAt,
		UpdatedAt: latest(b.CreatedAt, b.UpdatedAt),
	}, code, nil
}

// dealerOrg returns the organization of a hub dealer (0: not mapped).
func (u *unitCtx) dealerOrg(ctx context.Context, system string, dealerID int64) (int64, error) {
	if id, ok := u.dealers[dealerID]; ok {
		return id, nil
	}
	target, ok, err := u.m.Lookup(ctx, system, "dealers", strconv.FormatInt(dealerID, 10))
	if err != nil || !ok {
		return 0, err
	}
	id, err := u.q.MigratorOrganizationIDByUUID(ctx, target)
	if err != nil {
		return 0, fmt.Errorf("dealer organization: %w", err)
	}
	u.dealers[dealerID] = id
	return id, nil
}

// location returns the warehouse location a legacy location maps to (0: not
// mapped).
func (u *unitCtx) location(ctx context.Context, system, legacyID string) (int64, error) {
	if id, ok := u.locs[legacyID]; ok {
		return id, nil
	}
	target, ok, err := u.m.Lookup(ctx, system, "warehouse_locations", legacyID)
	if err != nil || !ok {
		return 0, err
	}
	row, err := u.q.MigratorLocationByUUID(ctx, db.MigratorLocationByUUIDParams{Uuid: target, OrganizationID: u.centerID})
	if err != nil {
		return 0, fmt.Errorf("warehouse location: %w", err)
	}
	u.locs[legacyID] = row.ID
	return row.ID, nil
}

func (u *unitCtx) product(ctx context.Context, target uuid.UUID) (db.MigratorProductForUnitRow, error) {
	if p, ok := u.products[target]; ok {
		return p, nil
	}
	p, err := u.q.MigratorProductForUnit(ctx, db.MigratorProductForUnitParams{Uuid: target, BrandID: u.brandID})
	if err != nil {
		return p, fmt.Errorf("product: %w", err)
	}
	u.products[target] = p
	return p, nil
}

// winner is the side that decides product, status and owner: the more
// recently updated one, the warehouse on a tie.
func (cand *unitCandidate) winner() *unitSide {
	switch {
	case cand.Hub == nil:
		return cand.WH
	case cand.WH == nil:
		return cand.Hub
	case cand.Hub.UpdatedAt.After(cand.WH.UpdatedAt):
		return cand.Hub
	}
	return cand.WH
}

func (cand *unitCandidate) sides() []*unitSide {
	var out []*unitSide
	for _, s := range []*unitSide{cand.Hub, cand.WH} {
		if s != nil {
			out = append(out, s)
		}
	}
	return out
}

func (s UnitsStep) importUnit(ctx context.Context, u *unitCtx, cand *unitCandidate, c counts) error {
	win := cand.winner()
	if cand.Hub != nil && cand.WH != nil {
		if cand.Hub.Product != cand.WH.Product {
			c.inc("product_conflict:" + cand.Barcode)
		}
		if cand.Hub.Own != cand.WH.Own {
			c.inc("owner_conflicts")
		}
	}
	product, err := u.product(ctx, win.Product)
	if err != nil {
		return err
	}
	if product.UnitType == "roll_meter" {
		// Roll units need meters the legacy rows do not have.
		c.inc("unit_skipped_roll_meter:" + cand.Barcode)
		// Each legacy row by id, for the validation report (report.go).
		if cand.Hub != nil {
			c.inc("hub_skipped_roll_meter:" + cand.Hub.Key.ID)
		}
		if cand.WH != nil {
			c.inc("wh_skipped_roll_meter:" + cand.WH.Key.ID)
		}
		return nil
	}
	kind := "serial"
	if product.UsesFixedBarcode {
		kind = "fixed"
	}

	// Resolve the unit: an existing mapping of either side, else a unit of
	// this database with the barcode, else a new one.
	var target uuid.UUID
	for _, side := range cand.sides() {
		got, ok, err := u.m.Lookup(ctx, side.Key.System, side.Key.Table, side.Key.ID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if target != uuid.Nil && got != target {
			c.inc("barcode_map_conflict:" + cand.Barcode)
			return nil
		}
		target = got
	}
	changed := false
	if target == uuid.Nil {
		existing, err := u.q.MigratorFindUnitByBarcode(ctx, db.MigratorFindUnitByBarcodeParams{BrandID: u.brandID, Barcode: cand.Barcode})
		switch {
		case err == nil:
			// A unit this database already has (issued here): link, never
			// rewrite its state.
			for _, side := range cand.sides() {
				if _, err := u.m.Link(ctx, side.Key, existing, side.Sum); err != nil {
					return err
				}
			}
			c.inc("units_linked_existing")
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find unit: %w", err)
		}
	}
	for _, side := range cand.sides() {
		_, mapped, err := u.m.Lookup(ctx, side.Key.System, side.Key.Table, side.Key.ID)
		if err != nil {
			return err
		}
		if !mapped && target != uuid.Nil {
			if _, err := u.m.Link(ctx, side.Key, target, side.Sum); err != nil {
				return err
			}
			changed = true
			continue
		}
		res, err := u.m.Upsert(ctx, side.Key, side.Sum)
		if err != nil {
			return err
		}
		target = res.UUID
		changed = changed || res.Changed || res.Created
	}

	unit, err := u.q.MigratorUnitByUUID(ctx, db.MigratorUnitByUUIDParams{Uuid: target, BrandID: u.brandID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id, err := u.q.MigratorInsertUnit(ctx, db.MigratorInsertUnitParams{
			Uuid: target, OrganizationID: u.centerID, BrandID: u.brandID, ProductID: product.ID,
			Barcode: cand.Barcode, UnitKind: kind, Status: win.Own.Status, CreatedAt: pgTime(earliest(cand)),
		})
		if err != nil {
			return fmt.Errorf("insert unit: %w", err)
		}
		c.inc("units_created")
		return writeOwnership(ctx, u, id, kind, win.Own, c)
	case err != nil:
		return fmt.Errorf("read unit: %w", err)
	}
	if !changed {
		c.inc("units_unchanged")
		return nil
	}
	moved, err := u.q.MigratorUnitHasMovements(ctx, unit.ID)
	if err != nil {
		return fmt.Errorf("unit movements: %w", err)
	}
	if moved {
		// The ledger owns this unit now; report the legacy change only.
		c.inc("units_kept_ledger")
		return nil
	}
	if err := u.q.MigratorUpdateUnit(ctx, db.MigratorUpdateUnitParams{
		ID: unit.ID, BrandID: u.brandID, ProductID: product.ID, Status: win.Own.Status,
	}); err != nil {
		return fmt.Errorf("update unit: %w", err)
	}
	c.inc("units_updated")
	return writeOwnership(ctx, u, unit.ID, unit.UnitKind, win.Own, c)
}

// earliest is the first creation time of the candidate's legacy rows.
func earliest(cand *unitCandidate) sql.NullTime {
	var out sql.NullTime
	for _, s := range cand.sides() {
		if s.CreatedAt.Valid && (!out.Valid || s.CreatedAt.Time.Before(out.Time)) {
			out = s.CreatedAt
		}
	}
	return out
}

// writeOwnership replaces the movement-less ownership row of a unit.
func writeOwnership(ctx context.Context, u *unitCtx, unitID int64, kind string, own ownership, c counts) error {
	if own.Pending != "" {
		c.inc("ownership_pending:" + own.Pending)
	}
	if kind == "fixed" {
		if err := u.q.MigratorDeleteFixedHoldings(ctx, unitID); err != nil {
			return fmt.Errorf("clear holdings: %w", err)
		}
		if own.OwnerType == "" {
			return nil
		}
		if own.Quantity <= 0 {
			c.inc("ownership_fixed_empty")
			return nil
		}
		if err := u.q.MigratorInsertFixedHolding(ctx, db.MigratorInsertFixedHoldingParams{
			UnitID: unitID, BrandID: u.brandID, OwnerType: own.OwnerType, OwnerID: own.OwnerID,
			HolderOrgID: own.Holder, QuantityOnHand: own.Quantity,
		}); err != nil {
			return fmt.Errorf("insert holding: %w", err)
		}
		c.inc("ownership_" + ownerCounter(own, u.centerID))
		return nil
	}
	if own.OwnerType == "" {
		if err := u.q.MigratorDeleteUnitState(ctx, unitID); err != nil {
			return fmt.Errorf("clear state: %w", err)
		}
		return nil
	}
	n, err := u.q.MigratorUpsertUnitState(ctx, db.MigratorUpsertUnitStateParams{
		UnitID: unitID, BrandID: u.brandID, OwnerType: own.OwnerType, OwnerID: own.OwnerID,
		HolderOrgID: own.Holder, Status: own.Status,
	})
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if n == 0 {
		c.inc("units_kept_ledger")
		return nil
	}
	c.inc("ownership_" + ownerCounter(own, u.centerID))
	return nil
}

// ownerCounter names an owner for the report: center, dealer, location or
// trash.
func ownerCounter(own ownership, centerID int64) string {
	switch {
	case own.OwnerType == ownerLocation:
		return "location"
	case own.OwnerType == ownerTrash:
		return "trash"
	case own.OwnerID == centerID:
		return "center"
	}
	return "dealer"
}

func (s UnitsStep) importBinStock(ctx context.Context, u *unitCtx, b whBinStock, c counts) error {
	switch whBrand(b.Brand) {
	case "glorian":
		c.inc("bin_stock_glorian_skipped")
		return nil
	case "":
		c.inc("bin_stock_brand_unknown:" + strings.TrimSpace(b.Brand))
		return nil
	}
	locationID, err := u.location(ctx, s.whSystem(), b.LocationID)
	if err != nil {
		return err
	}
	if locationID == 0 {
		c.inc("bin_stock_skipped_location_unmapped:" + b.ID)
		return nil
	}
	target, ok, err := u.m.Lookup(ctx, s.whSystem(), "products", b.ProductID)
	if err != nil {
		return err
	}
	if !ok {
		c.inc("bin_stock_skipped_product_unmapped:" + b.ID)
		return nil
	}
	product, err := u.product(ctx, target)
	if err != nil {
		return err
	}
	if b.Quantity < 0 {
		c.inc("bin_stock_skipped_negative:" + b.ID)
		return nil
	}
	pieces, err := u.q.MigratorCountLocationUnits(ctx, db.MigratorCountLocationUnitsParams{LocationID: locationID, ProductID: product.ID})
	if err != nil {
		return fmt.Errorf("count units: %w", err)
	}
	if pieces != int64(b.Quantity) {
		c.inc("bin_stock_mismatch:" + b.ID)
	}

	current, err := u.q.MigratorGetBinStock(ctx, db.MigratorGetBinStockParams{LocationID: locationID, ProductID: product.ID})
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read bin stock: %w", err)
	}
	if exists && current == b.Quantity {
		c.inc("bin_stocks_unchanged")
		return nil
	}
	if !exists && b.Quantity == 0 {
		c.inc("bin_stocks_empty_skipped")
		return nil
	}
	moved, err := u.q.MigratorLocationHasMovements(ctx, locationID)
	if err != nil {
		return fmt.Errorf("location movements: %w", err)
	}
	if moved {
		c.inc("bin_stocks_kept_ledger")
		return nil
	}
	if err := u.q.MigratorUpsertBinStock(ctx, db.MigratorUpsertBinStockParams{
		LocationID: locationID, OrganizationID: u.centerID, BrandID: u.brandID, ProductID: product.ID, Quantity: b.Quantity,
	}); err != nil {
		return fmt.Errorf("write bin stock: %w", err)
	}
	if exists {
		c.inc("bin_stocks_updated")
	} else {
		c.inc("bin_stocks_created")
	}
	return nil
}
