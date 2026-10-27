package usecase_test

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1007: a container entry. The header names the supplier, container and
// default bin; the list reads quantity, meters (default roll length), lot,
// box and supplier codes (remembered); the arrival scan reports missing
// lines and extras; the import undoes cleanly.
func TestContainerEntryImportScanAndUndo(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.center, "K")
	bin := importBin(t, e, w)
	c := e.caller(e.center)
	user, err := e.q.CreateUser(e.ctx, db.CreateUserParams{Email: pgtype.Text{String: "t1007-" + uuid.NewString() + "@example.test", Valid: true}, PasswordHash: "x", Name: "Scan", Surname: "Test", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	c.Principal = authctx.Principal{UserInternal: user.ID}

	sup, err := e.entries.CreateSupplier(e.ctx, c, wh.SupplierInput{Name: "Guangzhou Film " + e.suffix})
	if err != nil {
		t.Fatal(err)
	}
	list, _, err := e.entries.Suppliers(e.ctx, c, e.suffix, 50, 0)
	if err != nil || len(list) != 1 || list[0].UUID != sup.UUID {
		t.Fatalf("suppliers %+v %v", list, err)
	}
	roll, err := e.q.CreateProduct(e.ctx, db.CreateProductParams{
		OrganizationID: e.center.ID, BrandID: e.brand.ID, CategoryID: e.piece.CategoryID,
		Sku: "t1007-roll-" + e.suffix, Name: "t1007-roll", Images: []byte("[]"), UnitType: "roll_meter", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE products SET default_roll_meters=50 WHERE id=$1`, roll.ID); err != nil {
		t.Fatal(err)
	}

	supUUID, container, date, binUUID := sup.UUID.String(), "MSKU1234567", "2026-10-20", bin.UUID.String()
	entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting, Header: wh.EntryHeaderInput{
		SupplierUUID: &supUUID, ContainerNo: &container, ArrivalDate: &date, DefaultLocationUUID: &binUUID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Supplier == nil || entry.Supplier.UUID != sup.UUID || entry.ContainerNo == nil || *entry.ContainerNo != container ||
		entry.ArrivalDate == nil || *entry.ArrivalDate != date || entry.DefaultLocation == nil || entry.DefaultLocation.UUID != bin.UUID {
		t.Fatalf("header %+v", entry)
	}

	code := func(s string) string { return "T1007-" + s + "-" + e.suffix }
	adapter := wh.NewEntryBarcodeImporter(e.entries)
	job := importJob(t, e, adapter, entry.UUID, "none")
	rows := []map[string]any{
		{"supplier_code": "GZ-PIECE-" + e.suffix, "product_sku": e.piece.Sku, "barcode": code("1"), "lot": "L-77", "box_no": "K-01"},
		{"product_sku": roll.Sku, "barcode": code("2"), "meters": "30,5"},
		{"product_sku": roll.Sku, "barcode": code("3")},
		{"product_sku": e.fixed.Sku, "barcode": code("4"), "quantity": "5"},
		{"product_sku": e.piece.Sku, "barcode": code("5"), "quantity": "2"},
		{"supplier_code": "GZ-UNKNOWN-" + e.suffix, "barcode": code("6")},
		{"product_sku": e.piece.Sku, "barcode": code("7"), "meters": "3"},
	}
	preview, err := adapter.Stage(e.ctx, job, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Counts["new"] != 4 || preview.Counts["invalid"] != 3 {
		t.Fatalf("preview counts %+v errors %+v", preview.Counts, preview.Errors)
	}
	wantErr := map[int]string{5: "ENTRY_IMPORT_QUANTITY_NOT_ALLOWED", 6: "ENTRY_IMPORT_SUPPLIER_CODE_UNKNOWN", 7: "ENTRY_IMPORT_METERS_NOT_ALLOWED"}
	for _, er := range preview.Errors {
		if wantErr[er.Index] != er.Code {
			t.Fatalf("row %d error %s", er.Index, er.Code)
		}
	}
	applied, err := adapter.Apply(e.ctx, job)
	if err != nil || applied.Counts["applied"] != 4 {
		t.Fatalf("apply %+v %v", applied.Counts, err)
	}
	got, err := e.entries.Get(e.ctx, c, entry.UUID)
	if err != nil || len(got.Lines) != 4 {
		t.Fatalf("lines %+v %v", got.Lines, err)
	}
	byCode := map[string]wh.EntryLine{}
	for _, l := range got.Lines {
		byCode[l.Barcode] = l
		if l.Location == nil || l.Location.UUID != bin.UUID {
			t.Fatalf("line %s not on the default bin", l.Barcode)
		}
	}
	if l := byCode[code("1")]; l.Lot == nil || *l.Lot != "L-77" || l.BoxNo == nil || *l.BoxNo != "K-01" {
		t.Fatalf("lot/box %+v", l)
	}
	if l := byCode[code("2")]; l.Meters == nil || *l.Meters != "30.50" {
		t.Fatalf("meters %+v", l)
	}
	if l := byCode[code("3")]; l.Meters == nil || *l.Meters != "50.00" {
		t.Fatalf("default meters %+v", l)
	}
	if l := byCode[code("4")]; l.Quantity != 5 {
		t.Fatalf("quantity %+v", l)
	}

	// The supplier code is remembered: alone it resolves the product.
	entry2, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting, Header: wh.EntryHeaderInput{SupplierUUID: &supUUID}})
	if err != nil {
		t.Fatal(err)
	}
	job2 := importJob(t, e, adapter, entry2.UUID, "none")
	p2, err := adapter.Stage(e.ctx, job2, []map[string]any{{"supplier_code": "gz-piece-" + e.suffix, "barcode": code("8")}}, nil)
	if err != nil || p2.Counts["new"] != 1 || p2.Rows[0].Target["product_sku"] != e.piece.Sku {
		t.Fatalf("remembered code %+v %v", p2, err)
	}

	// Arrival scan.
	res, err := e.entries.Scan(e.ctx, c, entry.UUID, code("1"))
	if err != nil || res.Result != wh.ScanMatched || res.Scanned != 1 || res.Expected != 4 || res.Line == nil || res.Line.ScannedAt == nil {
		t.Fatalf("scan %+v %v", res, err)
	}
	if res, err = e.entries.Scan(e.ctx, c, entry.UUID, code("1")); err != nil || res.Result != wh.ScanAlready {
		t.Fatalf("scan again %+v %v", res, err)
	}
	if res, err = e.entries.Scan(e.ctx, c, entry.UUID, "STRAY-"+e.suffix); err != nil || res.Result != wh.ScanExtra || res.Extras != 1 {
		t.Fatalf("extra %+v %v", res, err)
	}
	report, err := e.entries.Report(e.ctx, c, entry.UUID)
	if err != nil || report.Expected != 4 || report.Scanned != 1 || len(report.Missing) != 3 || len(report.Extras) != 1 {
		t.Fatalf("report %+v %v", report, err)
	}
	if res, err = e.entries.Unscan(e.ctx, c, entry.UUID, "STRAY-"+e.suffix); err != nil || res.Extras != 0 || res.Result != "unscanned" {
		t.Fatalf("unscan %+v %v", res, err)
	}
	if res, err = e.entries.Unscan(e.ctx, c, entry.UUID, code("1")); err != nil || res.Scanned != 0 {
		t.Fatalf("unscan line %+v %v", res, err)
	}

	undone, err := adapter.Undo(e.ctx, job)
	if err != nil || undone.Counts["undone"] != 4 {
		t.Fatalf("undo %+v %v", undone.Counts, err)
	}
	if got, err = e.entries.Get(e.ctx, c, entry.UUID); err != nil || len(got.Lines) != 0 {
		t.Fatalf("after undo %+v %v", got.Lines, err)
	}

	// Header edit clears a field.
	empty := ""
	got, err = e.entries.UpdateHeader(e.ctx, c, entry.UUID, wh.EntryHeaderInput{ContainerNo: &empty})
	if err != nil || got.ContainerNo != nil || got.Supplier == nil {
		t.Fatalf("header update %+v %v", got, err)
	}

	// Mapping templates.
	tpl, err := e.entries.SaveMappingTemplate(e.ctx, c, wh.MappingTemplateInput{Name: "GZ " + e.suffix, SupplierUUID: &supUUID, Mapping: map[string]string{"Barcode No": "barcode", "Item Code": "supplier_code"}})
	if err != nil || tpl.Supplier == nil {
		t.Fatalf("template %+v %v", tpl, err)
	}
	if _, err := e.entries.SaveMappingTemplate(e.ctx, c, wh.MappingTemplateInput{Name: "bad", Mapping: map[string]string{"x": "nope"}}); err == nil {
		t.Fatal("unknown field accepted")
	}
	tpls, err := e.entries.MappingTemplates(e.ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range tpls {
		found = found || (x.UUID == tpl.UUID && x.Mapping["Item Code"] == "supplier_code")
	}
	if !found {
		t.Fatalf("templates %+v", tpls)
	}
	if err := e.entries.DeleteMappingTemplate(e.ctx, c, tpl.UUID); err != nil {
		t.Fatal(err)
	}
}
