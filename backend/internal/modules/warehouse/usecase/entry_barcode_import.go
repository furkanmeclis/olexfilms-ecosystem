package usecase

// TEC-760: record-scoped barcode import into a with_existing draft entry.
// Applying adds draft lines; the normal entry confirmation posts ledger
// movements. A staged job is bound to an entry before its preview is used.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const EntryBarcodeImportResource = "tenant.warehouse.entry_barcodes"

type entryImportContextKey struct{}

func EntryImportContext(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, entryImportContextKey{}, id)
}

type EntryBarcodeImporter struct{ entries *StockEntries }

func NewEntryBarcodeImporter(entries *StockEntries) *EntryBarcodeImporter {
	return &EntryBarcodeImporter{entries: entries}
}
func (im *EntryBarcodeImporter) Resource() string                 { return EntryBarcodeImportResource }
func (im *EntryBarcodeImporter) ExportColumns() []ioengine.Column { return nil }
func (im *EntryBarcodeImporter) Export(context.Context, ioengine.ExportQuery, i18n.Locale) (ioengine.Dataset, error) {
	return ioengine.Dataset{}, ioengine.ErrImportRejected
}
func (im *EntryBarcodeImporter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, ioengine.ErrImportRejected
}
func (im *EntryBarcodeImporter) RevertRow(context.Context, string, string, map[string]any) error {
	return ioengine.ErrImportRejected
}
func (im *EntryBarcodeImporter) ImportPermission() string { return rbac.PermWarehouseWrite }
func (im *EntryBarcodeImporter) MaxImportRows() int       { return MaxEntryLines }
func (im *EntryBarcodeImporter) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "product_name", LabelKey: "entry_import.product_name", Type: ioengine.ColumnTypeString, Kind: ioengine.FieldText, HintKey: "entry_import.hint.product_name"},
		{Key: "product_sku", LabelKey: "entry_import.product_sku", Type: ioengine.ColumnTypeString, Kind: ioengine.FieldCode, HintKey: "entry_import.hint.product_sku"},
		{Key: "barcode", LabelKey: "entry_import.barcode", Type: ioengine.ColumnTypeString, Kind: ioengine.FieldCode, Required: true, HintKey: "entry_import.hint.barcode"},
		{Key: "location_code", LabelKey: "entry_import.location_code", Type: ioengine.ColumnTypeString, Kind: ioengine.FieldCode, HintKey: "entry_import.hint.location_code"},
	}
}
func (im *EntryBarcodeImporter) SampleRows() []map[string]any {
	return []map[string]any{{"product_name": "", "product_sku": "PPF-GLOSS", "barcode": "OLX-000001", "location_code": "A-01"}}
}

func (im *EntryBarcodeImporter) SampleLookups(ctx context.Context) (map[string][]string, error) {
	scope, ok := orgctx.ScopeFrom(ctx)
	if !ok {
		return nil, ioengine.ErrImportRejected
	}
	id, ok := ctx.Value(entryImportContextKey{}).(uuid.UUID)
	if !ok {
		return nil, ioengine.ErrImportRejected
	}
	var result map[string][]string
	err := im.entries.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		e, err := im.entries.lockDraft(ctx, q, scope.InternalID, id)
		if err != nil {
			return err
		}
		if e.Mode != EntryModeWithExisting {
			return ioengine.ErrImportRejected
		}
		skus, err := q.ListActiveProductSKUsByBrand(ctx, e.BrandID)
		if err != nil {
			return err
		}
		locations, err := tx.Query(ctx, `SELECT full_code FROM warehouse_locations WHERE warehouse_id=$1 AND type='bin' AND active ORDER BY full_code`, e.WarehouseID.Int64)
		if err != nil {
			return err
		}
		defer locations.Close()
		codes := []string{}
		for locations.Next() {
			var code string
			if err := locations.Scan(&code); err != nil {
				return err
			}
			codes = append(codes, code)
		}
		if err := locations.Err(); err != nil {
			return err
		}
		result = map[string][]string{"product_sku": skus, "location_code": codes}
		return nil
	})
	return result, err
}

// Bind links a newly uploaded job to one draft entry. The upload route checks
// this before accepting the job; Stage and Apply check it again under a lock.
func (im *EntryBarcodeImporter) Bind(ctx context.Context, jobUUID uuid.UUID, entryUUID uuid.UUID, orgID int64, policy string) error {
	if policy == "" {
		policy = "none"
	}
	if policy != "none" && policy != "all_existing" && policy != "conflicts_only" {
		return fmt.Errorf("%w: existing_policy", ioengine.ErrImportRejected)
	}
	return im.entries.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var jobID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM import_jobs WHERE uuid=$1 AND organization_id=$2 AND resource=$3 FOR UPDATE`, jobUUID, orgID, EntryBarcodeImportResource).Scan(&jobID); err != nil {
			return fmt.Errorf("%w: import job missing", ioengine.ErrImportRejected)
		}
		e, err := im.entries.lockDraft(ctx, q, orgID, entryUUID)
		if err != nil {
			return err
		}
		if e.Mode != EntryModeWithExisting {
			return fmt.Errorf("%w: entry mode", ioengine.ErrImportRejected)
		}
		return q.CreateEntryBarcodeImport(ctx, db.CreateEntryBarcodeImportParams{JobID: jobID, EntryID: e.ID, OrganizationID: e.OrganizationID, BrandID: e.BrandID, ExistingPolicy: policy})
	})
}

type entryImportRow struct {
	Index      int            `json:"index"`
	Data       map[string]any `json:"data"`
	Barcode    string         `json:"barcode"`
	ProductID  int64          `json:"product_id"`
	LocationID int64          `json:"location_id"`
	Status     string         `json:"status"`
	Errors     []string       `json:"errors"`
	Target     map[string]any `json:"target"`
	UnitID     int64          `json:"unit_id,omitempty"`
	LineID     int64          `json:"line_id,omitempty"`
	Created    bool           `json:"created,omitempty"`
	UnitStatus string         `json:"unit_status,omitempty"`
}
type entryImportBatch struct {
	EntryID, OrganizationID, BrandID int64
	Policy, Status                   string
	Rows                             []entryImportRow
}

func (im *EntryBarcodeImporter) batch(ctx context.Context, q *db.Queries, job ioengine.ImportJob, lock bool) (entryImportBatch, error) {
	b := entryImportBatch{}
	var row db.EntryBarcodeImport
	var err error
	if lock {
		row, err = q.LockEntryBarcodeImport(ctx, job.ID)
	} else {
		row, err = q.GetEntryBarcodeImport(ctx, job.ID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return b, fmt.Errorf("%w: import is not bound to an entry", ioengine.ErrImportRejected)
	}
	if err != nil {
		return b, err
	}
	b.EntryID, b.OrganizationID, b.BrandID, b.Policy, b.Status = row.EntryID, row.OrganizationID, row.BrandID, row.ExistingPolicy, row.Status
	if b.OrganizationID != job.OrganizationID {
		return b, ioengine.ErrImportRejected
	}
	if err := json.Unmarshal(row.Rows, &b.Rows); err != nil {
		return b, err
	}
	return b, nil
}
func (im *EntryBarcodeImporter) draft(ctx context.Context, tx pgx.Tx, q *db.Queries, b entryImportBatch) (db.StockEntry, error) {
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT uuid FROM stock_entries WHERE id=$1 AND organization_id=$2 AND brand_id=$3`, b.EntryID, b.OrganizationID, b.BrandID).Scan(&id); err != nil {
		return db.StockEntry{}, fmt.Errorf("%w: entry missing", ioengine.ErrImportRejected)
	}
	e, err := im.entries.lockDraft(ctx, q, b.OrganizationID, id)
	if err != nil {
		return e, fmt.Errorf("%w: entry is not draft", ioengine.ErrImportRejected)
	}
	if e.Mode != EntryModeWithExisting {
		return e, ioengine.ErrImportRejected
	}
	return e, nil
}
func entryCell(row map[string]any, key string) string {
	v := row[key]
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
func entryImportError(loc i18n.Locale, code string) string {
	return i18n.Translate(loc, "entry_import.error."+code)
}
func entrySummary(rows []entryImportRow, locale string) ioengine.PreviewSummary {
	loc := i18n.Normalize(locale)
	s := ioengine.PreviewSummary{Total: len(rows), Counts: map[string]int{}, Rows: []ioengine.RowPreview{}, Errors: []ioengine.RowError{}}
	for _, r := range rows {
		s.Counts[r.Status]++
		if r.Status == "new" || r.Status == "link" || r.Status == "applied" {
			s.Valid++
			if r.LocationID != 0 {
				s.Counts["placed"]++
			}
		} else {
			s.Invalid++
		}
		if len(s.Rows) < 1000 {
			s.Rows = append(s.Rows, ioengine.RowPreview{Index: r.Index, Data: r.Data, Status: r.Status, StatusLabel: i18n.Translate(loc, "entry_import.status."+r.Status), Target: r.Target})
		}
		for _, code := range r.Errors {
			s.Errors = append(s.Errors, ioengine.RowError{Index: r.Index, Code: "ENTRY_IMPORT_" + strings.ToUpper(code), Error: entryImportError(loc, code)})
		}
	}
	return s
}
func (im *EntryBarcodeImporter) save(ctx context.Context, q *db.Queries, jobID int64, status string, rows []entryImportRow) error {
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return q.SetEntryBarcodeImport(ctx, db.SetEntryBarcodeImportParams{JobID: jobID, Status: status, Rows: raw})
}

type importProduct struct {
	ID              int64
	SKU, Name, Kind string
	Fixed           bool
}
type importLocation struct {
	ID   int64
	Code string
}
type importUnit struct {
	ID, ProductID     int64
	Status, Kind      string
	HasState, HasOpen bool
}

func (im *EntryBarcodeImporter) Stage(ctx context.Context, job ioengine.ImportJob, raw []map[string]any, defaults map[string]any) (ioengine.PreviewSummary, error) {
	var summary ioengine.PreviewSummary
	err := im.entries.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		b, err := im.batch(ctx, q, job, true)
		if err != nil {
			return err
		}
		if b.Status != "draft" && b.Status != "validated" {
			return ioengine.ErrImportRejected
		}
		e, err := im.draft(ctx, tx, q, b)
		if err != nil {
			return err
		}
		have, err := q.CountStockEntryLines(ctx, e.ID)
		if err != nil {
			return err
		}
		organization, err := q.GetOrganizationByID(ctx, b.OrganizationID)
		if err != nil {
			return err
		}
		center := organization.Type == "center"
		products, err := tx.Query(ctx, `SELECT id,sku,name,unit_type,uses_fixed_barcode FROM products WHERE brand_id=$1 AND active`, b.BrandID)
		if err != nil {
			return err
		}
		bySKU := map[string]importProduct{}
		byName := map[string][]importProduct{}
		for products.Next() {
			var p importProduct
			if err := products.Scan(&p.ID, &p.SKU, &p.Name, &p.Kind, &p.Fixed); err != nil {
				products.Close()
				return err
			}
			bySKU[strings.ToUpper(p.SKU)] = p
			name := strings.ToUpper(strings.TrimSpace(p.Name))
			byName[name] = append(byName[name], p)
		}
		err = products.Err()
		products.Close()
		if err != nil {
			return err
		}
		locs, err := tx.Query(ctx, `SELECT id,full_code,code FROM warehouse_locations WHERE warehouse_id=$1 AND organization_id=$2 AND type='bin' AND active`, e.WarehouseID.Int64, b.OrganizationID)
		if err != nil {
			return err
		}
		byCode := map[string][]importLocation{}
		for locs.Next() {
			var l importLocation
			var short string
			if err := locs.Scan(&l.ID, &l.Code, &short); err != nil {
				locs.Close()
				return err
			}
			byCode[strings.ToUpper(l.Code)] = append(byCode[strings.ToUpper(l.Code)], l)
			if short != l.Code {
				byCode[strings.ToUpper(short)] = append(byCode[strings.ToUpper(short)], l)
			}
		}
		err = locs.Err()
		locs.Close()
		if err != nil {
			return err
		}
		codes := make([]string, 0, len(raw))
		for _, v := range raw {
			code := entryCell(v, "barcode")
			if code != "" {
				codes = append(codes, code, strings.ToUpper(code))
			}
		}
		units := map[string]importUnit{}
		if len(codes) > 0 {
			found, err := tx.Query(ctx, `SELECT u.id,u.barcode,u.product_id,u.status,u.unit_kind,
     EXISTS(SELECT 1 FROM unit_current_state s WHERE s.unit_id=u.id),
     EXISTS(SELECT 1 FROM stock_entry_lines l JOIN stock_entries e ON e.id=l.entry_id WHERE l.unit_id=u.id AND (e.status='draft' OR (e.status='confirmed' AND u.unit_kind<>'fixed')))
     FROM units u WHERE u.brand_id=$1 AND u.barcode=ANY($2::text[])`, b.BrandID, codes)
			if err != nil {
				return err
			}
			for found.Next() {
				var u importUnit
				var barcode string
				if err := found.Scan(&u.ID, &barcode, &u.ProductID, &u.Status, &u.Kind, &u.HasState, &u.HasOpen); err != nil {
					found.Close()
					return err
				}
				units[barcode] = u
				units[strings.ToUpper(barcode)] = u
			}
			err = found.Err()
			found.Close()
			if err != nil {
				return err
			}
		}
		seen := map[string]int{}
		rows := make([]entryImportRow, 0, len(raw))
		for idx, data := range raw {
			barcode := entryCell(data, "barcode")
			sku := entryCell(data, "product_sku")
			name := entryCell(data, "product_name")
			locCode := entryCell(data, "location_code")
			r := entryImportRow{Index: idx + 1, Data: data, Barcode: barcode, Status: "invalid", Errors: []string{}, Target: map[string]any{}}
			if barcode == "" {
				r.Errors = append(r.Errors, "barcode_required")
			} else if utf8.RuneCountInString(barcode) > 64 {
				r.Errors = append(r.Errors, "barcode_too_long")
			}
			if sku == "" && name == "" {
				r.Errors = append(r.Errors, "product_required")
			}
			var p importProduct
			if sku != "" {
				p = bySKU[strings.ToUpper(sku)]
				if p.ID == 0 {
					r.Errors = append(r.Errors, "product_not_found")
				} else if name != "" && !strings.EqualFold(strings.TrimSpace(p.Name), name) {
					r.Errors = append(r.Errors, "product_name_mismatch")
				}
			}
			if sku == "" && name != "" {
				matches := byName[strings.ToUpper(name)]
				if len(matches) == 0 {
					r.Errors = append(r.Errors, "product_not_found")
				} else if len(matches) > 1 {
					r.Errors = append(r.Errors, "product_ambiguous")
				} else {
					p = matches[0]
				}
			}
			if p.ID > 0 {
				r.ProductID = p.ID
				r.Target["product_name"] = p.Name
				r.Target["product_sku"] = p.SKU
			}
			if locCode != "" {
				matches := byCode[strings.ToUpper(locCode)]
				if len(matches) == 0 {
					r.Errors = append(r.Errors, "location_not_found")
				} else if len(matches) > 1 {
					r.Errors = append(r.Errors, "location_ambiguous")
				} else {
					r.LocationID = matches[0].ID
					r.Target["location_code"] = matches[0].Code
				}
			}
			norm := strings.ToUpper(barcode)
			if barcode != "" {
				if first := seen[norm]; first > 0 {
					r.Status = "duplicate"
					r.Errors = append(r.Errors, "duplicate_in_file")
				} else {
					seen[norm] = idx + 1
				}
			}
			if len(r.Errors) == 0 {
				if u, exists := units[barcode]; exists {
					r.Status = classifyEntryImportUnit(u, p, b.Policy, &r)
				} else if u, exists := units[norm]; exists {
					r.Status = classifyEntryImportUnit(u, p, b.Policy, &r)
				} else if p.Kind == "roll_meter" && !p.Fixed {
					r.Status = "invalid"
					r.Errors = append(r.Errors, "roll_meters_required")
				} else if !center {
					r.Status = "invalid"
					r.Errors = append(r.Errors, "center_only")
				} else {
					r.Status = "new"
				}
			}
			rows = append(rows, r)
		}
		importable := int64(0)
		for _, r := range rows {
			if r.Status == "new" || r.Status == "link" {
				importable++
			}
		}
		if have+importable > MaxEntryLines {
			return fmt.Errorf("%w: entry has at most 2000 lines", ioengine.ErrImportRejected)
		}
		if err := im.save(ctx, q, job.ID, "validated", rows); err != nil {
			return err
		}
		summary = entrySummary(rows, job.Locale)
		return nil
	})
	summary.BatchUUID = job.UUID.String()
	return summary, err
}
func classifyEntryImportUnit(u importUnit, p importProduct, policy string, r *entryImportRow) string {
	conflict := ""
	enterableStatus := u.Status == "printed" || u.Status == "reserved" || (u.Kind == "fixed" && u.Status == "available")
	if u.ProductID != p.ID {
		conflict = "product_mismatch"
	} else if !enterableStatus || u.HasOpen || (u.Kind != "fixed" && u.HasState) {
		conflict = "unit_not_enterable"
	}
	if policy == "all_existing" || (policy == "conflicts_only" && conflict != "") {
		r.Errors = append(r.Errors, "existing_skipped")
		return "duplicate"
	}
	if conflict != "" {
		r.Errors = append(r.Errors, conflict)
		return "conflict"
	}
	return "link"
}
func (im *EntryBarcodeImporter) Apply(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	var summary ioengine.PreviewSummary
	err := im.entries.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		b, err := im.batch(ctx, q, job, true)
		if err != nil {
			return err
		}
		if b.Status == "applied" || b.Status == "undone" {
			summary = entrySummary(b.Rows, job.Locale)
			return nil
		}
		if b.Status != "validated" {
			return ioengine.ErrImportRejected
		}
		e, err := im.draft(ctx, tx, q, b)
		if err != nil {
			return err
		}
		have, err := q.CountStockEntryLines(ctx, e.ID)
		if err != nil {
			return err
		}
		n := 0
		for _, r := range b.Rows {
			if r.Status == "new" || r.Status == "link" {
				n++
			}
		}
		if have+int64(n) > MaxEntryLines {
			return ioengine.ErrImportRejected
		}
		org, err := q.GetOrganizationByID(ctx, b.OrganizationID)
		if err != nil {
			return err
		}
		for idx, r := range b.Rows {
			if r.Status != "new" && r.Status != "link" {
				continue
			}
			p, err := q.GetProduct(ctx, db.GetProductParams{ID: r.ProductID, BrandID: b.BrandID})
			if err != nil || !p.Active {
				return ioengine.ErrImportRejected
			}
			if r.LocationID != 0 {
				var valid bool
				err = tx.QueryRow(ctx, `SELECT active AND type='bin' AND warehouse_id=$2 FROM warehouse_locations WHERE id=$1`, r.LocationID, e.WarehouseID.Int64).Scan(&valid)
				if err != nil || !valid {
					return ioengine.ErrImportRejected
				}
			}
			var u db.Unit
			if r.Status == "new" {
				if org.Type != "center" {
					return ioengine.ErrImportRejected
				}
				var taken bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM units WHERE brand_id=$1 AND upper(barcode)=upper($2))`, b.BrandID, r.Barcode).Scan(&taken); err != nil {
					return err
				}
				if taken {
					return ioengine.ErrImportRejected
				}
				kind := "serial"
				if p.UsesFixedBarcode {
					kind = "fixed"
				}
				if p.UnitType == "roll_meter" && !p.UsesFixedBarcode {
					return ioengine.ErrImportRejected
				}
				u, err = q.CreateUnit(ctx, db.CreateUnitParams{OrganizationID: b.OrganizationID, BrandID: b.BrandID, ProductID: p.ID, Barcode: r.Barcode, UnitKind: kind, Source: "imported", Status: "printed"})
				if err != nil {
					return err
				}
			} else {
				u, err = q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: b.BrandID, Barcode: r.Barcode})
				if errors.Is(err, pgx.ErrNoRows) {
					u, err = q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: b.BrandID, Barcode: strings.ToUpper(r.Barcode)})
				}
				if err != nil {
					return ioengine.ErrImportRejected
				}
				u, err = q.LockUnit(ctx, u.ID)
				if err != nil {
					return err
				}
				if u.ProductID != p.ID {
					return ioengine.ErrImportRejected
				}
				if err := enterable(ctx, q, u); err != nil {
					return ioengine.ErrImportRejected
				}
			}
			arg := db.InsertStockEntryLineParams{EntryID: e.ID, UnitID: u.ID, Quantity: 1}
			if r.LocationID != 0 {
				arg.LocationID = pgtype.Int8{Int64: r.LocationID, Valid: true}
			}
			line, err := q.InsertStockEntryLine(ctx, arg)
			if err != nil {
				return err
			}
			b.Rows[idx].UnitID = u.ID
			b.Rows[idx].LineID = line.ID
			b.Rows[idx].Created = r.Status == "new"
			b.Rows[idx].UnitStatus = u.Status
			b.Rows[idx].Status = "applied"
		}
		if err := im.save(ctx, q, job.ID, "applied", b.Rows); err != nil {
			return err
		}
		summary = entrySummary(b.Rows, job.Locale)
		return nil
	})
	summary.BatchUUID = job.UUID.String()
	return summary, err
}
func (im *EntryBarcodeImporter) Undo(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	var summary ioengine.PreviewSummary
	err := im.entries.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		b, err := im.batch(ctx, q, job, true)
		if err != nil {
			return err
		}
		if b.Status == "undone" {
			summary = entrySummary(b.Rows, job.Locale)
			return nil
		}
		if b.Status != "applied" {
			return ioengine.ErrImportRejected
		}
		var entryStatus string
		err = tx.QueryRow(ctx, `SELECT status FROM stock_entries WHERE id=$1 FOR UPDATE`, b.EntryID).Scan(&entryStatus)
		if err != nil {
			return err
		}
		for idx, r := range b.Rows {
			if r.Status != "applied" {
				continue
			}
			var lineExists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stock_entry_lines l JOIN units u ON u.id=l.unit_id WHERE l.id=$1 AND l.entry_id=$2 AND l.unit_id=$3 AND l.entry_movement_id IS NULL AND l.placement_movement_id IS NULL AND l.location_id IS NOT DISTINCT FROM NULLIF($4::bigint,0) AND l.quantity=1 AND u.status=$5)`, r.LineID, b.EntryID, r.UnitID, r.LocationID, r.UnitStatus).Scan(&lineExists)
			if err != nil {
				return err
			}
			movements, err := q.CountStockMovementsByUnit(ctx, r.UnitID)
			if err != nil {
				return err
			}
			if entryStatus != "draft" || !lineExists || movements > 0 {
				b.Rows[idx].Status = "undo_rejected"
				b.Rows[idx].Errors = []string{"undo_touched"}
				continue
			}
			if _, err := tx.Exec(ctx, `DELETE FROM stock_entry_lines WHERE id=$1 AND entry_id=$2`, r.LineID, b.EntryID); err != nil {
				return err
			}
			if r.Created {
				if _, err := tx.Exec(ctx, `DELETE FROM units WHERE id=$1`, r.UnitID); err != nil {
					return err
				}
			}
			b.Rows[idx].Status = "undone"
		}
		if err := im.save(ctx, q, job.ID, "undone", b.Rows); err != nil {
			return err
		}
		summary = entrySummary(b.Rows, job.Locale)
		return nil
	})
	summary.BatchUUID = job.UUID.String()
	return summary, err
}
func (im *EntryBarcodeImporter) ReportRows(ctx context.Context, job ioengine.ImportJob) ([]ioengine.RowReport, error) {
	var reports []ioengine.RowReport
	err := im.entries.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		b, err := im.batch(ctx, q, job, false)
		if err != nil {
			return err
		}
		loc := i18n.Normalize(job.Locale)
		for _, r := range b.Rows {
			v := ioengine.RowReport{Index: r.Index, Status: r.Status, StatusLabel: i18n.Translate(loc, "entry_import.status."+r.Status), Failed: len(r.Errors) > 0}
			for _, code := range r.Errors {
				v.Errors = append(v.Errors, entryImportError(loc, code))
			}
			reports = append(reports, v)
		}
		return nil
	})
	return reports, err
}

var _ ioengine.ResourceAdapter = (*EntryBarcodeImporter)(nil)
var _ ioengine.StagedImporter = (*EntryBarcodeImporter)(nil)
var _ ioengine.SampleLookuper = (*EntryBarcodeImporter)(nil)
var _ ioengine.RowLimiter = (*EntryBarcodeImporter)(nil)
var _ ioengine.ImportPermissioner = (*EntryBarcodeImporter)(nil)
var _ ioengine.RowReporter = (*EntryBarcodeImporter)(nil)
