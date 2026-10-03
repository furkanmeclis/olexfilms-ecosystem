package usecase

// Safe bulk stock import (TEC-158, F1-02f). The import runs on the ioengine
// import flow (upload -> preview -> confirm -> rollback on
// /v1/tenant/imports/{uuid}, apply in the worker-core import queue); this
// adapter implements ioengine.StagedImporter:
//
//   - Stage (preview, dry run): the mapped rows are parsed, classified as
//     new / duplicate / invalid / conflict with their target (organization,
//     location, product, unit kind) and saved to stock_import_rows of the
//     job's batch (stock_import_batches, uuid = job uuid). Nothing else is
//     written; staging again replaces the rows.
//   - Apply (confirm): in one transaction every new row creates its unit
//     (source imported) and posts an entry movement through ledger.Post.
//     The idempotency key is import:stock_import_row:<row id>:entry:<barcode>
//     (batch + row), and an applied batch is never applied again, so a
//     second confirm writes no movement. The same transaction records the
//     batch as a confirmed stock entry document (TEC-204, mode import, one
//     line per applied row); the undo marks its lines (and, when every row
//     is undone, the entry) undone.
//   - Undo (rollback): in one transaction every applied row whose unit saw
//     no other movement is reversed with a void movement through
//     ledger.Post (key import_undo:stock_import_row:<row id>:void:<barcode>);
//     a unit that moved since is refused row by row and reported.
//
// Projections are never written here: ledger.Post keeps them, so rebuild and
// replay (TEC-156) find no drift after an import or an undo. Units are
// issued by the brand center only (K14), so the job must belong to a center.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ImportResource is the ioengine resource of the stock import.
const ImportResource = "tenant.stock.units"

// Idempotency key parts of the import movements.
const (
	importSource     = "import"
	importUndoSource = "import_undo"
	importRefType    = "stock_import_row"
)

// previewRowLimit caps the rows copied into the job summary.
const previewRowLimit = 1000

// ErrImportCenterOnly: only the brand center imports units (K14).
var ErrImportCenterOnly = fmt.Errorf("%w: only the brand center imports stock", ioengine.ErrImportRejected)

// Importer is the stock import adapter (ioengine.ResourceAdapter and
// ioengine.StagedImporter).
type Importer struct {
	pool   TxBeginner
	q      *db.Queries
	ledger *ledger.Ledger
}

// NewImporter creates the stock import adapter.
func NewImporter(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Importer {
	return &Importer{pool: pool, q: q, ledger: ledger.New(q, out)}
}

var (
	_ ioengine.ResourceAdapter = (*Importer)(nil)
	_ ioengine.StagedImporter  = (*Importer)(nil)
)

// Resource implements ioengine.ResourceAdapter.
func (im *Importer) Resource() string { return ImportResource }

// ExportColumns implements ioengine.ResourceAdapter (import only).
func (im *Importer) ExportColumns() []ioengine.Column { return nil }

// Export implements ioengine.ResourceAdapter (import only).
func (im *Importer) Export(context.Context, ioengine.ExportQuery, i18n.Locale) (ioengine.Dataset, error) {
	return ioengine.Dataset{}, errors.New("stock import: export not supported")
}

// ImportSchema implements ioengine.ResourceAdapter.
func (im *Importer) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "barcode", LabelKey: "stock_import.barcode", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "product_sku", LabelKey: "stock_import.product_sku", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "quantity", LabelKey: "stock_import.quantity", Type: ioengine.ColumnTypeString},
		{Key: "meters", LabelKey: "stock_import.meters", Type: ioengine.ColumnTypeString},
		{Key: "location_code", LabelKey: "stock_import.location_code", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows is the sample file content.
func (im *Importer) SampleRows() []map[string]any {
	return []map[string]any{
		{"barcode": "OLX-000001", "product_sku": "PPF-GLOSS", "quantity": "", "meters": "", "location_code": "A-01"},
		{"barcode": "OLX-ROLL-01", "product_sku": "PPF-ROLL", "quantity": "", "meters": "15.00", "location_code": ""},
		{"barcode": "OLX-FIX-01", "product_sku": "CLEANER", "quantity": "24", "meters": "", "location_code": ""},
	}
}

// ApplyRow implements ioengine.ResourceAdapter; the stock import is staged
// (Stage/Apply), never row by row.
func (im *Importer) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "stock import is staged"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (see Undo).
func (im *Importer) RevertRow(context.Context, string, string, map[string]any) error {
	return errors.New("stock import is staged")
}

// importCtx is the organization of a job and the lookups of one run.
type importCtx struct {
	org       db.Organization
	locale    i18n.Locale
	products  map[int64]*db.Product
	bySKU     map[string]*db.Product
	locations map[int64]*db.WarehouseLocation
	byCode    map[string]*db.WarehouseLocation
}

func (im *Importer) newCtx(ctx context.Context, job ioengine.ImportJob) (*importCtx, error) {
	if job.OrganizationID <= 0 {
		return nil, ErrImportCenterOnly
	}
	org, err := im.q.GetOrganizationByID(ctx, job.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("stock import: organization: %w", err)
	}
	if org.Type != "center" || org.BrandID == 0 {
		return nil, ErrImportCenterOnly
	}
	return &importCtx{
		org: org, locale: i18n.Normalize(job.Locale),
		products: map[int64]*db.Product{}, bySKU: map[string]*db.Product{},
		locations: map[int64]*db.WarehouseLocation{}, byCode: map[string]*db.WarehouseLocation{},
	}, nil
}

func (c *importCtx) productBySKU(ctx context.Context, q *db.Queries, sku string) (*db.Product, error) {
	if p, ok := c.bySKU[sku]; ok {
		return p, nil
	}
	p, err := q.GetProductBySKU(ctx, db.GetProductBySKUParams{BrandID: c.org.BrandID, Sku: sku})
	if errors.Is(err, pgx.ErrNoRows) {
		c.bySKU[sku] = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.bySKU[sku], c.products[p.ID] = &p, &p
	return &p, nil
}

func (c *importCtx) product(ctx context.Context, q *db.Queries, id int64) (*db.Product, error) {
	if p, ok := c.products[id]; ok {
		return p, nil
	}
	p, err := q.GetProduct(ctx, db.GetProductParams{ID: id, BrandID: c.org.BrandID})
	if err != nil {
		return nil, err
	}
	c.products[id] = &p
	return &p, nil
}

func (c *importCtx) locationByCode(ctx context.Context, q *db.Queries, code string) (*db.WarehouseLocation, error) {
	if l, ok := c.byCode[code]; ok {
		return l, nil
	}
	l, err := q.GetWarehouseLocationByCode(ctx, db.GetWarehouseLocationByCodeParams{OrganizationID: c.org.ID, Code: code})
	if errors.Is(err, pgx.ErrNoRows) {
		c.byCode[code] = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.byCode[code], c.locations[l.ID] = &l, &l
	return &l, nil
}

func (c *importCtx) location(ctx context.Context, q *db.Queries, id int64) (*db.WarehouseLocation, error) {
	if l, ok := c.locations[id]; ok {
		return l, nil
	}
	l, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: id, OrganizationID: c.org.ID})
	if err != nil {
		return nil, err
	}
	c.locations[id] = &l
	return &l, nil
}

// owner is the ledger owner a staged row enters.
func (c *importCtx) owner(row db.StockImportRow) ledger.Owner {
	if row.TargetOwnerType.String == string(ledger.OwnerWarehouseLocation) {
		return ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: row.TargetOwnerID.Int64, OrgID: c.org.ID}
	}
	return ledger.Owner{Type: ledger.OwnerOrganization, ID: c.org.ID, OrgID: c.org.ID}
}

func (im *Importer) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(im.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Stage implements ioengine.StagedImporter (preview, dry run).
func (im *Importer) Stage(ctx context.Context, job ioengine.ImportJob, rows []map[string]any, defaults map[string]any) (ioengine.PreviewSummary, error) {
	c, err := im.newCtx(ctx, job)
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	var summary ioengine.PreviewSummary
	err = im.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		batch, err := q.LockStockImportBatchByJob(ctx, i8(job.ID))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			batch, err = q.CreateStockImportBatchForJob(ctx, db.CreateStockImportBatchForJobParams{
				Uuid: job.UUID, OrganizationID: c.org.ID, BrandID: c.org.BrandID,
				CreatedByUserID: i8(job.ActorID), ImportJobID: i8(job.ID),
			})
			if err != nil {
				return err
			}
		case err != nil:
			return err
		}
		if !slices.Contains([]string{"draft", "validated", "failed"}, batch.Status) {
			return fmt.Errorf("%w: batch is %s", ioengine.ErrImportRejected, batch.Status)
		}
		if _, err := q.DeleteStockImportRows(ctx, batch.ID); err != nil {
			return err
		}
		seen := map[string]bool{}
		staged := make([]db.StockImportRow, 0, len(rows))
		for i, raw := range rows {
			row, err := im.stageRow(ctx, q, c, batch, int32(i+1), raw, defaults, seen)
			if err != nil {
				return err
			}
			staged = append(staged, row)
		}
		batch, err = q.SetStockImportBatchState(ctx, batchState(batch.ID, "validated", staged))
		if err != nil {
			return err
		}
		summary, err = im.summary(ctx, q, c, batch, staged)
		return err
	})
	return summary, err
}

// stageRow classifies and saves one row.
func (im *Importer) stageRow(ctx context.Context, q *db.Queries, c *importCtx, batch db.StockImportBatch,
	n int32, raw, defaults map[string]any, seen map[string]bool,
) (db.StockImportRow, error) {
	r := parseImportRow(raw, defaults)
	errs := slices.Clone(r.errs)
	var product *db.Product
	if r.sku != "" {
		p, err := c.productBySKU(ctx, q, truncate(r.sku, maxSKULen))
		if err != nil {
			return db.StockImportRow{}, err
		}
		switch {
		case p == nil || utf8.RuneCountInString(r.sku) > maxSKULen:
			errs = append(errs, ImportErrProductNotFound)
		case !p.Active:
			errs = append(errs, ImportErrProductInactive)
			product = p
		default:
			product = p
			errs = append(errs, checkImportShape(*p, r)...)
		}
	}
	target := ledger.Owner{Type: ledger.OwnerOrganization, ID: c.org.ID}
	if r.location != "" {
		loc, err := c.locationByCode(ctx, q, r.location)
		if err != nil {
			return db.StockImportRow{}, err
		}
		if loc == nil || !loc.Active {
			errs = append(errs, ImportErrLocationNotFound)
		} else {
			target = ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID}
		}
	}

	var status string
	switch {
	case len(errs) > 0:
		status = ImportRowInvalid
	case seen[r.barcode]:
		status, errs = ImportRowDuplicate, []string{ImportErrDuplicateInFile}
	default:
		st, code, err := classifyExisting(ctx, q, c.org, *product, r.barcode)
		if err != nil {
			return db.StockImportRow{}, err
		}
		status = st
		if code != "" {
			errs = []string{code}
		}
	}
	if status != ImportRowInvalid {
		seen[r.barcode] = true
	}

	arg := db.InsertStockImportRowParams{
		BatchID: batch.ID, RowNumber: n,
		Barcode:    pgtype.Text{String: truncate(r.barcode, maxBarcodeLen), Valid: r.barcode != ""},
		ProductSku: pgtype.Text{String: truncate(r.sku, maxSKULen), Valid: r.sku != ""},
		RowStatus:  status,
	}
	if product != nil {
		arg.ProductID = i8(product.ID)
	}
	if r.quantity != nil {
		arg.Quantity = pgtype.Int4{Int32: *r.quantity, Valid: true}
	} else if product != nil && !product.UsesFixedBarcode && status != ImportRowInvalid {
		arg.Quantity = pgtype.Int4{Int32: 1, Valid: true}
	}
	if r.cm != nil {
		arg.Meters = cmNumeric(*r.cm)
	}
	if status != ImportRowInvalid || !slices.Contains(errs, ImportErrLocationNotFound) {
		arg.TargetOwnerType, arg.TargetOwnerID = text(string(target.Type)), i8(target.ID)
	}
	var err error
	if arg.Raw, err = json.Marshal(rawObject(raw)); err != nil {
		return db.StockImportRow{}, err
	}
	if arg.Errors, err = json.Marshal(nonNil(errs)); err != nil {
		return db.StockImportRow{}, err
	}
	return q.InsertStockImportRow(ctx, arg)
}

// classifyExisting decides a well-formed row whose barcode may exist in the
// brand: an existing unit of the same product on hand at the importing
// center is a duplicate; any other existing unit is a conflict (held by
// another organization, another product, or a label not in stock).
func classifyExisting(ctx context.Context, q *db.Queries, org db.Organization, p db.Product, barcode string) (string, string, error) {
	unit, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: org.BrandID, Barcode: barcode})
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportRowNew, "", nil
	}
	if err != nil {
		return "", "", err
	}
	holders, onHand, err := unitHolders(ctx, q, unit)
	if err != nil {
		return "", "", err
	}
	for _, h := range holders {
		if h != org.ID {
			return ImportRowConflict, ImportErrHeldByOtherOrg, nil
		}
	}
	if unit.ProductID != p.ID {
		return ImportRowConflict, ImportErrProductMismatch, nil
	}
	if !onHand {
		return ImportRowConflict, ImportErrBarcodeTaken, nil
	}
	return ImportRowDuplicate, ImportErrAlreadyInStock, nil
}

// unitHolders returns the organizations holding a unit and whether it is on
// hand (available/placed serial unit, or fixed quantity > 0).
func unitHolders(ctx context.Context, q *db.Queries, unit db.Unit) ([]int64, bool, error) {
	if unit.UnitKind == ledger.KindFixed {
		hs, err := q.ListFixedBarcodeHoldingsByUnit(ctx, unit.ID)
		if err != nil {
			return nil, false, err
		}
		var out []int64
		for _, h := range hs {
			if h.QuantityOnHand > 0 {
				out = append(out, h.HolderOrgID)
			}
		}
		return out, len(out) > 0, nil
	}
	st, err := q.GetUnitCurrentState(ctx, unit.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	onHand := st.Status == string(ledger.StatusAvailable) || st.Status == string(ledger.StatusPlaced)
	return []int64{st.HolderOrgID}, onHand, nil
}

// Apply implements ioengine.StagedImporter (confirm, import queue).
func (im *Importer) Apply(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	c, err := im.newCtx(ctx, job)
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	var summary ioengine.PreviewSummary
	err = im.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		batch, err := q.LockStockImportBatchByJob(ctx, i8(job.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: import is not previewed", ioengine.ErrImportRejected)
		}
		if err != nil {
			return err
		}
		rows, err := q.ListStockImportRows(ctx, db.ListStockImportRowsParams{BatchID: batch.ID})
		if err != nil {
			return err
		}
		switch batch.Status {
		case "applied", "undone", "partially_undone":
			// Confirmed before: nothing new is written.
			summary, err = im.summary(ctx, q, c, batch, rows)
			return err
		case "validated", "failed":
		default:
			return fmt.Errorf("%w: batch is %s", ioengine.ErrImportRejected, batch.Status)
		}
		for i, row := range rows {
			if row.RowStatus != ImportRowNew {
				continue
			}
			if rows[i], err = im.applyRow(ctx, q, tx, c, job, batch, row); err != nil {
				return fmt.Errorf("stock import row %d: %w", row.RowNumber, err)
			}
		}
		batch, err = q.SetStockImportBatchState(ctx, batchState(batch.ID, "applied", rows))
		if err != nil {
			return err
		}
		if err := recordImportEntry(ctx, q, c, job, batch, rows); err != nil {
			return fmt.Errorf("stock import entry: %w", err)
		}
		summary, err = im.summary(ctx, q, c, batch, rows)
		return err
	})
	return summary, err
}

// recordImportEntry writes the stock entry document of an applied batch
// (TEC-204, mode import): confirmed at once, one line per applied row with
// its entry movement. A batch without applied rows records no document.
func recordImportEntry(ctx context.Context, q *db.Queries, c *importCtx, job ioengine.ImportJob,
	batch db.StockImportBatch, rows []db.StockImportRow,
) error {
	applied := 0
	for _, r := range rows {
		if r.RowStatus == ImportRowApplied {
			applied++
		}
	}
	if applied == 0 {
		return nil
	}
	entry, err := q.CreateImportStockEntry(ctx, db.CreateImportStockEntryParams{
		OrganizationID: c.org.ID, BrandID: c.org.BrandID, ImportBatchID: i8(batch.ID), UserID: i8(job.ActorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // recorded before
	}
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.RowStatus != ImportRowApplied {
			continue
		}
		arg := db.InsertStockEntryLineParams{
			EntryID: entry.ID, UnitID: r.UnitID.Int64, Quantity: 1, EntryMovementID: r.MovementID,
		}
		if r.Quantity.Valid && r.Quantity.Int32 > 0 {
			arg.Quantity = r.Quantity.Int32
		}
		if r.TargetOwnerType.String == string(ledger.OwnerWarehouseLocation) {
			arg.LocationID = r.TargetOwnerID
		}
		if _, err := q.InsertStockEntryLine(ctx, arg); err != nil {
			return err
		}
	}
	return nil
}

// applyRow creates the unit of a new row and enters it through the ledger.
func (im *Importer) applyRow(ctx context.Context, q *db.Queries, tx pgx.Tx, c *importCtx,
	job ioengine.ImportJob, batch db.StockImportBatch, row db.StockImportRow,
) (db.StockImportRow, error) {
	// The barcode may have been issued after the preview.
	if _, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: c.org.BrandID, Barcode: row.Barcode.String}); err == nil {
		errs, _ := json.Marshal([]string{ImportErrBarcodeTaken})
		return q.UpdateStockImportRowResult(ctx, db.UpdateStockImportRowResultParams{
			ID: row.ID, RowStatus: ImportRowConflict, Errors: errs,
		})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return row, err
	}
	p, err := c.product(ctx, q, row.ProductID.Int64)
	if err != nil {
		return row, err
	}
	kind := ledger.KindSerial
	if p.UsesFixedBarcode {
		kind = ledger.KindFixed
	}
	arg := db.CreateUnitParams{
		OrganizationID: c.org.ID, BrandID: c.org.BrandID, ProductID: p.ID, Barcode: row.Barcode.String,
		UnitKind: kind, Source: "imported", Status: string(ledger.StatusPrinted),
	}
	if p.UnitType == "roll_meter" && kind == ledger.KindSerial {
		arg.InitialMeters, arg.RemainingMeters = row.Meters, row.Meters
	}
	unit, err := q.CreateUnit(ctx, arg)
	if err != nil {
		return row, err
	}
	to := c.owner(row)
	m := ledger.Movement{
		Type: ledger.TypeEntry, UnitID: unit.ID, To: &to,
		Source: importSource, RefType: importRefType, RefID: row.ID,
		ActorUserID: actor(job.ActorID), Reason: "stock import",
		Metadata: map[string]any{"batch_uuid": batch.Uuid.String(), "row_number": row.RowNumber},
	}
	if kind == ledger.KindFixed {
		m.Quantity = row.Quantity.Int32
	}
	res, err := im.ledger.Post(ctx, tx, m)
	if err != nil {
		return row, err
	}
	return q.UpdateStockImportRowResult(ctx, db.UpdateStockImportRowResultParams{
		ID: row.ID, RowStatus: ImportRowApplied, Errors: []byte("[]"),
		UnitID: i8(unit.ID), MovementID: i8(res.Movement.ID),
	})
}

// Undo implements ioengine.StagedImporter (controlled undo).
func (im *Importer) Undo(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	c, err := im.newCtx(ctx, job)
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	var summary ioengine.PreviewSummary
	err = im.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		batch, err := q.LockStockImportBatchByJob(ctx, i8(job.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: import is not applied", ioengine.ErrImportRejected)
		}
		if err != nil {
			return err
		}
		rows, err := q.ListStockImportRows(ctx, db.ListStockImportRowsParams{BatchID: batch.ID})
		if err != nil {
			return err
		}
		switch batch.Status {
		case "undone", "partially_undone":
			summary, err = im.summary(ctx, q, c, batch, rows)
			return err
		case "applied":
		default:
			return fmt.Errorf("%w: batch is %s", ioengine.ErrImportRejected, batch.Status)
		}
		// The TEC-204 entry document of the batch (absent for batches
		// applied before it existed).
		entry, err := q.GetStockEntryByImportBatch(ctx, i8(batch.ID))
		hasEntry := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		rejected := 0
		for i, row := range rows {
			if row.RowStatus != ImportRowApplied {
				continue
			}
			var undone bool
			if rows[i], undone, err = im.undoRow(ctx, q, tx, c, job, row); err != nil {
				return fmt.Errorf("stock import undo row %d: %w", row.RowNumber, err)
			}
			if !undone {
				rejected++
				continue
			}
			if hasEntry {
				if err := q.SetStockEntryLineUndone(ctx, db.SetStockEntryLineUndoneParams{
					UndoMovementID: rows[i].UndoMovementID, EntryID: entry.ID, UnitID: rows[i].UnitID.Int64,
				}); err != nil {
					return err
				}
			}
		}
		status := "undone"
		if rejected > 0 {
			status = "partially_undone"
		}
		if hasEntry && rejected == 0 {
			if _, err := q.MarkStockEntryUndone(ctx, entry.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if batch, err = q.SetStockImportBatchState(ctx, batchState(batch.ID, status, rows)); err != nil {
			return err
		}
		summary, err = im.summary(ctx, q, c, batch, rows)
		return err
	})
	return summary, err
}

// undoRow voids the unit of an applied row when the import entry is its
// only movement; otherwise the row is refused (it stays applied).
func (im *Importer) undoRow(ctx context.Context, q *db.Queries, tx pgx.Tx, c *importCtx,
	job ioengine.ImportJob, row db.StockImportRow,
) (db.StockImportRow, bool, error) {
	unit, err := q.LockUnit(ctx, row.UnitID.Int64)
	if err != nil {
		return row, false, err
	}
	n, err := q.CountStockMovementsByUnit(ctx, unit.ID)
	if err != nil {
		return row, false, err
	}
	if n != 1 {
		errs, _ := json.Marshal([]string{ImportErrUndoUnitTouched})
		row, err = q.SetStockImportRowErrors(ctx, db.SetStockImportRowErrorsParams{ID: row.ID, Errors: errs})
		return row, false, err
	}
	from := c.owner(row)
	m := ledger.Movement{
		Type: ledger.TypeVoid, UnitID: unit.ID, From: &from,
		Source: importUndoSource, RefType: importRefType, RefID: row.ID,
		ActorUserID: actor(job.ActorID), Reason: "stock import undo",
		Metadata: map[string]any{"batch_uuid": job.UUID.String(), "row_number": row.RowNumber},
	}
	if unit.UnitKind == ledger.KindFixed {
		m.Quantity = row.Quantity.Int32
	}
	res, err := im.ledger.Post(ctx, tx, m)
	if err != nil {
		return row, false, err
	}
	row, err = q.MarkStockImportRowUndone(ctx, db.MarkStockImportRowUndoneParams{ID: row.ID, UndoMovementID: i8(res.Movement.ID)})
	return row, true, err
}

// batchState counts the rows per status.
func batchState(id int64, status string, rows []db.StockImportRow) db.SetStockImportBatchStateParams {
	st := db.SetStockImportBatchStateParams{ID: id, Status: status, RowsTotal: int32(len(rows))}
	for _, r := range rows {
		switch r.RowStatus {
		case ImportRowNew, ImportRowApplied, ImportRowUndone:
			st.RowsNew++
		case ImportRowDuplicate:
			st.RowsDuplicate++
		case ImportRowInvalid:
			st.RowsInvalid++
		case ImportRowConflict:
			st.RowsConflict++
		}
	}
	return st
}

// reportStatus is the report class of a row (an applied row the undo
// refused is undo_rejected).
func reportStatus(r db.StockImportRow, codes []string) string {
	if r.RowStatus == ImportRowApplied && slices.Contains(codes, ImportErrUndoUnitTouched) {
		return ImportRowUndoRejected
	}
	return r.RowStatus
}

// summary builds the job report of the batch rows.
func (im *Importer) summary(ctx context.Context, q *db.Queries, c *importCtx, batch db.StockImportBatch, rows []db.StockImportRow) (ioengine.PreviewSummary, error) {
	s := ioengine.PreviewSummary{
		Total: len(rows), BatchUUID: batch.Uuid.String(), Counts: map[string]int{},
		Rows: []ioengine.RowPreview{}, Errors: []ioengine.RowError{},
	}
	for _, st := range ImportRowStatuses {
		s.Counts[st] = 0
	}
	for _, r := range rows {
		var codes []string
		_ = json.Unmarshal(r.Errors, &codes)
		status := reportStatus(r, codes)
		s.Counts[status]++
		switch status {
		case ImportRowNew, ImportRowApplied, ImportRowUndone, ImportRowUndoRejected:
			s.Valid++
		default:
			s.Invalid++
		}
		for _, code := range codes {
			s.Errors = append(s.Errors, ioengine.RowError{
				Index: int(r.RowNumber), Field: importErrorField[code], Code: code,
				Error: i18n.Translate(c.locale, ImportErrorKey(code)),
			})
		}
		if len(s.Rows) >= previewRowLimit {
			continue
		}
		target, err := im.target(ctx, q, c, r)
		if err != nil {
			return s, err
		}
		var data map[string]any
		_ = json.Unmarshal(r.Raw, &data)
		s.Rows = append(s.Rows, ioengine.RowPreview{
			Index: int(r.RowNumber), Data: data, Status: status,
			StatusLabel: i18n.Translate(c.locale, ImportStatusKey(status)), Target: target,
		})
	}
	return s, nil
}

// target describes where a row is (or would be) written.
func (im *Importer) target(ctx context.Context, q *db.Queries, c *importCtx, r db.StockImportRow) (map[string]any, error) {
	t := map[string]any{
		"organization_uuid": c.org.Uuid.String(), "organization_name": c.org.Name,
	}
	if r.TargetOwnerType.Valid {
		t["owner_type"] = r.TargetOwnerType.String
		if r.TargetOwnerType.String == string(ledger.OwnerWarehouseLocation) {
			loc, err := c.location(ctx, q, r.TargetOwnerID.Int64)
			if err != nil {
				return nil, err
			}
			t["location_uuid"], t["location_code"] = loc.Uuid.String(), loc.Code
		}
	}
	if r.ProductID.Valid {
		p, err := c.product(ctx, q, r.ProductID.Int64)
		if err != nil {
			return nil, err
		}
		kind := ledger.KindSerial
		if p.UsesFixedBarcode {
			kind = ledger.KindFixed
		}
		t["product_uuid"], t["product_sku"], t["product_name"] = p.Uuid.String(), p.Sku, p.Name
		t["unit_kind"], t["unit_type"] = kind, p.UnitType
	}
	return t, nil
}

func rawObject(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func actor(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

func i8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v > 0} }

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func cmNumeric(cm int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(cm), Exp: -2, Valid: true}
}
