package httpserver

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/xuri/excelize/v2"
)

// uploadImportFile posts a file to an import upload route.
func (it *itest) uploadImportFile(path, token, filename, format, locale string, data []byte) (int, envelope) {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		it.t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.WriteField("format", format)
	_ = mw.WriteField("locale", locale)
	_ = mw.Close()
	rec := it.raw("POST", path, token, mw.FormDataContentType(), buf.Bytes(), nil)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func (it *itest) uploadedJob(code int, env envelope) stockImportJob {
	it.t.Helper()
	if code != http.StatusCreated {
		it.t.Fatalf("upload = %d %s", code, errCode(env))
	}
	var job stockImportJob
	if err := json.Unmarshal(env.Data, &job); err != nil {
		it.t.Fatal(err)
	}
	return job
}

// errorReport downloads GET <base>/error-report and returns its rows.
func (it *itest) errorReport(base, token string) ([][]string, []byte) {
	it.t.Helper()
	rec := it.raw("GET", base+"/error-report", token, "", nil, nil)
	if rec.Code != http.StatusOK {
		it.t.Fatalf("error report = %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		it.t.Fatalf("content type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, "-hatalar.xlsx") {
		it.t.Fatalf("content disposition = %q", cd)
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		it.t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(f.GetSheetList()[0])
	if err != nil {
		it.t.Fatal(err)
	}
	return rows, rec.Body.Bytes()
}

// TEC-757 acceptance (stock): the template has the instructions sheet, SKU
// and location drop-downs and a text barcode column; a file over the row
// limit is refused; the error report of an invalid file lists the failed
// rows first with their localized errors; the fixed report uploads clean.
// Confirm and rollback are open to the uploader and stock.import holders
// only; reading the job and its report stays with tenant.imports.read.
func TestIntegrationStockImportErrorReport(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	center := it.brandCenter("olex")
	wh, wpw := it.user("t757-wh")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	wh2, w2pw := it.user("t757-wh2")
	it.member(center, wh2, "staff", rbac.RoleCenterWarehouse)
	staff, spw := it.user("t757-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	whTok := it.loginOrg(wh, wpw, center)
	wh2Tok := it.loginOrg(wh2, w2pw, center)
	staffTok := it.loginOrg(staff, spw, center)

	piece := it.product(center, "T757P")
	c := it.stockChain()
	c.location(center, "C757")
	locCode := "C757-" + it.suffix

	// 1. Template.
	rec := it.raw("GET", "/v1/stock/import/sample?format=xlsx&locale=tr", whTok, "", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("sample = %d %s", rec.Code, rec.Body.String())
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if sheets := f.GetSheetList(); len(sheets) != 3 || sheets[0] != "Talimat" || sheets[1] != "Veri" {
		t.Fatalf("template sheets = %v", sheets)
	}
	if notes, _ := f.GetRows("Talimat"); !slices.ContainsFunc(notes, func(r []string) bool {
		return len(r) > 0 && strings.Contains(r[0], "Sabit barkodlu ürün")
	}) {
		t.Errorf("guide has no stock notes: %v", notes)
	}
	dvs, err := f.GetDataValidations("Veri")
	if err != nil || len(dvs) != 2 {
		t.Fatalf("validations = %v %v", dvs, err)
	}
	lists, _ := f.GetCols("_lists")
	if len(lists) != 2 || !slices.Contains(lists[0], piece.Sku) || !slices.Contains(lists[1], locCode) {
		t.Errorf("lists = %v, want %s and %s", lists, piece.Sku, locCode)
	}
	style, _ := f.GetCellStyle("Veri", "A2")
	if st, _ := f.GetStyle(style); st == nil || st.NumFmt != 49 {
		t.Errorf("barcode column is not text: %+v", st)
	}
	_ = f.Close()

	// 2. Row limit (5000).
	var big strings.Builder
	big.WriteString("barcode,product_sku\n")
	for i := 0; i < 5001; i++ {
		big.WriteString("B,S\n")
	}
	if code, env := it.uploadImportFile("/v1/stock/import", whTok, "big.csv", "csv", "en", []byte(big.String())); code != http.StatusBadRequest || errCode(env) != "IMPORT_TOO_MANY_ROWS" {
		t.Fatalf("over the limit = %d %s", code, errCode(env))
	}

	// 3. An invalid file: preview and error report.
	bc := func(n string) string { return "0757" + it.suffix + n }
	csv := "barcode,product_sku,location_code\n" +
		bc("1") + "," + piece.Sku + "," + locCode + "\n" +
		bc("2") + ",NOPE-" + it.suffix + ",\n"
	job := it.uploadedJob(it.uploadImportFile("/v1/stock/import", whTok, "stok.csv", "csv", "en", []byte(csv)))
	base := "/v1/tenant/imports/" + job.UUID
	if rec := it.raw("GET", base+"/error-report", whTok, "", nil, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("report before preview = %d", rec.Code)
	}
	pv := it.importJob("POST", base+"/preview", whTok, nil, http.StatusOK)
	if pv.PreviewSummary.Counts["new"] != 1 || pv.PreviewSummary.Counts["invalid"] != 1 {
		t.Fatalf("preview = %+v", pv.PreviewSummary)
	}
	rows, _ := it.errorReport(base, staffTok) // read access = job detail
	want := [][]string{
		{"Row", "Status", "Error", "barcode", "product_sku", "location_code"},
		{"2", "Invalid", "No product with this SKU in the brand.", bc("2"), "NOPE-" + it.suffix},
		{"1", "New", "", bc("1"), piece.Sku, locCode},
	}
	if len(rows) != 3 || !slices.Equal(rows[0], want[0]) || !slices.Equal(rows[1], want[1]) || !slices.Equal(rows[2], want[2]) {
		t.Fatalf("report = %q, want %q", rows, want)
	}

	// 4. The report, fixed and uploaded again, previews clean.
	_, data := it.errorReport(base, whTok)
	fx, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	sheet := fx.GetSheetList()[0]
	_ = fx.SetCellStr(sheet, "E2", piece.Sku)
	var fixed bytes.Buffer
	if err := fx.Write(&fixed); err != nil {
		t.Fatal(err)
	}
	_ = fx.Close()
	job2 := it.uploadedJob(it.uploadImportFile("/v1/stock/import", whTok, "stok-hatalar.xlsx", "xlsx", "en", fixed.Bytes()))
	base2 := "/v1/tenant/imports/" + job2.UUID
	pv2 := it.importJob("POST", base2+"/preview", whTok, nil, http.StatusOK)
	if pv2.PreviewSummary.Counts["new"] != 2 || pv2.PreviewSummary.Invalid != 0 {
		t.Fatalf("fixed preview = %+v", pv2.PreviewSummary)
	}

	// 5. Confirm / rollback: not for a reader without stock.import; the
	// uploader and another stock.import holder may.
	it.importJob("POST", base+"/confirm", staffTok, nil, http.StatusForbidden)
	it.importJob("POST", base2+"/confirm", staffTok, nil, http.StatusForbidden)
	applied := it.importJob("POST", base2+"/confirm", wh2Tok, nil, http.StatusAccepted)
	if applied.Status != "applied" || applied.PreviewSummary.Counts["applied"] != 2 {
		t.Fatalf("confirm by stock.import holder = %+v", applied)
	}
	it.importJob("POST", base2+"/rollback", staffTok, nil, http.StatusForbidden)
	undone := it.importJob("POST", base2+"/rollback", whTok, nil, http.StatusOK)
	if undone.Status != "rolled_back" {
		t.Fatalf("rollback by uploader = %+v", undone)
	}
	it.importJob("GET", base2, staffTok, nil, http.StatusOK)
	// The platform route does not serve tenant jobs.
	if rec := it.raw("GET", "/v1/platform/imports/"+job.UUID+"/error-report", whTok, "", nil, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("platform report of a tenant job = %d", rec.Code)
	}
}

// TEC-757 acceptance (catalog): the product import preview reports the
// invalid rows (unknown category, bad number, missing name, unit type)
// before anything is applied, and the error report shows them.
func TestIntegrationCatalogImportPreviewValidation(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	center := it.brandCenter("olex")
	u, pw := it.user("t757-cat")
	it.member(center, u, "staff", rbac.RoleCenterStaff)
	tok := it.loginOrg(u, pw, center)
	existing := it.product(center, "T757C")
	cat, err := it.q.GetProductCategory(t.Context(), db.GetProductCategoryParams{ID: existing.CategoryID, BrandID: center.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	sku := func(n string) string { return "T757-" + it.suffix + "-" + n }
	csv := "sku,name,category,unit_type,warranty_duration_months\n" +
		sku("1") + ",Film," + cat.Name + ",piece,12\n" +
		sku("2") + ",Film,NOPE-" + it.suffix + ",piece,12\n" +
		sku("3") + ",Film," + cat.Name + ",piece,abc\n" +
		sku("4") + ",," + cat.Name + ",box,700\n"
	job := it.uploadedJob(it.uploadImportFile("/v1/catalog/products/import", tok, "products.csv", "csv", "en", []byte(csv)))
	base := "/v1/tenant/imports/" + job.UUID
	pv := it.importJob("POST", base+"/preview", tok, nil, http.StatusOK)
	if pv.PreviewSummary.Total != 4 || pv.PreviewSummary.Invalid != 3 {
		t.Fatalf("preview = %+v", pv.PreviewSummary)
	}
	wantCodes := map[int][]string{
		2: {"IMPORT_NOT_FOUND"},
		3: {"IMPORT_INVALID_INTEGER"},
		4: {"IMPORT_REQUIRED", "IMPORT_INVALID_VALUE", "IMPORT_OUT_OF_RANGE"},
	}
	for i, want := range wantCodes {
		got := pv.rowCodes(i)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("row %d codes = %v, want %v", i, got, want)
		}
	}
	if st := pv.rowStatus(1); st != "valid" {
		t.Errorf("row 1 status = %q", st)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM products WHERE sku LIKE $1`, "T757-"+it.suffix+"-%"); n != 0 {
		t.Fatalf("preview wrote %d products", n)
	}
	rows, _ := it.errorReport(base, tok)
	if len(rows) != 5 || rows[1][0] != "2" || rows[1][1] != "Invalid" ||
		!strings.Contains(rows[1][2], "was not found") || rows[4][0] != "1" || rows[4][1] != "Valid" {
		t.Fatalf("report = %q", rows)
	}
}
