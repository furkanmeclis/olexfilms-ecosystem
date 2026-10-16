package usecase_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func newEntryImportEnv(t *testing.T) *entryEnv {
	e := newEntryEnv(t)
	// The shared integration DB keeps previous test rows; randomize short codes.
	e.suffix = strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
	return e
}

func importJob(t *testing.T, e *entryEnv, adapter *wh.EntryBarcodeImporter, entry uuid.UUID, policy string) ioengine.ImportJob {
	t.Helper()
	row, err := e.q.CreateImportJob(e.ctx, db.CreateImportJobParams{Resource: wh.EntryBarcodeImportResource, ActorID: 1, Format: "csv", Locale: "en", OrganizationID: pgtype.Int8{Int64: e.center.ID, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(e.ctx, row.Uuid, entry, e.center.ID, policy); err != nil {
		t.Fatal(err)
	}
	return ioengine.ImportJob{ID: row.ID, UUID: row.Uuid, OrganizationID: e.center.ID, Locale: "en", ActorID: 1}
}
func importBin(t *testing.T, e *entryEnv, w wh.Warehouse) wh.Location {
	t.Helper()
	c := e.caller(e.center).Caller
	room, err := e.tree.CreateRoom(e.ctx, c, w.UUID, wh.RoomInput{Code: "R9", Name: "import room"})
	if err != nil {
		t.Fatal(err)
	}
	aisle, err := e.tree.CreateLocation(e.ctx, c, wh.LocationInput{RoomUUID: room.UUID, Type: wh.TypeAisle, Code: "A9", Name: "aisle"})
	if err != nil {
		t.Fatal(err)
	}
	shelf, err := e.tree.CreateLocation(e.ctx, c, wh.LocationInput{RoomUUID: room.UUID, ParentUUID: &aisle.UUID, Type: wh.TypeShelf, Code: "S9", Name: "shelf"})
	if err != nil {
		t.Fatal(err)
	}
	bin, err := e.tree.CreateLocation(e.ctx, c, wh.LocationInput{RoomUUID: room.UUID, ParentUUID: &shelf.UUID, Type: wh.TypeBin, Code: "B9", Name: "bin"})
	if err != nil {
		t.Fatal(err)
	}
	return bin
}
func TestEntryBarcodeImportPoliciesAndUndo(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.center, "I")
	bin := importBin(t, e, w)
	probe, err := e.entries.Create(e.ctx, e.caller(e.center), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := e.scope(e.center)
	lookups, err := wh.NewEntryBarcodeImporter(e.entries).SampleLookups(wh.EntryImportContext(orgctx.WithScope(e.ctx, scope), probe.UUID))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(lookups["product_sku"], e.piece.Sku) || !slices.Contains(lookups["location_code"], bin.FullCode) {
		t.Fatalf("sample lookups: %+v", lookups)
	}
	for _, tc := range []struct{ policy, status string }{{"none", "link"}, {"all_existing", "duplicate"}, {"conflicts_only", "link"}} {
		t.Run(tc.policy, func(t *testing.T) {
			code := "T760-" + tc.policy + e.suffix
			_, err := e.q.CreateUnit(e.ctx, db.CreateUnitParams{OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: e.piece.ID, Barcode: code, UnitKind: "serial", Source: "imported", Status: "printed"})
			if err != nil {
				t.Fatal(err)
			}
			entry, err := e.entries.Create(e.ctx, e.caller(e.center), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
			if err != nil {
				t.Fatal(err)
			}
			adapter := wh.NewEntryBarcodeImporter(e.entries)
			job := importJob(t, e, adapter, entry.UUID, tc.policy)
			rows := []map[string]any{{"product_sku": e.piece.Sku, "barcode": code, "location_code": bin.FullCode}, {"product_sku": e.piece.Sku, "barcode": "T760-NEW-" + tc.policy + e.suffix}}
			preview, err := adapter.Stage(e.ctx, job, rows, nil)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Rows[0].Status != tc.status || preview.Rows[1].Status != "new" {
				t.Fatalf("preview %+v", preview)
			}
			if preview.Counts["placed"] != map[bool]int{true: 1, false: 0}[tc.status == "link"] {
				t.Fatalf("placed %+v", preview.Counts)
			}
			applied, err := adapter.Apply(e.ctx, job)
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if tc.policy == "all_existing" {
				want = 1
			}
			if applied.Counts["applied"] != want {
				t.Fatalf("applied %+v", applied.Counts)
			}
			got, err := e.entries.Get(e.ctx, e.caller(e.center), entry.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Lines) != want {
				t.Fatalf("lines %+v", got.Lines)
			}
			again, err := adapter.Apply(e.ctx, job)
			if err != nil || again.Counts["applied"] != want {
				t.Fatalf("idempotence %+v %v", again, err)
			}
			undone, err := adapter.Undo(e.ctx, job)
			if err != nil {
				t.Fatal(err)
			}
			if undone.Counts["undone"] != want {
				t.Fatalf("undo %+v", undone.Counts)
			}
			got, err = e.entries.Get(e.ctx, e.caller(e.center), entry.UUID)
			if err != nil || len(got.Lines) != 0 {
				t.Fatalf("after undo %+v %v", got.Lines, err)
			}
		})
	}
}
func TestEntryBarcodeImportValidationAndAtomicApply(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.center, "J")
	entry, err := e.entries.Create(e.ctx, e.caller(e.center), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	adapter := wh.NewEntryBarcodeImporter(e.entries)
	job := importJob(t, e, adapter, entry.UUID, "none")
	// The active catalog may have more than one product with the same name.
	_, err = e.q.CreateProduct(e.ctx, db.CreateProductParams{OrganizationID: e.center.ID, BrandID: e.brand.ID, CategoryID: e.piece.CategoryID, Sku: "T760-DUP-" + e.suffix, Name: e.piece.Name, Images: []byte("[]"), UnitType: "piece", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{{"product_name": e.piece.Name, "barcode": "T760-AMB-" + e.suffix}, {"product_sku": e.piece.Sku, "product_name": "wrong", "barcode": "T760-MISMATCH-" + e.suffix}, {"product_sku": e.piece.Sku, "barcode": "T760-FIRST-" + e.suffix}, {"product_sku": e.piece.Sku, "barcode": "T760-VALID-" + e.suffix}, {"product_sku": e.piece.Sku, "barcode": "T760-VALID-" + e.suffix}}
	summary, err := adapter.Stage(e.ctx, job, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"invalid", "invalid", "new", "new", "duplicate"} {
		if summary.Rows[i].Status != want {
			t.Fatalf("row %d: %+v", i, summary.Rows[i])
		}
	}
	// A later issuer takes the barcode: apply must fail atomically, including
	// any earlier valid rows in the same batch.
	_, err = e.q.CreateUnit(e.ctx, db.CreateUnitParams{OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: e.piece.ID, Barcode: "T760-VALID-" + e.suffix, UnitKind: "serial", Source: "imported", Status: "printed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.Apply(e.ctx, job); !errors.Is(err, ioengine.ErrImportRejected) {
		t.Fatalf("apply conflict: %v", err)
	}
	got, err := e.entries.Get(e.ctx, e.caller(e.center), entry.UUID)
	if err != nil || len(got.Lines) != 0 {
		t.Fatalf("atomic apply: %+v %v", got.Lines, err)
	}
	var firstExists bool
	if err := e.pool.QueryRow(e.ctx, `SELECT EXISTS(SELECT 1 FROM units WHERE brand_id=$1 AND barcode=$2)`, e.brand.ID, "T760-FIRST-"+e.suffix).Scan(&firstExists); err != nil || firstExists {
		t.Fatalf("first unit survived failed transaction: %v %v", firstExists, err)
	}
}
func TestEntryBarcodeImportPreview2000(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.center, "K")
	entry, err := e.entries.Create(e.ctx, e.caller(e.center), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	adapter := wh.NewEntryBarcodeImporter(e.entries)
	job := importJob(t, e, adapter, entry.UUID, "none")
	rows := make([]map[string]any, 2000)
	for i := range rows {
		rows[i] = map[string]any{"product_sku": e.piece.Sku, "barcode": fmt.Sprintf("T760-%s-%04d", e.suffix, i)}
	}
	start := time.Now()
	summary, err := adapter.Stage(e.ctx, job, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Counts["new"] != 2000 {
		t.Fatalf("counts %+v", summary.Counts)
	}
	t.Logf("2000-row preview: %s", time.Since(start))
}

func TestEntryBarcodeImportConflictPolicyAndConfirmedUndo(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.center, "L")
	bin := importBin(t, e, w)
	// A barcode of another product is a conflict, or a policy skip.
	code := "T760-WRONG-" + e.suffix
	_, err := e.q.CreateUnit(e.ctx, db.CreateUnitParams{OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: e.fixed.ID, Barcode: code, UnitKind: "fixed", Source: "imported", Status: "printed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ policy, want string }{{"none", "conflict"}, {"all_existing", "duplicate"}, {"conflicts_only", "duplicate"}} {
		entry, err := e.entries.Create(e.ctx, e.caller(e.center), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
		if err != nil {
			t.Fatal(err)
		}
		im := wh.NewEntryBarcodeImporter(e.entries)
		job := importJob(t, e, im, entry.UUID, tc.policy)
		result, err := im.Stage(e.ctx, job, []map[string]any{{"product_sku": e.piece.Sku, "barcode": code}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.Rows[0].Status != tc.want {
			t.Fatalf("%s: %+v", tc.policy, result.Rows[0])
		}
	}
	entry, err := e.entries.Create(e.ctx, e.caller(e.center), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	im := wh.NewEntryBarcodeImporter(e.entries)
	job := importJob(t, e, im, entry.UUID, "none")
	newCode := "T760-PLACED-" + e.suffix
	preview, err := im.Stage(e.ctx, job, []map[string]any{{"product_sku": e.piece.Sku, "barcode": newCode, "location_code": bin.FullCode}}, nil)
	if err != nil || preview.Counts["placed"] != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err := im.Apply(e.ctx, job); err != nil {
		t.Fatal(err)
	}
	confirmed, err := e.entries.Confirm(e.ctx, e.caller(e.center), entry.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "confirmed" || len(confirmed.Lines) != 1 || confirmed.Lines[0].Location == nil || confirmed.Lines[0].PlacementMovementUUID == nil {
		t.Fatalf("confirmed: %+v", confirmed)
	}
	undone, err := im.Undo(e.ctx, job)
	if err != nil || undone.Counts["undo_rejected"] != 1 {
		t.Fatalf("undo: %+v %v", undone, err)
	}
}

func TestEntryBarcodeImportDistributorCannotIssue(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.dist, "D")
	entry, err := e.entries.Create(e.ctx, e.caller(e.dist), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	im := wh.NewEntryBarcodeImporter(e.entries)
	row, err := e.q.CreateImportJob(e.ctx, db.CreateImportJobParams{Resource: wh.EntryBarcodeImportResource, ActorID: 1, Format: "csv", Locale: "en", OrganizationID: pgtype.Int8{Int64: e.dist.ID, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := im.Bind(e.ctx, row.Uuid, entry.UUID, e.dist.ID, "none"); err != nil {
		t.Fatal(err)
	}
	job := ioengine.ImportJob{ID: row.ID, UUID: row.Uuid, OrganizationID: e.dist.ID, Locale: "en", ActorID: 1}
	result, err := im.Stage(e.ctx, job, []map[string]any{{"product_sku": e.piece.Sku, "barcode": "T760-DIST-" + e.suffix}}, nil)
	if err != nil || result.Rows[0].Status != "invalid" || result.Errors[0].Code != "ENTRY_IMPORT_CENTER_ONLY" {
		t.Fatalf("distributor: %+v %v", result, err)
	}
}

func TestEntryBarcodeImportUndoRejectsEditedDraftLine(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.center, "M")
	bin := importBin(t, e, w)
	entry, err := e.entries.Create(e.ctx, e.caller(e.center), wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatal(err)
	}
	im := wh.NewEntryBarcodeImporter(e.entries)
	job := importJob(t, e, im, entry.UUID, "none")
	code := "T760-EDIT-" + e.suffix
	if _, err := im.Stage(e.ctx, job, []map[string]any{{"product_sku": e.piece.Sku, "barcode": code}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := im.Apply(e.ctx, job); err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.Place(e.ctx, e.caller(e.center), entry.UUID, wh.PlaceInput{LocationCode: bin.FullCode}); err != nil {
		t.Fatal(err)
	}
	result, err := im.Undo(e.ctx, job)
	if err != nil || result.Counts["undo_rejected"] != 1 {
		t.Fatalf("edited draft undo: %+v %v", result, err)
	}
}
