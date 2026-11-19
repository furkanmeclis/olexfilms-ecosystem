package usecase

import (
	"testing"

	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
)

// TEC-1244 acceptance (kur dönüşümü, uçtan uca): a brand currency change
// carries the meter prices with it. The meter list price is converted (60
// TRY -> 1.50 USD), an empty one stays empty (computed); an open order's
// meter line is repriced like a box line (3.5 m × 1.50 = 5.25); after the
// change the dealer's meter price in USD follows the converted list (box
// 25, meter 1.50; computed 50 / 15 = 3.3333).
func TestBrandCurrencyMeterPrices(t *testing.T) {
	f := newFixture(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`UPDATE products SET default_roll_meters = 15, sellable_by_meter = TRUE WHERE id IN ($1, $2)`, f.product, f.product2)
	exec(`UPDATE product_prices SET meter_price = 60 WHERE product_id = $1 AND currency = 'TRY'`, f.product)
	exec(`INSERT INTO order_items (order_id, organization_id, brand_id, product_id, meters, line_credits, unit_price, price_source, line_total)
SELECT o.id, o.organization_id, o.brand_id, $2, 3.5, 5, 60, 'list', 210 FROM orders o WHERE o.id = $1`, f.order, f.product)
	exec(`UPDATE orders SET subtotal = 2210, total = 2210 WHERE id = $1`, f.order)
	svc := New(f.tx, f.q, fakeRates{"USD/TRY": "40", "TRY/USD": "0.025"}, fixedLocal("TRY"))

	cp, err := svc.ConvertPrices(f.ctx, f.brand, ConvertInput{TargetCurrency: "USD"})
	if err != nil || !cp.Applied {
		t.Fatalf("convert = %+v, %v", cp, err)
	}
	for _, r := range cp.Rows {
		switch r.SKU[len(r.SKU)-1] {
		case 'a':
			if r.Source.MeterPrice == nil || r.Converted.MeterPrice == nil || *r.Converted.MeterPrice != "1.50" {
				t.Fatalf("converted a = %+v", r.Converted)
			}
		case 'b':
			if r.Converted.MeterPrice != nil {
				t.Fatalf("converted b meter = %v (an empty meter list stays computed)", *r.Converted.MeterPrice)
			}
		}
	}
	var stored *string
	if err := f.tx.QueryRow(f.ctx, `SELECT meter_price::text FROM product_prices WHERE product_id = $1 AND currency = 'USD'`, f.product).
		Scan(&stored); err != nil || stored == nil || *stored != "1.5000" {
		t.Fatalf("stored USD meter price = %v, %v", stored, err)
	}

	if _, err := svc.ChangeCurrency(f.ctx, f.brand, ChangeInput{Currency: "USD", OpenOrders: OpenOrdersReprice}); err != nil {
		t.Fatalf("change: %v", err)
	}
	var unit, line, total string
	if err := f.tx.QueryRow(f.ctx, `SELECT i.unit_price::text, i.line_total::text, o.total::text FROM orders o
JOIN order_items i ON i.order_id = o.id WHERE o.id = $1 AND i.sale_measure = 'meter'`, f.order).Scan(&unit, &line, &total); err != nil {
		t.Fatal(err)
	}
	if unit != "1.5000" || line != "5.25" || total != "55.25" {
		t.Fatalf("repriced meter line = %s %s, order total %s", unit, line, total)
	}

	var dealer int64
	if err := f.tx.QueryRow(f.ctx, `SELECT buyer_org_id FROM orders WHERE id = $1`, f.order).Scan(&dealer); err != nil {
		t.Fatal(err)
	}
	prices, err := pricingusecase.New(f.q).BuyerPurchasePrices(f.ctx, dealer, "dealer", f.brandID, []int64{f.product, f.product2}, "USD")
	if err != nil {
		t.Fatal(err)
	}
	a, b := prices[f.product], prices[f.product2]
	if a.Price != "25.0000" && a.Price != "25.00" && a.Price != "25" {
		t.Fatalf("dealer box price a = %+v", a)
	}
	if a.MeterPrice != "1.5000" {
		t.Fatalf("dealer meter price a = %+v", a)
	}
	if b.MeterPrice != "3.3333" {
		t.Fatalf("dealer computed meter price b = %+v", b)
	}
}
