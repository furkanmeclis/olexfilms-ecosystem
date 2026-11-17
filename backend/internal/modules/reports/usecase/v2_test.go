package usecase

// TEC-1210: envelope v2 — compare, totals and deltas, KPIs, typed columns,
// row links, role reports, export and catalog access.

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/xuri/excelize/v2"
)

func fp(v float64) *float64 { return &v }

func TestPreviousRange(t *testing.T) {
	s := &Service{now: func() time.Time { return testNow }}
	trend, _ := DefinitionByKey(ReportServicesTrend)
	perf, _ := DefinitionByKey(ReportDealersPerformance)
	stock, _ := DefinitionByKey(ReportStockSummary)
	fb := in30.Fallback
	cases := []struct {
		name     string
		d        Definition
		in       Input
		compare  string
		from, to string
	}{
		{"30d previous", trend, Input{Compare: "previous", Fallback: fb}, ComparePrevious, "2026-08-11", "2026-09-09"},
		{"30d year", trend, Input{Compare: "year", Fallback: fb}, CompareYear, "2025-09-10", "2025-10-09"},
		{"today previous", trend, Input{Period: "today", Compare: "previous", Fallback: fb}, ComparePrevious, "2026-10-08", "2026-10-08"},
		{"12m previous", trend, Input{Period: "12m", Compare: "previous", Fallback: fb}, ComparePrevious, "2024-11-01", "2025-10-09"},
		{"custom previous", trend, Input{RangeFrom: "2026-09-01", RangeTo: "2026-09-30", Compare: "previous", Fallback: fb}, ComparePrevious, "2026-08-02", "2026-08-31"},
		{"performance 7d previous = month before", perf, Input{Period: "7d", Compare: "previous", Fallback: fb}, ComparePrevious, "2026-09-01", "2026-09-30"},
		{"none", trend, Input{Fallback: fb}, CompareNone, "", ""},
		{"snapshot ignores compare", stock, Input{Compare: "previous", Fallback: fb}, CompareNone, "", ""},
	}
	for _, tc := range cases {
		q, err := s.resolveQuery(tc.d, tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if q.compare != tc.compare {
			t.Fatalf("%s: compare %s, want %s", tc.name, q.compare, tc.compare)
		}
		if tc.from == "" {
			continue
		}
		if got := q.prevFromDate.Format(dateLayout) + ".." + q.prevToDate.Format(dateLayout); got != tc.from+".."+tc.to {
			t.Fatalf("%s: previous %s, want %s..%s", tc.name, got, tc.from, tc.to)
		}
		pi := q.periodInfo()
		if pi.Compare != tc.compare || pi.PreviousFrom == nil || *pi.PreviousFrom != tc.from || *pi.PreviousTo != tc.to {
			t.Fatalf("%s: period_info %+v", tc.name, pi)
		}
	}
	var ve *ValidationError
	if _, err := s.resolveQuery(trend, Input{Compare: "week", Fallback: fb}); !errors.As(err, &ve) || ve.Field != "compare" {
		t.Fatalf("bad compare: err = %v", err)
	}
}

func TestMergePreviousAndFinalize(t *testing.T) {
	cur := Envelope{
		Series: []Series{
			{Key: seriesCards, Points: []Point{{Key: "orders_created", Value: 4}, {Key: "orders_open", Value: 2}}},
			{Key: "created", timeline: true, Points: []Point{{Key: "2026-10-01", Value: 1}, {Key: "2026-10-02", Value: 3}}},
			{Key: "status", Points: []Point{{Key: "draft", Value: 2}, {Key: "new_item", Value: 1}}},
			{Key: "zero", timeline: true, Points: []Point{{Key: "2026-10-01", Value: 5}}},
		},
		KPIs: []KPI{{Key: "orders_created", Value: 4}, {Key: "orders_open", Value: 2}},
	}
	prev := Envelope{
		Series: []Series{
			{Key: seriesCards, Points: []Point{{Key: "orders_created", Value: 2}, {Key: "orders_open", Value: 9}}},
			{Key: "created", timeline: true, Points: []Point{{Key: "2026-09-01", Value: 2}, {Key: "2026-09-02", Value: 0}}},
			{Key: "status", Points: []Point{{Key: "draft", Value: 1}}},
			{Key: "zero", timeline: true, Points: []Point{{Key: "2026-09-01", Value: 0}}},
		},
		KPIs: []KPI{{Key: "orders_created", Value: 2}, {Key: "orders_open", Value: 9}},
	}
	mergePrevious(&cur, prev)
	finalize(&cur)
	cards, created, status, zero := cur.Series[0], cur.Series[1], cur.Series[2], cur.Series[3]
	if cards.Total != nil || cards.PreviousTotal != nil || cards.Points[1].Previous != nil || *cards.Points[0].Previous != 2 {
		t.Fatalf("cards = %+v", cards)
	}
	if *created.Total != 4 || *created.PreviousTotal != 2 || *created.DeltaPct != 100 || *created.Points[0].Previous != 2 || *created.Points[1].Previous != 0 {
		t.Fatalf("created = %+v", created)
	}
	if *status.Points[0].Previous != 1 || *status.Points[1].Previous != 0 || *status.DeltaPct != 200 {
		t.Fatalf("status = %+v", status)
	}
	if zero.DeltaPct != nil || *zero.PreviousTotal != 0 {
		t.Fatalf("delta with previous 0 = %v", zero.DeltaPct)
	}
	if k := cur.KPIs[0]; *k.Previous != 2 || *k.DeltaPct != 100 {
		t.Fatalf("kpi = %+v", k)
	}
	if k := cur.KPIs[1]; k.Previous != nil || k.DeltaPct != nil {
		t.Fatalf("snapshot kpi has a previous: %+v", k)
	}
	if d := deltaPct(fp(1), fp(3)); *d != -66.7 {
		t.Fatalf("delta = %v", *d)
	}
	if got := lastN([]float64{1, 2, 3}, 2); len(got) != 2 || got[0] != 2 {
		t.Fatalf("lastN = %v", got)
	}
}

// oldOrder adds a received order of buyer created at (outside the current
// 30 days: the compared range).
func (f *fixture) oldOrder(t *testing.T, seller, buyer db.Organization, at time.Time) {
	t.Helper()
	f.exec(t, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
 rate_snapshot, try_rate, subtotal, total, created_at, approved_at, received_at)
VALUES ($1, $2, $1, $3, 'received', 'TRY', '{}', 1, 10, 10, $4, $4, $4)`, seller.ID, f.brandID, buyer.ID, at)
}

func TestCompareEnvelope(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	center := f.caller(t, f.center, rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	f.oldOrder(t, f.distA, f.dealer1, testNow.AddDate(0, 0, -35))
	in := in30
	in.Compare = ComparePrevious

	env, err := s.Report(f.ctx, center, ReportOrdersTrend, in)
	if err != nil {
		t.Fatal(err)
	}
	if env.PeriodInfo == nil || env.PeriodInfo.Compare != ComparePrevious || *env.PeriodInfo.PreviousFrom != "2026-08-11" ||
		env.PeriodInfo.Granularity != "day" || env.PeriodInfo.From != "2026-09-10" {
		t.Fatalf("period_info = %+v", env.PeriodInfo)
	}
	created := env.Series[0]
	if *created.Total != 6 || *created.PreviousTotal != 1 || *created.DeltaPct != 500 {
		t.Fatalf("created totals = %v %v %v", *created.Total, *created.PreviousTotal, *created.DeltaPct)
	}
	var prevSum float64
	for i, p := range created.Points {
		if p.Previous == nil {
			t.Fatalf("point %d has no previous", i)
		}
		prevSum += *p.Previous
	}
	// 2026-09-04 is bucket 24 of 2026-08-11..2026-09-09.
	if prevSum != 1 || *created.Points[24].Previous != 1 {
		t.Fatalf("previous points sum %v, bucket 24 = %v", prevSum, *created.Points[24].Previous)
	}

	// Without compare: totals but no previous.
	env, err = s.Report(f.ctx, center, ReportOrdersTrend, in30)
	if err != nil {
		t.Fatal(err)
	}
	if sr := env.Series[0]; *sr.Total != 6 || sr.PreviousTotal != nil || sr.DeltaPct != nil || sr.Points[0].Previous != nil ||
		env.PeriodInfo.Compare != CompareNone || env.PeriodInfo.PreviousFrom != nil {
		t.Fatalf("compare=none series = %+v / %+v", sr, env.PeriodInfo)
	}

	// Distribution: previous by key.
	env, err = s.Report(f.ctx, center, ReportOrdersStatusDistribution, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range env.Series[0].Points {
		want := 0.0
		if p.Key == "received" {
			want = 1
		}
		if p.Previous == nil || *p.Previous != want {
			t.Fatalf("status %s previous = %v, want %v", p.Key, p.Previous, want)
		}
	}

	// Overview KPIs: comparable ones get previous and sparks, snapshot ones not.
	env, err = s.Report(f.ctx, center, ReportOverview, in)
	if err != nil {
		t.Fatal(err)
	}
	kpis := map[string]KPI{}
	for _, k := range env.KPIs {
		kpis[k.Key] = k
	}
	if len(env.KPIs) != len(env.Series[0].Points) {
		t.Fatalf("kpis %d, cards %d", len(env.KPIs), len(env.Series[0].Points))
	}
	oc := kpis["orders_created"]
	var spark float64
	for _, v := range oc.Spark {
		spark += v
	}
	if oc.Value != 6 || oc.Previous == nil || *oc.Previous != 1 || *oc.DeltaPct != 500 || len(oc.Spark) != 30 || spark != 6 ||
		oc.Good != goodUp || oc.Link == nil || oc.Link.Kind != LinkReport || oc.Link.Report != ReportOrdersTrend {
		t.Fatalf("orders_created = %+v", oc)
	}
	if open := kpis["orders_open"]; open.Previous != nil || len(open.Spark) != 0 || open.Good != goodDown {
		t.Fatalf("orders_open = %+v", open)
	}
	if st := kpis["stock_meters"]; st.Unit != unitMeters || st.Previous != nil {
		t.Fatalf("stock_meters = %+v", st)
	}
	if sc := kpis["services_completed"]; sc.Value != 6 || sc.Previous == nil || *sc.Previous != 0 || sc.DeltaPct != nil {
		t.Fatalf("services_completed = %+v", sc)
	}

	// Year compare on a summary report.
	in.Compare = CompareYear
	env, err = s.Report(f.ctx, center, ReportWarrantiesSummary, in)
	if err != nil {
		t.Fatal(err)
	}
	if *env.PeriodInfo.PreviousFrom != "2025-09-10" {
		t.Fatalf("year previous_from = %v", *env.PeriodInfo.PreviousFrom)
	}
	ws := map[string]KPI{}
	for _, k := range env.KPIs {
		ws[k.Key] = k
	}
	if k := ws["warranties_started"]; k.Value != 6 || k.Previous == nil || *k.Previous != 0 || len(k.Spark) == 0 || k.Link != nil {
		t.Fatalf("warranties_started = %+v", k)
	}
	if k := ws["warranties_active"]; k.Previous != nil {
		t.Fatalf("warranties_active is a snapshot: %+v", k)
	}
	var seriesKeys []string
	for _, sr := range env.Series {
		seriesKeys = append(seriesKeys, sr.Key)
	}
	if strings.Join(seriesKeys, ",") != "cards,status,started" {
		t.Fatalf("summary series = %v", seriesKeys)
	}
}

var (
	columnTypes = map[string]bool{"text": true, "number": true, "money": true, "percent": true, "date": true,
		"datetime": true, "duration": true, "status": true, "org": true, "link": true}
	linkKinds = map[string]bool{LinkService: true, LinkOrder: true, LinkCustomer: true, LinkOrganization: true,
		LinkWarranty: true, LinkMeasurement: true, LinkProduct: true}
	tones = map[string]bool{ToneNeutral: true, ToneProgress: true, ToneDone: true, ToneProblem: true}
)

func TestColumnsAndRowMeta(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	center := f.caller(t, f.center, rbac.RoleCenterStaff, rbac.RoleCenterWarehouse, rbac.RoleCenterAccounting)
	in := in30
	for _, d := range Definitions {
		in.Limit = ""
		if d.Limited {
			in.Limit = "50"
		}
		env, err := s.Report(f.ctx, center, d.Key, in)
		if err != nil {
			t.Fatalf("%s: %v", d.Key, err)
		}
		for _, c := range env.Columns {
			if !columnTypes[c.Type] || (c.Align != "left" && c.Align != "right") ||
				(c.Aggregate != "sum" && c.Aggregate != "avg" && c.Aggregate != "none") {
				t.Fatalf("%s column %+v", d.Key, c)
			}
			if c.Label == "" || c.Label == "column."+c.Key {
				t.Fatalf("%s column %s has no label", d.Key, c.Key)
			}
			if c.Key == "rank" && c.Aggregate != "none" {
				t.Fatalf("%s rank aggregates", d.Key)
			}
		}
		for _, sr := range env.Series {
			if (sr.Key == seriesCards) != (sr.Total == nil) {
				t.Fatalf("%s series %s total = %v", d.Key, sr.Key, sr.Total)
			}
		}
		for _, r := range env.Rows {
			if l, ok := r[rowLink].(Link); ok {
				if !linkKinds[l.Kind] || l.UUID == "" {
					t.Fatalf("%s row link %+v", d.Key, l)
				}
			} else if hasColumn(env, "organization") {
				t.Fatalf("%s row without _link: %v", d.Key, r)
			}
			if st, ok := r[rowStatus].(map[string]StatusCell); ok {
				for k, cell := range st {
					if !hasColumn(env, k) || !tones[cell.Tone] || cell.Label == "" {
						t.Fatalf("%s _status %s = %+v", d.Key, k, cell)
					}
				}
			}
		}
		if d.Key == ReportActivitiesRecent {
			if len(env.Rows) == 0 {
				t.Fatal("no recent services")
			}
			r := env.Rows[0]
			if r[rowLink].(Link).Kind != LinkService || r[rowStatus].(map[string]StatusCell)["status"].Tone != ToneDone {
				t.Fatalf("activities row = %v", r)
			}
		}
	}
}

// TestPerformanceHidesPrices (TEC-1050): order_volume is a price.
func TestPerformanceHidesPrices(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	for _, roles := range [][]string{{rbac.RoleCenterSales}, {rbac.RoleCenterStaff}, {rbac.RoleCenterStaff, rbac.RoleCenterAccounting}} {
		c := f.caller(t, f.center, roles...)
		env, err := s.Report(f.ctx, c, ReportDealersPerformance, in30)
		if errors.Is(err, ErrForbidden) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if hasColumn(env, "order_volume") != orderAmountsVisible(c.Principal) {
			t.Fatalf("%v: order_volume column %v, prices visible %v", roles, hasColumn(env, "order_volume"), orderAmountsVisible(c.Principal))
		}
	}
	// center_sales without its pricing grants: no order_volume.
	c := f.caller(t, f.center, rbac.RoleCenterSales)
	delete(c.Principal.PermissionScopes, rbac.PermPricingSaleRead)
	delete(c.Principal.PermissionScopes, rbac.PermPricingPurchaseRead)
	perms := c.Principal.Permissions[:0]
	for _, p := range c.Principal.Permissions {
		if p != rbac.PermPricingSaleRead && p != rbac.PermPricingPurchaseRead {
			perms = append(perms, p)
		}
	}
	c.Principal.Permissions = perms
	env, err := s.Report(f.ctx, c, ReportDealersPerformance, in30)
	if err != nil {
		t.Fatal(err)
	}
	if hasColumn(env, "order_volume") || !hasColumn(env, "services_count") {
		t.Fatalf("columns without pricing = %v", env.Columns)
	}
	for _, r := range env.Rows {
		if _, ok := r["order_volume"]; ok {
			t.Fatalf("row carries order_volume: %v", r)
		}
	}
}

// seedMovements: warehouse W of distA (location L) and W2 of the center
// (location L2); an entry into L, a transfer L2 -> L, a placement inside W
// and a consumption out of L.
func (f *fixture) seedMovements(t *testing.T) (w, w2 int64) {
	t.Helper()
	loc := func(org db.Organization, code string) (int64, int64) {
		wh := f.id(t, `INSERT INTO warehouses (organization_id, code, name) VALUES ($1, $2, $3) RETURNING id`, org.ID, code, f.prefix+" "+code)
		room := f.id(t, `INSERT INTO rooms (organization_id, warehouse_id, code, name) VALUES ($1, $2, 'R1', 'Room') RETURNING id`, org.ID, wh)
		l := f.id(t, `INSERT INTO warehouse_locations (organization_id, code, name, warehouse_id, room_id, type, full_code)
VALUES ($1, 'A1', 'Aisle', $2, $3, 'aisle', $4) RETURNING id`, org.ID, wh, room, code+"-R1-A1")
		return wh, l
	}
	w, l := loc(f.distA, "WA")
	w2, l2 := loc(f.center, "WC")
	at := testNow.AddDate(0, 0, -2)
	move := func(typ string, org db.Organization, fromType string, fromID int64, toType string, toID int64) {
		f.seq++
		unit := f.id(t, `INSERT INTO units (organization_id, brand_id, product_id, barcode, status)
VALUES ($1, $2, $3, $4, 'available') RETURNING id`, f.center.ID, f.brandID, f.product, fmt.Sprintf("MV-%s-%d", f.prefix, f.seq))
		var ft, tt any
		var fid, tid any
		if fromType != "" {
			ft, fid = fromType, fromID
		}
		if toType != "" {
			tt, tid = toType, toID
		}
		f.exec(t, `INSERT INTO stock_movements (organization_id, brand_id, unit_id, product_id, type, quantity_delta,
 from_owner_type, from_owner_id, to_owner_type, to_owner_id, idempotency_key, created_at)
VALUES ($1, $2, $3, $4, $5, 1, $6, $7, $8, $9, $10, $11)`, org.ID, f.brandID, unit, f.product, typ, ft, fid, tt, tid,
			fmt.Sprintf("%s-mv-%d", f.prefix, f.seq), at)
	}
	move("entry", f.distA, "", 0, "warehouse_location", l)
	move("transfer_in", f.distA, "warehouse_location", l2, "warehouse_location", l)
	move("placement", f.distA, "warehouse_location", l, "warehouse_location", l)
	move("consumption", f.distA, "warehouse_location", l, "service", 1)
	return w, w2
}

func TestRoleReportsTEC1210(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	in := in30
	in.Limit = "50"

	// orders.fulfillment: distA sells the 3 orders of its dealers (approved
	// at = shipped at, 48 hours ago: not overdue) plus one preparing order of
	// warehouse W approved 72 hours ago (overdue). dealer1's order was
	// approved 6 hours before shipping: 6h / 3 shipped = 2.0h on average.
	w, _ := f.seedMovements(t)
	f.exec(t, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
 rate_snapshot, try_rate, subtotal, total, created_at, approved_at, seller_warehouse_id)
VALUES ($1, $2, $1, $3, 'preparing', 'TRY', '{}', 1, 10, 10, $4, $4, $5)`, f.distA.ID, f.brandID, f.dealer1.ID, testNow.Add(-72*time.Hour), w)
	f.exec(t, `UPDATE orders SET approved_at = approved_at - interval '6 hours' WHERE buyer_org_id = $1 AND status = 'approved'`, f.dealer1.ID)
	dist := f.caller(t, f.distA, rbac.RoleDistributorOwner)
	env, err := s.Report(f.ctx, dist, ReportOrdersFulfillment, in30)
	if err != nil {
		t.Fatal(err)
	}
	k := map[string]KPI{}
	for _, x := range env.KPIs {
		k[x.Key] = x
	}
	if k["fulfillment_shipped"].Value != 3 || k["fulfillment_open"].Value != 4 || k["fulfillment_overdue"].Value != 2 ||
		k["fulfillment_prep_hours"].Value != 2 || k["fulfillment_prep_hours"].Unit != unitHour || k["fulfillment_prep_hours"].Good != goodDown {
		t.Fatalf("fulfillment kpis = %+v", env.KPIs)
	}
	if sumSeries(env, "shipped") != 3 || len(env.Rows) != 2 || env.Rows[0]["warehouse_uuid"] != nil || env.Rows[0]["warehouse"] != "No warehouse" ||
		env.Rows[1]["open_count"] != int64(1) || env.Rows[1]["overdue_count"] != int64(1) {
		t.Fatalf("fulfillment rows = %v", env.Rows)
	}
	dealer := f.caller(t, f.dealer1, rbac.RoleDealerOwner)
	env, err = s.Report(f.ctx, dealer, ReportOrdersFulfillment, in30)
	if err != nil || len(env.Rows) != 0 || len(env.KPIs) != 5 {
		t.Fatalf("dealer fulfillment = %v / %v", env.Rows, err)
	}

	// network.summary: subtree of distA (dealer1 + dealer2), dealers forbidden.
	env, err = s.Report(f.ctx, dist, ReportNetworkSummary, in)
	if err != nil {
		t.Fatal(err)
	}
	if card(env, "network_services") != 3 || card(env, "network_warranties") != 3 || card(env, "network_active_dealers") != 2 ||
		len(env.Rows) != 2 || env.Rows[0]["organization_uuid"] != f.dealer2.Uuid || env.Rows[0]["services_count"] != int64(2) {
		t.Fatalf("network summary = %v / %v", env.Series[0], env.Rows)
	}
	if _, err := s.Report(f.ctx, dealer, ReportNetworkSummary, in30); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer network.summary: %v", err)
	}
	centerWh := f.caller(t, f.center, rbac.RoleCenterWarehouse)
	env, err = s.Report(f.ctx, centerWh, ReportNetworkSummary, in)
	if err != nil || len(env.Rows) != 3 {
		t.Fatalf("center network.summary rows = %v / %v", env.Rows, err)
	}

	// stock.movement: distA sees W (in 2, out 1); the center W (2/1) and W2 (0/1).
	env, err = s.Report(f.ctx, dist, ReportStockMovement, in)
	if err != nil {
		t.Fatal(err)
	}
	if sumSeries(env, "in") != 2 || sumSeries(env, "out") != 1 || len(env.Rows) != 1 || env.Rows[0]["net_count"] != int64(1) {
		t.Fatalf("distributor stock movement = %v / %v", env.Series, env.Rows)
	}
	env, err = s.Report(f.ctx, centerWh, ReportStockMovement, in)
	if err != nil {
		t.Fatal(err)
	}
	if sumSeries(env, "in") != 2 || sumSeries(env, "out") != 2 || len(env.Rows) != 2 {
		t.Fatalf("center stock movement = %v / %v", env.Series, env.Rows)
	}
}

// TestCatalogMatchesAccess: every report the caller can open is in its
// catalog and nothing else — no performance permission is needed for the
// catalog itself (TEC-1210).
func TestCatalogMatchesAccess(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	callers := map[string]Caller{
		"center_warehouse":  f.caller(t, f.center, rbac.RoleCenterWarehouse),
		"center_accounting": f.caller(t, f.center, rbac.RoleCenterAccounting),
		"center_sales":      f.caller(t, f.center, rbac.RoleCenterSales),
		"dist_warehouse":    f.caller(t, f.distA, rbac.RoleDistributorWarehouseStaff),
		"dealer_staff":      f.caller(t, f.dealer1, rbac.RoleDealerStaff),
	}
	for name, c := range callers {
		items, err := s.Catalog(f.ctx, c, i18n.LocaleTR)
		if err != nil {
			t.Fatal(err)
		}
		listed := map[string]CatalogItem{}
		for _, it := range items {
			listed[it.Key] = it
		}
		for _, d := range Definitions {
			_, err := s.Report(f.ctx, c, d.Key, in30)
			open := err == nil
			if err != nil && !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrFeatureDisabled) {
				t.Fatalf("%s %s: %v", name, d.Key, err)
			}
			it, ok := listed[d.Key]
			if ok != open {
				t.Fatalf("%s %s: listed %v, open %v", name, d.Key, ok, open)
			}
			if ok && (!it.Exportable || (d.Periodic && len(it.Compares) != 3) || (!d.Periodic && len(it.Compares) != 1)) {
				t.Fatalf("%s %s item = %+v", name, d.Key, it)
			}
		}
		if c.Principal.HasPermission(rbac.PermPerformanceRead) {
			continue
		}
		if len(listed) == 0 {
			t.Fatalf("%s without performance.read sees no report", name)
		}
	}
	wh := map[string]bool{}
	items, _ := s.Catalog(f.ctx, callers["center_warehouse"], i18n.LocaleTR)
	for _, it := range items {
		wh[it.Key] = true
	}
	for _, k := range []string{ReportOverview, ReportOrdersFulfillment, ReportStockMovement, ReportStockSummary, ReportNetworkSummary} {
		if !wh[k] {
			t.Fatalf("center_warehouse catalog misses %s: %v", k, wh)
		}
	}
}

func TestExportReport(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	center := f.caller(t, f.center, rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	tr := Input{Locale: "tr", Compare: ComparePrevious, Limit: "50", Fallback: in30.Fallback}

	// Table report, XLSX: translated headers, numeric count cells.
	file, err := s.Export(f.ctx, center, ReportDealersTopByWarranty, "xlsx", tr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(file.Filename, ".xlsx") || !strings.Contains(file.ContentType, "spreadsheetml") {
		t.Fatalf("file = %s %s", file.Filename, file.ContentType)
	}
	x, err := excelize.OpenReader(bytes.NewReader(file.Body))
	if err != nil {
		t.Fatal(err)
	}
	sheet := x.GetSheetName(0)
	rows, _ := x.GetRows(sheet)
	if len(rows) != 4 || rows[0][0] != translate(i18n.LocaleTR, "column.rank") || rows[0][3] != translate(i18n.LocaleTR, "column.warranty_count") {
		t.Fatalf("xlsx rows = %v", rows)
	}
	typ, _ := x.GetCellType(sheet, "D2")
	if typ == excelize.CellTypeSharedString || typ == excelize.CellTypeInlineString {
		t.Fatalf("warranty_count cell is text (%v)", typ)
	}
	if v, _ := x.GetCellValue(sheet, "D2"); v != "3" {
		t.Fatalf("D2 = %q", v)
	}

	// Time series, CSV: one row per bucket, value and previous per series.
	file, err = s.Export(f.ctx, center, ReportOrdersTrend, "csv", Input{Locale: "tr", Compare: ComparePrevious, Fallback: in30.Fallback})
	if err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(file.Body, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 31 || recs[0][0] != translate(i18n.LocaleTR, "column.bucket") || len(recs[0]) != 5 ||
		!strings.Contains(recs[0][2], translate(i18n.LocaleTR, "column.previous")) {
		t.Fatalf("csv header = %v (%d rows)", recs[0], len(recs))
	}

	// Summary: KPI rows.
	file, err = s.Export(f.ctx, center, ReportWarrantiesSummary, "xlsx", tr0(tr))
	if err != nil {
		t.Fatal(err)
	}
	x, _ = excelize.OpenReader(bytes.NewReader(file.Body))
	rows, _ = x.GetRows(x.GetSheetName(0))
	if len(rows) < 6 || rows[1][1] != translate(i18n.LocaleTR, "card.warranties_active") {
		t.Fatalf("summary rows = %v", rows)
	}

	// Same gates as the report.
	staff := f.caller(t, f.dealer1, rbac.RoleDealerStaff)
	if _, err := s.Export(f.ctx, staff, ReportStockSummary, "csv", Input{Fallback: in30.Fallback}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer staff stock export: %v", err)
	}
	var ve *ValidationError
	if _, err := s.Export(f.ctx, center, ReportOrdersTrend, "pdf", Input{Fallback: in30.Fallback}); !errors.As(err, &ve) || ve.Field != "format" {
		t.Fatalf("pdf export: %v", err)
	}
}

// tr0 drops the limit (summaries take none).
func tr0(in Input) Input { in.Limit = ""; return in }

// TEC-1229: stock.aging buckets the stock on hand by its last movement;
// the price columns need a pricing permission, so a warehouse role never
// sees them.
func TestStockAgingPriceGate(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	in := in30
	in.Limit = "50"
	wh := f.caller(t, f.center, rbac.RoleCenterWarehouse)
	env, err := s.Report(f.ctx, wh, ReportStockAging, in)
	if err != nil {
		t.Fatal(err)
	}
	if hasColumn(env, "unit_price") || hasColumn(env, "stock_value") || !hasColumn(env, "qty_0_30") {
		t.Fatalf("warehouse columns = %v", env.Columns)
	}
	for _, r := range env.Rows {
		if _, ok := r["unit_price"]; ok {
			t.Fatalf("warehouse row carries a price: %v", r)
		}
	}
	if kpi(env, "aging_quantity") < 6 {
		t.Fatalf("aging quantity = %v", kpi(env, "aging_quantity"))
	}
	sales := f.caller(t, f.center, rbac.RoleCenterSales, rbac.RoleCenterWarehouse)
	env, err = s.Report(f.ctx, sales, ReportStockAging, in)
	if err != nil {
		t.Fatal(err)
	}
	if !hasColumn(env, "unit_price") || !hasColumn(env, "stock_value") {
		t.Fatalf("priced columns = %v", env.Columns)
	}
}
