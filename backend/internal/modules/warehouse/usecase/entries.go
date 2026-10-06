package usecase

// Stock entry documents (TEC-204, F1-03d). The center brings printed labels
// into stock at one of its warehouses (K14):
//
//	draft -> lines -> print labels -> place -> confirm
//
//   - Create: target warehouse (active, own organization) and mode:
//     with_existing links printed (or reserved) barcodes, generate_new
//     reserves a new barcode batch through the TEC-202 use case (center
//     only, K14) in the same transaction.
//   - Labels: every line links its TEC-202 print endpoint (the batch
//     sheet for generated barcodes, the unit label otherwise).
//   - Place: a location of the target warehouse is chosen by uuid or by
//     its scanned QR / full_code (OFW:LOC:<full_code>, TEC-202/203) for all
//     lines, the given lines or the lines of the scanned barcodes.
//   - Confirm: in one transaction, under the entry row lock, every line is
//     posted through ledger.Post: serial units enter the organization
//     (entry) and are shelved on their location (placement); fixed
//     barcodes enter their location directly (a fixed barcode has no
//     placement, it moves as out/in pairs). Idempotency keys are
//     stock_entry:stock_entry_line:<line id>:<type>:<barcode>. A second
//     confirm finds the entry confirmed and is refused (ErrEntryNotDraft).
//     Stock enters at a center only (K14): a distributor's draft is
//     refused at confirm (ledger: printed -> entry is center-only); the
//     distributor receives stock through transfers.
//
// The safe stock import (TEC-158) records its own confirmed entry (mode
// import) when a batch is applied; see stock/usecase/import.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Stock entry errors.
var (
	ErrEntryNotFound     = errors.New("warehouse: stock entry not found")
	ErrEntryLineNotFound = errors.New("warehouse: stock entry line not found")
	// ErrEntryNotDraft: the entry is confirmed, cancelled or undone; it can
	// no longer change (a second confirm lands here).
	ErrEntryNotDraft = errors.New("warehouse: stock entry is not a draft")
	// ErrEntryEmpty: confirm without lines.
	ErrEntryEmpty = errors.New("warehouse: stock entry has no lines")
	// ErrEntryUnplaced: confirm while a line has no location.
	ErrEntryUnplaced = errors.New("warehouse: every line needs a location")
	// ErrEntryCenterOnly: stock enters at a center only and only the
	// center generates barcodes (K14).
	ErrEntryCenterOnly = errors.New("warehouse: stock enters at the center only")
	// ErrEntryUnitNotEnterable: the barcode is not a printed label
	// waiting for stock (already in stock, moved or void).
	ErrEntryUnitNotEnterable = errors.New("warehouse: unit is not a printed label")
	// ErrEntryUnitTaken: the unit is already a line of another entry.
	ErrEntryUnitTaken = errors.New("warehouse: unit belongs to another stock entry")
	// ErrEntryLedger: the ledger refused a movement at confirm.
	ErrEntryLedger = errors.New("warehouse: ledger refused the stock entry")
)

// Stock entry modes and statuses (stock_entries).
const (
	EntryModeWithExisting = "with_existing"
	EntryModeGenerateNew  = "generate_new"
	EntryModeImport       = "import"

	EntryStatusDraft     = "draft"
	EntryStatusConfirmed = "confirmed"
	EntryStatusCancelled = "cancelled"
	EntryStatusUndone    = "undone"
)

// Idempotency key parts of the entry movements.
const (
	entrySource  = "stock_entry"
	entryRefType = "stock_entry_line"
)

// Audit (activity_events).
const (
	AuditResourceStockEntry = "warehouse.stock_entry"
	AuditStockEntryCreated  = "warehouse.stock_entry.created"
	AuditStockEntryConfirm  = "warehouse.stock_entry.confirmed"
	AuditStockEntryCancel   = "warehouse.stock_entry.cancelled"
)

// Limits.
const (
	// MaxEntryLines caps the lines of one entry.
	MaxEntryLines = 2000
	// MaxEntryLinesPerCall caps the barcodes one add-lines call links or
	// reserves (the TEC-202 batch limit).
	MaxEntryLinesPerCall = stockusecase.MaxBatchQuantity
	maxNoteLen           = 500
)

// EntryCaller is the caller of the stock entry use cases.
type EntryCaller struct {
	Caller
	Principal authctx.Principal
}

func (c EntryCaller) actor() pgtype.Int8 {
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: c.Principal.UserInternal > 0}
}

// StockEntries implements the stock entry flow.
type StockEntries struct {
	pool     TxBeginner
	q        *db.Queries
	ledger   *ledger.Ledger
	barcodes *stockusecase.Barcodes
}

// NewStockEntries builds the use case; ledger events go to out.
func NewStockEntries(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *StockEntries {
	return &StockEntries{pool: pool, q: q, ledger: ledger.New(q, out), barcodes: stockusecase.NewBarcodes(pool, q)}
}

// --- views -----------------------------------------------------------------

// EntryWarehouse is the target warehouse of an entry.
type EntryWarehouse struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// EntryProduct is the product of a line.
type EntryProduct struct {
	UUID uuid.UUID `json:"uuid"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
}

// EntryLocation is the location of a line.
type EntryLocation struct {
	UUID     uuid.UUID `json:"uuid"`
	Code     string    `json:"code"`
	FullCode *string   `json:"full_code"`
}

// EntryLabelBatch links the TEC-202 print sheet of a reserved batch.
type EntryLabelBatch struct {
	BatchUUID uuid.UUID `json:"batch_uuid"`
	LabelsURL string    `json:"labels_url"`
}

// EntryLine is one unit of an entry.
type EntryLine struct {
	UUID                  uuid.UUID      `json:"uuid"`
	UnitUUID              uuid.UUID      `json:"unit_uuid"`
	Barcode               string         `json:"barcode"`
	UnitKind              string         `json:"unit_kind"`
	UnitStatus            string         `json:"unit_status"`
	Quantity              int32          `json:"quantity"`
	Product               EntryProduct   `json:"product"`
	Location              *EntryLocation `json:"location"`
	LabelURL              string         `json:"label_url"`
	EntryMovementUUID     *uuid.UUID     `json:"entry_movement_uuid"`
	PlacementMovementUUID *uuid.UUID     `json:"placement_movement_uuid"`
	Undone                bool           `json:"undone"`
}

// StockEntry is the entry document.
type StockEntry struct {
	UUID            uuid.UUID         `json:"uuid"`
	Mode            string            `json:"mode"`
	Status          string            `json:"status"`
	Note            *string           `json:"note"`
	Warehouse       *EntryWarehouse   `json:"warehouse"`
	ImportBatchUUID *uuid.UUID        `json:"import_batch_uuid"`
	LineCount       int64             `json:"line_count"`
	PlacedCount     *int64            `json:"placed_count,omitempty"`
	Lines           []EntryLine       `json:"lines,omitempty"`
	LabelBatches    []EntryLabelBatch `json:"label_batches,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	ConfirmedAt     *time.Time        `json:"confirmed_at"`
	CancelledAt     *time.Time        `json:"cancelled_at"`
}

// --- inputs ------------------------------------------------------------------

// EntryInput opens a draft.
type EntryInput struct {
	WarehouseUUID string
	Mode          string
	Note          *string
}

// EntryLinesInput adds lines to a draft. with_existing: Barcodes.
// generate_new: ProductUUID, Count (barcodes to reserve), Meters (rolls),
// Prefix, TemplateUUID. FixedQuantity is the quantity of each fixed
// barcode (fixed barcode products only, default 1).
type EntryLinesInput struct {
	Barcodes      []string
	ProductUUID   string
	Count         int
	Meters        string
	Prefix        string
	TemplateUUID  string
	FixedQuantity *int32
}

// PlaceInput places lines on one location. Lines: LineUUIDs, or the lines
// of the scanned Barcodes, or every line when both are empty. Location:
// LocationUUID or LocationCode (scanned OFW:LOC:<full_code> or full_code).
type PlaceInput struct {
	LineUUIDs    []uuid.UUID
	Barcodes     []string
	LocationUUID *uuid.UUID
	LocationCode string
}

// --- use cases ---------------------------------------------------------------

// Create opens a draft entry at a warehouse of the active organization.
func (s *StockEntries) Create(ctx context.Context, c EntryCaller, in EntryInput) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	mode := strings.TrimSpace(in.Mode)
	if mode != EntryModeWithExisting && mode != EntryModeGenerateNew {
		return StockEntry{}, invalid("mode", "must be with_existing or generate_new")
	}
	if mode == EntryModeGenerateNew && c.Org.OrgType != rbac.OrgTypeCenter {
		return StockEntry{}, ErrEntryCenterOnly
	}
	note, err := normNote(in.Note)
	if err != nil {
		return StockEntry{}, err
	}
	wid, err := uuid.Parse(strings.TrimSpace(in.WarehouseUUID))
	if err != nil {
		return StockEntry{}, invalid("warehouse_uuid", "must be a uuid")
	}
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: wid, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return StockEntry{}, ErrWarehouseNotFound
	}
	if err != nil {
		return StockEntry{}, err
	}
	if !w.Active {
		return StockEntry{}, invalid("warehouse_uuid", "warehouse is inactive")
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		e, err = q.CreateStockEntry(ctx, db.CreateStockEntryParams{
			OrganizationID: org, BrandID: c.Org.BrandID, WarehouseID: pgtype.Int8{Int64: w.ID, Valid: true},
			Mode: mode, Note: note, CreatedByUserID: c.actor(),
		})
		if err != nil {
			return err
		}
		return audit(ctx, q, c, AuditStockEntryCreated, e, map[string]any{"warehouse_id": w.ID, "mode": mode})
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// List returns the entries of the active organization (TEC-375: list
// contract, default newest first).
func (s *StockEntries) List(ctx context.Context, c EntryCaller, f EntryListFilter) ([]StockEntry, int64, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, 0, err
	}
	f.Sort = orDefault(f.Sort, EntrySort)
	cp := db.CountStockEntriesParams{
		OrganizationID: org, Statuses: f.Statuses, Modes: f.Modes, WarehouseUuids: f.WarehouseUUIDs,
		CreatedFrom: listTS(f.CreatedFrom), CreatedBefore: listTS(f.CreatedBefore), Q: listQ(f.Q),
	}
	rows, err := s.q.ListStockEntries(ctx, db.ListStockEntriesParams{
		OrganizationID: cp.OrganizationID, Statuses: cp.Statuses, Modes: cp.Modes, WarehouseUuids: cp.WarehouseUuids,
		CreatedFrom: cp.CreatedFrom, CreatedBefore: cp.CreatedBefore, Q: cp.Q,
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, PageLimit: f.Limit, PageOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountStockEntries(ctx, cp)
	if err != nil {
		return nil, 0, err
	}
	out := make([]StockEntry, 0, len(rows))
	for _, e := range rows {
		v, err := s.view(ctx, s.q, e, false)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// Get returns one entry with its lines.
func (s *StockEntries) Get(ctx context.Context, c EntryCaller, id uuid.UUID) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return StockEntry{}, notFound(err, ErrEntryNotFound)
	}
	return s.view(ctx, s.q, e, true)
}

// AddLines links or reserves barcodes on a draft.
func (s *StockEntries) AddLines(ctx context.Context, c EntryCaller, id uuid.UUID, in EntryLinesInput) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	if in.FixedQuantity != nil && *in.FixedQuantity < 1 {
		return StockEntry{}, invalid("fixed_quantity", "must be at least 1")
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if e, err = s.lockDraft(ctx, q, org, id); err != nil {
			return err
		}
		have, err := q.CountStockEntryLines(ctx, e.ID)
		if err != nil {
			return err
		}
		var units []db.Unit
		switch e.Mode {
		case EntryModeWithExisting:
			if units, err = s.existingUnits(ctx, q, e, in); err != nil {
				return err
			}
		case EntryModeGenerateNew:
			if units, err = s.generateUnits(ctx, q, c, in); err != nil {
				return err
			}
		default:
			return ErrEntryNotDraft
		}
		if have+int64(len(units)) > MaxEntryLines {
			return invalid("barcodes", fmt.Sprintf("an entry holds at most %d lines", MaxEntryLines))
		}
		for _, u := range units {
			qty := int32(1)
			if u.UnitKind == ledger.KindFixed {
				if in.FixedQuantity != nil {
					qty = *in.FixedQuantity
				}
			} else if in.FixedQuantity != nil && *in.FixedQuantity != 1 {
				return invalid("fixed_quantity", "only fixed barcode products carry a quantity")
			}
			if _, err := q.InsertStockEntryLine(ctx, db.InsertStockEntryLineParams{
				EntryID: e.ID, UnitID: u.ID, Quantity: qty,
			}); err != nil {
				if isUnique(err) {
					return invalid("barcodes", "barcode "+u.Barcode+" is already on this entry")
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// existingUnits resolves with_existing barcodes in the entry's brand: each
// must be a printed (or reserved) label not yet in stock and not on another
// open entry. A fixed barcode may enter again (quantities add up).
func (s *StockEntries) existingUnits(ctx context.Context, q *db.Queries, e db.StockEntry, in EntryLinesInput) ([]db.Unit, error) {
	if strings.TrimSpace(in.ProductUUID) != "" || in.Count != 0 {
		return nil, invalid("product_uuid", "with_existing entries link barcodes; product_uuid and quantity are for generate_new")
	}
	codes, err := normBarcodes(in.Barcodes, true)
	if err != nil {
		return nil, err
	}
	units := make([]db.Unit, 0, len(codes))
	for _, code := range codes {
		u, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: e.BrandID, Barcode: code})
		if errors.Is(err, pgx.ErrNoRows) {
			u, err = q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: e.BrandID, Barcode: strings.ToUpper(code)})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invalid("barcodes", "barcode "+code+" not found")
		}
		if err != nil {
			return nil, err
		}
		if err := enterable(ctx, q, u); err != nil {
			return nil, fmt.Errorf("%w: %s", err, u.Barcode)
		}
		units = append(units, u)
	}
	return units, nil
}

// enterable checks that a unit may still enter stock through an entry.
func enterable(ctx context.Context, q *db.Queries, u db.Unit) error {
	fixed := u.UnitKind == ledger.KindFixed
	switch ledger.Status(u.Status) {
	case ledger.StatusReserved, ledger.StatusPrinted:
	case ledger.StatusAvailable:
		if !fixed {
			return ErrEntryUnitNotEnterable
		}
	default:
		return ErrEntryUnitNotEnterable
	}
	if !fixed {
		if _, err := q.GetUnitCurrentState(ctx, u.ID); err == nil {
			return ErrEntryUnitNotEnterable
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if _, err := q.FindOpenStockEntryForUnit(ctx, db.FindOpenStockEntryForUnitParams{
		UnitID: u.ID, IncludeConfirmed: !fixed,
	}); err == nil {
		return ErrEntryUnitTaken
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

// generateUnits reserves a TEC-202 batch in the entry's transaction.
func (s *StockEntries) generateUnits(ctx context.Context, q *db.Queries, c EntryCaller, in EntryLinesInput) ([]db.Unit, error) {
	if len(in.Barcodes) > 0 {
		return nil, invalid("barcodes", "generate_new entries reserve new barcodes; barcodes are for with_existing")
	}
	sc := stockusecase.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter}
	_, units, err := s.barcodes.ReserveInTx(ctx, q, sc, stockusecase.BatchInput{
		ProductUUID: in.ProductUUID, Quantity: in.Count, Meters: in.Meters,
		Prefix: in.Prefix, TemplateUUID: in.TemplateUUID,
	})
	var ve *stockusecase.ValidationError
	switch {
	case err == nil:
		return units, nil
	case errors.As(err, &ve):
		return nil, invalid(ve.Field, ve.Message)
	case errors.Is(err, stockusecase.ErrBarcodesCenterOnly):
		return nil, ErrEntryCenterOnly
	case errors.Is(err, stockusecase.ErrProductInactive):
		return nil, invalid("product_uuid", "product is inactive")
	default:
		return nil, err
	}
}

// DeleteLine removes a line from a draft. A generated label stays printed
// and can be linked again (with_existing).
func (s *StockEntries) DeleteLine(ctx context.Context, c EntryCaller, id, lineID uuid.UUID) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if e, err = s.lockDraft(ctx, q, org, id); err != nil {
			return err
		}
		n, err := q.DeleteStockEntryLine(ctx, db.DeleteStockEntryLineParams{Uuid: lineID, EntryID: e.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrEntryLineNotFound
		}
		return nil
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// Place sets the location of lines of a draft.
func (s *StockEntries) Place(ctx context.Context, c EntryCaller, id uuid.UUID, in PlaceInput) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	code := strings.TrimSpace(in.LocationCode)
	if (in.LocationUUID == nil) == (code == "") {
		return StockEntry{}, invalid("location_uuid", "give exactly one of location_uuid or location_code")
	}
	if len(in.LineUUIDs) > 0 && len(in.Barcodes) > 0 {
		return StockEntry{}, invalid("line_uuids", "give line_uuids or barcodes, not both")
	}
	codes, err := normBarcodes(in.Barcodes, false)
	if err != nil {
		return StockEntry{}, err
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if e, err = s.lockDraft(ctx, q, org, id); err != nil {
			return err
		}
		loc, err := s.entryLocation(ctx, q, e, in.LocationUUID, code)
		if err != nil {
			return err
		}
		lines, err := q.ListStockEntryLines(ctx, e.ID)
		if err != nil {
			return err
		}
		ids, err := pickLines(ctx, q, lines, in.LineUUIDs, codes)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return ErrEntryEmpty
		}
		_, err = q.SetStockEntryLineLocation(ctx, db.SetStockEntryLineLocationParams{
			LocationID: pgtype.Int8{Int64: loc.ID, Valid: true}, Ids: ids, EntryID: e.ID,
		})
		return err
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// entryLocation resolves the target location: active, of the entry's
// warehouse, by uuid or by a scanned QR / full_code.
func (s *StockEntries) entryLocation(ctx context.Context, q *db.Queries, e db.StockEntry, id *uuid.UUID, code string) (db.WarehouseLocation, error) {
	var (
		l   db.WarehouseLocation
		err error
	)
	field := "location_uuid"
	if id != nil {
		l, err = q.GetTypedLocationByUUID(ctx, db.GetTypedLocationByUUIDParams{Uuid: *id, OrganizationID: e.OrganizationID})
	} else {
		field = "location_code"
		if utf8.RuneCountInString(code) > MaxScanLen {
			return l, invalid(field, "is too long")
		}
		full := strings.ToUpper(code)
		if rest, ok := strings.CutPrefix(full, labels.LocationPrefix); ok {
			full = strings.TrimSpace(rest)
		}
		l, err = q.GetWarehouseLocationByCode(ctx, db.GetWarehouseLocationByCodeParams{OrganizationID: e.OrganizationID, Code: full})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrLocationNotFound
	}
	if err != nil {
		return l, err
	}
	if !l.WarehouseID.Valid || l.WarehouseID.Int64 != e.WarehouseID.Int64 {
		return l, invalid(field, "location is not in the entry's warehouse")
	}
	if !l.Active {
		return l, invalid(field, "location is inactive")
	}
	return l, nil
}

// pickLines selects line ids by uuid, by barcode, or all lines.
func pickLines(ctx context.Context, q *db.Queries, lines []db.StockEntryLine, uuids []uuid.UUID, codes []string) ([]int64, error) {
	if len(uuids) == 0 && len(codes) == 0 {
		ids := make([]int64, 0, len(lines))
		for _, l := range lines {
			ids = append(ids, l.ID)
		}
		return ids, nil
	}
	byUUID := make(map[uuid.UUID]int64, len(lines))
	for _, l := range lines {
		byUUID[l.Uuid] = l.ID
	}
	ids := make([]int64, 0, len(uuids)+len(codes))
	for _, u := range uuids {
		id, ok := byUUID[u]
		if !ok {
			return nil, ErrEntryLineNotFound
		}
		ids = append(ids, id)
	}
	if len(codes) > 0 {
		byBarcode := make(map[string]int64, len(lines))
		for _, l := range lines {
			u, err := q.GetUnit(ctx, l.UnitID)
			if err != nil {
				return nil, err
			}
			byBarcode[strings.ToUpper(u.Barcode)] = l.ID
		}
		for _, code := range codes {
			c := strings.ToUpper(code)
			if rest, ok := strings.CutPrefix(c, labels.UnitPrefix); ok {
				c = strings.TrimSpace(rest)
			}
			id, ok := byBarcode[c]
			if !ok {
				return nil, invalid("barcodes", "barcode "+code+" is not on this entry")
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Confirm posts the entry and placement movements of every line in one
// transaction and closes the entry. A confirmed entry is refused
// (ErrEntryNotDraft); nothing is written twice.
func (s *StockEntries) Confirm(ctx context.Context, c EntryCaller, id uuid.UUID) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if e, err = s.lockDraft(ctx, q, org, id); err != nil {
			return err
		}
		lines, err := q.ListStockEntryLines(ctx, e.ID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return ErrEntryEmpty
		}
		for _, l := range lines {
			if !l.LocationID.Valid {
				return ErrEntryUnplaced
			}
		}
		// Printed -> entry happens at a center only (K14); the ledger
		// enforces the same rule.
		if c.Org.OrgType != rbac.OrgTypeCenter {
			return ErrEntryCenterOnly
		}
		actor := c.Principal.UserInternal
		for _, l := range lines {
			if err := s.postLine(ctx, q, tx, e, l, actor); err != nil {
				return err
			}
		}
		if e, err = q.ConfirmStockEntry(ctx, db.ConfirmStockEntryParams{UserID: c.actor(), ID: e.ID}); err != nil {
			return notFound(err, ErrEntryNotDraft)
		}
		return audit(ctx, q, c, AuditStockEntryConfirm, e, map[string]any{"lines": len(lines)})
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// postLine writes the movements of one line.
func (s *StockEntries) postLine(ctx context.Context, q *db.Queries, tx pgx.Tx, e db.StockEntry, l db.StockEntryLine, actor int64) error {
	unit, err := q.GetUnit(ctx, l.UnitID)
	if err != nil {
		return err
	}
	var actorPtr *int64
	if actor > 0 {
		actorPtr = &actor
	}
	meta := map[string]any{"stock_entry_uuid": e.Uuid.String(), "line_uuid": l.Uuid.String()}
	loc := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: l.LocationID.Int64, OrgID: e.OrganizationID}
	base := ledger.Movement{
		UnitID: unit.ID, Source: entrySource, RefType: entryRefType, RefID: l.ID,
		ActorUserID: actorPtr, Metadata: meta,
	}
	post := func(m ledger.Movement) (db.StockMovement, error) {
		res, err := s.ledger.Post(ctx, tx, m)
		if err != nil {
			return db.StockMovement{}, fmt.Errorf("%w: %s %s: %w", ErrEntryLedger, m.Type, unit.Barcode, err)
		}
		return res.Movement, nil
	}
	arg := db.SetStockEntryLineMovementsParams{ID: l.ID}
	if unit.UnitKind == ledger.KindFixed {
		m := base
		m.Type, m.To, m.Quantity, m.Reason = ledger.TypeEntry, &loc, l.Quantity, "stock entry"
		mv, err := post(m)
		if err != nil {
			return err
		}
		arg.EntryMovementID = pgtype.Int8{Int64: mv.ID, Valid: true}
		return q.SetStockEntryLineMovements(ctx, arg)
	}
	orgOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: e.OrganizationID}
	m := base
	m.Type, m.To, m.Reason = ledger.TypeEntry, &orgOwner, "stock entry"
	entry, err := post(m)
	if err != nil {
		return err
	}
	m = base
	m.Type, m.From, m.To, m.Reason = ledger.TypePlacement, &orgOwner, &loc, "stock entry placement"
	placement, err := post(m)
	if err != nil {
		return err
	}
	arg.EntryMovementID = pgtype.Int8{Int64: entry.ID, Valid: true}
	arg.PlacementMovementID = pgtype.Int8{Int64: placement.ID, Valid: true}
	return q.SetStockEntryLineMovements(ctx, arg)
}

// Cancel closes a draft without writing stock.
func (s *StockEntries) Cancel(ctx context.Context, c EntryCaller, id uuid.UUID) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if e, err = s.lockDraft(ctx, q, org, id); err != nil {
			return err
		}
		if e, err = q.CancelStockEntry(ctx, e.ID); err != nil {
			return notFound(err, ErrEntryNotDraft)
		}
		return audit(ctx, q, c, AuditStockEntryCancel, e, nil)
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// --- helpers -----------------------------------------------------------------

func (s *StockEntries) lockDraft(ctx context.Context, q *db.Queries, org int64, id uuid.UUID) (db.StockEntry, error) {
	e, err := q.LockStockEntryByUUID(ctx, db.LockStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return e, notFound(err, ErrEntryNotFound)
	}
	if e.Status != EntryStatusDraft {
		return e, ErrEntryNotDraft
	}
	return e, nil
}

func (s *StockEntries) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func normNote(raw *string) (pgtype.Text, error) {
	if raw == nil {
		return pgtype.Text{}, nil
	}
	n := strings.TrimSpace(*raw)
	if n == "" {
		return pgtype.Text{}, nil
	}
	if utf8.RuneCountInString(n) > maxNoteLen {
		return pgtype.Text{}, invalid("note", "must be at most 500 characters")
	}
	return pgtype.Text{String: n, Valid: true}, nil
}

// normBarcodes trims, deduplicates and bounds scanned barcodes.
func normBarcodes(raw []string, required bool) ([]string, error) {
	if required && len(raw) == 0 {
		return nil, invalid("barcodes", "is required")
	}
	if len(raw) > MaxEntryLinesPerCall {
		return nil, invalid("barcodes", fmt.Sprintf("must hold at most %d items", MaxEntryLinesPerCall))
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		code := strings.TrimSpace(r)
		if rest, ok := strings.CutPrefix(strings.ToUpper(code), labels.UnitPrefix); ok {
			code = strings.TrimSpace(code[len(code)-len(rest):])
		}
		if code == "" || utf8.RuneCountInString(code) > 128 || strings.ContainsAny(code, "\r\n\t") {
			return nil, invalid("barcodes", "contains an empty or invalid barcode")
		}
		if seen[code] {
			return nil, invalid("barcodes", "barcode "+code+" repeats")
		}
		seen[code] = true
		out = append(out, code)
	}
	return out, nil
}

func audit(ctx context.Context, q *db.Queries, c EntryCaller, action string, e db.StockEntry, extra map[string]any) error {
	m := map[string]any{"stock_entry_id": e.ID, "organization_id": e.OrganizationID, "status": e.Status}
	for k, v := range extra {
		m[k] = v
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: c.actor(), Action: action, Resource: AuditResourceStockEntry,
		ResourceUuid: pgtype.UUID{Bytes: e.Uuid, Valid: true}, Payload: payload,
	})
	return err
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// view builds the document; full adds the lines and label links.
func (s *StockEntries) view(ctx context.Context, q *db.Queries, e db.StockEntry, full bool) (StockEntry, error) {
	out := StockEntry{
		UUID: e.Uuid, Mode: e.Mode, Status: e.Status, Note: textPtr(e.Note),
		CreatedAt: ts(e.CreatedAt), ConfirmedAt: tsPtr(e.ConfirmedAt), CancelledAt: tsPtr(e.CancelledAt),
	}
	if e.WarehouseID.Valid {
		w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: e.WarehouseID.Int64, OrganizationID: e.OrganizationID})
		if err != nil {
			return out, err
		}
		out.Warehouse = &EntryWarehouse{UUID: w.Uuid, Code: w.Code, Name: w.Name}
	}
	if e.ImportBatchID.Valid {
		id, err := q.GetStockImportBatchUUID(ctx, e.ImportBatchID.Int64)
		if err != nil {
			return out, err
		}
		out.ImportBatchUUID = &id
	}
	if !full {
		n, err := q.CountStockEntryLines(ctx, e.ID)
		if err != nil {
			return out, err
		}
		out.LineCount = n
		return out, nil
	}
	lines, err := q.ListStockEntryLines(ctx, e.ID)
	if err != nil {
		return out, err
	}
	out.LineCount = int64(len(lines))
	out.Lines = make([]EntryLine, 0, len(lines))
	out.LabelBatches = []EntryLabelBatch{}
	products := map[int64]db.Product{}
	locations := map[int64]db.WarehouseLocation{}
	batches := map[int64]bool{}
	var placed int64
	for _, l := range lines {
		u, err := q.GetUnit(ctx, l.UnitID)
		if err != nil {
			return out, err
		}
		p, ok := products[u.ProductID]
		if !ok {
			if p, err = q.GetProductByIDAnyBrand(ctx, u.ProductID); err != nil {
				return out, err
			}
			products[u.ProductID] = p
		}
		line := EntryLine{
			UUID: l.Uuid, UnitUUID: u.Uuid, Barcode: u.Barcode, UnitKind: u.UnitKind, UnitStatus: u.Status,
			Quantity: l.Quantity, Product: EntryProduct{UUID: p.Uuid, SKU: p.Sku, Name: p.Name},
			LabelURL: stockusecase.UnitLabelPath(u.Barcode), Undone: l.UndoMovementID.Valid,
		}
		if l.LocationID.Valid {
			placed++
			loc, ok := locations[l.LocationID.Int64]
			if !ok {
				if loc, err = q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: l.LocationID.Int64, OrganizationID: e.OrganizationID}); err != nil {
					return out, err
				}
				locations[l.LocationID.Int64] = loc
			}
			line.Location = &EntryLocation{UUID: loc.Uuid, Code: loc.Code, FullCode: textPtr(loc.FullCode)}
		}
		if line.EntryMovementUUID, err = movementUUID(ctx, q, l.EntryMovementID); err != nil {
			return out, err
		}
		if line.PlacementMovementUUID, err = movementUUID(ctx, q, l.PlacementMovementID); err != nil {
			return out, err
		}
		if u.BatchID.Valid && !batches[u.BatchID.Int64] {
			batches[u.BatchID.Int64] = true
			bid, err := q.GetUnitBatchUUID(ctx, u.BatchID.Int64)
			if err != nil {
				return out, err
			}
			out.LabelBatches = append(out.LabelBatches, EntryLabelBatch{BatchUUID: bid, LabelsURL: stockusecase.BatchLabelsPath(bid)})
		}
		out.Lines = append(out.Lines, line)
	}
	out.PlacedCount = &placed
	return out, nil
}

func movementUUID(ctx context.Context, q *db.Queries, id pgtype.Int8) (*uuid.UUID, error) {
	if !id.Valid {
		return nil, nil
	}
	m, err := q.GetStockMovement(ctx, id.Int64)
	if err != nil {
		return nil, err
	}
	u := m.Uuid
	return &u, nil
}
