package httpserver

// TEC-373 (DT-BE-5): list contract of the order, stock transfer, stock
// unit and product stock lists (sort, multi-value filters, ranges) and the
// order / stock unit list exports.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/jackc/pgx/v5/pgtype"
)

// dt5Integration is an integration server with an export queue.
func dt5Integration(t *testing.T) *itest {
	t.Helper()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	return newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory(); d.Queue = qc })
}

// dt5JobQuery reads the stored query of an export job.
func (it *itest) dt5JobQuery(jobUUID string) ioengine.ExportQuery {
	it.t.Helper()
	var raw []byte
	if err := it.pool.QueryRow(context.Background(), "SELECT query_json FROM export_jobs WHERE uuid = $1", jobUUID).Scan(&raw); err != nil {
		it.t.Fatal(err)
	}
	var q ioengine.ExportQuery
	if err := json.Unmarshal(raw, &q); err != nil {
		it.t.Fatal(err)
	}
	return q
}

// dt5Tag is a short per-test tag for order / transfer numbers.
func (it *itest) dt5Tag() string {
	s := it.suffix
	if len(s) > 9 {
		s = s[len(s)-9:]
	}
	return "D5" + s
}

func (it *itest) dt5Exec(sql string, args ...any) {
	it.t.Helper()
	if _, err := it.pool.Exec(context.Background(), sql, args...); err != nil {
		it.t.Fatalf("%s: %v", sql, err)
	}
}

// dt5List GETs a list and returns the status and the value of field of
// every item, in order.
func (it *itest) dt5List(token, path, field string) (int, []string, int64) {
	it.t.Helper()
	code, env := it.do("GET", path, hostOlex, token, nil)
	if code != http.StatusOK {
		return code, nil, 0
	}
	var page struct {
		Total int64             `json:"total"`
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		it.t.Fatal(err)
	}
	out := make([]string, 0, len(page.Items))
	for _, raw := range page.Items {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			it.t.Fatal(err)
		}
		v := m
		parts := strings.Split(field, ".")
		for _, p := range parts[:len(parts)-1] {
			v, _ = v[p].(map[string]any)
		}
		s, _ := v[parts[len(parts)-1]].(string)
		out = append(out, s)
	}
	return code, out, page.Total
}

func dt5Want(t *testing.T, name string, code int, got []string, want ...string) {
	t.Helper()
	if code != http.StatusOK || strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %d %v, want %v", name, code, got, want)
	}
}

func (it *itest) dt5Currency(center db.Organization) string {
	it.t.Helper()
	b, err := it.q.GetBrandByID(context.Background(), center.BrandID)
	if err != nil {
		it.t.Fatal(err)
	}
	return strings.TrimSpace(b.Currency)
}

func TestIntegrationOrderStockListsOrders(t *testing.T) {
	it := dt5Integration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t373-dist", "distributor", center)
	dealerA := it.org("t373-a", "dealer", dist)
	dealerB := it.org("t373-b", "dealer", dist)
	dist2 := it.org("t373-dist2", "distributor", center)
	owner, pw := it.user("t373-dist-owner")
	it.member(dist, owner, "owner")
	tok := it.loginOrg(owner, pw, dist)
	cur := it.dt5Currency(center)
	tag := it.dt5Tag()

	mk := func(n int, seller, buyer db.Organization, status, total, created string) db.Order {
		o, err := it.q.CreateOrder(ctx, db.CreateOrderParams{SellerOrgID: seller.ID, BrandID: center.BrandID, BuyerOrgID: buyer.ID, Currency: cur})
		if err != nil {
			t.Fatalf("order: %v", err)
		}
		cancelled := "NULL"
		if status == "cancelled" {
			cancelled = "NOW()"
		}
		it.dt5Exec(`UPDATE orders SET order_no = $2, status = $3, total = $4::numeric, created_at = $5::timestamptz,
			cancelled_at = `+cancelled+` WHERE id = $1`, o.ID, tag+"-"+strconv.Itoa(n), status, total, created)
		o.OrderNo = tag + "-" + strconv.Itoa(n)
		return o
	}
	o1 := mk(1, dist, dealerA, "draft", "100", "2026-01-01T10:00:00Z")
	o2 := mk(2, dist, dealerB, "submitted", "300", "2026-01-03T10:00:00Z")
	o3 := mk(3, center, dist, "cancelled", "200", "2026-01-02T10:00:00Z")
	mk(4, center, dist2, "draft", "50", "2026-01-02T12:00:00Z") // outside the distributor's scope
	base := "/v1/orders?q=" + tag

	list := func(name, query string, want ...db.Order) {
		t.Helper()
		code, got, total := it.dt5List(tok, base+query, "order_no")
		names := make([]string, len(want))
		for i, o := range want {
			names[i] = o.OrderNo
		}
		dt5Want(t, name, code, got, names...)
		if total != int64(len(want)) {
			t.Fatalf("%s total = %d", name, total)
		}
	}
	list("default -created_at", "", o2, o3, o1)
	list("total asc", "&sort=total", o1, o3, o2)
	list("total desc", "&sort=-total", o2, o3, o1)
	list("order_no desc", "&sort=-order_no", o3, o2, o1)
	list("status rank", "&sort=status", o1, o2, o3)
	list("status rank desc", "&sort=-status", o3, o2, o1)
	list("created asc", "&sort=created_at", o1, o3, o2)
	list("multi status", "&status=draft,cancelled&sort=order_no", o1, o3)
	list("buyer filter", "&buyer_org_uuid="+dealerA.Uuid.String()+","+dealerB.Uuid.String()+"&sort=order_no", o1, o2)
	list("seller filter", "&seller_org_uuid="+center.Uuid.String(), o3)
	list("out of scope party", "&buyer_org_uuid="+dist2.Uuid.String())
	list("total range", "&total_min=150&total_max=300&sort=total", o3, o2)
	list("created range", "&created_from=2026-01-02&created_to=2026-01-02", o3)
	list("side seller", "&side=seller&sort=order_no", o1, o2)
	for _, bad := range []string{"&sort=buyer", "&status=draft,bogus", "&buyer_org_uuid=nope", "&total_min=x",
		"&total_min=5&total_max=1", "&created_from=2026-01-03&created_to=2026-01-01"} {
		if code, _, _ := it.dt5List(tok, base+bad, "order_no"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}

	// Export: same filters and sort; a bad parameter is 400 at request time.
	if code, env := it.do("POST", "/v1/orders/export", hostOlex, tok, map[string]any{
		"format": "csv", "query": map[string]string{"sort": "buyer"},
	}); code != http.StatusBadRequest {
		t.Fatalf("export bad sort = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/orders/export", hostOlex, tok, map[string]any{"format": "docx"}); code != http.StatusBadRequest {
		t.Fatalf("export bad format = %d", code)
	}
	job := decodeData[exportJob](t, it.custDo("POST", "/v1/orders/export", tok, map[string]any{
		"format": "csv", "locale": "en", "query": map[string]string{"q": tag, "status": "draft,submitted", "sort": "-total", "limit": "1"},
	}, http.StatusAccepted))
	if job.Resource != ordersusecase.ResourceListExport {
		t.Fatalf("job = %+v", job)
	}
	stored := it.dt5JobQuery(job.UUID)
	if stored[ioengine.QueryScopeFilter] == "" || stored["sort"] != "-total" || stored["limit"] != "" {
		t.Fatalf("stored query = %v", stored)
	}
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(dist.ID, 10) // set by the worker
	adapter := ordersusecase.NewListExportAdapter(ordersusecase.New(it.pool, it.q, nil, nil))
	ds, err := adapter.Export(ctx, stored, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 2 || ds.Rows[0]["order_no"] != o2.OrderNo || ds.Rows[1]["order_no"] != o1.OrderNo ||
		ds.Rows[0]["status"] != "Submitted" || ds.Rows[0]["buyer"] != dealerB.Name {
		t.Fatalf("export rows = %v", ds.Rows)
	}
	// The worker re-authorizes the stored scope: dist2 is outside a dist job.
	stored[ioengine.QueryScopeFilter] = strconv.FormatInt(dist2.ID, 10)
	if _, err := adapter.Export(ctx, stored, i18n.Locale("en")); err == nil {
		t.Fatal("a job scope outside the job organization must fail")
	}
}

func TestIntegrationOrderStockListsTransfers(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t373t-dist", "distributor", center)
	dealerA := it.org("t373t-a", "dealer", dist)
	dealerB := it.org("t373t-b", "dealer", dist)
	ua, apw := it.user("t373t-a-owner")
	it.member(dealerA, ua, "owner")
	tok := it.loginOrg(ua, apw, dealerA)
	cur := it.dt5Currency(center)
	tag := it.dt5Tag()

	mk := func(n int, from, to, approver db.Organization, kind, status, extra, created string) string {
		r, err := it.q.InsertTransferRequest(ctx, db.InsertTransferRequestParams{
			FromOrgID: from.ID, BrandID: center.BrandID, ToOrgID: to.ID, ApproverOrgID: approver.ID,
			Currency: cur, Kind: kind, Reason: pgtype.Text{String: "t373", Valid: true},
		})
		if err != nil {
			t.Fatalf("transfer: %v", err)
		}
		no := tag + "-" + strconv.Itoa(n)
		it.dt5Exec(`UPDATE stock_transfer_requests SET transfer_no = $2, status = $3, created_at = $4::timestamptz`+extra+` WHERE id = $1`,
			r.ID, no, status, created)
		return no
	}
	t1 := mk(1, dealerA, dealerB, dist, "sibling", "requested", "", "2026-01-01T10:00:00Z")
	t2 := mk(2, dealerA, dist, dist, "return", "rejected", ", decided_at = NOW()", "2026-01-03T10:00:00Z")
	t3 := mk(3, dealerB, dealerA, dist, "sibling", "cancelled", ", cancelled_at = NOW()", "2026-01-02T10:00:00Z")
	base := "/v1/stock-transfers?q=" + tag

	list := func(name, query string, want ...string) {
		t.Helper()
		code, got, total := it.dt5List(tok, base+query, "transfer_no")
		dt5Want(t, name, code, got, want...)
		if total != int64(len(want)) {
			t.Fatalf("%s total = %d", name, total)
		}
	}
	list("default -created_at", "", t2, t3, t1)
	list("transfer_no asc", "&sort=transfer_no", t1, t2, t3)
	list("transfer_no desc", "&sort=-transfer_no", t3, t2, t1)
	list("status rank", "&sort=status", t1, t2, t3) // requested < rejected < cancelled
	list("created asc", "&sort=created_at", t1, t3, t2)
	list("multi status", "&status=requested,cancelled&sort=transfer_no", t1, t3)
	list("multi kind", "&kind=sibling,return&sort=transfer_no", t1, t2, t3)
	list("kind return", "&kind=return", t2)
	list("multi direction", "&direction=incoming&sort=transfer_no", t3)
	list("outgoing+incoming", "&direction=outgoing,incoming&sort=transfer_no", t1, t2, t3)
	list("created range", "&created_from=2026-01-02&created_to=2026-01-03&sort=created_at", t3, t2)
	list("organization filter", "&organization_uuid="+dist.Uuid.String(), t2)
	list("organization filter b", "&organization_uuid="+dealerB.Uuid.String()+"&sort=transfer_no", t1, t3)
	// q also matches the sender / receiver name.
	code, got, _ := it.dt5List(tok, "/v1/stock-transfers?q="+url.QueryEscape(dist.Name)+"&sort=transfer_no", "transfer_no")
	dt5Want(t, "q on organization name", code, got, t2)
	for _, bad := range []string{"&sort=kind", "&status=bogus", "&kind=sibling,x", "&direction=sideways",
		"&organization_uuid=nope", "&created_from=bad"} {
		if code, _, _ := it.dt5List(tok, base+bad, "transfer_no"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}
}

func TestIntegrationOrderStockListsUnits(t *testing.T) {
	it := dt5Integration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t373u-dist", "distributor", center)
	cOwner, cpw := it.user("t373u-center-owner")
	it.member(center, cOwner, "staff", rbac.RoleCenterWarehouse)
	dOwner, dpw := it.user("t373u-dist-owner")
	it.member(dist, dOwner, "owner")
	tok := it.loginOrg(cOwner, cpw, center)
	distTok := it.loginOrg(dOwner, dpw, dist)

	pA := it.product(center, "T373A")
	pB := it.product(center, "T373B")
	pR := it.product(center, "T373R")
	it.dt5Exec(`UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, pR.ID)
	c := it.stockChain()
	l1 := c.location(center, "U1")
	l2 := c.location(center, "U2")
	u1 := c.unit(center, pA, 1)
	u2 := c.unit(center, pB, 2)
	u3 := c.unit(center, pA, 3)
	r1 := c.rollUnit(center, pR, 4, "30.00")
	c.post(ledger.TypeEntry, u1, c.nextRef(), l1)
	c.post(ledger.TypeEntry, u2, c.nextRef(), l2)
	c.post(ledger.TypeEntry, u3, c.nextRef(), l1)
	c.post(ledger.TypeEntry, r1, c.nextRef(), l2)
	// A trigger stamps updated_at with NOW(): touch the rows one by one
	// so the order is u2 < u3 < u1 < r1.
	stamps := map[int64]string{}
	for _, id := range []int64{u2.ID, u3.ID, u1.ID, r1.ID} {
		var at time.Time
		if err := it.pool.QueryRow(ctx, `UPDATE unit_current_state SET updated_at = NOW() WHERE unit_id = $1 RETURNING updated_at`, id).Scan(&at); err != nil {
			t.Fatal(err)
		}
		stamps[id] = url.QueryEscape(at.UTC().Format(time.RFC3339Nano))
	}
	it.dt5Exec(`UPDATE units SET status = 'placed' WHERE id = $1`, u3.ID)
	var l1UUID string
	if err := it.pool.QueryRow(ctx, `SELECT uuid::text FROM warehouse_locations WHERE id = $1`, l1.ID).Scan(&l1UUID); err != nil {
		t.Fatal(err)
	}
	base := "/v1/stock/organizations/" + center.Uuid.String() + "/units?q=" + it.suffix
	list := func(name, query string, want ...db.Unit) {
		t.Helper()
		code, got, total := it.dt5List(tok, base+query, "barcode")
		names := make([]string, len(want))
		for i, u := range want {
			names[i] = u.Barcode
		}
		dt5Want(t, name, code, got, names...)
		if total != int64(len(want)) {
			t.Fatalf("%s total = %d", name, total)
		}
	}
	list("default product", "", u1, u3, u2, r1)
	list("product desc", "&sort=-product", r1, u2, u3, u1)
	list("barcode asc", "&sort=barcode", u1, u2, u3, r1)
	list("barcode desc", "&sort=-barcode", r1, u3, u2, u1)
	list("meters asc (pieces last)", "&sort=meters", r1, u1, u2, u3)
	list("meters desc (pieces last)", "&sort=-meters", r1, u3, u2, u1)
	list("updated asc", "&sort=updated_at", u2, u3, u1, r1)
	list("updated desc", "&sort=-updated_at", r1, u1, u3, u2)
	list("status rank", "&sort=status", u1, u2, r1, u3) // available < placed
	list("status desc", "&sort=-status", u3, r1, u2, u1)
	list("multi status", "&status=placed&sort=barcode", u3)
	list("status csv", "&status=available,placed&sort=barcode", u1, u2, u3, r1)
	list("barcode prefix", "&barcode_match=prefix&barcode="+strings.TrimSuffix(u1.Barcode, "1")+"&sort=barcode", u1, u2, u3) // the roll barcode has another prefix
	list("barcode exact prefix-value", "&barcode="+strings.TrimSuffix(u1.Barcode, "1"))
	list("barcode exact", "&barcode="+u2.Barcode, u2)
	list("location", "&location_uuid="+l1UUID+"&sort=barcode", u1, u3)
	list("updated range", "&updated_from="+stamps[u3.ID]+"&updated_to="+stamps[u1.ID]+"&sort=updated_at", u3, u1)
	for _, bad := range []string{"&sort=price", "&status=bogus", "&barcode_match=fuzzy", "&location_uuid=nope",
		"&updated_from=2026-01-05&updated_to=2026-01-01", "&updated_to=yesterday"} {
		if code, _, _ := it.dt5List(tok, base+bad, "barcode"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}

	// Product stock list: sort by sku / quantity, unknown sort 400.
	pbase := "/v1/stock/organizations/" + center.Uuid.String() + "/products?q=" + it.suffix
	code, got, _ := it.dt5List(tok, pbase+"&sort=sku", "product.sku")
	dt5Want(t, "products sku", code, got, pA.Sku, pB.Sku, pR.Sku)
	code, got, _ = it.dt5List(tok, pbase+"&sort=-sku", "product.sku")
	dt5Want(t, "products -sku", code, got, pR.Sku, pB.Sku, pA.Sku)
	code, got, _ = it.dt5List(tok, pbase+"&sort=-quantity&status=in_stock", "product.sku")
	if code != http.StatusOK || len(got) != 3 || got[0] != pA.Sku {
		t.Fatalf("products -quantity = %d %v", code, got)
	}
	if code, _, _ := it.dt5List(tok, pbase+"&sort=barcode", "product.sku"); code != http.StatusBadRequest {
		t.Fatalf("products bad sort = %d", code)
	}
	code, got, _ = it.dt5List(tok, "/v1/stock/locations/"+l1UUID+"/products?q="+it.suffix+"&sort=-product", "product.sku")
	dt5Want(t, "location products", code, got, pA.Sku)

	// Unit list export: same filters and sort; the listed organization
	// must be inside the scope.
	exportPath := "/v1/stock/organizations/" + center.Uuid.String() + "/units/export"
	if code, env := it.do("POST", exportPath, hostOlex, tok, map[string]any{
		"format": "csv", "query": map[string]string{"sort": "price"},
	}); code != http.StatusBadRequest {
		t.Fatalf("export bad sort = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", exportPath, hostOlex, distTok, map[string]any{"format": "csv"}); code != http.StatusNotFound {
		t.Fatalf("distributor export of center units = %d, want 404", code)
	}
	job := decodeData[exportJob](t, it.custDo("POST", exportPath, tok, map[string]any{
		"format": "xlsx", "locale": "en", "query": map[string]string{"q": it.suffix, "sort": "-barcode", "location_uuid": l1UUID},
	}, http.StatusAccepted))
	if job.Resource != stockusecase.ResourceUnitsExport {
		t.Fatalf("job = %+v", job)
	}
	stored := it.dt5JobQuery(job.UUID)
	if stored[stockusecase.QueryListedOrganization] != center.Uuid.String() || stored["sort"] != "-barcode" {
		t.Fatalf("stored query = %v", stored)
	}
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(center.ID, 10)
	adapter := stockusecase.NewUnitsExportAdapter(stockusecase.New(it.q))
	ds, err := adapter.Export(ctx, stored, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 2 || ds.Rows[0]["barcode"] != u3.Barcode || ds.Rows[1]["barcode"] != u1.Barcode ||
		ds.Rows[0]["status"] != "Placed" || ds.Rows[0]["sku"] != pA.Sku {
		t.Fatalf("export rows = %v", ds.Rows)
	}
	// A distributor job cannot reach the center's units.
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(dist.ID, 10)
	if _, err := adapter.Export(ctx, stored, i18n.Locale("en")); err == nil {
		t.Fatal("a distributor job must not export the center's units")
	}
}
