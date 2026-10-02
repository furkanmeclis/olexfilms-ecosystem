package usecase

import (
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

func TestParseImportRow(t *testing.T) {
	cases := []struct {
		name string
		row  map[string]any
		want []string
	}{
		{"ok", map[string]any{"barcode": "B1", "product_sku": "S1"}, nil},
		{"missing", map[string]any{}, []string{ImportErrBarcodeRequired, ImportErrSKURequired}},
		{"long barcode", map[string]any{"barcode": strings.Repeat("x", 65), "product_sku": "S"}, []string{ImportErrBarcodeTooLong}},
		{"bad quantity", map[string]any{"barcode": "B", "product_sku": "S", "quantity": "-2"}, []string{ImportErrQuantityInvalid}},
		{"bad meters", map[string]any{"barcode": "B", "product_sku": "S", "meters": "1.234"}, []string{ImportErrMetersInvalid}},
		{"comma meters", map[string]any{"barcode": "B", "product_sku": "S", "meters": "12,5"}, nil},
		{"json number", map[string]any{"barcode": "B", "product_sku": "S", "quantity": float64(3)}, nil},
	}
	for _, c := range cases {
		got := parseImportRow(c.row, nil)
		if !slices.Equal(got.errs, c.want) {
			t.Errorf("%s: errs = %v, want %v", c.name, got.errs, c.want)
		}
	}
	r := parseImportRow(map[string]any{"barcode": " B ", "product_sku": ""}, map[string]any{"product_sku": "DEF", "location_code": "A-1"})
	if r.barcode != "B" || r.sku != "DEF" || r.location != "A-1" {
		t.Fatalf("defaults: %+v", r)
	}
	if m := parseImportRow(map[string]any{"barcode": "B", "product_sku": "S", "meters": "12,5"}, nil); m.cm == nil || *m.cm != 1250 {
		t.Fatalf("meters = %v", m.cm)
	}
}

func TestCheckImportShape(t *testing.T) {
	piece := db.Product{UnitType: "piece"}
	roll := db.Product{UnitType: "roll_meter"}
	fixed := db.Product{UnitType: "piece", UsesFixedBarcode: true}
	row := func(m map[string]any) importRow {
		m["barcode"], m["product_sku"] = "B", "S"
		return parseImportRow(m, nil)
	}
	cases := []struct {
		name string
		p    db.Product
		r    importRow
		want []string
	}{
		{"piece", piece, row(map[string]any{}), nil},
		{"piece qty 1", piece, row(map[string]any{"quantity": "1"}), nil},
		{"piece qty 3", piece, row(map[string]any{"quantity": "3"}), []string{ImportErrQuantityForbidden}},
		{"piece meters", piece, row(map[string]any{"meters": "2"}), []string{ImportErrMetersForbidden}},
		{"roll", roll, row(map[string]any{"meters": "30"}), nil},
		{"roll no meters", roll, row(map[string]any{}), []string{ImportErrMetersInvalid}},
		{"fixed", fixed, row(map[string]any{"quantity": "24"}), nil},
		{"fixed no qty", fixed, row(map[string]any{}), []string{ImportErrQuantityInvalid}},
		{"fixed meters", fixed, row(map[string]any{"quantity": "2", "meters": "1"}), []string{ImportErrMetersForbidden}},
	}
	for _, c := range cases {
		if got := checkImportShape(c.p, c.r); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBatchStateAndReportStatus(t *testing.T) {
	rows := []db.StockImportRow{
		{RowStatus: ImportRowApplied, Errors: []byte(`[]`)},
		{RowStatus: ImportRowDuplicate}, {RowStatus: ImportRowInvalid}, {RowStatus: ImportRowConflict},
		{RowStatus: ImportRowUndone},
	}
	st := batchState(7, "applied", rows)
	if st.RowsTotal != 5 || st.RowsNew != 2 || st.RowsDuplicate != 1 || st.RowsInvalid != 1 || st.RowsConflict != 1 {
		t.Fatalf("state = %+v", st)
	}
	if got := reportStatus(rows[0], []string{ImportErrUndoUnitTouched}); got != ImportRowUndoRejected {
		t.Fatalf("report status = %s", got)
	}
	if got := reportStatus(rows[0], nil); got != ImportRowApplied {
		t.Fatalf("report status = %s", got)
	}
}

// Every row error code and class has a catalog label in every locale
// (TestTranslatedCatalogParity checks the translations themselves).
func TestImportCatalogKeys(t *testing.T) {
	keys := []string{"resources." + ImportResource}
	for _, c := range ImportErrorCodes {
		keys = append(keys, ImportErrorKey(c))
	}
	for _, s := range ImportRowStatuses {
		keys = append(keys, ImportStatusKey(s))
	}
	for _, f := range (&Importer{}).ImportSchema() {
		keys = append(keys, f.LabelKey)
	}
	for _, k := range keys {
		for _, l := range []i18n.Locale{i18n.LocaleTR, i18n.LocaleEN, i18n.LocaleAR} {
			if got := i18n.Translate(l, k); got == k {
				t.Errorf("%s: missing %s", l, k)
			}
		}
	}
	if i18n.Translate(i18n.LocaleTR, ImportErrorKey(ImportErrHeldByOtherOrg)) == i18n.Translate(i18n.LocaleEN, ImportErrorKey(ImportErrHeldByOtherOrg)) {
		t.Fatal("tr equals en")
	}
}
