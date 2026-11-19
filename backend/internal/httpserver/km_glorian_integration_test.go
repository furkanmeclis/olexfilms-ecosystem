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

// TEC-1251 (TEC-1236 K1, 19.11): a product synced from Glorian is sold by
// the meter like an Olex product. The center turns the switch on with the
// box length, the minimum cut and the meter price; the dealer orders 3 m;
// the warehouse opens a closed box and cuts 3 m; the line is ready.
func TestIntegrationKutuMetreGlorianSyncedProduct(t *testing.T) {
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
	dealer := it.org("tgl-dealer", "dealer", center)
	admin, apw := it.user("tgl-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff")
	sales, spw := it.user("tgl-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff)
	wh, wpw := it.user("tgl-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	owner, opw := it.user("tgl-dealer-owner")
	it.member(dealer, owner, "owner")
	adminTok := it.loginOrg(admin, apw, center)
	salesTok := it.loginOrg(sales, spw, center)
	whTok := it.loginOrg(wh, wpw, center)
	dealerTok := it.loginOrg(owner, opw, dealer)

	film := it.product(center, "TGL")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET connection_id = 987654321, external_id = 'g-1251-' || $2 WHERE id = $1`, film.ID, it.suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: film.ID, BrandID: film.BrandID, Currency: cur,
		SaleToDistributorPrice: pgtype.Text{String: "400", Valid: true},
		MeterPrice:             pgtype.Text{String: "30", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"sellable_by_meter": true, "default_meters": 15, "min_cut_meters": 1})
	if rec := it.raw("PUT", "/v1/catalog/products/"+film.Uuid.String()+"/roll", adminTok, "application/json", raw, nil); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"sellable_by_meter":true`) {
		t.Fatalf("switch on a synced product = %d %s", rec.Code, rec.Body.String())
	}

	chain := it.stockChain()
	loc := chain.location(center, "TGL")
	box := chain.unit(center, film, 12511)
	chain.post(ledger.TypeEntry, box, chain.nextRef(), loc)

	o := it.boxMeterOrder("POST", "/v1/orders", dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "3", "line_credits": 5},
	}}, http.StatusCreated)
	line := o.line(t, "meter")
	if !sameAmount(line.UnitPrice, "30") || !sameAmount(line.LineTotal, "90") || line.LineCredits == nil || *line.LineCredits != 5 {
		t.Fatalf("meter line of a synced product = %+v", line)
	}
	for _, step := range []struct{ tok, status string }{
		{dealerTok, "submitted"}, {salesTok, "approved"}, {salesTok, "preparing"},
	} {
		if code, ec := it.transition(step.tok, o.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	path := "/v1/orders/" + o.UUID + "/items/" + line.UUID
	code, env := it.do("GET", path+"/cut-preview?"+url.Values{"barcode": {box.Barcode}}.Encode(), hostOlex, whTok, nil)
	var p cutPreview
	if code != http.StatusOK || json.Unmarshal(env.Data, &p) != nil || !p.ClosedBox || p.CutMeters != "3.00" || p.RemainingMeters != "12.00" {
		t.Fatalf("preview = %d %s", code, env.Data)
	}
	code, env = it.do("POST", path+"/units", hostOlex, whTok, map[string]any{"barcode": box.Barcode, "open_box": true})
	var a cutAssign
	if code != http.StatusOK || json.Unmarshal(env.Data, &a) != nil || a.Split == nil || !a.Split.BoxOpened || a.Split.Meters != "3.00" {
		t.Fatalf("open and cut = %d %s", code, env.Data)
	}
	if code, ec := it.transition(whTok, o.UUID, "ready"); code != http.StatusOK {
		t.Fatalf("ready = %d %s", code, ec)
	}
}
