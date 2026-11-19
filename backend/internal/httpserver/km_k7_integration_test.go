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

// TEC-1250 (TEC-1236 K7, 19.11): a roll is not cut when what would stay
// is shorter than the minimum cut; the whole roll goes with the order and
// the extra meters are free. Over HTTP on one product (box 15 m, minimum
// cut 1 m, meter price 30):
//
//  1. 3 m from a 3.5 m open roll: the suggestion prefers a roll cut
//     cleanly (5 m), the warehouse scans the 3.5 m roll anyway; the preview
//     says the whole 3.5 m goes with 0.5 m free; the roll is assigned
//     without a cut, the line reads 0.5 m excess and goes ready; shipped,
//     the center's stock loses 3.5 m and keeps no remnant; the order is
//     billed 3 m (90); the piece carries the line's credits; its return is
//     credited 3 m × 30 = 90.
//  2. 14.6 m from a closed 15 m box: the box is suggested whole (0.4 m
//     free), opened on confirmation and assigned whole.
//  3. 4 m from the 5 m roll leaves exactly the minimum cut (1 m): it is
//     cut as before.
func TestIntegrationKutuMetreK7WholeRoll(t *testing.T) {
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
	// TEC-1250 (19.11): this scenario is the default free mode.
	if _, err := it.pool.Exec(ctx, `UPDATE organizations SET excess_meter_billing = 'free' WHERE id = $1`, center.ID); err != nil {
		t.Fatal(err)
	}
	dealer := it.org("tk7-dealer", "dealer", center)
	var dealerCur string
	if err := it.pool.QueryRow(ctx, `SELECT currency FROM organizations WHERE id = $1`, dealer.ID).Scan(&dealerCur); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(dealerCur) != cur {
		t.Skipf("dealer currency %s differs from the brand currency %s", dealerCur, cur)
	}
	admin, apw := it.user("tk7-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff")
	sales, spw := it.user("tk7-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff, rbac.RoleCenterAccounting)
	wh, wpw := it.user("tk7-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	owner, opw := it.user("tk7-dealer-owner")
	it.member(dealer, owner, "owner")
	adminTok := it.loginOrg(admin, apw, center)
	salesTok := it.loginOrg(sales, spw, center)
	whTok := it.loginOrg(wh, wpw, center)
	dealerTok := it.loginOrg(owner, opw, dealer)

	film := it.product(center, "TK7")
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
	loc := chain.location(center, "TK7")
	roll35 := chain.rollUnit(center, film, 12501, "3.50")
	chain.post(ledger.TypeEntry, roll35, chain.nextRef(), loc)
	roll5 := chain.rollUnit(center, film, 12502, "5.00")
	chain.post(ledger.TypeEntry, roll5, chain.nextRef(), loc)
	box := chain.unit(center, film, 12503)
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
	suggest := func(o boxMeterOrder) string {
		code, env := it.do("GET", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID+"/suggestions?include_unlocated=true", hostOlex, whTok, nil)
		if code != http.StatusOK {
			t.Fatalf("suggestions = %d %s", code, errCode(env))
		}
		return string(env.Data)
	}
	preview := func(o boxMeterOrder, barcode string) cutPreview {
		code, env := it.do("GET", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID+"/cut-preview?"+
			url.Values{"barcode": {barcode}}.Encode(), hostOlex, whTok, nil)
		var p cutPreview
		if code != http.StatusOK || json.Unmarshal(env.Data, &p) != nil {
			t.Fatalf("preview = %d %s", code, env.Data)
		}
		if strings.Contains(string(env.Data), "price") {
			t.Fatalf("preview carries a price: %s", env.Data)
		}
		return p
	}
	assign := func(o boxMeterOrder, body map[string]any) cutAssign {
		code, env := it.do("POST", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID+"/units", hostOlex, whTok, body)
		var a cutAssign
		if code != http.StatusOK || json.Unmarshal(env.Data, &a) != nil {
			t.Fatalf("assign = %d %s %s", code, errCode(env), env.Data)
		}
		return a
	}
	centerMeters := func() string {
		var m string
		if err := it.pool.QueryRow(ctx, `SELECT COALESCE(meters, 0)::numeric(14,2)::text FROM organization_product_stocks
			WHERE organization_id = $1 AND product_id = $2`, center.ID, film.ID).Scan(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	// --- 1. 3 m from the 3.5 m roll.
	o1 := order("3")
	if l := o1.Items[0]; !sameAmount(l.UnitPrice, "30") || !sameAmount(l.LineTotal, "90") {
		t.Fatalf("meter line = %+v", l)
	}
	// The roll cut cleanly (5 m → 3 m + 2 m) is suggested before the roll
	// that would go whole.
	if s := suggest(o1); !strings.Contains(s, roll5.Barcode) || strings.Contains(s, roll35.Barcode) ||
		!strings.Contains(s, `"excess_meters":null`) {
		t.Fatalf("suggestion = %s", s)
	}
	p := preview(o1, roll35.Barcode)
	if p.ClosedBox || !p.Whole || p.SourceMeters != "3.50" || p.CutMeters != "3.50" || p.RemainingMeters != "0.00" ||
		p.ExcessMeters != "0.50" || p.Remnant || p.Partial || p.Credits != 5 {
		t.Fatalf("3.5 m roll preview = %+v", p)
	}
	a := assign(o1, map[string]any{"barcode": roll35.Barcode})
	if a.Split != nil || a.Items[0].Assigned != "3.50" || a.Items[0].ExcessMeters == nil || *a.Items[0].ExcessMeters != "0.50" ||
		len(a.Items[0].Units) != 1 || a.Items[0].Units[0].Barcode != roll35.Barcode || a.Items[0].UnitPrice != nil {
		t.Fatalf("whole roll on the line = %+v", a)
	}
	// The line is covered: nothing more is suggested.
	if s := suggest(o1); !strings.Contains(s, `"missing":"0.00"`) || !strings.Contains(s, `"suggestions":[]`) {
		t.Fatalf("suggestion of a covered line = %s", s)
	}
	before := centerMeters()
	for _, step := range []struct{ tok, status string }{{whTok, "ready"}, {whTok, "shipped"}, {dealerTok, "received"}} {
		if code, ec := it.transition(step.tok, o1.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	if b, a := before, centerMeters(); !sameAmount(&b, "8.5") || !sameAmount(&a, "5") {
		t.Fatalf("center meters %s → %s, want 8.50 → 5.00 (the 3.5 m roll left)", b, a)
	}
	var remnants int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM units u JOIN unit_current_state st ON st.unit_id = u.id
		WHERE u.product_id = $1 AND st.holder_org_id = $2 AND u.remaining_meters > 0 AND u.remaining_meters < 1`,
		film.ID, center.ID).Scan(&remnants); err != nil || remnants != 0 {
		t.Fatalf("remnants at the center = %d %v", remnants, err)
	}
	if st, ot, oid := it.unitStateOf(roll35.ID); st != "available" || ot != "organization" || oid != dealer.ID {
		t.Fatalf("roll after receipt: %s %s %d", st, ot, oid)
	}
	var meters string
	var credits int
	if err := it.pool.QueryRow(ctx, `SELECT remaining_meters::text, unit_credit_default(id) FROM units WHERE id = $1`, roll35.ID).
		Scan(&meters, &credits); err != nil || meters != "3.50" || credits != 5 {
		t.Fatalf("piece at the dealer = %s m, %d credits, %v", meters, credits, err)
	}
	var total string
	if err := it.pool.QueryRow(ctx, `SELECT total::text FROM orders WHERE uuid = $1`, o1.UUID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if code, env := it.do("GET", "/v1/orders/"+o1.UUID, hostOlex, salesTok, nil); code != http.StatusOK {
		t.Fatalf("order = %d", code)
	} else {
		var v boxMeterOrder
		if err := json.Unmarshal(env.Data, &v); err != nil || !sameAmount(v.Total, total) || !sameAmount(v.Items[0].LineTotal, "90") {
			t.Fatalf("billed = %s / %+v (stored total %s)", env.Data, v.Items, total)
		}
	}
	// The return is credited at the line's meters: 3 × 30 = 90, not 105.
	ret, _ := it.transferCall("POST", "/v1/stock-transfers", dealerTok, map[string]any{
		"kind": "return", "items": []map[string]any{{"barcode": roll35.Barcode}},
	}, http.StatusCreated)
	if code, ec := it.transferMove(salesTok, ret.UUID, "approved"); code != http.StatusOK {
		t.Fatalf("approve return = %d %s", code, ec)
	}
	var unitPrice, lineTotal string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(i.unit_price::text, ''), COALESCE(i.line_total::text, '')
		FROM stock_transfer_request_items i JOIN stock_transfer_requests r ON r.id = i.request_id
		WHERE r.uuid = $1 AND i.unit_id = $2`, ret.UUID, roll35.ID).Scan(&unitPrice, &lineTotal); err != nil {
		t.Fatal(err)
	}
	if !sameAmount(&unitPrice, "30") || !sameAmount(&lineTotal, "90") {
		t.Fatalf("return amount = %s / %s, want 30 / 90", unitPrice, lineTotal)
	}

	// --- 2. 14.6 m from a closed 15 m box: the box goes whole.
	o2 := order("14.6")
	if s := suggest(o2); !strings.Contains(s, box.Barcode) || !strings.Contains(s, `"open_box":true`) ||
		!strings.Contains(s, `"meters":"15.00"`) || !strings.Contains(s, `"excess_meters":"0.40"`) {
		t.Fatalf("box suggestion = %s", s)
	}
	p = preview(o2, box.Barcode)
	if !p.ClosedBox || !p.Whole || p.CutMeters != "15.00" || p.RemainingMeters != "0.00" || p.ExcessMeters != "0.40" || p.Remnant {
		t.Fatalf("box preview = %+v", p)
	}
	a = assign(o2, map[string]any{"barcode": box.Barcode, "open_box": true})
	if a.Split != nil || a.Items[0].Assigned != "15.00" || a.Items[0].ExcessMeters == nil || *a.Items[0].ExcessMeters != "0.40" {
		t.Fatalf("whole box = %+v", a)
	}
	var reason string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(metadata->>'reason', '') FROM stock_movements
		WHERE unit_id = $1 AND type = 'measure_conversion'`, box.ID).Scan(&reason); err != nil || reason != "box_opened" {
		t.Fatalf("box opened movement = %q %v", reason, err)
	}
	if code, ec := it.transition(whTok, o2.UUID, "ready"); code != http.StatusOK {
		t.Fatalf("o2 ready = %d %s", code, ec)
	}

	// --- 3. Exactly the minimum cut left: 4 m from the 5 m roll is cut.
	o3 := order("4")
	p = preview(o3, roll5.Barcode)
	if p.Whole || p.CutMeters != "4.00" || p.RemainingMeters != "1.00" || p.ExcessMeters != "0.00" || p.Remnant {
		t.Fatalf("exact minimum preview = %+v", p)
	}
	a = assign(o3, map[string]any{"barcode": roll5.Barcode})
	if a.Split == nil || a.Split.Meters != "4.00" || a.Split.SourceRemainingMeters != "1.00" || a.Split.SourceRemnant ||
		a.Items[0].ExcessMeters != nil {
		t.Fatalf("exact minimum cut = %+v", a)
	}
}
