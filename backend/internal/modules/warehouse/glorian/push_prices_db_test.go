package glorian_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1265, kullanıcı kararı 5: the USD list prices of the synced products
// go to the hub (PUT /products/prices) against the fake hub and the
// migrated database (CI sets TEST_DATABASE_URL).

func (f *pullFixture) pricePusher() *glorian.PricePusher {
	factory := glorian.HTTPClientFactory(glorian.Options{HTTPClient: f.srv.Client(), RetryBaseDelay: time.Millisecond})
	return glorian.NewPricePusher(f.q, f.box, factory, nil)
}

func (f *pullFixture) setPrice(t *testing.T, p db.Product, currency, sale, meter string) {
	t.Helper()
	text := func(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }
	if _, err := f.q.UpsertProductPrice(f.ctx, db.UpsertProductPriceParams{
		ProductID: p.ID, BrandID: p.BrandID, Currency: currency,
		PurchasePrice: text("1.0000"), SaleToDistributorPrice: text(sale), MeterPrice: text(meter),
	}); err != nil {
		t.Fatalf("price %s: %v", currency, err)
	}
}

type pricesBody struct {
	Currency string                 `json:"currency"`
	Items    []glorian.ProductPrice `json:"items"`
}

func pricePushCounts(t *testing.T, run db.IntegrationSyncRun) glorian.PricePushCounts {
	t.Helper()
	var c glorian.PricePushCounts
	if err := json.Unmarshal(run.Counts, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPricePushSendsUSDListPrices(t *testing.T) {
	f := newPullFixture(t, true)
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("pull: %v", err)
	}
	synced := f.syncedProducts(t) // external_id 10, 11, 12
	if len(synced) != 3 {
		t.Fatalf("synced products = %d", len(synced))
	}
	f.setPrice(t, synced[0], "USD", "120.5000", "11.2550") // rounded to 2 decimals
	f.setPrice(t, synced[1], "USD", "75.0000", "")         // no meter list price: cleared on the hub
	f.setPrice(t, synced[2], "TRY", "999.0000", "")        // no USD row: not sent

	if err := f.pricePusher().PushPricesTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v", err)
	}

	reqs := f.srv.RequestsTo("PUT", "/products/prices")
	if len(reqs) != 1 {
		t.Fatalf("price calls = %d", len(reqs))
	}
	var body pricesBody
	if err := reqs[0].JSON(&body); err != nil {
		t.Fatal(err)
	}
	if body.Currency != "USD" || len(body.Items) != 2 {
		t.Fatalf("body = %+v", body)
	}
	if it := body.Items[0]; it.ProductID != "10" || it.Price != "120.50" || it.MeterPrice == nil || *it.MeterPrice != "11.26" {
		t.Fatalf("item 10 = %+v", it)
	}
	if it := body.Items[1]; it.ProductID != "11" || it.Price != "75.00" || it.MeterPrice != nil {
		t.Fatalf("item 11 = %+v", it)
	}
	if p, ok := f.srv.ProductPrice("10"); !ok || p.Price != "120.50" {
		t.Fatalf("hub price 10 = %+v %v", p, ok)
	}
	if _, ok := f.srv.ProductPrice("12"); ok {
		t.Fatal("product without a USD list row must not be sent")
	}

	run := latestRun(t, f.runs(t), glorian.KindPushPrices)
	if run.Status != glorian.RunSucceeded {
		t.Fatalf("run = %s %v", run.Status, run.Error)
	}
	if c := pricePushCounts(t, run); c.Products != 2 || c.Calls != 1 || c.Updated != 2 || c.Unchanged != 0 {
		t.Fatalf("counts = %+v", c)
	}

	// A second run sends the same absolute values; the hub reports them
	// unchanged.
	if err := f.pricePusher().PushPricesTask(f.ctx, 0); err != nil {
		t.Fatalf("second push: %v", err)
	}
	if c := pricePushCounts(t, latestRun(t, f.runs(t), glorian.KindPushPrices)); c.Updated != 0 || c.Unchanged != 2 {
		t.Fatalf("second counts = %+v", c)
	}
}

func TestPricePushWithoutPricesMakesNoRun(t *testing.T) {
	f := newPullFixture(t, true)
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if err := f.pricePusher().PushPricesTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v", err)
	}
	if n := len(f.srv.RequestsTo("PUT", "/products/prices")); n != 0 {
		t.Fatalf("price calls = %d", n)
	}
	for _, r := range f.runs(t) {
		if r.Kind == glorian.KindPushPrices {
			t.Fatal("no priced product must not record a run")
		}
	}
}

func TestPricePushHeldOnInactiveConnection(t *testing.T) {
	f := newPullFixture(t, true)
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("pull: %v", err)
	}
	synced := f.syncedProducts(t)
	f.setPrice(t, synced[0], "USD", "10.0000", "")
	if _, err := f.tx.Exec(f.ctx, `UPDATE integration_connections SET active = false WHERE id = $1`, f.conn.ID); err != nil {
		t.Fatal(err)
	}

	// The cron form skips an inactive connection without a run.
	if err := f.pricePusher().PushPricesTask(f.ctx, 0); err != nil {
		t.Fatalf("cron push: %v", err)
	}
	// The manual form records a held run (acknowledged, not retried) and
	// makes no request.
	if err := f.pricePusher().PushPricesTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("held push = %v; want nil (held work is not retried)", err)
	}
	if n := len(f.srv.RequestsTo("PUT", "/products/prices")); n != 0 {
		t.Fatalf("price calls = %d", n)
	}
	run := latestRun(t, f.runs(t), glorian.KindPushPrices)
	if run.Status != glorian.RunFailed || pricePushCounts(t, run).Held != 1 {
		t.Fatalf("run = %s %+v", run.Status, pricePushCounts(t, run))
	}
}
