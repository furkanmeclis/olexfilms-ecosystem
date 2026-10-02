package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockrebuild "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

type stockImportJob struct {
	UUID           string `json:"uuid"`
	Status         string `json:"status"`
	PreviewSummary struct {
		Total     int            `json:"total"`
		BatchUUID string         `json:"batch_uuid"`
		Counts    map[string]int `json:"counts"`
		Rows      []struct {
			Index       int            `json:"index"`
			Status      string         `json:"status"`
			StatusLabel string         `json:"status_label"`
			Target      map[string]any `json:"target"`
		} `json:"rows"`
		Errors []struct {
			Index int    `json:"index"`
			Code  string `json:"code"`
			Error string `json:"error"`
		} `json:"errors"`
	} `json:"preview_summary"`
}

func (j stockImportJob) rowStatus(i int) string {
	for _, r := range j.PreviewSummary.Rows {
		if r.Index == i {
			return r.Status
		}
	}
	return ""
}

func (j stockImportJob) rowTarget(i int) map[string]any {
	for _, r := range j.PreviewSummary.Rows {
		if r.Index == i {
			return r.Target
		}
	}
	return nil
}

func (j stockImportJob) rowCodes(i int) []string {
	var out []string
	for _, e := range j.PreviewSummary.Errors {
		if e.Index == i {
			out = append(out, e.Code)
		}
	}
	return out
}

// uploadStockImport posts a CSV to POST /v1/stock/import.
func (it *itest) uploadStockImport(token, csv string) (int, stockImportJob) {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "stock.csv")
	if err != nil {
		it.t.Fatal(err)
	}
	_, _ = part.Write([]byte(csv))
	_ = mw.WriteField("format", "csv")
	_ = mw.WriteField("locale", "en")
	_ = mw.Close()
	rec := it.raw("POST", "/v1/stock/import", token, mw.FormDataContentType(), buf.Bytes(), nil)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	var job stockImportJob
	if rec.Code < 300 {
		if err := json.Unmarshal(env.Data, &job); err != nil {
			it.t.Fatal(err)
		}
	}
	return rec.Code, job
}

func (it *itest) importJob(method, path, token string, body any, want int) stockImportJob {
	it.t.Helper()
	got, env := it.do(method, path, hostOlex, token, body)
	if got != want {
		it.t.Fatalf("%s %s = %d %s, want %d", method, path, got, errCode(env), want)
	}
	var job stockImportJob
	if got < 300 {
		if err := json.Unmarshal(env.Data, &job); err != nil {
			it.t.Fatal(err)
		}
	}
	return job
}

// TEC-158 acceptance: a mixed CSV (new rows, a duplicate barcode, an unknown
// product, a unit held by another organization) is classified in the
// preview with its targets; confirm writes the new units through ledger
// entry movements in one batch and a second confirm writes nothing; undo
// voids the untouched units and refuses the one that moved; the rebuild
// check finds no drift after the import and the undo.
func TestIntegrationStockImport(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t158-dist", "distributor", center)
	wh, wpw := it.user("t158-wh")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t158-dist-owner")
	it.member(dist, distOwner, "owner")
	whTok := it.loginOrg(wh, wpw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)

	piece := it.product(center, "T158P")
	roll := it.product(center, "T158R")
	fixed := it.product(center, "T158F")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, fixed.ID); err != nil {
		t.Fatalf("fixed product: %v", err)
	}

	c := it.stockChain()
	cLoc := c.location(center, "C158")
	dLoc := c.location(dist, "D158")
	locCode := "C158-" + it.suffix
	inStock := c.unit(center, piece, 1580)
	c.post(ledger.TypeEntry, inStock, c.nextRef(), cLoc)
	atDist := c.unit(center, piece, 1581)
	c.post(ledger.TypeEntry, atDist, c.nextRef(), cLoc)
	c.ship(atDist, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)

	bc := func(n string) string { return "T158-" + it.suffix + "-" + n }
	csv := strings.Join([]string{
		"barcode,product_sku,quantity,meters,location_code",
		bc("N1") + "," + piece.Sku + ",,," + locCode, // 1 new, at a location
		bc("N2") + "," + roll.Sku + ",,30.50,",       // 2 new roll
		bc("N3") + "," + fixed.Sku + ",24,,",         // 3 new fixed barcode
		bc("N1") + "," + piece.Sku + ",,,",           // 4 duplicate in the file
		inStock.Barcode + "," + piece.Sku + ",,,",    // 5 duplicate: already in stock
		bc("X1") + ",NOPE-" + it.suffix + ",,,",      // 6 invalid product
		atDist.Barcode + "," + piece.Sku + ",,,",     // 7 conflict: another organization's unit
		bc("N4") + "," + piece.Sku + ",,,",           // 8 new (moves before the undo)
	}, "\n") + "\n"

	// Only the brand center imports (K14).
	if code, _ := it.uploadStockImport(distTok, csv); code != http.StatusForbidden {
		t.Fatalf("distributor upload = %d, want 403", code)
	}
	code, job := it.uploadStockImport(whTok, csv)
	if code != http.StatusCreated || job.UUID == "" {
		t.Fatalf("upload = %d %+v", code, job)
	}
	base := "/v1/tenant/imports/" + job.UUID
	mapping := map[string]any{"mapping": map[string]string{
		"barcode": "barcode", "product_sku": "product_sku", "quantity": "quantity",
		"meters": "meters", "location_code": "location_code",
	}}
	it.importJob("PATCH", base+"/mapping", whTok, mapping, http.StatusOK)

	// 1. Preview (dry run): rows classified with their targets, nothing
	// written to the ledger.
	// Movements and units of this batch only (other packages may write the
	// ledger concurrently).
	importMoves := func() int {
		return it.countRows(`SELECT COUNT(*) FROM stock_movements m
			JOIN stock_import_rows r ON m.reference_type = 'stock_import_row' AND m.reference_id = r.id
			JOIN stock_import_batches b ON b.id = r.batch_id
			WHERE b.uuid = $1::uuid`, job.UUID)
	}
	pv := it.importJob("POST", base+"/preview", whTok, nil, http.StatusOK)
	want := map[int]string{1: "new", 2: "new", 3: "new", 4: "duplicate", 5: "duplicate", 6: "invalid", 7: "conflict", 8: "new"}
	for i, st := range want {
		if got := pv.rowStatus(i); got != st {
			t.Errorf("preview row %d = %q, want %q", i, got, st)
		}
	}
	wantCodes := map[int]string{4: "STOCK_IMPORT_DUPLICATE_IN_FILE", 5: "STOCK_IMPORT_ALREADY_IN_STOCK",
		6: "STOCK_IMPORT_PRODUCT_NOT_FOUND", 7: "STOCK_IMPORT_HELD_BY_OTHER_ORG"}
	for i, code := range wantCodes {
		if got := pv.rowCodes(i); len(got) != 1 || got[0] != code {
			t.Errorf("preview row %d codes = %v, want %s", i, got, code)
		}
	}
	for _, e := range pv.PreviewSummary.Errors {
		if e.Index == 6 && e.Error != "No product with this SKU in the brand." {
			t.Errorf("row 6 message = %q", e.Error)
		}
	}
	if pv.PreviewSummary.Counts["new"] != 4 || pv.PreviewSummary.Counts["duplicate"] != 2 ||
		pv.PreviewSummary.Counts["invalid"] != 1 || pv.PreviewSummary.Counts["conflict"] != 1 ||
		pv.PreviewSummary.BatchUUID != job.UUID {
		t.Fatalf("preview summary = %+v", pv.PreviewSummary)
	}
	if tg := pv.rowTarget(1); tg["owner_type"] != "warehouse_location" || tg["location_code"] != locCode ||
		tg["organization_uuid"] != center.Uuid.String() || tg["product_uuid"] != piece.Uuid.String() || tg["unit_kind"] != "serial" {
		t.Errorf("row 1 target = %+v", tg)
	}
	if tg := pv.rowTarget(3); tg["owner_type"] != "organization" || tg["unit_kind"] != "fixed" || tg["product_sku"] != fixed.Sku {
		t.Errorf("row 3 target = %+v", tg)
	}
	if tg := pv.rowTarget(6); tg["product_uuid"] != nil || tg["organization_uuid"] != center.Uuid.String() {
		t.Errorf("row 6 target = %+v", tg)
	}
	if n := importMoves(); n != 0 {
		t.Fatalf("preview wrote %d movements", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM units WHERE barcode LIKE $1`, "T158-"+it.suffix+"-%"); n != 0 {
		t.Fatalf("preview created %d units", n)
	}

	// 2. Confirm: one batch, four units, four ledger entries keyed by
	// batch row.
	applied := it.importJob("POST", base+"/confirm", whTok, nil, http.StatusAccepted)
	if applied.Status != "applied" || applied.PreviewSummary.Counts["applied"] != 4 {
		t.Fatalf("confirm = %+v", applied)
	}
	type stagedRow struct {
		id                 int64
		barcode            string
		unitID, movementID int64
	}
	rowsOf := func() map[int32]stagedRow {
		rs, err := it.pool.Query(ctx, `SELECT r.row_number, r.id, COALESCE(r.barcode, ''), COALESCE(r.unit_id, 0), COALESCE(r.movement_id, 0)
			FROM stock_import_rows r JOIN stock_import_batches b ON b.id = r.batch_id
			WHERE b.uuid = $1::uuid ORDER BY r.row_number`, job.UUID)
		if err != nil {
			t.Fatalf("rows: %v", err)
		}
		defer rs.Close()
		out := map[int32]stagedRow{}
		for rs.Next() {
			var n int32
			var r stagedRow
			if err := rs.Scan(&n, &r.id, &r.barcode, &r.unitID, &r.movementID); err != nil {
				t.Fatal(err)
			}
			out[n] = r
		}
		return out
	}
	rows := rowsOf()
	var unitIDs []int64
	for _, n := range []int32{1, 2, 3, 8} {
		r := rows[n]
		key := fmt.Sprintf("import:stock_import_row:%d:entry:%s", r.id, r.barcode)
		mv, err := it.q.GetStockMovementByIdempotencyKey(ctx, key)
		if err != nil || mv.ID != r.movementID || mv.UnitID != r.unitID || mv.Type != "entry" || mv.OrganizationID != center.ID {
			t.Fatalf("row %d movement %s: %+v %v", n, key, mv, err)
		}
		u, err := it.q.GetUnit(ctx, r.unitID)
		if err != nil || u.Source != "imported" || u.Barcode != r.barcode {
			t.Fatalf("row %d unit: %+v %v", n, u, err)
		}
		unitIDs = append(unitIDs, r.unitID)
	}
	for _, n := range []int32{4, 5, 6, 7} {
		if rows[n].unitID != 0 || rows[n].movementID != 0 {
			t.Fatalf("row %d was written: %+v", n, rows[n])
		}
	}
	if st, err := it.q.GetUnitCurrentState(ctx, rows[1].unitID); err != nil || st.OwnerType != "warehouse_location" || st.OwnerID != cLoc.ID {
		t.Fatalf("row 1 state: %+v %v", st, err)
	}
	var meters string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(remaining_meters::text, '') FROM units WHERE id = $1`, rows[2].unitID).Scan(&meters); err != nil || meters != "30.50" {
		t.Fatalf("roll meters = %q %v", meters, err)
	}
	if n := it.countRows(`SELECT COALESCE(SUM(quantity_on_hand), 0)::int FROM fixed_barcode_holdings WHERE unit_id = $1 AND holder_org_id = $2`,
		rows[3].unitID, center.ID); n != 24 {
		t.Fatalf("fixed quantity = %d, want 24", n)
	}
	if n := importMoves(); n != 4 {
		t.Fatalf("confirm wrote %d movements, want 4", n)
	}

	// 3. A second confirm of the same batch writes nothing.
	again := it.importJob("POST", base+"/confirm", whTok, nil, http.StatusAccepted)
	if again.Status != "applied" {
		t.Fatalf("second confirm = %+v", again)
	}
	if n := importMoves(); n != 4 {
		t.Fatalf("second confirm: %d movements, want 4", n)
	}

	// 4. Rebuild/replay after the import: no drift.
	it.noStockDrift(ctx, center.ID, unitIDs, piece.ID, roll.ID, fixed.ID)

	// 5. Row 8's unit moves; undo voids rows 1-3 and refuses row 8.
	n4, err := it.q.GetUnit(ctx, rows[8].unitID)
	if err != nil {
		t.Fatal(err)
	}
	c.post(ledger.TypePlacement, n4, c.nextRef(), cLoc)
	undone := it.importJob("POST", base+"/rollback", whTok, nil, http.StatusOK)
	if undone.Status != "rolled_back" || undone.PreviewSummary.Counts["undone"] != 3 || undone.PreviewSummary.Counts["undo_rejected"] != 1 {
		t.Fatalf("undo = %+v", undone.PreviewSummary)
	}
	if got := undone.rowCodes(8); len(got) != 1 || got[0] != "STOCK_IMPORT_UNDO_UNIT_TOUCHED" {
		t.Fatalf("row 8 undo codes = %v", got)
	}
	if n := importMoves(); n != 7 {
		t.Fatalf("after undo: %d movements, want 4 entries + 3 voids", n)
	}
	rows = rowsOf()
	for _, n := range []int32{1, 2, 3} {
		key := fmt.Sprintf("import_undo:stock_import_row:%d:void:%s", rows[n].id, rows[n].barcode)
		if mv, err := it.q.GetStockMovementByIdempotencyKey(ctx, key); err != nil || mv.Type != "void" {
			t.Fatalf("row %d undo movement: %+v %v", n, mv, err)
		}
	}
	if u, _ := it.q.GetUnit(ctx, rows[1].unitID); u.Status != "void" {
		t.Fatalf("row 1 unit after undo = %s", u.Status)
	}
	if n := it.countRows(`SELECT COALESCE(SUM(quantity_on_hand), 0)::int FROM fixed_barcode_holdings WHERE unit_id = $1`, rows[3].unitID); n != 0 {
		t.Fatalf("fixed quantity after undo = %d", n)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, rows[8].unitID); err != nil || st.Status != "placed" {
		t.Fatalf("row 8 unit after undo: %+v %v", st, err)
	}
	var batchStatus string
	if err := it.pool.QueryRow(ctx, `SELECT status FROM stock_import_batches WHERE uuid = $1::uuid`, job.UUID).Scan(&batchStatus); err != nil || batchStatus != "partially_undone" {
		t.Fatalf("batch status = %q %v", batchStatus, err)
	}

	// 6. A second undo writes nothing.
	it.importJob("POST", base+"/rollback", whTok, nil, http.StatusOK)
	if n := importMoves(); n != 7 {
		t.Fatalf("second undo: %d movements, want 7", n)
	}

	// 7. Rebuild/replay after the undo: no drift.
	it.noStockDrift(ctx, center.ID, unitIDs, piece.ID, roll.ID, fixed.ID)
}

// noStockDrift runs the rebuild check of an organization and fails on any
// diff or anomaly of the given units, and on any product stock drift of the
// organization.
func (it *itest) noStockDrift(ctx context.Context, orgID int64, unitIDs []int64, productIDs ...int64) {
	it.t.Helper()
	rep, err := stockrebuild.New(it.pool, it.q).Check(ctx, orgID)
	if err != nil {
		it.t.Fatalf("rebuild check: %v", err)
	}
	units := map[int64]db.Unit{}
	for _, id := range unitIDs {
		u, err := it.q.GetUnit(ctx, id)
		if err != nil {
			it.t.Fatal(err)
		}
		units[id] = u
	}
	for _, d := range rep.Diffs {
		if _, ok := units[d.UnitID]; ok {
			it.t.Fatalf("drift on an imported unit: %+v", d)
		}
		for _, p := range productIDs {
			if d.Key == fmt.Sprintf("organization:%d/product:%d", orgID, p) ||
				strings.HasPrefix(d.Key, "location:") && strings.HasSuffix(d.Key, fmt.Sprintf("/product:%d", p)) {
				it.t.Fatalf("product stock drift: %+v", d)
			}
		}
	}
	for _, a := range rep.Anomalies {
		for _, u := range units {
			if strings.Contains(a, "("+u.Barcode+")") {
				it.t.Fatalf("anomaly on an imported unit: %s", a)
			}
		}
	}
}
