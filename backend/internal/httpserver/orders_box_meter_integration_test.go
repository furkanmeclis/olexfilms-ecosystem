package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type boxMeterLine struct {
	UUID    string `json:"uuid"`
	Product struct {
		UUID string `json:"uuid"`
		// TEC-1239: the meter sale settings of the line's product.
		SellableByMeter *bool   `json:"sellable_by_meter"`
		BoxMeters       *string `json:"box_meters"`
		MinCutMeters    *string `json:"min_cut_meters"`
	} `json:"product"`
	SaleMeasure string  `json:"sale_measure"`
	Quantity    *int32  `json:"quantity"`
	Meters      *string `json:"meters"`
	LineCredits *int32  `json:"line_credits"`
	UnitPrice   *string `json:"unit_price"`
	LineTotal   *string `json:"line_total"`
}

type boxMeterOrder struct {
	UUID   string         `json:"uuid"`
	Status string         `json:"status"`
	Total  *string        `json:"total"`
	Items  []boxMeterLine `json:"items"`
}

func (it *itest) boxMeterOrder(method, path, access string, body any, want int) boxMeterOrder {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, access, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s %s, want %d", method, path, code, errCode(env), env.Data, want)
	}
	var v boxMeterOrder
	if err := json.Unmarshal(env.Data, &v); err != nil {
		it.t.Fatal(err)
	}
	return v
}

func (o boxMeterOrder) line(t *testing.T, measure string) boxMeterLine {
	t.Helper()
	for _, l := range o.Items {
		if l.SaleMeasure == measure {
			return l
		}
	}
	t.Fatalf("no %s line in %+v", measure, o.Items)
	return boxMeterLine{}
}

// TEC-1238 acceptance (kutu/metraj 1/7, K1-K14 of TEC-1236): the center
// turns on "sold by the meter" on the product card (TEC-1251: a Glorian
// synced product too); the dealer orders 1 box and 3 m of the same product on
// one order; the meter price carries the dealer's discount like the box
// price (400 -> 360, meter list 30 -> 27); a meter line below the minimum
// cut and a meter line of a product not sold by the meter are refused; the
// meter line carries 5 credits by default; the warehouse role sees the
// lines without any price.
func TestIntegrationOrdersBoxAndMeterLines(t *testing.T) {
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
	dealer := it.org("t1238-dealer", "dealer", center)
	admin, apw := it.user("t1238-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff")
	sales, spw := it.user("t1238-center-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff, rbac.RoleCenterAccounting)
	wh, wpw := it.user("t1238-center-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	dealerOwner, dpw := it.user("t1238-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	adminTok := it.loginOrg(admin, apw, center)
	salesTok := it.loginOrg(sales, spw, center)
	whTok := it.loginOrg(wh, wpw, center)
	dealerTok := it.loginOrg(dealerOwner, dpw, dealer)

	film := it.product(center, "T1238F")
	boxOnly := it.product(center, "T1238B")
	synced := it.product(center, "T1238G")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET connection_id = 987654321, external_id = 'g-1238' WHERE id = $1`, synced.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: film.ID, BrandID: film.BrandID, Currency: cur,
		SaleToDistributorPrice: pgtype.Text{String: "400", Valid: true},
		MeterPrice:             pgtype.Text{String: "30", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	it.setListPrice(boxOnly, cur, "50")

	// K1: the switch on the product card (roll settings).
	roll := func(p db.Product, body map[string]any) (int, string) {
		raw, _ := json.Marshal(body)
		rec := it.raw("PUT", "/v1/catalog/products/"+p.Uuid.String()+"/roll", adminTok, "application/json", raw, nil)
		return rec.Code, rec.Body.String()
	}
	if code, body := roll(film, map[string]any{"sellable_by_meter": true, "min_cut_meters": 2}); code != http.StatusUnprocessableEntity ||
		!strings.Contains(body, "box_meters_required") {
		t.Fatalf("no box length = %d %s", code, body)
	}
	// TEC-1251 (K1, 19.11): a Glorian synced product is sold by the meter too.
	if code, body := roll(synced, map[string]any{"sellable_by_meter": true, "default_meters": 15}); code != http.StatusOK ||
		!strings.Contains(body, `"sellable_by_meter":true`) {
		t.Fatalf("glorian product = %d %s", code, body)
	}
	code, body := roll(film, map[string]any{"sellable_by_meter": true, "default_meters": 15, "min_cut_meters": 2})
	if code != http.StatusOK || !strings.Contains(body, `"sellable_by_meter":true`) {
		t.Fatalf("switch on = %d %s", code, body)
	}
	// A roll PUT without the field keeps the switch.
	if code, body := roll(film, map[string]any{"default_meters": 15, "min_cut_meters": 2, "width_cm": 152}); code != http.StatusOK ||
		!strings.Contains(body, `"sellable_by_meter":true`) {
		t.Fatalf("switch kept = %d %s", code, body)
	}
	// The center's price view shows the meter list price.
	code, env := it.do("GET", "/v1/tenant/pricing/products/"+film.Uuid.String(), hostOlex, salesTok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"meter_price":"30`) {
		t.Fatalf("center price view = %d %s", code, env.Data)
	}

	// The dealer's discount: 10 % (box 400 -> 360).
	it.stepUp(sales.Uuid)
	if code, env := it.do("PUT", "/v1/tenant/pricing/discounts/"+dealer.Uuid.String(), hostOlex, salesTok,
		map[string]any{"standard_discount_pct": 10}); code != http.StatusOK {
		t.Fatalf("discount = %d %s", code, errCode(env))
	}

	// K6 / K1: refused meter lines.
	order := func(items ...map[string]any) []byte {
		b, _ := json.Marshal(map[string]any{"items": items})
		return b
	}
	rec := it.raw("POST", "/v1/orders", dealerTok, "application/json",
		order(map[string]any{"product_uuid": film.Uuid.String(), "meters": "1.5"}), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "ORDER_LINE_BELOW_MIN_CUT") {
		t.Fatalf("below min cut = %d %s", rec.Code, rec.Body.String())
	}
	rec = it.raw("POST", "/v1/orders", dealerTok, "application/json",
		order(map[string]any{"product_uuid": boxOnly.Uuid.String(), "meters": "3"}), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PRODUCT_NOT_SOLD_BY_METER") {
		t.Fatalf("not sold by the meter = %d %s", rec.Code, rec.Body.String())
	}
	rec = it.raw("POST", "/v1/orders", dealerTok, "application/json", order(
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "3"},
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "4"}), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("two meter lines = %d %s", rec.Code, rec.Body.String())
	}
	rec = it.raw("POST", "/v1/orders", dealerTok, "application/json",
		order(map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1, "line_credits": 3}), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("credits on a box line = %d %s", rec.Code, rec.Body.String())
	}

	// The quote previews the meter price with the discount.
	code, env = it.do("POST", "/v1/orders/quote", hostOlex, dealerTok, map[string]any{"product_uuids": []string{film.Uuid.String()}})
	if code != http.StatusOK {
		t.Fatalf("quote = %d %s", code, errCode(env))
	}
	var qv struct {
		Items []struct {
			Sellable  bool    `json:"sellable_by_meter"`
			UnitPrice *string `json:"unit_price"`
			MeterUnit *string `json:"meter_unit_price"`
			MeterList *string `json:"meter_list_unit_price"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &qv); err != nil {
		t.Fatal(err)
	}
	if len(qv.Items) != 1 || !qv.Items[0].Sellable || !sameAmount(qv.Items[0].UnitPrice, "360") ||
		!sameAmount(qv.Items[0].MeterUnit, "27") || qv.Items[0].MeterList != nil {
		t.Fatalf("dealer quote = %s", env.Data)
	}

	// K10: 1 box + 3 m of the same product on one order.
	own := it.boxMeterOrder("POST", "/v1/orders", dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "3"},
	}}, http.StatusCreated)
	if len(own.Items) != 2 {
		t.Fatalf("lines = %+v", own.Items)
	}
	box, meter := own.line(t, "box"), own.line(t, "meter")
	if box.Quantity == nil || *box.Quantity != 1 || box.LineCredits != nil || !sameAmount(box.UnitPrice, "360") {
		t.Fatalf("box line = %+v", box)
	}
	if !sameAmount(meter.Meters, "3") || meter.LineCredits == nil || *meter.LineCredits != 5 ||
		!sameAmount(meter.UnitPrice, "27") || !sameAmount(meter.LineTotal, "81") {
		t.Fatalf("meter line = %+v", meter)
	}
	if !sameAmount(own.Total, "441") {
		t.Fatalf("total = %v", own.Total)
	}

	// The draft edit keeps both lines; the meter line takes new credits.
	edited := it.boxMeterOrder("PUT", "/v1/orders/"+own.UUID+"/items", dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "4", "line_credits": 7},
	}}, http.StatusOK)
	if m := edited.line(t, "meter"); !sameAmount(m.Meters, "4") || m.LineCredits == nil || *m.LineCredits != 7 ||
		!sameAmount(m.LineTotal, "108") {
		t.Fatalf("edited meter line = %+v", m)
	}

	// The warehouse role sees the measure and the credits, never a price.
	whView := it.boxMeterOrder("GET", "/v1/orders/"+own.UUID, whTok, nil, http.StatusOK)
	if whView.Total != nil || len(whView.Items) != 2 {
		t.Fatalf("warehouse view = %+v", whView)
	}
	for _, l := range whView.Items {
		if l.UnitPrice != nil || l.LineTotal != nil || l.SaleMeasure == "" {
			t.Fatalf("warehouse line = %+v", l)
		}
	}
	if m := whView.line(t, "meter"); m.LineCredits == nil || *m.LineCredits != 7 {
		t.Fatalf("warehouse meter line = %+v", m)
	}
	// TEC-1239: the detail line names the product's box length and minimum
	// cut, so the order screens can hint them.
	if p := whView.line(t, "meter").Product; p.SellableByMeter == nil || !*p.SellableByMeter ||
		!sameAmount(p.BoxMeters, "15") || !sameAmount(p.MinCutMeters, "2") {
		t.Fatalf("meter line product = %+v", p)
	}

	// The center's order for the dealer: the same meter price rule.
	co := it.boxMeterOrder("POST", "/v1/orders", salesTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items": []any{
			map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1},
			map[string]any{"product_uuid": film.Uuid.String(), "sale_measure": "meter", "meters": 3, "line_credits": 4},
		},
	}, http.StatusCreated)
	if m := co.line(t, "meter"); !sameAmount(m.UnitPrice, "27") || m.LineCredits == nil || *m.LineCredits != 4 {
		t.Fatalf("center order meter line = %+v", m)
	}
	if b := co.line(t, "box"); !sameAmount(b.UnitPrice, "360") {
		t.Fatalf("center order box line = %+v", b)
	}

	// Without a meter list price: box price / box length (360 / 15 = 24).
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: film.ID, BrandID: film.BrandID, Currency: cur,
		SaleToDistributorPrice: pgtype.Text{String: "400", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	computed := it.boxMeterOrder("POST", "/v1/orders", dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "2"},
	}}, http.StatusCreated)
	if m := computed.line(t, "meter"); !sameAmount(m.UnitPrice, "24") {
		t.Fatalf("computed meter price = %+v", m)
	}

	// The switch off: the draft's meter line stays, a new one is refused.
	if code, body := roll(film, map[string]any{"sellable_by_meter": false, "default_meters": 15, "min_cut_meters": 2}); code != http.StatusOK {
		t.Fatalf("switch off = %d %s", code, body)
	}
	it.boxMeterOrder("PUT", "/v1/orders/"+own.UUID+"/items", dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "quantity": 2},
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "4"},
	}}, http.StatusOK)
	rec = it.raw("POST", "/v1/orders", dealerTok, "application/json",
		order(map[string]any{"product_uuid": film.Uuid.String(), "meters": "3"}), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PRODUCT_NOT_SOLD_BY_METER") {
		t.Fatalf("switched off = %d %s", rec.Code, rec.Body.String())
	}
}

// TEC-1238 (K3/K4): a cut piece (a unit with meters) carries the credits of
// the meter line it was shipped on; a roll without such a line carries none.
func TestIntegrationUnitCreditDefaultMeterLine(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	var n int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM pg_constraint WHERE conname = 'uq_order_items_order_product_measure'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("unique per product and measure: %d %v", n, err)
	}
	var src string
	if err := it.pool.QueryRow(ctx, `SELECT pg_get_functiondef('unit_credit_default(bigint)'::regprocedure)`).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "line_credits") {
		t.Fatalf("unit_credit_default does not read line_credits: %s", src)
	}
}
