package glorian_test

import (
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
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

// TEC-1252: an opened box of a synced product goes to the hub with its
// meters (opened, absolute remaining) next to the closed boxes, which go
// as before; its exit is patched like a box (the hub knows the barcode).
func TestPushSendsOpenedBoxWithMeters(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.pushUnits(t, f.glorian, 2, true)
	opened := s.units[1]
	if _, err := f.tx.Exec(f.ctx, `UPDATE units SET initial_meters = 15, remaining_meters = 12 WHERE id = $1`, opened.ID); err != nil {
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
	if len(body.Items) != 2 {
		t.Fatalf("bulk items = %+v; want the closed and the opened box", body.Items)
	}
	box, roll := body.Items[0], body.Items[1]
	if box.Barcode != s.units[0].Barcode || box.InitialMeters != "" || box.RemainingMeters != "" || box.Opened || box.SourceBarcode != "" {
		t.Fatalf("closed box = %+v; want no meter fields", box)
	}
	if roll.Barcode != opened.Barcode || roll.InitialMeters != "15.00" || roll.RemainingMeters != "12.00" || !roll.Opened || roll.SourceBarcode != "" {
		t.Fatalf("opened box = %+v", roll)
	}
	if row, _ := f.srv.StockItem(opened.Barcode); row["kind"] != glorian.StockKindOpenedBox || row["remaining_meters"] != 12.0 {
		t.Fatalf("hub row of the opened box = %+v", row)
	}
	mv, _ := f.post(t, ledger.Movement{Type: ledger.TypeExternalOutbound, UnitID: opened.ID})
	if err := p.PatchTask(f.ctx, mv.ID); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if n := len(f.srv.RequestsTo(http.MethodPatch, "/stock-items/by-barcode/"+opened.Barcode)); n != 1 {
		t.Fatalf("patch calls for the opened box = %d; want 1", n)
	}
	if row, _ := f.srv.StockItem(opened.Barcode); row["status"] != glorian.StockStatusExternalOutbound {
		t.Fatalf("hub status after the exit = %v", row["status"])
	}
}

// TEC-1252 (K5): a closed box opened in a bin and cut. The opening pushes
// the box as opened (15 m); the cut pushes the new piece with its source
// barcode and the source's absolute remaining in the same bulk call, so
// the hub ends at the warehouse's meters (12 m) whatever it had; the
// piece's exit is patched.
func TestPushOpenAndCutFlowsToHub(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.pushUnits(t, f.glorian, 1, true)
	box := s.units[0]
	f.enter(t, s, f.glorian)
	p := f.pusher()
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v", err)
	}
	if row, _ := f.srv.StockItem(box.Barcode); row["initial_meters"] != nil {
		t.Fatalf("closed box on the hub = %+v; want no meters", row)
	}

	l := ledger.New(f.q, &captureOutbox{})
	if _, err := l.ConvertMeasure(f.ctx, f.tx, ledger.MeasureConversion{
		UnitID: box.ID, Centimeters: 1500, HolderOrgID: f.glorian.ID,
		Source: "test", RefType: "t1252", RefID: 1, Reason: "box_opened",
	}); err != nil {
		t.Fatalf("open box: %v", err)
	}
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push after opening: %v", err)
	}
	if row, _ := f.srv.StockItem(box.Barcode); row["kind"] != glorian.StockKindOpenedBox || row["remaining_meters"] != 15.0 {
		t.Fatalf("hub row after opening = %+v", row)
	}

	res, err := l.Split(f.ctx, f.tx, ledger.SplitCommand{
		SourceUnitID: box.ID, Centimeters: 300, HolderOrgID: f.glorian.ID, IdempotencyKey: "t1252-cut-" + f.suffix,
	})
	if err != nil {
		t.Fatalf("cut: %v", err)
	}
	piece := res.New
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push after the cut: %v", err)
	}
	calls := f.srv.RequestsTo(http.MethodPost, "/stock-items/bulk")
	var body bulkBody
	if err := calls[len(calls)-1].JSON(&body); err != nil {
		t.Fatal(err)
	}
	byBarcode := map[string]glorian.BarcodeUpsert{}
	for _, it := range body.Items {
		byBarcode[it.Barcode] = it
	}
	got, ok := byBarcode[piece.Barcode]
	if !ok || got.SourceBarcode != box.Barcode || got.InitialMeters != "3.00" || got.RemainingMeters != "3.00" || got.Opened {
		t.Fatalf("cut piece in the bulk = %+v (%v); items %+v", got, ok, body.Items)
	}
	if src := byBarcode[box.Barcode]; src.RemainingMeters != "12.00" || src.InitialMeters != "15.00" || !src.Opened {
		t.Fatalf("source in the same bulk = %+v", src)
	}
	if row, _ := f.srv.StockItem(piece.Barcode); row["kind"] != glorian.StockKindCutPiece || row["source_barcode"] != box.Barcode || row["initial_meters"] != 3.0 {
		t.Fatalf("hub row of the piece = %+v", row)
	}
	if row, _ := f.srv.StockItem(box.Barcode); row["remaining_meters"] != 12.0 {
		t.Fatalf("hub remaining of the source = %v; want 12", row["remaining_meters"])
	}

	mv, _ := f.post(t, ledger.Movement{Type: ledger.TypeExternalOutbound, UnitID: piece.ID})
	if err := p.PatchTask(f.ctx, mv.ID); err != nil {
		t.Fatalf("patch the piece: %v", err)
	}
	if row, _ := f.srv.StockItem(piece.Barcode); row["status"] != glorian.StockStatusExternalOutbound {
		t.Fatalf("hub status of the piece after the exit = %v", row["status"])
	}
}
