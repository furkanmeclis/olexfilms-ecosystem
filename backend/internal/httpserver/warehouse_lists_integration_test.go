package httpserver

// TEC-375 (DT-BE-6): list contract of the warehouse lists (stock entries,
// stock counts and their scans, warehouse transfers, end-of-day reports)
// and the barcode batch list: sort, multi-value filters, ranges and q.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// dt6Env is a center warehouse user with two fresh warehouses.
type dt6Env struct {
	it     *itest
	center db.Organization
	tok    string
	tag    string
	whA    int64
	whB    int64
	whAU   string
	whBU   string
}

func dt6Setup(t *testing.T) *dt6Env {
	t.Helper()
	it := newIntegration(t)
	center := it.brandCenter("olex")
	u, pw := it.user("t375-wh")
	it.member(center, u, "staff", rbac.RoleCenterWarehouse)
	e := &dt6Env{it: it, center: center, tok: it.loginOrg(u, pw, center)}
	s := it.suffix
	if len(s) > 9 {
		s = s[len(s)-9:]
	}
	e.tag = strings.ToUpper("D6" + s)
	e.whA, e.whAU = e.warehouse("A"+e.tag, "Alpha "+e.tag)
	e.whB, e.whBU = e.warehouse("B"+e.tag, "Beta "+e.tag)
	return e
}

func (e *dt6Env) warehouse(code, name string) (int64, string) {
	e.it.t.Helper()
	var id int64
	var uuid string
	e.row(&[]any{&id, &uuid}, "INSERT INTO warehouses (organization_id, code, name) VALUES ($1, $2, $3) RETURNING id, uuid::text",
		e.center.ID, code, name)
	return id, uuid
}

// row runs an INSERT ... RETURNING and scans into dst.
func (e *dt6Env) row(dst *[]any, sql string, args ...any) {
	e.it.t.Helper()
	if err := e.it.pool.QueryRow(context.Background(), sql, args...).Scan(*dst...); err != nil {
		e.it.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *dt6Env) uuid(sql string, args ...any) string {
	e.it.t.Helper()
	var u string
	e.row(&[]any{&u}, sql, args...)
	return u
}

// list GETs path and returns the status, the item uuids in order and total.
func (e *dt6Env) list(path string) (int, []string, int64) {
	e.it.t.Helper()
	code, env := e.it.do("GET", path, hostOlex, e.tok, nil)
	if code != http.StatusOK {
		return code, nil, 0
	}
	var page struct {
		Total int64 `json:"total"`
		Limit int32 `json:"limit"`
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		e.it.t.Fatal(err)
	}
	out := make([]string, 0, len(page.Items))
	for _, i := range page.Items {
		out = append(out, i.UUID)
	}
	return code, out, page.Total
}

// want asserts the uuid order of a list.
func (e *dt6Env) want(path string, want ...string) {
	e.it.t.Helper()
	code, got, _ := e.list(path)
	if code != http.StatusOK || strings.Join(got, ",") != strings.Join(want, ",") {
		e.it.t.Fatalf("GET %s = %d %v, want %v", path, code, got, want)
	}
}

// bad asserts a 400 VALIDATION_ERROR.
func (e *dt6Env) bad(path string) {
	e.it.t.Helper()
	code, env := e.it.do("GET", path, hostOlex, e.tok, nil)
	if code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "VALIDATION_ERROR" {
		e.it.t.Fatalf("GET %s = %d %+v, want 400 VALIDATION_ERROR", path, code, env.Error)
	}
}

func TestIntegrationWarehouseListsEntries(t *testing.T) {
	e := dt6Setup(t)
	ins := func(wh int64, mode, status, note, created string) string {
		confirmed := "NULL"
		if status == "confirmed" || status == "undone" {
			confirmed = "NOW()"
		}
		var n any
		if note != "" {
			n = note
		}
		return e.uuid(`INSERT INTO stock_entries (organization_id, brand_id, warehouse_id, mode, status, note, created_at, confirmed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, `+confirmed+`) RETURNING uuid::text`,
			e.center.ID, e.center.BrandID, wh, mode, status, n, created)
	}
	e1 := ins(e.whA, "with_existing", "draft", "red pallet", "2031-01-01T12:00:00Z")
	e2 := ins(e.whB, "generate_new", "confirmed", "blue", "2031-01-02T12:00:00Z")
	e3 := ins(e.whA, "generate_new", "cancelled", "", "2031-01-03T12:00:00Z")
	e4 := ins(e.whB, "with_existing", "undone", "", "2031-01-04T12:00:00Z")

	base := "/v1/warehouse/stock-entries?warehouse_uuid=" + e.whAU + "," + e.whBU
	e.want(base, e4, e3, e2, e1)
	e.want(base+"&sort=created_at", e1, e2, e3, e4)
	e.want(base+"&sort=status", e1, e2, e4, e3)
	e.want(base+"&sort=-status", e3, e4, e2, e1)
	e.want(base+"&sort=warehouse", e1, e3, e2, e4)
	e.want(base+"&sort=-warehouse", e4, e2, e3, e1)
	e.want(base+"&status=draft,confirmed", e2, e1)
	e.want(base+"&mode=generate_new", e3, e2)
	e.want("/v1/warehouse/stock-entries?warehouse_uuid="+e.whAU, e3, e1)
	e.want(base+"&created_from=2031-01-02&created_to=2031-01-03", e3, e2)
	e.want(base+"&q=RED", e1)
	e.want(base+"&q=Alpha", e3, e1)
	e.want(base + "&q=%25") // a literal %, not a wildcard
	if _, got, total := e.list(base + "&limit=2&offset=1"); total != 4 || strings.Join(got, ",") != e3+","+e2 {
		t.Fatalf("page = %v total %d", got, total)
	}
	e.bad(base + "&sort=note")
	e.bad(base + "&status=draft,bogus")
	e.bad(base + "&mode=bogus")
	e.bad("/v1/warehouse/stock-entries?warehouse_uuid=nope")
	e.bad(base + "&created_from=2031-01-03&created_to=2031-01-02")
}

func TestIntegrationWarehouseListsCounts(t *testing.T) {
	e := dt6Setup(t)
	ins := func(wh int64, method, visibility, status, note, created string) (int64, string) {
		var id int64
		var u string
		var n any
		if note != "" {
			n = note
		}
		completed, approved := "NULL", "NULL"
		if status == "approved" || status == "pending_review" {
			completed = "NOW()"
		}
		if status == "approved" {
			approved = "NOW()"
		}
		e.row(&[]any{&id, &u}, `INSERT INTO stock_counts (organization_id, brand_id, warehouse_id, method, visibility, scope_type,
			status, note, created_at, completed_at, approved_at)
			VALUES ($1, $2, $3, $4, $5, 'warehouse', $6, $7, $8, `+completed+`, `+approved+`) RETURNING id, uuid::text`,
			e.center.ID, e.center.BrandID, wh, method, visibility, status, n, created)
		return id, u
	}
	c1ID, c1 := ins(e.whA, "location_first", "blind", "draft", "", "2031-01-03T12:00:00Z")
	_, c2 := ins(e.whB, "unit_first", "guided", "in_progress", "cycle count", "2031-01-01T12:00:00Z")
	_, c3 := ins(e.whA, "product_qty", "guided", "approved", "", "2031-01-04T12:00:00Z")
	_, c4 := ins(e.whB, "unit_first", "blind", "cancelled", "", "2031-01-02T12:00:00Z")

	base := "/v1/warehouse/stock-counts?warehouse_uuid=" + e.whAU + "," + e.whBU
	e.want(base, c3, c1, c4, c2)
	e.want(base+"&sort=created_at", c2, c4, c1, c3)
	e.want(base+"&sort=status", c1, c2, c3, c4)
	e.want(base+"&sort=-status", c4, c3, c2, c1)
	e.want(base+"&sort=warehouse", c1, c3, c2, c4)
	e.want(base+"&method=unit_first", c4, c2)
	e.want(base+"&method=unit_first,product_qty&visibility=guided", c3, c2)
	e.want(base+"&visibility=blind", c1, c4)
	e.want(base+"&status=draft,approved", c3, c1)
	e.want(base+"&scope_type=warehouse,room", c3, c1, c4, c2)
	e.want(base + "&scope_type=room")
	e.want(base+"&created_to=2031-01-02", c4, c2)
	e.want(base+"&q=cycle", c2)
	e.want(base+"&q=beta", c4, c2)
	e.bad(base + "&sort=method")
	e.bad(base + "&method=bogus")
	e.bad(base + "&visibility=bogus")
	e.bad(base + "&status=bogus")

	// Scans: the full list without limit/offset, a page with them.
	p := e.it.product(e.center, "t375")
	var scans []string
	for i := range 5 {
		scans = append(scans, e.uuid(`INSERT INTO stock_count_scans (count_id, kind, raw_code, product_id, quantity)
			VALUES ($1, 'product', $2, $3, $4) RETURNING uuid::text`, c1ID, p.Sku, p.ID, i+1))
	}
	path := "/v1/warehouse/stock-counts/" + c1 + "/scans"
	code, env := e.it.do("GET", path, hostOlex, e.tok, nil)
	var all struct {
		Items  []struct{ UUID string } `json:"items"`
		Total  int64                   `json:"total"`
		Limit  int32                   `json:"limit"`
		Offset int32                   `json:"offset"`
	}
	if code != http.StatusOK {
		t.Fatalf("scans = %d", code)
	}
	if err := json.Unmarshal(env.Data, &all); err != nil {
		t.Fatal(err)
	}
	if len(all.Items) != 5 || all.Total != 5 || all.Limit != 5 || all.Items[0].UUID != scans[0] {
		t.Fatalf("all scans = %+v", all)
	}
	if _, got, total := e.list(path + "?limit=2&offset=1"); total != 5 || strings.Join(got, ",") != scans[1]+","+scans[2] {
		t.Fatalf("scan page = %v total %d", got, total)
	}
	if _, got, total := e.list(path + "?offset=4"); total != 5 || len(got) != 1 || got[0] != scans[4] {
		t.Fatalf("scan offset = %v total %d", got, total)
	}
}

func TestIntegrationWarehouseListsTransfers(t *testing.T) {
	e := dt6Setup(t)
	ins := func(no string, from, to int64, status, note, created string) string {
		shipped, completed, cancelled := "NULL", "NULL", "NULL"
		switch status {
		case "completed":
			shipped, completed = "NOW()", "NOW()"
		case "in_transit":
			shipped = "NOW()"
		case "cancelled":
			cancelled = "NOW()"
		}
		var n any
		if note != "" {
			n = note
		}
		return e.uuid(`INSERT INTO warehouse_transfers (transfer_no, organization_id, brand_id, from_warehouse_id, to_warehouse_id,
			status, note, created_at, shipped_at, completed_at, cancelled_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, `+shipped+`, `+completed+`, `+cancelled+`) RETURNING uuid::text`,
			no, e.center.ID, e.center.BrandID, from, to, status, n, created)
	}
	t1 := ins(e.tag+"-3", e.whA, e.whB, "completed", "", "2031-01-02T12:00:00Z")
	t2 := ins(e.tag+"-1", e.whB, e.whA, "draft", "urgent move", "2031-01-03T12:00:00Z")
	t3 := ins(e.tag+"-2", e.whA, e.whB, "cancelled", "", "2031-01-01T12:00:00Z")

	base := "/v1/warehouse/transfers?from_warehouse_uuid=" + e.whAU + "," + e.whBU
	e.want(base, t2, t1, t3)
	e.want(base+"&sort=transfer_no", t2, t3, t1)
	e.want(base+"&sort=-transfer_no", t1, t3, t2)
	e.want(base+"&sort=created_at", t3, t1, t2)
	e.want(base+"&sort=status", t2, t1, t3)
	e.want(base+"&sort=-status", t3, t1, t2)
	e.want(base+"&status=draft,cancelled", t2, t3)
	e.want("/v1/warehouse/transfers?from_warehouse_uuid="+e.whAU, t1, t3)
	e.want("/v1/warehouse/transfers?to_warehouse_uuid="+e.whAU, t2)
	e.want(base+"&to_warehouse_uuid="+e.whBU, t1, t3)
	e.want(base+"&q="+strings.ToLower(e.tag)+"-2", t3)
	e.want(base+"&q=urgent", t2)
	e.want(base+"&created_to=2031-01-02", t1, t3)
	e.bad(base + "&sort=note")
	e.bad(base + "&status=bogus")
	e.bad("/v1/warehouse/transfers?to_warehouse_uuid=nope")
}

func TestIntegrationWarehouseListsEOD(t *testing.T) {
	e := dt6Setup(t)
	ins := func(date, kind, generated string) string {
		return e.uuid(`INSERT INTO eod_reports (organization_id, brand_id, warehouse_id, report_date, timezone,
			period_start, period_end, kind, generated_at)
			VALUES ($1, $2, $3, $4::date, 'UTC', $4::date, $4::date + 1, $5, $6) RETURNING uuid::text`,
			e.center.ID, e.center.BrandID, e.whA, date, kind, generated)
	}
	r1 := ins("2031-01-01", "auto", "2031-01-05T00:00:00Z")
	r2 := ins("2031-01-02", "manual", "2031-01-03T00:00:00Z")
	r3 := ins("2031-01-03", "auto", "2031-01-04T00:00:00Z")

	base := "/v1/warehouse/eod-reports?warehouse_uuid=" + e.whAU
	e.want(base, r3, r2, r1)
	e.want(base+"&sort=report_date", r1, r2, r3)
	e.want(base+"&sort=generated_at", r2, r3, r1)
	e.want(base+"&sort=-generated_at", r1, r3, r2)
	e.want(base+"&kind=manual", r2)
	e.want(base+"&kind=auto,manual&date_from=2031-01-02", r3, r2)
	e.want(base+"&kind=auto&scope=warehouse", r3, r1)
	e.bad(base + "&sort=kind")
	e.bad(base + "&kind=bogus")
}

func TestIntegrationWarehouseListsBarcodes(t *testing.T) {
	e := dt6Setup(t)
	p := e.it.product(e.center, "t375b")
	other := e.it.product(e.center, "t375c")
	ins := func(prod db.Product, qty, prints int, prefix string, first int, created string) string {
		return e.uuid(`INSERT INTO barcode_batches (organization_id, brand_id, product_id, quantity, prefix, first_seq, last_seq,
			print_count, created_at)
			VALUES ($1, $2, $3, $4::int, $5, $6::bigint, $6::bigint + $4::int - 1, $7, $8) RETURNING uuid::text`,
			e.center.ID, e.center.BrandID, prod.ID, qty, prefix, first, prints, created)
	}
	b1 := ins(p, 5, 0, "TSA", 1, "2031-01-01T12:00:00Z")
	b2 := ins(p, 2, 3, "TSB", 10, "2031-01-03T12:00:00Z")
	b3 := ins(p, 9, 1, "TSA", 6, "2031-01-02T12:00:00Z")
	b4 := ins(other, 1, 0, "TSC", 1, "2031-01-02T12:00:00Z")

	base := "/v1/stock/barcodes?product_uuid=" + p.Uuid.String()
	e.want(base, b2, b3, b1)
	e.want(base+","+other.Uuid.String(), b2, b4, b3, b1) // b3 and b4 tie: id desc
	e.want(base+"&sort=created_at", b1, b3, b2)
	e.want(base+"&sort=quantity", b2, b1, b3)
	e.want(base+"&sort=-quantity", b3, b1, b2)
	e.want(base+"&sort=print_count", b1, b3, b2)
	e.want(base+"&printed=true", b2, b3)
	e.want(base+"&printed=false", b1)
	e.want(base+"&created_from=2031-01-02", b2, b3)
	e.want(base+"&q=tsb", b2)
	e.want(base+"&q=TSA-00000014", b3)
	e.want("/v1/stock/barcodes?q="+other.Sku, b4)
	e.bad(base + "&sort=prefix")
	e.bad(base + "&printed=maybe")
	e.bad("/v1/stock/barcodes?product_uuid=nope")
}
