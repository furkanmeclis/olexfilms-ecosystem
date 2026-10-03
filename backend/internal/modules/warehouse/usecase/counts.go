package usecase

// Stock counts (TEC-206, F1-03f). A count covers one warehouse of the
// active organization and a scope inside it (warehouse, room, location
// subtree or one product). Flow:
//
//	create (draft) -> [initial_placement: approve-start] -> start
//	  -> scans (TEC-203 resolver) -> complete (lines, no stock change)
//	  -> approve (resolutions through ledger.Post, one transaction)
//
// Methods: location_first (a location is required per unit scan: the
// scanning user's last location scan or an explicit location_uuid),
// unit_first (the location is optional for serial units), product_qty
// (SKUs of serial products and fixed barcodes with a quantity per
// location), initial_placement (the organization's unlocated units are
// placed into the scope's locations; needs a start approval).
//
// Blind counts never return expected values while counting; guided counts
// return them on scans and on the count detail. Once completed the lines
// (report) show expected and counted values for the approver.
//
// Approve resolutions and ledger movements (idempotency key
// stock_count:stock_count_line:<line id>:<type>:<barcode>):
//
//	missing serial         void_missing       -> void from the expected owner
//	wrong_location/unlocated relocate         -> placement to the counted location
//	                                             (+ count_adjustment of measured roll meters)
//	meter_variance         increase_unlocated -> count_adjustment (meters)
//	fixed qty_variance     increase_unlocated -> count_adjustment (quantity to the counted value)
//	any difference         ignore             -> nothing
//
// Approval needs warehouse.write (route) plus stock.adjust in the active
// organization (checked here).

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Count methods, visibilities, scopes, statuses, results and resolutions.
const (
	CountMethodLocationFirst    = "location_first"
	CountMethodUnitFirst        = "unit_first"
	CountMethodProductQty       = "product_qty"
	CountMethodInitialPlacement = "initial_placement"

	CountVisibilityBlind  = "blind"
	CountVisibilityGuided = "guided"

	CountScopeWarehouse = "warehouse"
	CountScopeRoom      = "room"
	CountScopeLocation  = "location"
	CountScopeProduct   = "product"

	CountStatusDraft         = "draft"
	CountStatusInProgress    = "in_progress"
	CountStatusPendingReview = "pending_review"
	CountStatusApproved      = "approved"
	CountStatusCancelled     = "cancelled"

	CountScanLocation = "location"
	CountScanSerial   = "serial"
	CountScanFixed    = "fixed"
	CountScanProduct  = "product"

	CountResultMatched       = "matched"
	CountResultMissing       = "missing"
	CountResultWrongLocation = "wrong_location"
	CountResultUnlocated     = "unlocated"
	CountResultUnexpected    = "unexpected"
	CountResultQtyVariance   = "qty_variance"
	CountResultMeterVariance = "meter_variance"

	CountResolveIgnore            = "ignore"
	CountResolveRelocate          = "relocate"
	CountResolveVoidMissing       = "void_missing"
	CountResolveIncreaseUnlocated = "increase_unlocated"
)

// Ledger reference of count movements.
const (
	countSource  = "stock_count"
	countRefType = "stock_count_line"
)

// Limits.
const (
	MaxCountQuantity = 100000
	maxCountNote     = 500
)

// Count errors.
var (
	ErrCountNotFound     = errors.New("warehouse: stock count not found")
	ErrCountScanNotFound = errors.New("warehouse: stock count scan not found")
	// ErrCountStatus: the action does not fit the count's status.
	ErrCountStatus = errors.New("warehouse: stock count status does not allow this")
	// ErrCountStartApproval: initial_placement starts after a start approval.
	ErrCountStartApproval = errors.New("warehouse: stock count needs a start approval")
	// ErrCountLocationOutOfScope: the location is outside the count scope.
	ErrCountLocationOutOfScope = errors.New("warehouse: location outside the count scope")
	// ErrCountProductOutOfScope: the unit/product is not the scoped product.
	ErrCountProductOutOfScope = errors.New("warehouse: product outside the count scope")
	// ErrCountScanKind: the scanned kind does not fit the count method.
	ErrCountScanKind = errors.New("warehouse: scan kind not allowed for this count method")
	// ErrCountLocationRequired: the method needs a location for this scan.
	ErrCountLocationRequired = errors.New("warehouse: scan needs a location")
	// ErrCountUnitScanned: a serial unit is counted once.
	ErrCountUnitScanned = errors.New("warehouse: unit already scanned in this count")
	// ErrCountAdjustForbidden: approval needs stock.adjust.
	ErrCountAdjustForbidden = errors.New("warehouse: stock count approval needs stock.adjust")
	// ErrCountStale: the stock changed since completion; the line can no
	// longer be applied (recount or ignore it).
	ErrCountStale = errors.New("warehouse: stock changed since the count was completed")
)

// Counts implements the stock count use cases.
type Counts struct {
	pool    TxBeginner
	q       *db.Queries
	ledger  *ledger.Ledger
	scanner *Scanner
}

// NewCounts builds the service; scans resolve through scanner (TEC-203).
func NewCounts(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, scanner *Scanner) *Counts {
	return &Counts{pool: pool, q: q, ledger: ledger.New(q, out), scanner: scanner}
}

// ---------------------------------------------------------------------------
// Views.

// CountWarehouse is the counted warehouse.
type CountWarehouse struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// CountRoom is a scoped room.
type CountRoom struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// CountLocation is a location reference.
type CountLocation struct {
	UUID     uuid.UUID `json:"uuid"`
	FullCode string    `json:"full_code"`
	Name     string    `json:"name"`
}

// CountProduct is a product reference.
type CountProduct struct {
	UUID uuid.UUID `json:"uuid"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
}

// CountUnit is a unit reference.
type CountUnit struct {
	UUID     uuid.UUID `json:"uuid"`
	Barcode  string    `json:"barcode"`
	UnitKind string    `json:"unit_kind"`
}

// CountProgress counts what has been scanned.
type CountProgress struct {
	Scans           int   `json:"scans"`
	SerialUnits     int   `json:"serial_units"`
	FixedQuantity   int64 `json:"fixed_quantity"`
	ProductQuantity int64 `json:"product_quantity"`
}

// CountExpectedLocation is the expected stock of one location (null
// location: held by the organization without a location).
type CountExpectedLocation struct {
	Location      *CountLocation `json:"location"`
	SerialUnits   int            `json:"serial_units"`
	FixedQuantity int64          `json:"fixed_quantity"`
}

// CountExpected is the expected stock of a guided count.
type CountExpected struct {
	SerialUnits   int                     `json:"serial_units"`
	FixedQuantity int64                   `json:"fixed_quantity"`
	Locations     []CountExpectedLocation `json:"locations"`
}

// CountSummary counts the lines of a completed count.
type CountSummary struct {
	Lines      int            `json:"lines"`
	ByResult   map[string]int `json:"by_result"`
	Unresolved int            `json:"unresolved"`
}

// StockCount is the API view of a count.
type StockCount struct {
	UUID                  uuid.UUID      `json:"uuid"`
	Status                string         `json:"status"`
	Method                string         `json:"method"`
	Visibility            string         `json:"visibility"`
	ScopeType             string         `json:"scope_type"`
	Warehouse             CountWarehouse `json:"warehouse"`
	ScopeRoom             *CountRoom     `json:"scope_room"`
	ScopeLocation         *CountLocation `json:"scope_location"`
	ScopeProduct          *CountProduct  `json:"scope_product"`
	Note                  *string        `json:"note"`
	StartApprovalRequired bool           `json:"start_approval_required"`
	StartApprovedAt       *time.Time     `json:"start_approved_at"`
	StartedAt             *time.Time     `json:"started_at"`
	CompletedAt           *time.Time     `json:"completed_at"`
	ApprovedAt            *time.Time     `json:"approved_at"`
	CancelledAt           *time.Time     `json:"cancelled_at"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	// Detail only.
	Progress *CountProgress `json:"progress,omitempty"`
	// Guided counts while counting only; a blind count never has it.
	Expected *CountExpected `json:"expected,omitempty"`
	// Completed counts only.
	Summary *CountSummary `json:"summary,omitempty"`
}

// CountScan is one recorded scan.
type CountScan struct {
	UUID      uuid.UUID      `json:"uuid"`
	Kind      string         `json:"kind"`
	RawCode   string         `json:"raw_code"`
	Unit      *CountUnit     `json:"unit"`
	Product   *CountProduct  `json:"product"`
	Location  *CountLocation `json:"location"`
	Quantity  int32          `json:"quantity"`
	Meters    *string        `json:"meters"`
	CreatedAt time.Time      `json:"created_at"`
}

// CountScanExpected is what the ledger expects for a scan (guided only).
type CountScanExpected struct {
	// InScope: the unit is expected inside this count.
	InScope bool `json:"in_scope"`
	// Location: where the ledger has the unit (null: unlocated or not held
	// by the organization).
	Location *CountLocation `json:"location"`
	// Quantity: units expected at the scanned location (location scans,
	// fixed barcodes) or 1/0 for a serial unit.
	Quantity int64 `json:"quantity"`
	// Meters: remaining roll meters in the ledger.
	Meters *string `json:"meters"`
}

// CountScanResult answers a scan.
type CountScanResult struct {
	Scan            CountScan      `json:"scan"`
	LocationContext *CountLocation `json:"location_context"`
	// Guided counts only; omitted for blind counts.
	Expected *CountScanExpected `json:"expected,omitempty"`
}

// CountLine is one difference (or match) of a completed count.
type CountLine struct {
	UUID               uuid.UUID      `json:"uuid"`
	LineKind           string         `json:"line_kind"`
	Unit               *CountUnit     `json:"unit"`
	Product            CountProduct   `json:"product"`
	ExpectedLocation   *CountLocation `json:"expected_location"`
	CountedLocation    *CountLocation `json:"counted_location"`
	ExpectedQuantity   int32          `json:"expected_quantity"`
	CountedQuantity    int32          `json:"counted_quantity"`
	ExpectedMeters     *string        `json:"expected_meters"`
	CountedMeters      *string        `json:"counted_meters"`
	Result             string         `json:"result"`
	Resolution         *string        `json:"resolution"`
	AllowedResolutions []string       `json:"allowed_resolutions"`
	Note               *string        `json:"note"`
	ResolvedAt         *time.Time     `json:"resolved_at"`
}

// CountReport is a completed count with its lines.
type CountReport struct {
	Count StockCount  `json:"count"`
	Lines []CountLine `json:"lines"`
}

// ---------------------------------------------------------------------------
// Inputs.

// CountInput creates a count.
type CountInput struct {
	WarehouseUUID string
	Method        string
	Visibility    string
	ScopeType     string
	RoomUUID      *string
	LocationUUID  *string
	ProductUUID   *string
	Note          *string
}

// CountScanInput records one scan.
type CountScanInput struct {
	Code         string
	LocationUUID *string
	Quantity     *int32
	Meters       *string
}

// CountResolution resolves one line.
type CountResolution struct {
	LineUUID   string
	Resolution string
	Note       *string
}

// ---------------------------------------------------------------------------
// Helpers.

func i8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v > 0} }

func userArg(c ScanCaller) pgtype.Int8 { return i8(c.Principal.UserInternal) }

func parseUUIDField(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, invalid(field, "must be a uuid")
	}
	return id, nil
}

func numToCm(n pgtype.Numeric) (int64, bool) {
	s := stockusecase.MetersString(n)
	if s == nil {
		return 0, false
	}
	cm, err := ledger.ParseMeters(*s)
	if err != nil {
		return 0, false
	}
	return cm, true
}

func cmToNum(cm int64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(ledger.FormatMeters(cm))
	return n
}

func stocked(status string) bool { return status == "available" || status == "placed" }

func validMethod(m string) bool {
	return m == CountMethodLocationFirst || m == CountMethodUnitFirst || m == CountMethodProductQty || m == CountMethodInitialPlacement
}

// allowedResolutions lists the resolutions a line accepts (none: matched).
func allowedResolutions(kind, result string) []string {
	switch result {
	case CountResultMissing:
		if kind == CountScanSerial {
			return []string{CountResolveIgnore, CountResolveVoidMissing}
		}
	case CountResultWrongLocation, CountResultUnlocated:
		return []string{CountResolveIgnore, CountResolveRelocate}
	case CountResultMeterVariance:
		return []string{CountResolveIgnore, CountResolveIncreaseUnlocated}
	case CountResultQtyVariance:
		if kind == CountScanFixed {
			return []string{CountResolveIgnore, CountResolveIncreaseUnlocated}
		}
	case CountResultMatched:
		return []string{}
	}
	return []string{CountResolveIgnore}
}

func (s *Counts) inTx(ctx context.Context, fn func(tx pgx.Tx, q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Counts) lockCount(ctx context.Context, q *db.Queries, org int64, raw uuid.UUID) (db.StockCount, error) {
	row, err := q.LockStockCountByUUID(ctx, db.LockStockCountByUUIDParams{Uuid: raw, OrganizationID: org})
	if err != nil {
		return db.StockCount{}, notFound(err, ErrCountNotFound)
	}
	return row, nil
}

// scope returns the location ids inside the count scope.
func scopeLocations(ctx context.Context, q *db.Queries, cnt db.StockCount) (map[int64]bool, []int64, error) {
	var ids []int64
	var err error
	switch cnt.ScopeType {
	case CountScopeLocation:
		ids, err = q.ListLocationSubtreeIDs(ctx, db.ListLocationSubtreeIDsParams{RootID: cnt.ScopeLocationID.Int64, OrganizationID: cnt.OrganizationID})
	default:
		ids, err = q.ListWarehouseScopeLocationIDs(ctx, db.ListWarehouseScopeLocationIDsParams{
			OrganizationID: cnt.OrganizationID, WarehouseID: i8(cnt.WarehouseID), RoomID: cnt.ScopeRoomID,
		})
	}
	if err != nil {
		return nil, nil, fmt.Errorf("warehouse: count scope: %w", err)
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	slices.Sort(ids)
	return set, ids, nil
}

// refs batches the references of scans and lines.
type refs struct {
	locations map[int64]CountLocation
	products  map[int64]CountProduct
	units     map[int64]CountUnit
}

func loadRefs(ctx context.Context, q *db.Queries, locIDs, productIDs, unitIDs []int64) (refs, error) {
	r := refs{locations: map[int64]CountLocation{}, products: map[int64]CountProduct{}, units: map[int64]CountUnit{}}
	if len(locIDs) > 0 {
		rows, err := q.ListWarehouseLocationsByIDs(ctx, locIDs)
		if err != nil {
			return r, fmt.Errorf("warehouse: count locations: %w", err)
		}
		for _, l := range rows {
			r.locations[l.ID] = locationRef(l)
		}
	}
	if len(unitIDs) > 0 {
		rows, err := q.ListStockCountUnits(ctx, unitIDs)
		if err != nil {
			return r, fmt.Errorf("warehouse: count units: %w", err)
		}
		for _, u := range rows {
			r.units[u.ID] = CountUnit{UUID: u.Uuid, Barcode: u.Barcode, UnitKind: u.UnitKind}
			productIDs = append(productIDs, u.ProductID)
		}
	}
	if len(productIDs) > 0 {
		rows, err := q.ListStockCountProducts(ctx, productIDs)
		if err != nil {
			return r, fmt.Errorf("warehouse: count products: %w", err)
		}
		for _, p := range rows {
			r.products[p.ID] = CountProduct{UUID: p.Uuid, SKU: p.Sku, Name: p.Name}
		}
	}
	return r, nil
}

func locationRef(l db.WarehouseLocation) CountLocation {
	code := l.Code
	if l.FullCode.Valid {
		code = l.FullCode.String
	}
	return CountLocation{UUID: l.Uuid, FullCode: code, Name: l.Name}
}

func (r refs) location(id pgtype.Int8) *CountLocation {
	if !id.Valid {
		return nil
	}
	if l, ok := r.locations[id.Int64]; ok {
		return &l
	}
	return nil
}

func (r refs) unit(id pgtype.Int8) *CountUnit {
	if !id.Valid {
		return nil
	}
	if u, ok := r.units[id.Int64]; ok {
		return &u
	}
	return nil
}

func (r refs) product(id pgtype.Int8) *CountProduct {
	if !id.Valid {
		return nil
	}
	if p, ok := r.products[id.Int64]; ok {
		return &p
	}
	return nil
}

func scanView(sc db.StockCountScan, r refs) CountScan {
	return CountScan{
		UUID: sc.Uuid, Kind: sc.Kind, RawCode: sc.RawCode, Unit: r.unit(sc.UnitID),
		Product: r.product(sc.ProductID), Location: r.location(sc.LocationID),
		Quantity: sc.Quantity, Meters: stockusecase.MetersString(sc.Meters), CreatedAt: ts(sc.CreatedAt),
	}
}

func (s *Counts) view(ctx context.Context, q *db.Queries, cnt db.StockCount) (StockCount, error) {
	w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: cnt.WarehouseID, OrganizationID: cnt.OrganizationID})
	if err != nil {
		return StockCount{}, fmt.Errorf("warehouse: count warehouse: %w", err)
	}
	out := StockCount{
		UUID: cnt.Uuid, Status: cnt.Status, Method: cnt.Method, Visibility: cnt.Visibility, ScopeType: cnt.ScopeType,
		Warehouse: CountWarehouse{UUID: w.Uuid, Code: w.Code, Name: w.Name}, Note: textPtr(cnt.Note),
		StartApprovalRequired: cnt.Method == CountMethodInitialPlacement,
		StartApprovedAt:       tsPtr(cnt.StartApprovedAt), StartedAt: tsPtr(cnt.StartedAt),
		CompletedAt: tsPtr(cnt.CompletedAt), ApprovedAt: tsPtr(cnt.ApprovedAt), CancelledAt: tsPtr(cnt.CancelledAt),
		CreatedAt: ts(cnt.CreatedAt), UpdatedAt: ts(cnt.UpdatedAt),
	}
	if cnt.ScopeRoomID.Valid {
		r, err := q.GetRoomByID(ctx, db.GetRoomByIDParams{ID: cnt.ScopeRoomID.Int64, OrganizationID: cnt.OrganizationID})
		if err != nil {
			return StockCount{}, fmt.Errorf("warehouse: count room: %w", err)
		}
		out.ScopeRoom = &CountRoom{UUID: r.Uuid, Code: r.Code, Name: r.Name}
	}
	if cnt.ScopeLocationID.Valid {
		l, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: cnt.ScopeLocationID.Int64, OrganizationID: cnt.OrganizationID})
		if err != nil {
			return StockCount{}, fmt.Errorf("warehouse: count location: %w", err)
		}
		ref := locationRef(l)
		out.ScopeLocation = &ref
	}
	if cnt.ScopeProductID.Valid {
		p, err := q.GetProductByIDAnyBrand(ctx, cnt.ScopeProductID.Int64)
		if err != nil {
			return StockCount{}, fmt.Errorf("warehouse: count product: %w", err)
		}
		out.ScopeProduct = &CountProduct{UUID: p.Uuid, SKU: p.Sku, Name: p.Name}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Create, list, get, lifecycle.

// Create opens a draft count.
func (s *Counts) Create(ctx context.Context, c ScanCaller, in CountInput) (StockCount, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockCount{}, err
	}
	method := strings.TrimSpace(in.Method)
	if !validMethod(method) {
		return StockCount{}, invalid("method", "must be location_first, unit_first, product_qty or initial_placement")
	}
	visibility := strings.TrimSpace(in.Visibility)
	if visibility != CountVisibilityBlind && visibility != CountVisibilityGuided {
		return StockCount{}, invalid("visibility", "must be blind or guided")
	}
	scope := strings.TrimSpace(in.ScopeType)
	if scope == "" {
		scope = CountScopeWarehouse
	}
	whID, err := parseUUIDField("warehouse_uuid", in.WarehouseUUID)
	if err != nil {
		return StockCount{}, err
	}
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: whID, OrganizationID: org})
	if err != nil {
		return StockCount{}, notFound(err, ErrWarehouseNotFound)
	}
	arg := db.CreateStockCountParams{
		OrganizationID: org, BrandID: c.Org.BrandID, WarehouseID: w.ID, Method: method, Visibility: visibility,
		ScopeType: scope, CreatedByUserID: userArg(c),
	}
	need := func(field string, v *string) (uuid.UUID, error) {
		if v == nil || strings.TrimSpace(*v) == "" {
			return uuid.Nil, invalid(field, "is required for this scope")
		}
		return parseUUIDField(field, *v)
	}
	switch scope {
	case CountScopeWarehouse:
	case CountScopeRoom:
		id, err := need("room_uuid", in.RoomUUID)
		if err != nil {
			return StockCount{}, err
		}
		r, err := s.q.GetRoomByUUID(ctx, db.GetRoomByUUIDParams{Uuid: id, OrganizationID: org})
		if err != nil || r.WarehouseID != w.ID {
			return StockCount{}, invalid("room_uuid", "is not a room of the warehouse")
		}
		arg.ScopeRoomID = i8(r.ID)
	case CountScopeLocation:
		id, err := need("location_uuid", in.LocationUUID)
		if err != nil {
			return StockCount{}, err
		}
		l, err := s.q.GetTypedLocationByUUID(ctx, db.GetTypedLocationByUUIDParams{Uuid: id, OrganizationID: org})
		if err != nil || l.WarehouseID.Int64 != w.ID {
			return StockCount{}, invalid("location_uuid", "is not a location of the warehouse")
		}
		arg.ScopeLocationID = i8(l.ID)
	case CountScopeProduct:
		if method == CountMethodInitialPlacement {
			return StockCount{}, invalid("scope_type", "initial_placement cannot be scoped to a product")
		}
		id, err := need("product_uuid", in.ProductUUID)
		if err != nil {
			return StockCount{}, err
		}
		p, err := s.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
		if err != nil {
			return StockCount{}, invalid("product_uuid", "is not a product of this brand")
		}
		arg.ScopeProductID = i8(p.ID)
	default:
		return StockCount{}, invalid("scope_type", "must be warehouse, room, location or product")
	}
	if in.Note != nil {
		note := strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(note) > maxCountNote {
			return StockCount{}, invalid("note", "must be at most 500 characters")
		}
		if note != "" {
			arg.Note = pgtype.Text{String: note, Valid: true}
		}
	}
	row, err := s.q.CreateStockCount(ctx, arg)
	if err != nil {
		return StockCount{}, mapDBError(err)
	}
	return s.view(ctx, s.q, row)
}

// List lists the organization's counts, newest first.
func (s *Counts) List(ctx context.Context, c ScanCaller, status string, limit, offset int32) ([]StockCount, int64, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, 0, err
	}
	var st pgtype.Text
	if status != "" {
		switch status {
		case CountStatusDraft, CountStatusInProgress, CountStatusPendingReview, CountStatusApproved, CountStatusCancelled:
		default:
			return nil, 0, invalid("status", "is not a count status")
		}
		st = pgtype.Text{String: status, Valid: true}
	}
	rows, err := s.q.ListStockCounts(ctx, db.ListStockCountsParams{OrganizationID: org, Status: st, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("warehouse: list counts: %w", err)
	}
	total, err := s.q.CountStockCounts(ctx, db.CountStockCountsParams{OrganizationID: org, Status: st})
	if err != nil {
		return nil, 0, fmt.Errorf("warehouse: count counts: %w", err)
	}
	out := make([]StockCount, 0, len(rows))
	for _, r := range rows {
		v, err := s.view(ctx, s.q, r)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// Get returns a count with its progress, the expected stock of a guided
// count while counting and the line summary of a completed count.
func (s *Counts) Get(ctx context.Context, c ScanCaller, id uuid.UUID) (StockCount, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockCount{}, err
	}
	cnt, err := s.q.GetStockCountByUUID(ctx, db.GetStockCountByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return StockCount{}, notFound(err, ErrCountNotFound)
	}
	return s.detail(ctx, s.q, cnt)
}

func (s *Counts) detail(ctx context.Context, q *db.Queries, cnt db.StockCount) (StockCount, error) {
	out, err := s.view(ctx, q, cnt)
	if err != nil {
		return StockCount{}, err
	}
	scans, err := q.ListStockCountScans(ctx, cnt.ID)
	if err != nil {
		return StockCount{}, fmt.Errorf("warehouse: count scans: %w", err)
	}
	p := &CountProgress{Scans: len(scans)}
	for _, sc := range scans {
		switch sc.Kind {
		case CountScanSerial:
			p.SerialUnits++
		case CountScanFixed:
			p.FixedQuantity += int64(sc.Quantity)
		case CountScanProduct:
			p.ProductQuantity += int64(sc.Quantity)
		}
	}
	out.Progress = p
	open := cnt.Status == CountStatusDraft || cnt.Status == CountStatusInProgress
	if cnt.Visibility == CountVisibilityGuided && open {
		exp, err := s.expected(ctx, q, cnt)
		if err != nil {
			return StockCount{}, err
		}
		out.Expected = exp
	}
	if cnt.CompletedAt.Valid {
		lines, err := q.ListStockCountLines(ctx, cnt.ID)
		if err != nil {
			return StockCount{}, fmt.Errorf("warehouse: count lines: %w", err)
		}
		sum := &CountSummary{Lines: len(lines), ByResult: map[string]int{}}
		for _, l := range lines {
			sum.ByResult[l.Result]++
			if l.Result != CountResultMatched && !l.Resolution.Valid {
				sum.Unresolved++
			}
		}
		out.Summary = sum
	}
	return out, nil
}

// expected aggregates the expected stock of the count per location.
func (s *Counts) expected(ctx context.Context, q *db.Queries, cnt db.StockCount) (*CountExpected, error) {
	set, err := s.expectedSet(ctx, q, cnt)
	if err != nil {
		return nil, err
	}
	type agg struct {
		units int
		qty   int64
	}
	byLoc := map[int64]*agg{}
	get := func(loc int64) *agg {
		if a, ok := byLoc[loc]; ok {
			return a
		}
		a := &agg{}
		byLoc[loc] = a
		return a
	}
	out := &CountExpected{Locations: []CountExpectedLocation{}}
	for _, e := range set.serial {
		get(e.loc).units++
		out.SerialUnits++
	}
	for k, e := range set.fixed {
		get(k.loc).qty += int64(e.qty)
		out.FixedQuantity += int64(e.qty)
	}
	locIDs := make([]int64, 0, len(byLoc))
	for id := range byLoc {
		if id != 0 {
			locIDs = append(locIDs, id)
		}
	}
	r, err := loadRefs(ctx, q, locIDs, nil, nil)
	if err != nil {
		return nil, err
	}
	keys := make([]int64, 0, len(byLoc))
	for id := range byLoc {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	for _, id := range keys {
		a := byLoc[id]
		out.Locations = append(out.Locations, CountExpectedLocation{Location: r.location(i8(id)), SerialUnits: a.units, FixedQuantity: a.qty})
	}
	return out, nil
}

// ApproveStart records the start approval (initial_placement).
func (s *Counts) ApproveStart(ctx context.Context, c ScanCaller, id uuid.UUID) (StockCount, error) {
	return s.transition(ctx, c, id, func(q *db.Queries, cnt db.StockCount) (db.StockCount, error) {
		if cnt.Status != CountStatusDraft {
			return cnt, ErrCountStatus
		}
		if cnt.StartApprovedAt.Valid {
			return cnt, nil
		}
		return q.ApproveStockCountStart(ctx, db.ApproveStockCountStartParams{UserID: userArg(c), ID: cnt.ID})
	})
}

// Start opens the count for scanning.
func (s *Counts) Start(ctx context.Context, c ScanCaller, id uuid.UUID) (StockCount, error) {
	return s.transition(ctx, c, id, func(q *db.Queries, cnt db.StockCount) (db.StockCount, error) {
		if cnt.Status != CountStatusDraft {
			return cnt, ErrCountStatus
		}
		if cnt.Method == CountMethodInitialPlacement && !cnt.StartApprovedAt.Valid {
			return cnt, ErrCountStartApproval
		}
		return q.StartStockCount(ctx, db.StartStockCountParams{UserID: userArg(c), ID: cnt.ID})
	})
}

// Cancel cancels an open or pending count; no stock is touched.
func (s *Counts) Cancel(ctx context.Context, c ScanCaller, id uuid.UUID) (StockCount, error) {
	return s.transition(ctx, c, id, func(q *db.Queries, cnt db.StockCount) (db.StockCount, error) {
		switch cnt.Status {
		case CountStatusDraft, CountStatusInProgress, CountStatusPendingReview:
			return q.CancelStockCount(ctx, cnt.ID)
		}
		return cnt, ErrCountStatus
	})
}

func (s *Counts) transition(ctx context.Context, c ScanCaller, id uuid.UUID, fn func(q *db.Queries, cnt db.StockCount) (db.StockCount, error)) (StockCount, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockCount{}, err
	}
	var out StockCount
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		cnt, err := s.lockCount(ctx, q, org, id)
		if err != nil {
			return err
		}
		if cnt, err = fn(q, cnt); err != nil {
			return err
		}
		out, err = s.detail(ctx, q, cnt)
		return err
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Scans.

// Scan resolves one scanned code (TEC-203) and records it.
func (s *Counts) Scan(ctx context.Context, c ScanCaller, id uuid.UUID, in CountScanInput) (CountScanResult, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return CountScanResult{}, err
	}
	var out CountScanResult
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		cnt, err := s.lockCount(ctx, q, org, id)
		if err != nil {
			return err
		}
		if cnt.Status != CountStatusInProgress {
			return ErrCountStatus
		}
		res, err := s.scanner.Resolve(ctx, c, in.Code)
		if err != nil {
			return err
		}
		set, _, err := scopeLocations(ctx, q, cnt)
		if err != nil {
			return err
		}
		out, err = s.record(ctx, q, c, cnt, set, res, in)
		return err
	})
	return out, err
}

func (s *Counts) record(ctx context.Context, q *db.Queries, c ScanCaller, cnt db.StockCount, scope map[int64]bool, res ScanResult, in CountScanInput) (CountScanResult, error) {
	user := userArg(c)
	arg := db.InsertStockCountScanParams{CountID: cnt.ID, RawCode: strings.TrimSpace(in.Code), Quantity: 1, ScannedByUserID: user}
	guided := cnt.Visibility == CountVisibilityGuided
	var expected *CountScanExpected
	var ctxLoc *db.WarehouseLocation

	quantity := func() (int32, error) {
		if in.Quantity == nil {
			return 1, nil
		}
		if *in.Quantity < 1 || *in.Quantity > MaxCountQuantity {
			return 0, invalid("quantity", "must be between 1 and 100000")
		}
		return *in.Quantity, nil
	}

	switch res.Type {
	case ScanTypeLocation:
		if in.Quantity != nil || in.Meters != nil || in.LocationUUID != nil {
			return CountScanResult{}, invalid("code", "a location scan takes no quantity, meters or location_uuid")
		}
		l, err := q.GetWarehouseLocationByUUID(ctx, res.Location.UUID)
		if err != nil || l.OrganizationID != cnt.OrganizationID {
			return CountScanResult{}, ErrLocationNotFound
		}
		if !scope[l.ID] {
			return CountScanResult{}, ErrCountLocationOutOfScope
		}
		arg.Kind, arg.LocationID = CountScanLocation, i8(l.ID)
		ctxLoc = &l
		if guided {
			set, err := s.expectedSet(ctx, q, cnt)
			if err != nil {
				return CountScanResult{}, err
			}
			var n int64
			for _, e := range set.serial {
				if e.loc == l.ID {
					n++
				}
			}
			for k, e := range set.fixed {
				if k.loc == l.ID {
					n += int64(e.qty)
				}
			}
			ref := locationRef(l)
			expected = &CountScanExpected{InScope: true, Location: &ref, Quantity: n}
		}

	case ScanTypeProduct:
		if cnt.Method != CountMethodProductQty {
			return CountScanResult{}, fmt.Errorf("%w: products are counted by quantity in product_qty counts only", ErrCountScanKind)
		}
		p, err := q.GetProductByUUIDAnyBrand(ctx, res.Product.UUID)
		if err != nil {
			return CountScanResult{}, fmt.Errorf("warehouse: count product: %w", err)
		}
		if p.UsesFixedBarcode {
			return CountScanResult{}, fmt.Errorf("%w: scan the fixed barcode of this product", ErrCountScanKind)
		}
		if cnt.ScopeProductID.Valid && cnt.ScopeProductID.Int64 != p.ID {
			return CountScanResult{}, ErrCountProductOutOfScope
		}
		if in.Meters != nil {
			return CountScanResult{}, invalid("meters", "is only for roll units")
		}
		loc, err := s.scanLocation(ctx, q, cnt, scope, in.LocationUUID, user, true)
		if err != nil {
			return CountScanResult{}, err
		}
		if arg.Quantity, err = quantity(); err != nil {
			return CountScanResult{}, err
		}
		arg.Kind, arg.ProductID, arg.LocationID = CountScanProduct, i8(p.ID), i8(loc.ID)
		ctxLoc = loc
		if guided {
			set, err := s.expectedSet(ctx, q, cnt)
			if err != nil {
				return CountScanResult{}, err
			}
			var n int64
			for _, e := range set.serial {
				if e.loc == loc.ID && e.product == p.ID {
					n++
				}
			}
			ref := locationRef(*loc)
			expected = &CountScanExpected{InScope: true, Location: &ref, Quantity: n}
		}

	case ScanTypeUnit:
		u, err := q.GetUnitByUUID(ctx, res.Unit.UUID)
		if err != nil {
			return CountScanResult{}, fmt.Errorf("warehouse: count unit: %w", err)
		}
		serial := u.UnitKind == ledger.KindSerial
		if serial && cnt.Method == CountMethodProductQty {
			return CountScanResult{}, fmt.Errorf("%w: product_qty counts scan SKUs and fixed barcodes", ErrCountScanKind)
		}
		if cnt.ScopeProductID.Valid && cnt.ScopeProductID.Int64 != u.ProductID {
			return CountScanResult{}, ErrCountProductOutOfScope
		}
		required := !serial || cnt.Method != CountMethodUnitFirst
		loc, err := s.scanLocation(ctx, q, cnt, scope, in.LocationUUID, user, required)
		if err != nil {
			return CountScanResult{}, err
		}
		arg.UnitID, arg.ProductID = i8(u.ID), i8(u.ProductID)
		if loc != nil {
			arg.LocationID = i8(loc.ID)
			ctxLoc = loc
		}
		if serial {
			if in.Quantity != nil && *in.Quantity != 1 {
				return CountScanResult{}, invalid("quantity", "a serial unit counts as 1")
			}
			if in.Meters != nil {
				if !u.InitialMeters.Valid {
					return CountScanResult{}, invalid("meters", "is only for roll units")
				}
				cm, err := ledger.ParseMeters(*in.Meters)
				if err != nil || cm < 0 {
					return CountScanResult{}, invalid("meters", "must be a non-negative decimal with at most 2 decimals")
				}
				init, _ := numToCm(u.InitialMeters)
				if cm > init {
					return CountScanResult{}, invalid("meters", "exceeds the roll's initial meters")
				}
				arg.Meters = cmToNum(cm)
			}
			dup, err := q.StockCountSerialScanExists(ctx, db.StockCountSerialScanExistsParams{CountID: cnt.ID, UnitID: i8(u.ID)})
			if err != nil {
				return CountScanResult{}, fmt.Errorf("warehouse: count duplicate: %w", err)
			}
			if dup {
				return CountScanResult{}, ErrCountUnitScanned
			}
			arg.Kind = CountScanSerial
			if guided {
				if expected, err = s.serialExpectation(ctx, q, cnt, scope, u); err != nil {
					return CountScanResult{}, err
				}
			}
		} else {
			if in.Meters != nil {
				return CountScanResult{}, invalid("meters", "is only for roll units")
			}
			if arg.Quantity, err = quantity(); err != nil {
				return CountScanResult{}, err
			}
			arg.Kind = CountScanFixed
			if guided {
				n, err := q.GetFixedHoldingQuantity(ctx, db.GetFixedHoldingQuantityParams{UnitID: u.ID, OwnerType: string(ledger.OwnerWarehouseLocation), OwnerID: loc.ID})
				if err != nil {
					return CountScanResult{}, fmt.Errorf("warehouse: count holding: %w", err)
				}
				ref := locationRef(*loc)
				expected = &CountScanExpected{InScope: n > 0, Location: &ref, Quantity: int64(n)}
			}
		}
	default:
		return CountScanResult{}, ErrScanNoMatch
	}

	row, err := q.InsertStockCountScan(ctx, arg)
	if err != nil {
		if mapped := mapDBError(err); errors.Is(mapped, ErrCodeTaken) {
			return CountScanResult{}, ErrCountUnitScanned
		}
		return CountScanResult{}, fmt.Errorf("warehouse: count scan: %w", err)
	}
	var locIDs []int64
	if row.LocationID.Valid {
		locIDs = append(locIDs, row.LocationID.Int64)
	}
	var unitIDs, productIDs []int64
	if row.UnitID.Valid {
		unitIDs = append(unitIDs, row.UnitID.Int64)
	}
	if row.ProductID.Valid {
		productIDs = append(productIDs, row.ProductID.Int64)
	}
	r, err := loadRefs(ctx, q, locIDs, productIDs, unitIDs)
	if err != nil {
		return CountScanResult{}, err
	}
	out := CountScanResult{Scan: scanView(row, r)}
	if ctxLoc != nil {
		ref := locationRef(*ctxLoc)
		out.LocationContext = &ref
	}
	if guided {
		out.Expected = expected
	}
	return out, nil
}

// scanLocation picks the counted location: the explicit location_uuid,
// else the scanning user's last location scan (location context).
func (s *Counts) scanLocation(ctx context.Context, q *db.Queries, cnt db.StockCount, scope map[int64]bool, explicit *string, user pgtype.Int8, required bool) (*db.WarehouseLocation, error) {
	if explicit != nil && strings.TrimSpace(*explicit) != "" {
		id, err := parseUUIDField("location_uuid", *explicit)
		if err != nil {
			return nil, err
		}
		l, err := q.GetWarehouseLocationByUUID(ctx, id)
		if err != nil || l.OrganizationID != cnt.OrganizationID {
			return nil, ErrLocationNotFound
		}
		if !scope[l.ID] {
			return nil, ErrCountLocationOutOfScope
		}
		return &l, nil
	}
	lid, err := q.LastStockCountLocationScan(ctx, db.LastStockCountLocationScanParams{CountID: cnt.ID, UserID: user})
	switch {
	case errors.Is(err, pgx.ErrNoRows) || (err == nil && !lid.Valid):
		if required {
			return nil, ErrCountLocationRequired
		}
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("warehouse: count location context: %w", err)
	}
	l, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: lid.Int64, OrganizationID: cnt.OrganizationID})
	if err != nil {
		return nil, fmt.Errorf("warehouse: count location context: %w", err)
	}
	if !scope[l.ID] {
		return nil, ErrCountLocationOutOfScope
	}
	return &l, nil
}

// serialExpectation tells a guided counter where the ledger has the unit.
func (s *Counts) serialExpectation(ctx context.Context, q *db.Queries, cnt db.StockCount, scope map[int64]bool, u db.Unit) (*CountScanExpected, error) {
	exp := &CountScanExpected{Meters: stockusecase.MetersString(u.RemainingMeters)}
	st, err := q.GetUnitCurrentState(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return exp, nil
	}
	if err != nil {
		return nil, fmt.Errorf("warehouse: count unit state: %w", err)
	}
	if st.HolderOrgID != cnt.OrganizationID || !stocked(st.Status) {
		return exp, nil
	}
	if st.OwnerLocationID.Valid {
		l, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: st.OwnerLocationID.Int64, OrganizationID: cnt.OrganizationID})
		if err == nil {
			ref := locationRef(l)
			exp.Location = &ref
		}
		exp.InScope = cnt.Method != CountMethodInitialPlacement && scope[st.OwnerLocationID.Int64] &&
			(!cnt.ScopeProductID.Valid || cnt.ScopeProductID.Int64 == u.ProductID)
	} else {
		exp.InScope = cnt.Method == CountMethodInitialPlacement
	}
	if exp.InScope {
		exp.Quantity = 1
	}
	return exp, nil
}

// Scans lists the recorded scans (no expected values).
func (s *Counts) Scans(ctx context.Context, c ScanCaller, id uuid.UUID) ([]CountScan, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, err
	}
	cnt, err := s.q.GetStockCountByUUID(ctx, db.GetStockCountByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return nil, notFound(err, ErrCountNotFound)
	}
	rows, err := s.q.ListStockCountScans(ctx, cnt.ID)
	if err != nil {
		return nil, fmt.Errorf("warehouse: count scans: %w", err)
	}
	var locIDs, productIDs, unitIDs []int64
	for _, r := range rows {
		if r.LocationID.Valid {
			locIDs = append(locIDs, r.LocationID.Int64)
		}
		if r.ProductID.Valid {
			productIDs = append(productIDs, r.ProductID.Int64)
		}
		if r.UnitID.Valid {
			unitIDs = append(unitIDs, r.UnitID.Int64)
		}
	}
	r, err := loadRefs(ctx, s.q, locIDs, productIDs, unitIDs)
	if err != nil {
		return nil, err
	}
	out := make([]CountScan, 0, len(rows))
	for _, row := range rows {
		out = append(out, scanView(row, r))
	}
	return out, nil
}

// DeleteScan removes a scan of an in-progress count.
func (s *Counts) DeleteScan(ctx context.Context, c ScanCaller, id, scanID uuid.UUID) error {
	org, err := guard(c.Caller)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		cnt, err := s.lockCount(ctx, q, org, id)
		if err != nil {
			return err
		}
		if cnt.Status != CountStatusInProgress {
			return ErrCountStatus
		}
		sc, err := q.GetStockCountScanByUUID(ctx, db.GetStockCountScanByUUIDParams{Uuid: scanID, CountID: cnt.ID})
		if err != nil {
			return notFound(err, ErrCountScanNotFound)
		}
		_, err = q.DeleteStockCountScan(ctx, db.DeleteStockCountScanParams{ID: sc.ID, CountID: cnt.ID})
		return err
	})
}

// ---------------------------------------------------------------------------
// Expected set and completion.

type expSerial struct {
	loc     int64 // 0: held by the organization (unlocated)
	product int64
	meters  pgtype.Numeric
}

type holdKey struct {
	unit int64
	loc  int64 // 0: held by the organization
}

type expFixed struct {
	qty     int32
	product int64
}

type expectedSet struct {
	serial map[int64]expSerial
	fixed  map[holdKey]expFixed
}

// expectedSet is the stock the ledger expects inside the count: units and
// fixed holdings at the scope's locations (initial_placement: held by the
// organization without a location).
func (s *Counts) expectedSet(ctx context.Context, q *db.Queries, cnt db.StockCount) (expectedSet, error) {
	set := expectedSet{serial: map[int64]expSerial{}, fixed: map[holdKey]expFixed{}}
	if cnt.Method == CountMethodInitialPlacement {
		rows, err := q.ListCountExpectedUnlocatedSerial(ctx, cnt.OrganizationID)
		if err != nil {
			return set, fmt.Errorf("warehouse: count expected: %w", err)
		}
		for _, r := range rows {
			set.serial[r.UnitID] = expSerial{product: r.ProductID, meters: r.RemainingMeters}
		}
		frows, err := q.ListCountExpectedUnlocatedFixed(ctx, cnt.OrganizationID)
		if err != nil {
			return set, fmt.Errorf("warehouse: count expected: %w", err)
		}
		for _, r := range frows {
			set.fixed[holdKey{unit: r.UnitID}] = expFixed{qty: r.QuantityOnHand, product: r.ProductID}
		}
		return set, nil
	}
	_, ids, err := scopeLocations(ctx, q, cnt)
	if err != nil {
		return set, err
	}
	if len(ids) == 0 {
		return set, nil
	}
	rows, err := q.ListCountExpectedSerial(ctx, db.ListCountExpectedSerialParams{OrganizationID: cnt.OrganizationID, LocationIds: ids, ProductID: cnt.ScopeProductID})
	if err != nil {
		return set, fmt.Errorf("warehouse: count expected: %w", err)
	}
	for _, r := range rows {
		set.serial[r.UnitID] = expSerial{loc: r.LocationID, product: r.ProductID, meters: r.RemainingMeters}
	}
	frows, err := q.ListCountExpectedFixed(ctx, db.ListCountExpectedFixedParams{OrganizationID: cnt.OrganizationID, LocationIds: ids, ProductID: cnt.ScopeProductID})
	if err != nil {
		return set, fmt.Errorf("warehouse: count expected: %w", err)
	}
	for _, r := range frows {
		set.fixed[holdKey{unit: r.UnitID, loc: r.LocationID}] = expFixed{qty: r.QuantityOnHand, product: r.ProductID}
	}
	return set, nil
}

// Complete closes scanning and writes the lines. No stock is touched.
func (s *Counts) Complete(ctx context.Context, c ScanCaller, id uuid.UUID) (StockCount, error) {
	return s.transition(ctx, c, id, func(q *db.Queries, cnt db.StockCount) (db.StockCount, error) {
		if cnt.Status != CountStatusInProgress {
			return cnt, ErrCountStatus
		}
		if err := s.buildLines(ctx, q, cnt); err != nil {
			return cnt, err
		}
		return q.CompleteStockCount(ctx, db.CompleteStockCountParams{UserID: userArg(c), ID: cnt.ID})
	})
}

func nullLoc(loc int64) pgtype.Int8 { return i8(loc) }

func (s *Counts) buildLines(ctx context.Context, q *db.Queries, cnt db.StockCount) error {
	scans, err := q.ListStockCountScans(ctx, cnt.ID)
	if err != nil {
		return fmt.Errorf("warehouse: count scans: %w", err)
	}
	set, err := s.expectedSet(ctx, q, cnt)
	if err != nil {
		return err
	}
	insert := func(arg db.InsertStockCountLineParams) error {
		arg.CountID = cnt.ID
		if _, err := q.InsertStockCountLine(ctx, arg); err != nil {
			return fmt.Errorf("warehouse: count line: %w", err)
		}
		return nil
	}

	// Serial units: one line per scanned unit, then the missing ones.
	if cnt.Method != CountMethodProductQty {
		for _, sc := range scans {
			if sc.Kind != CountScanSerial {
				continue
			}
			unitID := sc.UnitID.Int64
			counted := sc.LocationID.Int64 // 0: unit_first without a location
			u, err := q.GetUnit(ctx, unitID)
			if err != nil {
				return fmt.Errorf("warehouse: count unit: %w", err)
			}
			_, inExpected := set.serial[unitID]
			delete(set.serial, unitID)
			arg := db.InsertStockCountLineParams{
				LineKind: CountScanSerial, UnitID: i8(unitID), ProductID: u.ProductID,
				CountedLocationID: sc.LocationID, CountedQuantity: 1, ExpectedMeters: u.RemainingMeters, CountedMeters: sc.Meters,
			}
			if inExpected {
				arg.ExpectedQuantity = 1
			}
			st, err := q.GetUnitCurrentState(ctx, unitID)
			held := err == nil && st.HolderOrgID == cnt.OrganizationID && stocked(st.Status) &&
				(st.OwnerType == string(ledger.OwnerWarehouseLocation) || st.OwnerType == string(ledger.OwnerOrganization))
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("warehouse: count unit state: %w", err)
			}
			var cur int64
			if held {
				cur = st.OwnerLocationID.Int64
				arg.ExpectedLocationID = st.OwnerLocationID
			}
			switch {
			case !held:
				arg.Result = CountResultUnexpected
			case counted == 0:
				arg.Result = CountResultUnexpected
				if inExpected {
					arg.Result = CountResultMatched
				}
			case cur == counted:
				arg.Result = CountResultMatched
			case cur == 0:
				arg.Result = CountResultUnlocated
			default:
				arg.Result = CountResultWrongLocation
			}
			if arg.Result == CountResultMatched && sc.Meters.Valid {
				got, _ := numToCm(sc.Meters)
				want, _ := numToCm(u.RemainingMeters)
				if got != want {
					arg.Result = CountResultMeterVariance
				}
			}
			if err := insert(arg); err != nil {
				return err
			}
		}
		missing := make([]int64, 0, len(set.serial))
		for id := range set.serial {
			missing = append(missing, id)
		}
		slices.Sort(missing)
		for _, id := range missing {
			e := set.serial[id]
			if err := insert(db.InsertStockCountLineParams{
				LineKind: CountScanSerial, UnitID: i8(id), ProductID: e.product, ExpectedLocationID: nullLoc(e.loc),
				ExpectedQuantity: 1, ExpectedMeters: e.meters, Result: CountResultMissing,
			}); err != nil {
				return err
			}
		}
	}

	// Fixed barcodes: one line per (unit, owner) expected or counted.
	counted := map[holdKey]expFixed{}
	for _, sc := range scans {
		if sc.Kind != CountScanFixed {
			continue
		}
		k := holdKey{unit: sc.UnitID.Int64, loc: sc.LocationID.Int64}
		e := counted[k]
		e.qty += sc.Quantity
		e.product = sc.ProductID.Int64
		counted[k] = e
	}
	keys := make([]holdKey, 0, len(set.fixed)+len(counted))
	for k := range set.fixed {
		keys = append(keys, k)
	}
	for k := range counted {
		if _, ok := set.fixed[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b holdKey) int {
		if a.unit != b.unit {
			return int(a.unit - b.unit)
		}
		return int(a.loc - b.loc)
	})
	for _, k := range keys {
		exp, inExp := set.fixed[k]
		got := counted[k]
		product := exp.product
		if !inExp {
			product = got.product
			ownerType, ownerID := string(ledger.OwnerWarehouseLocation), k.loc
			if k.loc == 0 {
				ownerType, ownerID = string(ledger.OwnerOrganization), cnt.OrganizationID
			}
			n, err := q.GetFixedHoldingQuantity(ctx, db.GetFixedHoldingQuantityParams{UnitID: k.unit, OwnerType: ownerType, OwnerID: ownerID})
			if err != nil {
				return fmt.Errorf("warehouse: count holding: %w", err)
			}
			exp.qty = n
		}
		result := CountResultMatched
		if exp.qty != got.qty {
			result = CountResultQtyVariance
		}
		if err := insert(db.InsertStockCountLineParams{
			LineKind: CountScanFixed, UnitID: i8(k.unit), ProductID: product,
			ExpectedLocationID: nullLoc(k.loc), CountedLocationID: nullLoc(k.loc),
			ExpectedQuantity: exp.qty, CountedQuantity: got.qty, Result: result,
		}); err != nil {
			return err
		}
	}

	// Products (product_qty): serial units per (product, location).
	if cnt.Method == CountMethodProductQty {
		type pk struct{ product, loc int64 }
		expQ, gotQ := map[pk]int32{}, map[pk]int32{}
		for _, e := range set.serial {
			expQ[pk{e.product, e.loc}]++
		}
		for _, sc := range scans {
			if sc.Kind == CountScanProduct {
				gotQ[pk{sc.ProductID.Int64, sc.LocationID.Int64}] += sc.Quantity
			}
		}
		pkeys := make([]pk, 0, len(expQ)+len(gotQ))
		for k := range expQ {
			pkeys = append(pkeys, k)
		}
		for k := range gotQ {
			if _, ok := expQ[k]; !ok {
				pkeys = append(pkeys, k)
			}
		}
		slices.SortFunc(pkeys, func(a, b pk) int {
			if a.product != b.product {
				return int(a.product - b.product)
			}
			return int(a.loc - b.loc)
		})
		for _, k := range pkeys {
			result := CountResultMatched
			if expQ[k] != gotQ[k] {
				result = CountResultQtyVariance
			}
			if err := insert(db.InsertStockCountLineParams{
				LineKind: CountScanProduct, ProductID: k.product, ExpectedLocationID: nullLoc(k.loc), CountedLocationID: nullLoc(k.loc),
				ExpectedQuantity: expQ[k], CountedQuantity: gotQ[k], Result: result,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Report and export.

// Report returns a completed count with every line.
func (s *Counts) Report(ctx context.Context, c ScanCaller, id uuid.UUID) (CountReport, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return CountReport{}, err
	}
	cnt, err := s.q.GetStockCountByUUID(ctx, db.GetStockCountByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return CountReport{}, notFound(err, ErrCountNotFound)
	}
	if !cnt.CompletedAt.Valid {
		return CountReport{}, ErrCountStatus
	}
	view, err := s.detail(ctx, s.q, cnt)
	if err != nil {
		return CountReport{}, err
	}
	lines, err := s.lines(ctx, s.q, cnt.ID)
	if err != nil {
		return CountReport{}, err
	}
	return CountReport{Count: view, Lines: lines}, nil
}

func (s *Counts) lines(ctx context.Context, q *db.Queries, countID int64) ([]CountLine, error) {
	rows, err := q.ListStockCountLines(ctx, countID)
	if err != nil {
		return nil, fmt.Errorf("warehouse: count lines: %w", err)
	}
	var locIDs, productIDs, unitIDs []int64
	for _, l := range rows {
		if l.ExpectedLocationID.Valid {
			locIDs = append(locIDs, l.ExpectedLocationID.Int64)
		}
		if l.CountedLocationID.Valid {
			locIDs = append(locIDs, l.CountedLocationID.Int64)
		}
		if l.UnitID.Valid {
			unitIDs = append(unitIDs, l.UnitID.Int64)
		}
		productIDs = append(productIDs, l.ProductID)
	}
	r, err := loadRefs(ctx, q, locIDs, productIDs, unitIDs)
	if err != nil {
		return nil, err
	}
	out := make([]CountLine, 0, len(rows))
	for _, l := range rows {
		v := CountLine{
			UUID: l.Uuid, LineKind: l.LineKind, Unit: r.unit(l.UnitID), Product: r.products[l.ProductID],
			ExpectedLocation: r.location(l.ExpectedLocationID), CountedLocation: r.location(l.CountedLocationID),
			ExpectedQuantity: l.ExpectedQuantity, CountedQuantity: l.CountedQuantity,
			ExpectedMeters: stockusecase.MetersString(l.ExpectedMeters), CountedMeters: stockusecase.MetersString(l.CountedMeters),
			Result: l.Result, Resolution: textPtr(l.Resolution), AllowedResolutions: allowedResolutions(l.LineKind, l.Result),
			Note: textPtr(l.Note), ResolvedAt: tsPtr(l.ResolvedAt),
		}
		out = append(out, v)
	}
	return out, nil
}

// Export writes the lines of a completed count as CSV.
func (s *Counts) Export(ctx context.Context, c ScanCaller, id uuid.UUID, w io.Writer) error {
	rep, err := s.Report(ctx, c, id)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	loc := func(l *CountLocation) string {
		if l == nil {
			return ""
		}
		return l.FullCode
	}
	_ = cw.Write([]string{"line_uuid", "line_kind", "barcode", "sku", "product", "expected_location", "counted_location",
		"expected_quantity", "counted_quantity", "expected_meters", "counted_meters", "result", "resolution", "note"})
	for _, l := range rep.Lines {
		barcode := ""
		if l.Unit != nil {
			barcode = l.Unit.Barcode
		}
		if err := cw.Write([]string{
			l.UUID.String(), l.LineKind, csvSafe(barcode), csvSafe(l.Product.SKU), csvSafe(l.Product.Name),
			csvSafe(loc(l.ExpectedLocation)), csvSafe(loc(l.CountedLocation)),
			strconv.Itoa(int(l.ExpectedQuantity)), strconv.Itoa(int(l.CountedQuantity)),
			str(l.ExpectedMeters), str(l.CountedMeters), l.Result, str(l.Resolution), csvSafe(str(l.Note)),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvSafe neutralises spreadsheet formula injection.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// ---------------------------------------------------------------------------
// Approval.

// Approve applies the resolutions of a completed count in one transaction.
// Every difference needs a resolution; matched lines take none.
func (s *Counts) Approve(ctx context.Context, c ScanCaller, id uuid.UUID, in []CountResolution) (CountReport, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return CountReport{}, err
	}
	f, err := scopefilter.Resolve(ctx, s.q, c.Principal, &c.Org, rbac.PermStockAdjust)
	if err != nil || f.UserOnly() || !f.AllowsOrg(org, c.Org.BrandID) {
		return CountReport{}, ErrCountAdjustForbidden
	}
	err = s.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		cnt, err := s.lockCount(ctx, q, org, id)
		if err != nil {
			return err
		}
		if cnt.Status != CountStatusPendingReview {
			return ErrCountStatus
		}
		lines, err := q.ListStockCountLines(ctx, cnt.ID)
		if err != nil {
			return fmt.Errorf("warehouse: count lines: %w", err)
		}
		byUUID := make(map[uuid.UUID]int, len(lines))
		for i, l := range lines {
			byUUID[l.Uuid] = i
		}
		chosen := make(map[int]CountResolution, len(in))
		for _, r := range in {
			lid, err := parseUUIDField("resolutions", r.LineUUID)
			if err != nil {
				return err
			}
			i, ok := byUUID[lid]
			if !ok {
				return invalid("resolutions", "line "+lid.String()+" is not a line of this count")
			}
			if _, dup := chosen[i]; dup {
				return invalid("resolutions", "line "+lid.String()+" is resolved twice")
			}
			if !slices.Contains(allowedResolutions(lines[i].LineKind, lines[i].Result), r.Resolution) {
				return invalid("resolutions", fmt.Sprintf("line %s (%s) does not accept %q", lid, lines[i].Result, r.Resolution))
			}
			if r.Note != nil && utf8.RuneCountInString(strings.TrimSpace(*r.Note)) > maxCountNote {
				return invalid("resolutions", "note must be at most 500 characters")
			}
			chosen[i] = r
		}
		for i, l := range lines {
			if _, ok := chosen[i]; !ok && l.Result != CountResultMatched {
				return invalid("resolutions", "line "+l.Uuid.String()+" ("+l.Result+") needs a resolution")
			}
		}
		for i, l := range lines {
			r, ok := chosen[i]
			if !ok {
				continue
			}
			if err := s.apply(ctx, tx, q, c, cnt, l, r.Resolution); err != nil {
				return err
			}
			var note pgtype.Text
			if r.Note != nil && strings.TrimSpace(*r.Note) != "" {
				note = pgtype.Text{String: strings.TrimSpace(*r.Note), Valid: true}
			}
			if _, err := q.ResolveStockCountLine(ctx, db.ResolveStockCountLineParams{
				Resolution: pgtype.Text{String: r.Resolution, Valid: true}, Note: note, ID: l.ID,
			}); err != nil {
				return fmt.Errorf("warehouse: resolve line: %w", err)
			}
		}
		_, err = q.ApproveStockCount(ctx, db.ApproveStockCountParams{UserID: userArg(c), ID: cnt.ID})
		return err
	})
	if err != nil {
		return CountReport{}, err
	}
	return s.Report(ctx, c, id)
}

func countOwner(org int64, loc pgtype.Int8) ledger.Owner {
	if loc.Valid {
		return ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.Int64, OrgID: org}
	}
	return ledger.Owner{Type: ledger.OwnerOrganization, ID: org}
}

// apply posts the movements of one resolved line.
func (s *Counts) apply(ctx context.Context, tx pgx.Tx, q *db.Queries, c ScanCaller, cnt db.StockCount, l db.StockCountLine, resolution string) error {
	if resolution == CountResolveIgnore || !l.UnitID.Valid {
		return nil
	}
	org := cnt.OrganizationID
	base := ledger.Movement{
		UnitID: l.UnitID.Int64, Source: countSource, RefType: countRefType, RefID: l.ID,
		Reason:   "stock count " + cnt.Uuid.String(),
		Metadata: map[string]any{"stock_count_uuid": cnt.Uuid.String(), "line_uuid": l.Uuid.String(), "resolution": resolution},
	}
	if c.Principal.UserInternal > 0 {
		uid := c.Principal.UserInternal
		base.ActorUserID = &uid
	}
	post := func(m ledger.Movement) error {
		if _, err := s.ledger.Post(ctx, tx, m); err != nil {
			if isLedgerConflict(err) {
				return fmt.Errorf("%w: line %s: %v", ErrCountStale, l.Uuid, err)
			}
			return err
		}
		return nil
	}
	// meters adjusts a roll to the measured meters.
	meters := func() error {
		if !l.CountedMeters.Valid {
			return nil
		}
		u, err := q.GetUnit(ctx, l.UnitID.Int64)
		if err != nil {
			return fmt.Errorf("warehouse: count unit: %w", err)
		}
		want, _ := numToCm(l.CountedMeters)
		have, ok := numToCm(u.RemainingMeters)
		if !ok || want == have {
			return nil
		}
		m := base
		m.Type, m.Centimeters = ledger.TypeCountAdjustment, want-have
		return post(m)
	}

	switch resolution {
	case CountResolveVoidMissing:
		from := countOwner(org, l.ExpectedLocationID)
		m := base
		m.Type, m.From = ledger.TypeVoid, &from
		return post(m)
	case CountResolveRelocate:
		if !l.CountedLocationID.Valid {
			return invalid("resolutions", "line "+l.Uuid.String()+" has no counted location")
		}
		from, to := countOwner(org, l.ExpectedLocationID), countOwner(org, l.CountedLocationID)
		m := base
		m.Type, m.From, m.To = ledger.TypePlacement, &from, &to
		if err := post(m); err != nil {
			return err
		}
		return meters()
	case CountResolveIncreaseUnlocated:
		if l.LineKind == CountScanSerial {
			return meters()
		}
		own := countOwner(org, l.CountedLocationID)
		cur, err := q.GetFixedHoldingQuantity(ctx, db.GetFixedHoldingQuantityParams{UnitID: l.UnitID.Int64, OwnerType: string(own.Type), OwnerID: own.ID})
		if err != nil {
			return fmt.Errorf("warehouse: count holding: %w", err)
		}
		delta := l.CountedQuantity - cur
		if delta == 0 {
			return nil
		}
		m := base
		m.Type, m.Quantity = ledger.TypeCountAdjustment, delta
		if delta > 0 {
			m.To = &own
		} else {
			m.From = &own
		}
		return post(m)
	}
	return nil
}

func isLedgerConflict(err error) bool {
	for _, e := range []error{
		ledger.ErrTransitionNotAllowed, ledger.ErrOwnerMismatch, ledger.ErrOwnerNotAllowed,
		ledger.ErrInsufficientStock, ledger.ErrInsufficientMeters, ledger.ErrInvalidMovement,
		ledger.ErrConcurrentUpdate, ledger.ErrIdempotencyConflict,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
