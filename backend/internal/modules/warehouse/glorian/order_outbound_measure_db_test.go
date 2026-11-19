package glorian_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/jackc/pgx/v5/pgtype"
)

// meterLine adds a 3.5 m meter line (5 credits) of the case's product with
// one cut piece assigned.
func (f *pullFixture) meterLine(t *testing.T, c orderCase) db.Unit {
	t.Helper()
	var m, price, total pgtype.Numeric
	_ = m.Scan("3.50")
	_ = price.Scan("30.0000")
	_ = total.Scan("105.00")
	item, err := f.q.CreateOrderItem(f.ctx, db.CreateOrderItemParams{
		OrderID: c.order.ID, ProductID: c.units.product.ID, Meters: m, LineCredits: pgtype.Int4{Int32: 5, Valid: true},
		UnitPrice: price, PriceSource: "list", LineTotal: total,
	})
	if err != nil {
		t.Fatalf("meter line: %v", err)
	}
	f.seq++
	u, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
		OrganizationID: c.order.SellerOrgID, BrandID: c.order.BrandID, ProductID: c.units.product.ID,
		Barcode: fmt.Sprintf("GT1244-%s-%d", f.suffix, f.seq), UnitKind: ledger.KindSerial,
		Source: "generated", Status: string(ledger.StatusPrinted), InitialMeters: m, RemainingMeters: m,
	})
	if err != nil {
		t.Fatalf("cut piece: %v", err)
	}
	if _, err := f.q.CreateOrderItemUnit(f.ctx, db.CreateOrderItemUnitParams{OrderItemID: item.ID, UnitID: u.ID, Meters: m}); err != nil {
		t.Fatalf("assign cut piece: %v", err)
	}
	return u
}

// TEC-1252: a meter line of a synced product goes to the hub as a meter
// line (meters, credits, plotter) with its piece; the box line next to it
// goes as before. The piece's credit share goes as barcode_credits, the
// hub records the piece and reserves it.
func TestOrderOutboundSendsMeterLines(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 2, true, true)
	cut := f.meterLine(t, c)
	if _, err := f.tx.Exec(f.ctx, `UPDATE order_item_units SET credits = 4 WHERE unit_id = $1`, cut.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tx.Exec(f.ctx, `UPDATE order_items SET plotter_cut = true
		WHERE id = (SELECT order_item_id FROM order_item_units WHERE unit_id = $1)`, cut.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.orders().SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	posts := f.srv.RequestsTo(http.MethodPost, "/orders")
	if len(posts) != 1 {
		t.Fatalf("POST /orders = %d; want 1", len(posts))
	}
	var body glorian.CreateOrderInput
	if err := posts[0].JSON(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("create body = %+v; want a box and a meter line", body)
	}
	box, meter := body.Items[0], body.Items[1]
	if box.Quantity != 2 || len(box.Barcodes) != 2 || box.SaleMeasure != "" || box.Meters != "" || box.Pieces != nil || box.LineCredits != nil {
		t.Fatalf("box line = %+v; want it as before", box)
	}
	if meter.SaleMeasure != glorian.SaleMeasureMeter || meter.Meters != "3.50" || meter.LineCredits == nil || *meter.LineCredits != 5 ||
		!meter.Plotter || meter.Quantity != 1 || len(meter.Barcodes) != 1 || meter.Barcodes[0] != cut.Barcode {
		t.Fatalf("meter line = %+v", meter)
	}
	if len(meter.Pieces) != 1 || meter.Pieces[0].Barcode != cut.Barcode || meter.Pieces[0].Meters != "3.50" ||
		meter.Pieces[0].Credits == nil || *meter.Pieces[0].Credits != 4 {
		t.Fatalf("pieces = %+v", meter.Pieces)
	}
	if meter.BarcodeCredits[cut.Barcode] != 4 {
		t.Fatalf("barcode_credits = %v", meter.BarcodeCredits)
	}
	row, ok := f.srv.StockItem(cut.Barcode)
	if !ok || row["status"] != glorian.StockStatusReserved || row["credits"] != 4 || row["remaining_meters"] != 3.5 {
		t.Fatalf("hub row of the piece = %+v (%v)", row, ok)
	}
	order, _ := f.srv.Order("1")
	lines, _ := order["items"].([]any)
	if len(lines) != 2 || lines[1].(fake.Row)["sale_measure"] != glorian.SaleMeasureMeter {
		t.Fatalf("hub order lines = %+v", lines)
	}
}

// TEC-1252: a meter line whose piece has no credit share yet sends no
// barcode_credits (the hub gives a single piece the whole line).
func TestOrderOutboundMeterLineWithoutShares(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 1, true, true)
	f.meterLine(t, c)
	if err := f.orders().SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var body glorian.CreateOrderInput
	if err := f.srv.RequestsTo(http.MethodPost, "/orders")[0].JSON(&body); err != nil {
		t.Fatal(err)
	}
	meter := body.Items[1]
	if meter.BarcodeCredits != nil || meter.Pieces[0].Credits != nil || meter.Plotter {
		t.Fatalf("meter line = %+v; want no credit shares, no plotter", meter)
	}
}
