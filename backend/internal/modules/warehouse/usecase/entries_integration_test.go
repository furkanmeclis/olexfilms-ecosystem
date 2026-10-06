package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-204 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18): a confirmed stock entry places its units on the target
// location and writes the ledger movements; a second confirm is refused;
// a printed -> entry outside the center is refused (K14).

type entryEnv struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	brand   db.Brand
	center  db.Organization
	dist    db.Organization
	piece   db.Product
	fixed   db.Product
	suffix  string
	tree    *wh.Service
	entries *wh.StockEntries
	bc      *stockusecase.Barcodes
}

func newEntryEnv(t *testing.T) *entryEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	e := &entryEnv{ctx: ctx, pool: pool, q: q, suffix: fmt.Sprintf("%d", time.Now().UnixNano())}
	if e.brand, err = q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	if e.center, err = q.GetBrandCenter(ctx, e.brand.ID); err != nil {
		t.Fatalf("olex center: %v", err)
	}
	e.dist, err = q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t204-dist-" + e.suffix, Name: "dist", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "distributor", ParentID: pgtype.Int8{Int64: e.center.ID, Valid: true},
		BrandID: e.brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("distributor: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: e.center.ID, BrandID: e.brand.ID, Name: "t204-cat-" + e.suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product := func(sku string, fixed bool) db.Product {
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: e.center.ID, BrandID: e.brand.ID, CategoryID: cat.ID,
			Sku: sku + "-" + e.suffix, Name: sku, Images: []byte("[]"), UnitType: "piece", Active: true,
			UsesFixedBarcode: fixed,
		})
		if err != nil {
			t.Fatalf("product %s: %v", sku, err)
		}
		return p
	}
	e.piece = product("t204-piece", false)
	e.fixed = product("t204-fixed", true)
	e.tree = wh.New(pool, q)
	e.entries = wh.NewStockEntries(pool, q, outbox.NewStore(pool, q))
	e.bc = stockusecase.NewBarcodes(pool, q)
	return e
}

func (e *entryEnv) scope(o db.Organization) (orgctx.Scope, scopefilter.Filter) {
	return orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, Slug: o.Slug, OrgType: o.Type, BrandID: o.BrandID, BrandSlug: e.brand.Slug},
		scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{o.ID}, OrgID: o.ID}
}

func (e *entryEnv) caller(o db.Organization) wh.EntryCaller {
	s, f := e.scope(o)
	return wh.EntryCaller{Caller: wh.Caller{Org: s, Filter: f}}
}

func (e *entryEnv) stockCaller(o db.Organization) stockusecase.Caller {
	s, f := e.scope(o)
	return stockusecase.Caller{Org: s, Filter: f}
}

// prefix returns a unique barcode prefix (A-Z0-9, 2-8 characters).
func (e *entryEnv) prefix(letter string) string { return letter + e.suffix[len(e.suffix)-7:] }

// location creates warehouse -> room -> aisle for o and returns the
// warehouse and the location.
func (e *entryEnv) location(t *testing.T, o db.Organization, code string) (wh.Warehouse, wh.Location) {
	t.Helper()
	c := e.caller(o).Caller
	w, err := e.tree.CreateWarehouse(e.ctx, c, wh.WarehouseInput{Code: code + e.suffix[len(e.suffix)-6:], Name: "t204 " + code})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	r, err := e.tree.CreateRoom(e.ctx, c, w.UUID, wh.RoomInput{Code: "R1", Name: "room"})
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	l, err := e.tree.CreateLocation(e.ctx, c, wh.LocationInput{RoomUUID: r.UUID, Type: wh.TypeAisle, Code: "A1", Name: "aisle"})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	return w, l
}

func (e *entryEnv) movements(t *testing.T, unitID int64) []db.StockMovement {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `SELECT type, idempotency_key, to_owner_type, to_owner_id FROM stock_movements WHERE unit_id = $1 ORDER BY id`, unitID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []db.StockMovement
	for rows.Next() {
		var m db.StockMovement
		if err := rows.Scan(&m.Type, &m.IdempotencyKey, &m.ToOwnerType, &m.ToOwnerID); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// generate_new at the center: the barcodes are reserved (TEC-202 batch,
// printed, label links), confirm is refused while unplaced, the scanned
// location QR places every line, confirm posts entry + placement per unit
// with the stock_entry keys and the units end up placed on the location.
// A second confirm is 409 (ErrEntryNotDraft) and writes nothing.
func TestStockEntryGenerateNewConfirm(t *testing.T) {
	e := newEntryEnv(t)
	c := e.caller(e.center)
	w, loc := e.location(t, e.center, "W")

	entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeGenerateNew})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if entry.Status != wh.EntryStatusDraft || entry.Warehouse == nil || entry.Warehouse.UUID != w.UUID {
		t.Fatalf("draft = %+v", entry)
	}
	entry, err = e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{ProductUUID: e.piece.Uuid.String(), Count: 3, Prefix: e.prefix("E")})
	if err != nil {
		t.Fatalf("add lines: %v", err)
	}
	if len(entry.Lines) != 3 || len(entry.LabelBatches) != 1 || entry.LabelBatches[0].LabelsURL == "" {
		t.Fatalf("lines = %+v", entry)
	}
	for _, l := range entry.Lines {
		if l.UnitStatus != "printed" || l.Location != nil || l.LabelURL == "" || l.Quantity != 1 {
			t.Fatalf("line = %+v", l)
		}
	}
	if _, err := e.entries.Confirm(e.ctx, c, entry.UUID); !errors.Is(err, wh.ErrEntryUnplaced) {
		t.Fatalf("confirm unplaced: %v", err)
	}

	entry, err = e.entries.Place(e.ctx, c, entry.UUID, wh.PlaceInput{LocationCode: "OFW:LOC:" + loc.FullCode})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if entry.PlacedCount == nil || *entry.PlacedCount != 3 {
		t.Fatalf("placed = %+v", entry.PlacedCount)
	}

	entry, err = e.entries.Confirm(e.ctx, c, entry.UUID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if entry.Status != wh.EntryStatusConfirmed || entry.ConfirmedAt == nil {
		t.Fatalf("confirmed = %+v", entry)
	}
	lines, err := e.pool.Query(e.ctx, `SELECT l.id, u.id, u.barcode, u.status FROM stock_entry_lines l JOIN units u ON u.id = l.unit_id
		JOIN stock_entries e ON e.id = l.entry_id WHERE e.uuid = $1 ORDER BY l.id`, entry.UUID)
	if err != nil {
		t.Fatal(err)
	}
	type lineRow struct {
		id, unit        int64
		barcode, status string
	}
	var rows []lineRow
	for lines.Next() {
		var r lineRow
		if err := lines.Scan(&r.id, &r.unit, &r.barcode, &r.status); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	lines.Close()
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	locRow, err := e.q.GetTypedLocationByUUID(e.ctx, db.GetTypedLocationByUUIDParams{Uuid: loc.UUID, OrganizationID: e.center.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.status != "placed" {
			t.Fatalf("unit %s status = %s", r.barcode, r.status)
		}
		st, err := e.q.GetUnitCurrentState(e.ctx, r.unit)
		if err != nil || st.OwnerType != "warehouse_location" || st.OwnerID != locRow.ID || st.HolderOrgID != e.center.ID || st.Status != "placed" {
			t.Fatalf("state of %s = %+v %v", r.barcode, st, err)
		}
		mv := e.movements(t, r.unit)
		if len(mv) != 2 || mv[0].Type != "entry" || mv[1].Type != "placement" {
			t.Fatalf("movements of %s = %+v", r.barcode, mv)
		}
		if want := fmt.Sprintf("stock_entry:stock_entry_line:%d:entry:%s", r.id, r.barcode); mv[0].IdempotencyKey != want {
			t.Fatalf("entry key = %s, want %s", mv[0].IdempotencyKey, want)
		}
		if mv[0].ToOwnerType.String != "organization" || mv[0].ToOwnerID.Int64 != e.center.ID {
			t.Fatalf("entry target = %+v", mv[0])
		}
		if mv[1].ToOwnerType.String != "warehouse_location" || mv[1].ToOwnerID.Int64 != locRow.ID {
			t.Fatalf("placement target = %+v", mv[1])
		}
	}
	for _, l := range entry.Lines {
		if l.EntryMovementUUID == nil || l.PlacementMovementUUID == nil || l.Location == nil || l.Location.UUID != loc.UUID {
			t.Fatalf("confirmed line = %+v", l)
		}
	}

	// Second confirm: refused, nothing new.
	if _, err := e.entries.Confirm(e.ctx, c, entry.UUID); !errors.Is(err, wh.ErrEntryNotDraft) {
		t.Fatalf("second confirm: %v", err)
	}
	for _, r := range rows {
		if n := len(e.movements(t, r.unit)); n != 2 {
			t.Fatalf("movements after second confirm = %d", n)
		}
	}
	if _, err := e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{ProductUUID: e.piece.Uuid.String(), Count: 1}); !errors.Is(err, wh.ErrEntryNotDraft) {
		t.Fatalf("add to confirmed: %v", err)
	}
	if _, err := e.entries.Cancel(e.ctx, c, entry.UUID); !errors.Is(err, wh.ErrEntryNotDraft) {
		t.Fatalf("cancel confirmed: %v", err)
	}

	// A unit in stock cannot be linked again.
	other, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.AddLines(e.ctx, c, other.UUID, wh.EntryLinesInput{Barcodes: []string{rows[0].barcode}}); !errors.Is(err, wh.ErrEntryUnitNotEnterable) {
		t.Fatalf("link stocked unit: %v", err)
	}
	list, total, err := e.entries.List(e.ctx, c, wh.EntryListFilter{Statuses: []string{wh.EntryStatusConfirmed}, Limit: 50})
	if err != nil || total < 1 || len(list) < 1 || list[0].Lines != nil {
		t.Fatalf("list = %d %v", total, err)
	}
}

// with_existing at the center: printed labels of a TEC-202 batch (one
// serial, one fixed barcode) are linked by scan, a label on an open draft
// is refused on a second draft, lines are placed by barcode on a location
// picked by uuid and the confirm enters the fixed quantity straight into
// the location.
func TestStockEntryWithExistingConfirm(t *testing.T) {
	e := newEntryEnv(t)
	c := e.caller(e.center)
	w, loc := e.location(t, e.center, "X")
	sb, err := e.bc.Create(e.ctx, e.stockCaller(e.center), stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 1, Prefix: e.prefix("S")})
	if err != nil {
		t.Fatalf("serial batch: %v", err)
	}
	fb, err := e.bc.Create(e.ctx, e.stockCaller(e.center), stockusecase.BatchInput{ProductUUID: e.fixed.Uuid.String(), Quantity: 1, Prefix: e.prefix("F")})
	if err != nil {
		t.Fatalf("fixed batch: %v", err)
	}
	serial, fixed := sb.Units[0].Barcode, fb.Units[0].Barcode

	entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{Barcodes: []string{"OFW:UNIT:" + serial}}); err != nil {
		t.Fatalf("link serial: %v", err)
	}
	qty := int32(24)
	if _, err := e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{Barcodes: []string{fixed}, FixedQuantity: &qty}); err != nil {
		t.Fatalf("link fixed: %v", err)
	}
	second, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.AddLines(e.ctx, c, second.UUID, wh.EntryLinesInput{Barcodes: []string{serial}}); !errors.Is(err, wh.ErrEntryUnitTaken) {
		t.Fatalf("link taken: %v", err)
	}
	if _, err := e.entries.Cancel(e.ctx, c, second.UUID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	locID := loc.UUID
	if _, err := e.entries.Place(e.ctx, c, entry.UUID, wh.PlaceInput{Barcodes: []string{serial, fixed}, LocationUUID: &locID}); err != nil {
		t.Fatalf("place: %v", err)
	}
	got, err := e.entries.Confirm(e.ctx, c, entry.UUID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	locRow, err := e.q.GetTypedLocationByUUID(e.ctx, db.GetTypedLocationByUUIDParams{Uuid: loc.UUID, OrganizationID: e.center.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got.Lines {
		u, err := e.q.GetUnitByUUID(e.ctx, l.UnitUUID)
		if err != nil {
			t.Fatal(err)
		}
		mv := e.movements(t, u.ID)
		switch l.Barcode {
		case serial:
			if u.Status != "placed" || len(mv) != 2 || mv[1].Type != "placement" {
				t.Fatalf("serial = %s %+v", u.Status, mv)
			}
		case fixed:
			if u.Status != "available" || len(mv) != 1 || mv[0].Type != "entry" || mv[0].ToOwnerID.Int64 != locRow.ID || l.PlacementMovementUUID != nil {
				t.Fatalf("fixed = %s %+v", u.Status, mv)
			}
			var onHand int32
			if err := e.pool.QueryRow(e.ctx, `SELECT quantity_on_hand FROM fixed_barcode_holdings WHERE unit_id = $1 AND owner_type = 'warehouse_location' AND owner_id = $2`, u.ID, locRow.ID).Scan(&onHand); err != nil || onHand != 24 {
				t.Fatalf("fixed holding = %d %v", onHand, err)
			}
		default:
			t.Fatalf("unexpected line %+v", l)
		}
	}
}

// Outside the center a printed label never enters stock (K14): the
// distributor's draft is refused at confirm and nothing is written, the
// distributor cannot reserve barcodes, and the ledger itself refuses an
// entry into a distributor location.
func TestStockEntryOutsideCenterRefused(t *testing.T) {
	e := newEntryEnv(t)
	dc := e.caller(e.dist)
	w, loc := e.location(t, e.dist, "D")
	b, err := e.bc.Create(e.ctx, e.stockCaller(e.center), stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 1, Prefix: e.prefix("D")})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	barcode := b.Units[0].Barcode

	if _, err := e.entries.Create(e.ctx, dc, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeGenerateNew}); !errors.Is(err, wh.ErrEntryCenterOnly) {
		t.Fatalf("distributor generate_new: %v", err)
	}
	entry, err := e.entries.Create(e.ctx, dc, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatalf("distributor draft: %v", err)
	}
	if _, err := e.entries.AddLines(e.ctx, dc, entry.UUID, wh.EntryLinesInput{Barcodes: []string{barcode}}); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := e.entries.Place(e.ctx, dc, entry.UUID, wh.PlaceInput{LocationCode: loc.FullCode}); err != nil {
		t.Fatalf("place: %v", err)
	}
	if _, err := e.entries.Confirm(e.ctx, dc, entry.UUID); !errors.Is(err, wh.ErrEntryCenterOnly) {
		t.Fatalf("distributor confirm: %v", err)
	}
	unit, err := e.q.GetUnitByUUID(e.ctx, b.Units[0].UUID)
	if err != nil {
		t.Fatal(err)
	}
	if unit.Status != "printed" || len(e.movements(t, unit.ID)) != 0 {
		t.Fatalf("unit after refused confirm = %s", unit.Status)
	}
	still, err := e.entries.Get(e.ctx, dc, entry.UUID)
	if err != nil || still.Status != wh.EntryStatusDraft {
		t.Fatalf("entry after refused confirm = %+v %v", still, err)
	}

	// The ledger rule itself (printed -> entry at a center only).
	locRow, err := e.q.GetTypedLocationByUUID(e.ctx, db.GetTypedLocationByUUIDParams{Uuid: loc.UUID, OrganizationID: e.dist.ID})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	_, err = ledger.New(e.q, outbox.NewStore(e.pool, e.q)).Post(e.ctx, tx, ledger.Movement{
		Type: ledger.TypeEntry, UnitID: unit.ID,
		To:     &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: locRow.ID, OrgID: e.dist.ID},
		Source: "stock_entry", RefType: "stock_entry_line", RefID: 1,
	})
	if !errors.Is(err, ledger.ErrOwnerNotAllowed) {
		t.Fatalf("ledger entry at distributor: %v", err)
	}
}

// The safe stock import (TEC-158) records a confirmed stock entry (mode
// import) with one line per applied row; the undo marks the lines and the
// entry undone.
func TestStockImportRecordsEntry(t *testing.T) {
	e := newEntryEnv(t)
	_, loc := e.location(t, e.center, "I")
	user, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
		Email: pgtype.Text{String: "t204-" + e.suffix + "@example.test", Valid: true}, PasswordHash: "x",
		Name: "Import", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	jobRow, err := e.q.CreateImportJob(e.ctx, db.CreateImportJobParams{
		Resource: stockusecase.ImportResource, ActorID: user.ID, Format: "xlsx", Locale: "tr",
		OrganizationID: pgtype.Int8{Int64: e.center.ID, Valid: true}, SourceFilename: "t204.xlsx",
	})
	if err != nil {
		t.Fatalf("import job: %v", err)
	}
	job := ioengine.ImportJob{ID: jobRow.ID, UUID: jobRow.Uuid, ActorID: user.ID, OrganizationID: e.center.ID, Locale: "tr"}
	im := stockusecase.NewImporter(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	b1, b2 := "T204I-"+e.suffix+"-1", "T204I-"+e.suffix+"-2"
	rows := []map[string]any{
		{"barcode": b1, "product_sku": e.piece.Sku, "location_code": loc.FullCode},
		{"barcode": b2, "product_sku": e.piece.Sku},
	}
	if _, err := im.Stage(e.ctx, job, rows, nil); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := im.Apply(e.ctx, job); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var entryUUID string
	var status, mode string
	var lines int
	if err := e.pool.QueryRow(e.ctx, `SELECT e.uuid::text, e.status, e.mode, (SELECT count(*) FROM stock_entry_lines l WHERE l.entry_id = e.id AND l.entry_movement_id IS NOT NULL)
		FROM stock_entries e JOIN stock_import_batches b ON b.id = e.import_batch_id WHERE b.import_job_id = $1`, job.ID).Scan(&entryUUID, &status, &mode, &lines); err != nil {
		t.Fatalf("import entry: %v", err)
	}
	if status != wh.EntryStatusConfirmed || mode != wh.EntryModeImport || lines != 2 {
		t.Fatalf("import entry = %s %s %d", status, mode, lines)
	}
	// A second apply records nothing new.
	if _, err := im.Apply(e.ctx, job); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM stock_entries e JOIN stock_import_batches b ON b.id = e.import_batch_id WHERE b.import_job_id = $1`, job.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("entries after second apply = %d %v", n, err)
	}
	if _, err := im.Undo(e.ctx, job); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx, `SELECT e.status, (SELECT count(*) FROM stock_entry_lines l WHERE l.entry_id = e.id AND l.undo_movement_id IS NOT NULL)
		FROM stock_entries e WHERE e.uuid = $1::uuid`, entryUUID).Scan(&status, &lines); err != nil {
		t.Fatal(err)
	}
	if status != wh.EntryStatusUndone || lines != 2 {
		t.Fatalf("after undo = %s %d", status, lines)
	}
}
