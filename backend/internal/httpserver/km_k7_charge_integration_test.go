package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// k7Line is a meter line with the TEC-1250 (19.11) excess billing fields.
type k7Line struct {
	UUID          string  `json:"uuid"`
	Meters        *string `json:"meters"`
	OrderedMeters *string `json:"ordered_meters"`
	ExcessMeters  *string `json:"excess_meters"`
	ExcessBilling *string `json:"excess_billing"`
	Assigned      string  `json:"assigned"`
	UnitPrice     *string `json:"unit_price"`
	LineTotal     *string `json:"line_total"`
}

type k7Order struct {
	UUID     string   `json:"uuid"`
	Subtotal *string  `json:"subtotal"`
	Total    *string  `json:"total"`
	Items    []k7Line `json:"items"`
	Timeline []struct {
		Action string  `json:"action"`
		Meters *string `json:"meters"`
	} `json:"timeline"`
}

// TEC-1250 (K7, 19.11 kapsam ekleme): the extra meters of a whole roll are
// free or charged by the seller's setting (organizations
// .excess_meter_billing). Over HTTP on one product (box 15 m, minimum cut
// 1 m, meter price 30):
//
//  1. The setting: free by default; only tenant.settings.write changes it
//     (the warehouse cannot); an unknown mode is 400.
//  2. charge: 3 m from a 3.5 m roll. The preview says charge (no price);
//     the line is billed 3.5 m (105), ordered_meters keeps 3, the order
//     total follows and the history has the step. The warehouse sees no
//     amount. Unassigning the roll puts the line back to 3 m (90);
//     assigning it again charges it again.
//  3. The setting turns free: the charged line keeps its frozen mode;
//     shipped and received, its return is credited 3.5 × 30 = 105.
//  4. free: 14.6 m from a closed 15 m box. The box goes whole, the line
//     stays 14.6 m (438) with excess_billing free.
//  5. charge again: 3 m from the 3.5 m roll is cut-free in charge, the
//     exact minimum (1 m left) is cut and billed as ordered (no excess).
func TestIntegrationKutuMetreK7ExcessBilling(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	cur := strings.TrimSpace(brand.Currency)
	if cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	dealer := it.org("tk7c-dealer", "dealer", center)
	var dealerCur string
	if err := it.pool.QueryRow(ctx, `SELECT currency FROM organizations WHERE id = $1`, dealer.ID).Scan(&dealerCur); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(dealerCur) != cur {
		t.Skipf("dealer currency %s differs from the brand currency %s", dealerCur, cur)
	}
	admin, apw := it.user("tk7c-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff")
	sales, spw := it.user("tk7c-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff, rbac.RoleCenterAccounting)
	wh, wpw := it.user("tk7c-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	owner, opw := it.user("tk7c-dealer-owner")
	it.member(dealer, owner, "owner")
	adminTok := it.loginOrg(admin, apw, center)
	salesTok := it.loginOrg(sales, spw, center)
	whTok := it.loginOrg(wh, wpw, center)
	dealerTok := it.loginOrg(owner, opw, dealer)

	// The brand center is shared by the integration tests: back to the
	// default afterwards.
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `UPDATE organizations SET excess_meter_billing = 'free' WHERE id = $1`, center.ID)
	})
	if _, err := it.pool.Exec(ctx, `UPDATE organizations SET excess_meter_billing = 'free' WHERE id = $1`, center.ID); err != nil {
		t.Fatal(err)
	}

	// --- 1. The setting.
	setting := func(tok string) string {
		code, env := it.do("GET", "/v1/tenant/settings", hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("settings = %d %s", code, env.Data)
		}
		var s struct {
			ExcessMeterBilling string `json:"excess_meter_billing"`
		}
		if err := json.Unmarshal(env.Data, &s); err != nil {
			t.Fatal(err)
		}
		return s.ExcessMeterBilling
	}
	setMode := func(tok, mode string, want int) {
		t.Helper()
		if code, env := it.do("PATCH", "/v1/tenant/settings", hostOlex, tok, map[string]any{"excess_meter_billing": mode}); code != want {
			t.Fatalf("set %s = %d %s, want %d", mode, code, env.Data, want)
		}
	}
	if m := setting(adminTok); m != "free" {
		t.Fatalf("default setting = %q, want free", m)
	}
	setMode(whTok, "charge", http.StatusForbidden)
	setMode(adminTok, "sometimes", http.StatusBadRequest)
	setMode(adminTok, "charge", http.StatusOK)
	if m := setting(adminTok); m != "charge" {
		t.Fatalf("setting = %q, want charge", m)
	}

	film := it.product(center, "TK7C")
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: film.ID, BrandID: film.BrandID, Currency: cur,
		SaleToDistributorPrice: pgtype.Text{String: "400", Valid: true},
		MeterPrice:             pgtype.Text{String: "30", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"sellable_by_meter": true, "default_meters": 15, "min_cut_meters": 1})
	if rec := it.raw("PUT", "/v1/catalog/products/"+film.Uuid.String()+"/roll", adminTok, "application/json", raw, nil); rec.Code != http.StatusOK {
		t.Fatalf("roll settings = %d %s", rec.Code, rec.Body.String())
	}
	chain := it.stockChain()
	loc := chain.location(center, "TK7C")
	roll35 := chain.rollUnit(center, film, 12601, "3.50")
	chain.post(ledger.TypeEntry, roll35, chain.nextRef(), loc)
	roll5 := chain.rollUnit(center, film, 12602, "5.00")
	chain.post(ledger.TypeEntry, roll5, chain.nextRef(), loc)
	box := chain.unit(center, film, 12603)
	chain.post(ledger.TypeEntry, box, chain.nextRef(), loc)

	order := func(meters string) boxMeterOrder {
		o := it.boxMeterOrder("POST", "/v1/orders", dealerTok, map[string]any{"items": []any{
			map[string]any{"product_uuid": film.Uuid.String(), "meters": meters, "line_credits": 5},
		}}, http.StatusCreated)
		for _, step := range []struct{ tok, status string }{
			{dealerTok, "submitted"}, {salesTok, "approved"}, {salesTok, "preparing"},
		} {
			if code, ec := it.transition(step.tok, o.UUID, step.status); code != http.StatusOK {
				t.Fatalf("%s = %d %s", step.status, code, ec)
			}
		}
		return o
	}
	get := func(o boxMeterOrder, tok string) (k7Order, string) {
		t.Helper()
		code, env := it.do("GET", "/v1/orders/"+o.UUID, hostOlex, tok, nil)
		var v k7Order
		if code != http.StatusOK || json.Unmarshal(env.Data, &v) != nil {
			t.Fatalf("order = %d %s", code, env.Data)
		}
		return v, string(env.Data)
	}
	previewRaw := func(o boxMeterOrder, barcode string) string {
		code, env := it.do("GET", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID+"/cut-preview?"+
			url.Values{"barcode": {barcode}}.Encode(), hostOlex, whTok, nil)
		if code != http.StatusOK {
			t.Fatalf("preview = %d %s", code, env.Data)
		}
		if strings.Contains(string(env.Data), "price") || strings.Contains(string(env.Data), "total") {
			t.Fatalf("preview carries an amount: %s", env.Data)
		}
		return string(env.Data)
	}
	assign := func(o boxMeterOrder, body map[string]any) string {
		code, env := it.do("POST", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID+"/units", hostOlex, whTok, body)
		if code != http.StatusOK {
			t.Fatalf("assign = %d %s %s", code, errCode(env), env.Data)
		}
		return string(env.Data)
	}
	check := func(name string, l k7Line, meters, ordered, excess, mode, total string) {
		t.Helper()
		str := func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		}
		if str(l.Meters) != meters || str(l.OrderedMeters) != ordered || str(l.ExcessMeters) != excess ||
			str(l.ExcessBilling) != mode || !sameAmount(l.LineTotal, total) {
			t.Fatalf("%s: meters %s ordered %s excess %s mode %s total %s; want %s %s %s %s %s", name,
				str(l.Meters), str(l.OrderedMeters), str(l.ExcessMeters), str(l.ExcessBilling), str(l.LineTotal),
				meters, ordered, excess, mode, total)
		}
	}

	// --- 2. charge: 3 m from the 3.5 m roll.
	o1 := order("3")
	if p := previewRaw(o1, roll35.Barcode); !strings.Contains(p, `"excess_meters":"0.50"`) || !strings.Contains(p, `"excess_billing":"charge"`) {
		t.Fatalf("charge preview = %s", p)
	}
	if a := assign(o1, map[string]any{"barcode": roll35.Barcode}); strings.Contains(a, `"line_total":"1`) || strings.Contains(a, `"unit_price":"3`) {
		t.Fatalf("the warehouse sees an amount: %s", a)
	}
	v, _ := get(o1, salesTok)
	check("charged", v.Items[0], "3.50", "3.00", "0.50", "charge", "105")
	if !sameAmount(v.Subtotal, "105") {
		t.Fatalf("subtotal = %v, want 105", v.Subtotal)
	}
	charged := false
	for _, e := range v.Timeline {
		if e.Action == "excess_charged" && e.Meters != nil && *e.Meters == "0.50" {
			charged = true
		}
	}
	if !charged {
		t.Fatalf("no excess_charged step in the timeline: %+v", v.Timeline)
	}
	if _, whView := get(o1, whTok); strings.Contains(whView, `"line_total":"`) || strings.Contains(whView, `"subtotal":"`) {
		t.Fatalf("the warehouse sees the order amounts: %s", whView)
	}
	// Unassigning the whole roll puts the line back to the order.
	if code, env := it.do("DELETE", "/v1/orders/"+o1.UUID+"/items/"+o1.Items[0].UUID+"/units/"+roll35.Uuid.String(), hostOlex, whTok, nil); code != http.StatusOK {
		t.Fatalf("unassign = %d %s", code, env.Data)
	}
	v, _ = get(o1, salesTok)
	check("unassigned", v.Items[0], "3.00", "", "", "", "90")
	if !sameAmount(v.Subtotal, "90") {
		t.Fatalf("subtotal after unassign = %v, want 90", v.Subtotal)
	}
	assign(o1, map[string]any{"barcode": roll35.Barcode})
	v, _ = get(o1, salesTok)
	check("charged again", v.Items[0], "3.50", "3.00", "0.50", "charge", "105")

	// --- 3. The setting turns free; the charged line keeps its mode.
	setMode(adminTok, "free", http.StatusOK)
	for _, step := range []struct{ tok, status string }{{whTok, "ready"}, {whTok, "shipped"}, {dealerTok, "received"}} {
		if code, ec := it.transition(step.tok, o1.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	v, _ = get(o1, salesTok)
	check("frozen", v.Items[0], "3.50", "3.00", "0.50", "charge", "105")
	ret, _ := it.transferCall("POST", "/v1/stock-transfers", dealerTok, map[string]any{
		"kind": "return", "items": []map[string]any{{"barcode": roll35.Barcode}},
	}, http.StatusCreated)
	if code, ec := it.transferMove(salesTok, ret.UUID, "approved"); code != http.StatusOK {
		t.Fatalf("approve return = %d %s", code, ec)
	}
	var lineTotal string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(i.line_total::text, '')
		FROM stock_transfer_request_items i JOIN stock_transfer_requests r ON r.id = i.request_id
		WHERE r.uuid = $1 AND i.unit_id = $2`, ret.UUID, roll35.ID).Scan(&lineTotal); err != nil {
		t.Fatal(err)
	}
	if !sameAmount(&lineTotal, "105") {
		t.Fatalf("charged return = %s, want 105 (3.5 × 30)", lineTotal)
	}

	// --- 4. free: 14.6 m from the closed 15 m box.
	o2 := order("14.6")
	if p := previewRaw(o2, box.Barcode); !strings.Contains(p, `"excess_meters":"0.40"`) || !strings.Contains(p, `"excess_billing":"free"`) {
		t.Fatalf("free box preview = %s", p)
	}
	assign(o2, map[string]any{"barcode": box.Barcode, "open_box": true})
	v, _ = get(o2, salesTok)
	check("free box", v.Items[0], "14.60", "", "0.40", "free", "438")
	if !sameAmount(v.Subtotal, "438") {
		t.Fatalf("free subtotal = %v, want 438", v.Subtotal)
	}

	// --- 5. charge: the exact minimum left is cut and billed as ordered.
	setMode(adminTok, "charge", http.StatusOK)
	o3 := order("4")
	assign(o3, map[string]any{"barcode": roll5.Barcode})
	v, _ = get(o3, salesTok)
	check("exact minimum", v.Items[0], "4.00", "", "", "", "120")
}
