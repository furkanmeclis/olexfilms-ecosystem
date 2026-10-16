package ioengine

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/xuri/excelize/v2"
)

func sampleFixture() Sample {
	return Sample{
		Resource: "tenant.stock.units",
		Fields: []ImportField{
			{Key: "barcode", LabelKey: "stock_import.barcode", Required: true, Kind: FieldCode, HintKey: "stock_import.hint.barcode"},
			{Key: "product_sku", LabelKey: "stock_import.product_sku", Required: true, Kind: FieldCode},
			{Key: "quantity", LabelKey: "stock_import.quantity", Kind: FieldInteger},
			{Key: "meters", LabelKey: "stock_import.meters", Kind: FieldDecimal},
			{Key: "location_code", LabelKey: "stock_import.location_code", Kind: FieldCode},
			{Key: "unit", LabelKey: "catalog.products.unit", Enum: []string{"piece", "roll_meter"}},
		},
		Rows: []map[string]any{
			{"barcode": "000123", "product_sku": "PPF-GLOSS", "quantity": "24", "meters": "15.50", "location_code": "=A-01", "unit": "piece"},
		},
		Lookups: map[string][]string{"product_sku": {"PPF-GLOSS", "PPF-MATTE"}, "location_code": {"A-01"}},
		MaxRows: 100,
	}
}

func openXLSX(t *testing.T, b []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// TEC-757: the XLSX template has the instructions sheet first, the data
// sheet with a frozen, marked header, typed cells and drop-down lists bound
// to a hidden sheet; the parser reads the data sheet.
func TestEncodeSampleXLSXTemplate(t *testing.T) {
	b, ct, err := EncodeSample(ImportXLSX, "tr", sampleFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("content type = %q", ct)
	}
	f := openXLSX(t, b)
	sheets := f.GetSheetList()
	if len(sheets) != 3 || sheets[0] != "Talimat" || sheets[1] != "Veri" || sheets[2] != listsSheet {
		t.Fatalf("sheets = %v", sheets)
	}
	if v, _ := f.GetSheetVisible(listsSheet); v {
		t.Error("lists sheet is visible")
	}
	// Instructions: title and the column table with the hint.
	title, _ := f.GetCellValue("Talimat", "A1")
	if !strings.Contains(title, "içe aktarma şablonu") {
		t.Errorf("guide title = %q", title)
	}
	rows, _ := f.GetRows("Talimat")
	var found bool
	for _, r := range rows {
		if len(r) >= 4 && r[0] == "Barkod *" && r[1] == "Evet" && strings.Contains(r[3], "baştaki sıfırlar") {
			found = true
		}
	}
	if !found {
		t.Errorf("guide has no barcode row: %v", rows)
	}

	// Header: required columns marked, frozen.
	if h, _ := f.GetCellValue("Veri", "A1"); h != "Barkod *" {
		t.Errorf("A1 = %q", h)
	}
	if h, _ := f.GetCellValue("Veri", "C1"); h != "Adet" {
		t.Errorf("C1 = %q", h)
	}
	panes, _ := f.GetPanes("Veri")
	if !panes.Freeze || panes.YSplit != 1 {
		t.Errorf("panes = %+v", panes)
	}
	// Typed cells: barcode is text (leading zeros), quantity/meters numbers.
	if typ, _ := f.GetCellType("Veri", "A2"); typ == excelize.CellTypeNumber {
		t.Error("barcode stored as a number")
	}
	if v, _ := f.GetCellValue("Veri", "A2"); v != "000123" {
		t.Errorf("barcode = %q", v)
	}
	numFmt := func(cell string) int {
		id, _ := f.GetCellStyle("Veri", cell)
		st, _ := f.GetStyle(id)
		return st.NumFmt
	}
	if n := numFmt("A2"); n != 49 {
		t.Errorf("barcode numFmt = %d, want 49 (@)", n)
	}
	if typ, _ := f.GetCellType("Veri", "C2"); typ != excelize.CellTypeNumber && typ != excelize.CellTypeUnset {
		t.Errorf("quantity type = %v", typ)
	}
	if v, _ := f.GetCellValue("Veri", "C2", excelize.Options{RawCellValue: true}); v != "24" {
		t.Errorf("quantity raw = %q", v)
	}
	if v, _ := f.GetCellValue("Veri", "D2"); v != "15.50" {
		t.Errorf("meters = %q", v)
	}
	// Formula guard on text cells.
	if v, _ := f.GetCellValue("Veri", "E2"); v != "'=A-01" {
		t.Errorf("location = %q, want formula-safe", v)
	}
	// Drop-down lists: SKU and location from the hidden sheet, the enum too.
	dvs, err := f.GetDataValidations("Veri")
	if err != nil {
		t.Fatal(err)
	}
	bySqref := map[string]string{}
	for _, dv := range dvs {
		bySqref[dv.Sqref] = dv.Formula1
	}
	for sqref, wantSrc := range map[string]string{
		"B2:B101": "'_lists'!$A$2:$A$3",
		"E2:E101": "'_lists'!$B$2:$B$2",
		"F2:F101": "'_lists'!$C$2:$C$3",
	} {
		if got := bySqref[sqref]; !strings.Contains(got, wantSrc) {
			t.Errorf("validation %s = %q, want %s (all: %v)", sqref, got, wantSrc, bySqref)
		}
	}
	if v, _ := f.GetCellValue(listsSheet, "A3"); v != "PPF-MATTE" {
		t.Errorf("lists A3 = %q", v)
	}

	// The parser skips the instructions sheet.
	headers, parsed, err := ParseUpload(ImportXLSX, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 6 || headers[0] != "Barkod *" || len(parsed) != 1 || parsed[0]["Barkod *"] != "000123" {
		t.Fatalf("parsed = %v %v", headers, parsed)
	}
	mapping := SuggestMapping(headers, sampleFixture().Fields, i18n.LocaleTR)
	if mapping["Barkod *"] != "barcode" || mapping["Ürün stok kodu (SKU) *"] != "product_sku" {
		t.Errorf("mapping = %v", mapping)
	}
}

// CSV and TSV samples carry a UTF-8 BOM; cells are formula-safe.
func TestEncodeSampleCSVBOMAndFormulaGuard(t *testing.T) {
	for _, format := range []ImportFormat{ImportCSV, ImportTSV} {
		b, _, err := EncodeSample(format, "en", sampleFixture())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
			t.Errorf("%s sample has no BOM", format)
		}
		if !strings.Contains(string(b), "'=A-01") {
			t.Errorf("%s sample not formula-safe: %s", format, b)
		}
	}
}

func TestExportFormulaGuard(t *testing.T) {
	ds := Dataset{
		Columns: []Column{{Key: "name", LabelKey: "users.name"}, {Key: "balance", LabelKey: "cari.balance"}},
		Rows: []map[string]any{
			{"name": "=HYPERLINK(\"http://x\")", "balance": "-120.50"},
			{"name": "@SUM(A1)", "balance": "+5"},
			{"name": "\tTab", "balance": "-1.234,50"},
		},
	}
	csv, err := EncodeCSV(ds, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(csv, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("export CSV has no BOM")
	}
	for _, want := range []string{`"'=HYPERLINK(""http://x"")"`, "'@SUM(A1)", "-120.50", "'\tTab", "+5"} {
		if !strings.Contains(string(csv), want) {
			t.Errorf("csv missing %q:\n%s", want, csv)
		}
	}
	x, err := EncodeXLSX(ds, "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	f := openXLSX(t, x)
	sheet := f.GetSheetName(0)
	if v, _ := f.GetCellValue(sheet, "A2"); v != `'=HYPERLINK("http://x")` {
		t.Errorf("xlsx A2 = %q", v)
	}
	if v, _ := f.GetCellValue(sheet, "B2"); v != "-120.50" {
		t.Errorf("xlsx B2 = %q (numbers stay)", v)
	}
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"": "", "ABC-1": "ABC-1", "=SUM(A1)": "'=SUM(A1)", "-5": "'-5", "@x": "'@x", "+1": "'+1", "\rx": "'\rx"} {
		if got := CSVSafe(in); got != want {
			t.Errorf("CSVSafe(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"-5": "-5", "-1.234,50": "-1.234,50", "-2+3": "'-2+3", "--5": "'--5", "-": "'-"} {
		if got := safeCell(in); got != want {
			t.Errorf("safeCell(%q) = %q, want %q", in, got, want)
		}
	}
}

// TEC-757: the error report lists the failed rows first and highlighted,
// with the row, status and error columns and the source cells as text.
func TestEncodeErrorReport(t *testing.T) {
	r := ErrorReport{
		Headers: []string{"barcode", "product_sku"},
		Lines: []ReportLine{
			{Index: 1, Cells: map[string]string{"barcode": "0001", "product_sku": "A"}, Status: "Yeni"},
			{Index: 2, Cells: map[string]string{"barcode": "=cmd", "product_sku": "B"}, Status: "Geçersiz", Errors: []string{"x", "y"}, Failed: true},
		},
	}
	b, err := EncodeErrorReport(r, "tr")
	if err != nil {
		t.Fatal(err)
	}
	f := openXLSX(t, b)
	sheet := f.GetSheetList()[0]
	if sheet != "Hatalar" {
		t.Errorf("sheet = %q", sheet)
	}
	rows, _ := f.GetRows(sheet)
	want := [][]string{
		{"Satır", "Durum", "Hata", "barcode", "product_sku"},
		{"2", "Geçersiz", "x; y", "'=cmd", "B"},
		{"1", "Yeni", "", "0001", "A"},
	}
	for i := range want {
		if i >= len(rows) || !slices.Equal(rows[i], want[i]) {
			t.Fatalf("rows = %q, want %q", rows, want)
		}
	}
	id, _ := f.GetCellStyle(sheet, "D2")
	st, _ := f.GetStyle(id)
	if len(st.Fill.Color) == 0 || st.Fill.Color[0] != reportFailedFill {
		t.Errorf("failed row fill = %+v", st.Fill)
	}
	if st.NumFmt != 49 {
		t.Errorf("report cell numFmt = %d", st.NumFmt)
	}
	// A re-uploaded report maps only the data columns.
	if m := SuggestMapping(want[0], []ImportField{{Key: "status", LabelKey: "users.status"}}, i18n.LocaleTR); len(m) != 0 {
		t.Errorf("report columns mapped: %v", m)
	}
	if got := ErrorReportFilename("stok girişi.xlsx"); got != "stok girişi-hatalar.xlsx" {
		t.Errorf("filename = %q", got)
	}
	if got := AttachmentDisposition("stok ş\".xlsx"); !strings.Contains(got, `filename="stok __.xlsx"`) || !strings.Contains(got, "filename*=UTF-8''") {
		t.Errorf("disposition = %q", got)
	}
}

func TestValidateSchemaAndRowLimit(t *testing.T) {
	fields := []ImportField{
		{Key: "sku", LabelKey: "catalog.products.sku", Required: true},
		{Key: "qty", LabelKey: "stock_import.quantity", Kind: FieldInteger},
		{Key: "m", LabelKey: "stock_import.meters", Kind: FieldDecimal},
		{Key: "on", LabelKey: "catalog.products.is_active", Type: ColumnTypeBoolean},
		{Key: "unit", LabelKey: "catalog.products.unit", Enum: []string{"piece", "roll_meter"}},
	}
	errs := ValidateSchema(fields, map[string]any{"qty": "1.5", "m": "15,5", "on": "evet", "unit": "box"}, nil, i18n.LocaleEN)
	codes := map[string]string{}
	for _, e := range errs {
		codes[e.Field] = e.Code
		if e.Error == "" || strings.Contains(e.Error, "{{") {
			t.Errorf("message = %q", e.Error)
		}
	}
	want := map[string]string{"sku": CodeRequired, "qty": CodeInvalidInteger, "unit": CodeInvalidValue}
	if len(codes) != len(want) {
		t.Errorf("codes = %v, want %v", codes, want)
	}
	for k, v := range want {
		if codes[k] != v {
			t.Errorf("%s = %q, want %q", k, codes[k], v)
		}
	}
	if errs := ValidateSchema(fields, map[string]any{"qty": "2"}, map[string]any{"sku": "X"}, i18n.LocaleEN); len(errs) != 0 {
		t.Errorf("defaults ignored: %v", errs)
	}
	for _, code := range RowErrorCodes {
		for _, l := range i18n.Supported {
			if k := RowErrorKey(code); i18n.Translate(l, k) == k {
				t.Errorf("%s: %s untranslated", l, k)
			}
		}
	}
	err := CheckRowLimit(limited{}, 4, i18n.LocaleTR)
	var rl *RowLimitError
	if !errors.As(err, &rl) || rl.APICode() != CodeTooManyRows || !strings.Contains(rl.Error(), "4 satır") {
		t.Fatalf("row limit err = %v", err)
	}
	if err := CheckRowLimit(limited{}, 3, i18n.LocaleTR); err != nil {
		t.Errorf("at the limit: %v", err)
	}
}

type limited struct{ ResourceAdapter }

func (limited) MaxImportRows() int { return 3 }
