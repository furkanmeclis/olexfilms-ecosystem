package usecase

// TEC-1007: stock entry from a container list. Builds on the TEC-204 entry
// document and the TEC-760 Excel barcode import:
//
//   - Header: supplier, container / waybill number, arrival date and a
//     default bin. Lines added later (scan, import) land on the default bin.
//   - Arrival scan: every scanned barcode is checked against the entry.
//     A line of the entry is marked received; a barcode that is not on the
//     entry is recorded as an extra. The report lists the missing lines
//     (expected, not scanned) and the extras.
//   - Suppliers: the warehouse picks and adds suppliers of its own
//     organization (the dealer accounting screens manage the rest).
//   - Mapping templates: the column mapping of a supplier's list is saved
//     and applied to the next file.
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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Container entry errors.
var (
	ErrSupplierNotFound        = errors.New("warehouse: supplier not found")
	ErrMappingTemplateNotFound = errors.New("warehouse: mapping template not found")
)

// Audit (activity_events).
const (
	AuditStockEntryHeader = "warehouse.stock_entry.header_updated"
)

const (
	maxDeliveryRefLen  = 64
	maxTemplateNameLen = 100
	// MaxMappingColumns bounds a saved mapping.
	MaxMappingColumns = 100
)

// Scan results.
const (
	ScanMatched = "matched"
	ScanAlready = "already"
	ScanExtra   = "extra"
)

// EntrySupplier is the supplier of an entry.
type EntrySupplier struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// EntryHeaderInput is the delivery of an entry. A nil field keeps the value,
// an empty string clears it.
type EntryHeaderInput struct {
	SupplierUUID        *string
	ContainerNo         *string
	WaybillNo           *string
	ArrivalDate         *string
	DefaultLocationUUID *string
	Note                *string
}

func (in EntryHeaderInput) empty() bool {
	return in.SupplierUUID == nil && in.ContainerNo == nil && in.WaybillNo == nil &&
		in.ArrivalDate == nil && in.DefaultLocationUUID == nil && in.Note == nil
}

// UpdateHeader changes the delivery of a draft.
func (s *StockEntries) UpdateHeader(ctx context.Context, c EntryCaller, id uuid.UUID, in EntryHeaderInput) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if e, err = s.lockWritable(ctx, q, c, org, id); err != nil {
			return err
		}
		if e.Mode == EntryModeImport {
			return ErrEntryNotDraft
		}
		if e, err = s.setHeader(ctx, q, e, in); err != nil {
			return err
		}
		return audit(ctx, q, c, AuditStockEntryHeader, e, map[string]any{
			"supplier_id": e.SupplierID.Int64, "container_no": e.ContainerNo.String, "waybill_no": e.WaybillNo.String,
		})
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// setHeader validates and writes the header fields of in over e.
func (s *StockEntries) setHeader(ctx context.Context, q *db.Queries, e db.StockEntry, in EntryHeaderInput) (db.StockEntry, error) {
	arg := db.SetStockEntryHeaderParams{
		ID: e.ID, SupplierID: e.SupplierID, ContainerNo: e.ContainerNo, WaybillNo: e.WaybillNo,
		ArrivalDate: e.ArrivalDate, DefaultLocationID: e.DefaultLocationID, Note: e.Note,
	}
	if in.SupplierUUID != nil {
		arg.SupplierID = pgtype.Int8{}
		if raw := strings.TrimSpace(*in.SupplierUUID); raw != "" {
			sid, err := uuid.Parse(raw)
			if err != nil {
				return e, invalid("supplier_uuid", "must be a uuid")
			}
			sup, err := q.GetSupplierByUUID(ctx, db.GetSupplierByUUIDParams{Uuid: sid, OrganizationID: e.OrganizationID})
			if err != nil {
				return e, notFound(err, ErrSupplierNotFound)
			}
			if sup.BrandID != e.BrandID {
				return e, ErrSupplierNotFound
			}
			arg.SupplierID = pgtype.Int8{Int64: sup.ID, Valid: true}
		}
	}
	var err error
	if in.ContainerNo != nil {
		if arg.ContainerNo, err = deliveryRef("container_no", *in.ContainerNo); err != nil {
			return e, err
		}
	}
	if in.WaybillNo != nil {
		if arg.WaybillNo, err = deliveryRef("waybill_no", *in.WaybillNo); err != nil {
			return e, err
		}
	}
	if in.ArrivalDate != nil {
		arg.ArrivalDate = pgtype.Date{}
		if raw := strings.TrimSpace(*in.ArrivalDate); raw != "" {
			d, err := time.Parse(time.DateOnly, raw)
			if err != nil {
				return e, invalid("arrival_date", "must be a date (YYYY-MM-DD)")
			}
			arg.ArrivalDate = pgtype.Date{Time: d, Valid: true}
		}
	}
	if in.DefaultLocationUUID != nil {
		arg.DefaultLocationID = pgtype.Int8{}
		if raw := strings.TrimSpace(*in.DefaultLocationUUID); raw != "" {
			lid, err := uuid.Parse(raw)
			if err != nil {
				return e, invalid("default_location_uuid", "must be a uuid")
			}
			l, err := s.entryLocation(ctx, q, e, &lid, "")
			if err != nil {
				var v *ValidationError
				if errors.As(err, &v) {
					v.Field = "default_location_uuid"
				}
				return e, err
			}
			arg.DefaultLocationID = pgtype.Int8{Int64: l.ID, Valid: true}
		}
	}
	if in.Note != nil {
		if arg.Note, err = normNote(in.Note); err != nil {
			return e, err
		}
	}
	out, err := q.SetStockEntryHeader(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrEntryNotDraft
	}
	return out, err
}

func deliveryRef(field, raw string) (pgtype.Text, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return pgtype.Text{}, nil
	}
	if utf8.RuneCountInString(v) > maxDeliveryRefLen || strings.ContainsAny(v, "\r\n\t") {
		return pgtype.Text{}, invalid(field, fmt.Sprintf("must be at most %d characters on one line", maxDeliveryRefLen))
	}
	return pgtype.Text{String: v, Valid: true}, nil
}

// viewHeader fills the delivery of the document.
func (s *StockEntries) viewHeader(ctx context.Context, q *db.Queries, e db.StockEntry, out *StockEntry) error {
	out.ContainerNo, out.WaybillNo = textPtr(e.ContainerNo), textPtr(e.WaybillNo)
	if e.ArrivalDate.Valid {
		d := e.ArrivalDate.Time.Format(time.DateOnly)
		out.ArrivalDate = &d
	}
	if e.SupplierID.Valid {
		sup, err := q.GetSupplierRef(ctx, e.SupplierID.Int64)
		if err != nil {
			return err
		}
		out.Supplier = &EntrySupplier{UUID: sup.Uuid, Name: sup.Name}
	}
	if e.DefaultLocationID.Valid {
		l, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: e.DefaultLocationID.Int64, OrganizationID: e.OrganizationID})
		if err != nil {
			return err
		}
		out.DefaultLocation = &EntryLocation{UUID: l.Uuid, Code: l.Code, FullCode: textPtr(l.FullCode)}
	}
	return nil
}

func numericString(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	cm, err := ledger.NumericToCentimeters(n)
	if err != nil {
		return nil
	}
	v := fmt.Sprintf("%d.%02d", cm/100, cm%100)
	return &v
}

// --- arrival scan ---------------------------------------------------------------

// ArrivalScan is the outcome of one arrival scan.
type ArrivalScan struct {
	Result   string     `json:"result"`
	Barcode  string     `json:"barcode"`
	Line     *EntryLine `json:"line"`
	Expected int64      `json:"expected"`
	Scanned  int64      `json:"scanned"`
	Extras   int64      `json:"extras"`
}

// EntryScanExtra is a scanned barcode that is not on the entry.
type EntryScanExtra struct {
	Barcode   string        `json:"barcode"`
	Product   *EntryProduct `json:"product"`
	ScannedAt time.Time     `json:"scanned_at"`
}

// ArrivalReport compares the entry with what was scanned.
type ArrivalReport struct {
	Expected int64            `json:"expected"`
	Scanned  int64            `json:"scanned"`
	Missing  []EntryLine      `json:"missing"`
	Extras   []EntryScanExtra `json:"extras"`
}

func scanCode(raw string) (string, error) {
	code := strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(strings.ToUpper(code), labels.UnitPrefix); ok {
		code = strings.TrimSpace(code[len(code)-len(rest):])
	}
	if code == "" || utf8.RuneCountInString(code) > maxDeliveryRefLen || strings.ContainsAny(code, "\r\n\t") {
		return "", invalid("barcode", "is empty or invalid")
	}
	return code, nil
}

// Scan checks one arrived barcode against a draft.
func (s *StockEntries) Scan(ctx context.Context, c EntryCaller, id uuid.UUID, barcode string) (ArrivalScan, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return ArrivalScan{}, err
	}
	code, err := scanCode(barcode)
	if err != nil {
		return ArrivalScan{}, err
	}
	if c.Principal.UserInternal <= 0 {
		return ArrivalScan{}, ErrForbidden
	}
	res := ArrivalScan{Barcode: code}
	var lineID int64
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		e, err := s.lockWritable(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		l, err := q.FindStockEntryLineByBarcode(ctx, db.FindStockEntryLineByBarcodeParams{EntryID: e.ID, Barcode: code})
		switch {
		case err == nil:
			lineID = l.ID
			if l.ScannedAt.Valid {
				res.Result = ScanAlready
			} else {
				if _, err := q.SetStockEntryLineScanned(ctx, db.SetStockEntryLineScannedParams{ID: l.ID, UserID: pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}}); err != nil {
					return err
				}
				res.Result = ScanMatched
			}
		case errors.Is(err, pgx.ErrNoRows):
			var unit pgtype.Int8
			u, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: e.BrandID, Barcode: code})
			if err == nil {
				unit = pgtype.Int8{Int64: u.ID, Valid: true}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if _, err := q.InsertStockEntryScan(ctx, db.InsertStockEntryScanParams{EntryID: e.ID, Barcode: code, UnitID: unit, ScannedBy: c.Principal.UserInternal}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			res.Result = ScanExtra
		default:
			return err
		}
		return s.scanCounts(ctx, q, e.ID, &res)
	})
	if err != nil || lineID == 0 {
		return res, err
	}
	doc, err := s.Get(ctx, c, id)
	if err != nil {
		return res, err
	}
	for i := range doc.Lines {
		if strings.EqualFold(doc.Lines[i].Barcode, code) {
			res.Line = &doc.Lines[i]
			break
		}
	}
	return res, nil
}

// Unscan takes back one arrival scan (a line or an extra).
func (s *StockEntries) Unscan(ctx context.Context, c EntryCaller, id uuid.UUID, barcode string) (ArrivalScan, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return ArrivalScan{}, err
	}
	code, err := scanCode(barcode)
	if err != nil {
		return ArrivalScan{}, err
	}
	res := ArrivalScan{Barcode: code, Result: "unscanned"}
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		e, err := s.lockWritable(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		l, err := q.FindStockEntryLineByBarcode(ctx, db.FindStockEntryLineByBarcodeParams{EntryID: e.ID, Barcode: code})
		if err == nil {
			if _, err := q.ClearStockEntryLineScan(ctx, l.ID); err != nil {
				return err
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			if _, err := q.DeleteStockEntryScan(ctx, db.DeleteStockEntryScanParams{EntryID: e.ID, Barcode: code}); err != nil {
				return err
			}
		} else {
			return err
		}
		return s.scanCounts(ctx, q, e.ID, &res)
	})
	return res, err
}

func (s *StockEntries) scanCounts(ctx context.Context, q *db.Queries, entryID int64, res *ArrivalScan) error {
	n, err := q.CountStockEntryScanned(ctx, entryID)
	if err != nil {
		return err
	}
	extras, err := q.ListStockEntryScans(ctx, entryID)
	if err != nil {
		return err
	}
	res.Expected, res.Scanned, res.Extras = n.Expected, n.Scanned, int64(len(extras))
	return nil
}

// Report lists the missing lines and the extras of the arrival scan.
func (s *StockEntries) Report(ctx context.Context, c EntryCaller, id uuid.UUID) (ArrivalReport, error) {
	doc, err := s.Get(ctx, c, id)
	if err != nil {
		return ArrivalReport{}, err
	}
	org, _ := guard(c.Caller)
	e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return ArrivalReport{}, notFound(err, ErrEntryNotFound)
	}
	out := ArrivalReport{Missing: []EntryLine{}, Extras: []EntryScanExtra{}, Expected: int64(len(doc.Lines))}
	for _, l := range doc.Lines {
		if l.ScannedAt != nil {
			out.Scanned++
		} else {
			out.Missing = append(out.Missing, l)
		}
	}
	scans, err := s.q.ListStockEntryScans(ctx, e.ID)
	if err != nil {
		return out, err
	}
	for _, sc := range scans {
		x := EntryScanExtra{Barcode: sc.Barcode, ScannedAt: ts(sc.CreatedAt)}
		if sc.UnitID.Valid {
			u, err := s.q.GetUnit(ctx, sc.UnitID.Int64)
			if err != nil {
				return out, err
			}
			p, err := s.q.GetProductByIDAnyBrand(ctx, u.ProductID)
			if err != nil {
				return out, err
			}
			x.Product = &EntryProduct{UUID: p.Uuid, SKU: p.Sku, Name: p.Name}
		}
		out.Extras = append(out.Extras, x)
	}
	return out, nil
}

// --- suppliers ------------------------------------------------------------------

// WarehouseSupplier is a supplier a container entry can name.
type WarehouseSupplier struct {
	UUID   uuid.UUID `json:"uuid"`
	Name   string    `json:"name"`
	TaxNo  *string   `json:"tax_no"`
	Active bool      `json:"active"`
}

// Suppliers lists the suppliers of the active organization.
func (s *StockEntries) Suppliers(ctx context.Context, c EntryCaller, query string, limit, offset int32) ([]WarehouseSupplier, int64, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, 0, err
	}
	var q pgtype.Text
	if v := strings.TrimSpace(query); v != "" {
		q = pgtype.Text{String: v, Valid: true}
	}
	active := pgtype.Bool{Bool: true, Valid: true}
	rows, err := s.q.ListSuppliers(ctx, db.ListSuppliersParams{OrganizationID: org, Active: active, Q: q, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountSuppliers(ctx, db.CountSuppliersParams{OrganizationID: org, Active: active, Q: q})
	if err != nil {
		return nil, 0, err
	}
	out := make([]WarehouseSupplier, 0, len(rows))
	for _, r := range rows {
		out = append(out, WarehouseSupplier{UUID: r.Uuid, Name: r.Name, TaxNo: textPtr(r.TaxNo), Active: r.Active})
	}
	return out, total, nil
}

// SupplierInput adds a supplier from the entry form.
type SupplierInput struct {
	Name  string
	TaxNo *string
}

// CreateSupplier adds a supplier to the active organization.
func (s *StockEntries) CreateSupplier(ctx context.Context, c EntryCaller, in SupplierInput) (WarehouseSupplier, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseSupplier{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return WarehouseSupplier{}, invalid("name", "is required (at most 200 characters)")
	}
	var tax pgtype.Text
	if in.TaxNo != nil {
		if tax, err = deliveryRef("tax_no", *in.TaxNo); err != nil {
			return WarehouseSupplier{}, err
		}
	}
	r, err := s.q.CreateSupplier(ctx, db.CreateSupplierParams{OrganizationID: org, BrandID: c.Org.BrandID, Name: name, TaxNo: tax})
	if err != nil {
		return WarehouseSupplier{}, err
	}
	return WarehouseSupplier{UUID: r.Uuid, Name: r.Name, TaxNo: textPtr(r.TaxNo), Active: r.Active}, nil
}

// --- mapping templates ----------------------------------------------------------

// MappingTemplate is a saved column mapping (source header -> field).
type MappingTemplate struct {
	UUID      uuid.UUID         `json:"uuid"`
	Name      string            `json:"name"`
	Supplier  *EntrySupplier    `json:"supplier"`
	Mapping   map[string]string `json:"mapping"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// MappingTemplateInput saves a mapping under a name (same name replaces).
type MappingTemplateInput struct {
	Name         string
	SupplierUUID *string
	Mapping      map[string]string
}

// MappingTemplates lists the saved mappings of the entry import.
func (s *StockEntries) MappingTemplates(ctx context.Context, c EntryCaller) ([]MappingTemplate, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListImportMappingTemplates(ctx, db.ListImportMappingTemplatesParams{OrganizationID: org, Resource: EntryBarcodeImportResource})
	if err != nil {
		return nil, err
	}
	out := make([]MappingTemplate, 0, len(rows))
	for _, r := range rows {
		t := MappingTemplate{UUID: r.Uuid, Name: r.Name, Mapping: map[string]string{}, UpdatedAt: ts(r.UpdatedAt)}
		if err := json.Unmarshal(r.Mapping, &t.Mapping); err != nil {
			return nil, err
		}
		if r.SupplierUuid.Valid {
			t.Supplier = &EntrySupplier{UUID: r.SupplierUuid.Bytes, Name: r.SupplierName.String}
		}
		out = append(out, t)
	}
	return out, nil
}

// SaveMappingTemplate stores a mapping of the entry import.
func (s *StockEntries) SaveMappingTemplate(ctx context.Context, c EntryCaller, in MappingTemplateInput) (MappingTemplate, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return MappingTemplate{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > maxTemplateNameLen {
		return MappingTemplate{}, invalid("name", fmt.Sprintf("is required (at most %d characters)", maxTemplateNameLen))
	}
	fields := map[string]bool{}
	for _, f := range NewEntryBarcodeImporter(s).ImportSchema() {
		fields[f.Key] = true
	}
	mapping := map[string]string{}
	for header, field := range in.Mapping {
		h := strings.TrimSpace(header)
		if h == "" || field == "" {
			continue
		}
		if !fields[field] {
			return MappingTemplate{}, invalid("mapping", "unknown field "+field)
		}
		mapping[h] = field
	}
	if len(mapping) == 0 || len(mapping) > MaxMappingColumns {
		return MappingTemplate{}, invalid("mapping", fmt.Sprintf("must map 1-%d columns", MaxMappingColumns))
	}
	var supplier pgtype.Int8
	var ref *EntrySupplier
	if in.SupplierUUID != nil && strings.TrimSpace(*in.SupplierUUID) != "" {
		sid, err := uuid.Parse(strings.TrimSpace(*in.SupplierUUID))
		if err != nil {
			return MappingTemplate{}, invalid("supplier_uuid", "must be a uuid")
		}
		sup, err := s.q.GetSupplierByUUID(ctx, db.GetSupplierByUUIDParams{Uuid: sid, OrganizationID: org})
		if err != nil {
			return MappingTemplate{}, notFound(err, ErrSupplierNotFound)
		}
		supplier = pgtype.Int8{Int64: sup.ID, Valid: true}
		ref = &EntrySupplier{UUID: sup.Uuid, Name: sup.Name}
	}
	raw, err := json.Marshal(mapping)
	if err != nil {
		return MappingTemplate{}, err
	}
	r, err := s.q.CreateImportMappingTemplate(ctx, db.CreateImportMappingTemplateParams{
		OrganizationID: org, Resource: EntryBarcodeImportResource, Name: name, SupplierID: supplier,
		Mapping: raw, CreatedBy: c.actor(),
	})
	if err != nil {
		return MappingTemplate{}, err
	}
	return MappingTemplate{UUID: r.Uuid, Name: r.Name, Supplier: ref, Mapping: mapping, UpdatedAt: ts(r.UpdatedAt)}, nil
}

// DeleteMappingTemplate removes a saved mapping.
func (s *StockEntries) DeleteMappingTemplate(ctx context.Context, c EntryCaller, id uuid.UUID) error {
	org, err := guard(c.Caller)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteImportMappingTemplate(ctx, db.DeleteImportMappingTemplateParams{Uuid: id, OrganizationID: org, Resource: EntryBarcodeImportResource})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMappingTemplateNotFound
	}
	return nil
}
