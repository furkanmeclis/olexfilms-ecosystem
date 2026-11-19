package glorian_test

import (
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1251 (K1, 19.11): a synced product may be sold by the meter; the
// pull rewrites the remote-sourced columns but keeps the meter sale
// settings (the switch, the box length, the minimum cut, the meter price).
func TestPullKeepsMeterSaleSettings(t *testing.T) {
	f := newPullFixture(t, true)
	p := f.puller()
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("first run: %v", err)
	}
	prod, err := f.q.GetProductByConnectionExternalID(f.ctx, db.GetProductByConnectionExternalIDParams{
		ConnectionID: pgtype.Int8{Int64: f.conn.ID, Valid: true}, ExternalID: pgtype.Text{String: "10", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tx.Exec(f.ctx, `UPDATE products SET sellable_by_meter = true, default_roll_meters = 15,
		min_cut_meters = 1 WHERE id = $1`, prod.ID); err != nil {
		t.Fatalf("switch on a synced product: %v", err)
	}
	if _, err := f.q.UpsertProductPrice(f.ctx, db.UpsertProductPriceParams{
		ProductID: prod.ID, BrandID: prod.BrandID, Currency: "TRY",
		SaleToDistributorPrice: pgtype.Text{String: "400", Valid: true},
		MeterPrice:             pgtype.Text{String: "30", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	// The hub renames the product; the pull updates it.
	f.srv.SetFixture(fake.FixtureProducts, []fake.Row{
		f.product("10", "HYDRA-150", "HYDRA 150 Yeni", "1", "2026-09-12T11:00:00+00:00"),
		f.product("11", "HYDRA-200", "HYDRA 200", "1", "2026-09-02T11:00:00+00:00"),
		f.product("12", "TINT-35", "Tint 35", "2", "2026-09-03T11:00:00+00:00"),
	})
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var name, box, minCut, meterPrice string
	var sellable bool
	if err := f.tx.QueryRow(f.ctx, `SELECT p.name, p.sellable_by_meter, p.default_roll_meters::text, p.min_cut_meters::text,
		pp.meter_price::text FROM products p JOIN product_prices pp ON pp.product_id = p.id AND pp.currency = 'TRY'
		WHERE p.id = $1`, prod.ID).Scan(&name, &sellable, &box, &minCut, &meterPrice); err != nil {
		t.Fatal(err)
	}
	if name != f.name("HYDRA 150 Yeni") {
		t.Fatalf("remote rename not applied: %q", name)
	}
	if !sellable || box != "15.00" || minCut != "1.00" || meterPrice != "30.0000" {
		t.Fatalf("meter settings after the pull = %v %s %s %s", sellable, box, minCut, meterPrice)
	}
}

// TEC-1251 (K1): an opened box or a cut piece of a synced product has no
// Glorian model yet: the bulk push leaves it out and its exit sends no
// PATCH (the hub would answer 404 forever); the closed boxes go as before.
func TestPushLeavesOutMeteredPieces(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.pushUnits(t, f.glorian, 2, true)
	cut := s.units[1]
	if _, err := f.tx.Exec(f.ctx, `UPDATE units SET initial_meters = 3, remaining_meters = 3 WHERE id = $1`, cut.ID); err != nil {
		t.Fatal(err)
	}
	f.enter(t, s, f.glorian)
	p := f.pusher()
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v", err)
	}
	calls := f.srv.RequestsTo(http.MethodPost, "/stock-items/bulk")
	if len(calls) != 1 {
		t.Fatalf("bulk calls = %d; want 1", len(calls))
	}
	var body bulkBody
	if err := calls[0].JSON(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Barcode != s.units[0].Barcode {
		t.Fatalf("bulk items = %+v; want the closed box only", body.Items)
	}
	mv, _ := f.post(t, ledger.Movement{Type: ledger.TypeExternalOutbound, UnitID: cut.ID})
	if err := p.PatchTask(f.ctx, mv.ID); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if n := len(f.srv.RequestsTo(http.MethodPatch, "/stock-items/by-barcode/"+cut.Barcode)); n != 0 {
		t.Fatalf("patch calls for the cut piece = %d; want 0", n)
	}
}
