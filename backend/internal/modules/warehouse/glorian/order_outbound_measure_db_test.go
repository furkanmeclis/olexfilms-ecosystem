package glorian_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/jackc/pgx/v5/pgtype"
)

// meterLine adds a 3.5 m meter line of the case's product with one cut
// piece assigned (TEC-1244: what K1 forbids for a synced product, e.g. a
// draft meter line kept after the Glorian sync turned the switch off).
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

// TEC-1244 (K1 savunması): a meter line of a synced product never goes to
// Glorian; the box line next to it goes as before.
func TestOrderOutboundLeavesOutMeterLines(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 2, true, true)
	cut := f.meterLine(t, c)
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
	if len(body.Items) != 1 || body.Items[0].Quantity != 2 || len(body.Items[0].Barcodes) != 2 {
		t.Fatalf("create body = %+v", body)
	}
	for _, bc := range body.Items[0].Barcodes {
		if bc == cut.Barcode {
			t.Fatalf("cut piece %s sent to Glorian", bc)
		}
	}
}
