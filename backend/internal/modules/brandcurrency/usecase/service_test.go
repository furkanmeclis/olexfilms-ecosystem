package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeRates map[string]string

func (f fakeRates) ResolveRate(_ context.Context, on time.Time, base, quote string) (fxrates.Snapshot, error) {
	if base == quote {
		return fxrates.Snapshot{Base: base, Quote: quote, Rate: "1"}, nil
	}
	r, ok := f[base+"/"+quote]
	if !ok {
		return fxrates.Snapshot{}, fxrates.ErrRateNotFound
	}
	return fxrates.Snapshot{Base: base, Quote: quote, Rate: r, RateDate: on.Format(time.DateOnly), Source: "test"}, nil
}

type fixedLocal string

func (l fixedLocal) LocalCurrency(context.Context) string { return string(l) }

type fixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	brandID  int64
	brand    uuid.UUID
	product  int64
	product2 int64
	order    int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &fixture{ctx: ctx, tx: tx, q: db.New(tx)}
	prefix := fmt.Sprintf("t1214-%d", time.Now().UnixNano())
	if err := tx.QueryRow(ctx, `INSERT INTO brands (slug, name) VALUES ($1, $1) RETURNING id, uuid`, prefix[:20]).Scan(&f.brandID, &f.brand); err != nil {
		t.Fatalf("brand: %v", err)
	}
	org := func(typ string, parent int64) int64 {
		o, err := f.q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: prefix + "-" + typ, Name: prefix + " " + typ, Status: "active", Type: typ,
			ParentID: pgtype.Int8{Int64: parent, Valid: parent != 0}, BrandID: f.brandID,
			Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatalf("org %s: %v", typ, err)
		}
		return o.ID
	}
	center := org("center", 0)
	dealer := org("dealer", center)
	id := func(sql string, args ...any) int64 {
		var v int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return v
	}
	cat := id(`INSERT INTO product_categories (organization_id, brand_id, name) VALUES ($1, $2, 'cat') RETURNING id`, center, f.brandID)
	f.product = id(`INSERT INTO products (organization_id, brand_id, category_id, sku, name, unit_type)
VALUES ($1, $2, $3, $4, 'Film A', 'piece') RETURNING id`, center, f.brandID, cat, prefix+"-a")
	f.product2 = id(`INSERT INTO products (organization_id, brand_id, category_id, sku, name, unit_type)
VALUES ($1, $2, $3, $4, 'Film B', 'piece') RETURNING id`, center, f.brandID, cat, prefix+"-b")
	if _, err := tx.Exec(ctx, `INSERT INTO product_prices (product_id, brand_id, currency, purchase_price, sale_to_distributor_price, recommended_sale_price)
VALUES ($1, $3, 'TRY', 400, 1000, NULL), ($2, $3, 'TRY', NULL, 2000, 3000)`, f.product, f.product2, f.brandID); err != nil {
		t.Fatal(err)
	}
	f.order = id(`INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency, subtotal, total)
VALUES ($1, $2, $1, $3, 'submitted', 'TRY', 2000, 2000) RETURNING id`, center, f.brandID, dealer)
	if _, err := tx.Exec(ctx, `INSERT INTO order_items (order_id, organization_id, brand_id, product_id, quantity, unit_price, price_source, line_total)
SELECT o.id, o.organization_id, o.brand_id, $2, 2, 1000, 'list', 2000 FROM orders o WHERE o.id = $1`, f.order, f.product); err != nil {
		t.Fatal(err)
	}
	return f
}

// TEC-1214: preview -> blocked -> price conversion -> change with reprice.
func TestBrandCurrencyChangeFlow(t *testing.T) {
	f := newFixture(t)
	svc := New(f.tx, f.q, fakeRates{"USD/TRY": "40", "TRY/USD": "0.025"}, fixedLocal("TRY"))

	// Dry run: no USD prices, one open order, USD -> TRY rate present.
	p, err := svc.ChangeCurrency(f.ctx, f.brand, ChangeInput{Currency: "usd", DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if p.From != "TRY" || p.To != "USD" || p.MissingPriceCount != 2 || p.OpenOrderCount != 1 || p.RateToLocal == nil || p.Applied {
		t.Fatalf("preview = %+v", p)
	}
	if !slices.Contains(p.Blockers, BlockerOpenOrders) || !slices.Contains(p.Blockers, BlockerMissingPrices) {
		t.Fatalf("blockers = %v", p.Blockers)
	}
	// Applying with blockers changes nothing.
	var blocked *BlockedError
	if _, err := svc.ChangeCurrency(f.ctx, f.brand, ChangeInput{Currency: "USD"}); !errors.As(err, &blocked) {
		t.Fatalf("apply with blockers err = %v", err)
	}

	// Price conversion: preview, apply, a second run skips existing rows.
	cp, err := svc.ConvertPrices(f.ctx, f.brand, ConvertInput{TargetCurrency: "USD", DryRun: true})
	if err != nil {
		t.Fatalf("convert dry run: %v", err)
	}
	if cp.Source != "TRY" || cp.Total != 2 || cp.Create != 2 || cp.Applied {
		t.Fatalf("conversion preview = %+v", cp)
	}
	for _, r := range cp.Rows {
		if r.SKU[len(r.SKU)-1] == 'a' && (r.Converted.SaleToDistributorPrice == nil || *r.Converted.SaleToDistributorPrice != "25.00" ||
			r.Converted.RecommendedSalePrice != nil || *r.Converted.PurchasePrice != "10.00") {
			t.Fatalf("converted a = %+v", r.Converted)
		}
	}
	if cp, err = svc.ConvertPrices(f.ctx, f.brand, ConvertInput{TargetCurrency: "USD"}); err != nil || !cp.Applied || cp.Create != 2 {
		t.Fatalf("convert apply = %+v, %v", cp, err)
	}
	if cp, err = svc.ConvertPrices(f.ctx, f.brand, ConvertInput{TargetCurrency: "USD", DryRun: true}); err != nil || cp.Skip != 2 {
		t.Fatalf("convert rerun = %+v, %v", cp, err)
	}
	if _, err := svc.ConvertPrices(f.ctx, f.brand, ConvertInput{TargetCurrency: "EUR", DryRun: true}); !errors.Is(err, ErrRateNotFound) {
		t.Fatalf("convert without rate err = %v", err)
	}

	// Change with reprice: the open order moves to USD at TRY/USD.
	p, err = svc.ChangeCurrency(f.ctx, f.brand, ChangeInput{Currency: "USD", OpenOrders: OpenOrdersReprice})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !p.Applied || p.RepricedOrders != 1 || p.Brand.Currency != "USD" || len(p.Blockers) != 0 {
		t.Fatalf("applied = %+v", p)
	}
	var cur, total, unit string
	if err := f.tx.QueryRow(f.ctx, `SELECT o.currency, o.total::text, i.unit_price::text FROM orders o
JOIN order_items i ON i.order_id = o.id WHERE o.id = $1`, f.order).Scan(&cur, &total, &unit); err != nil {
		t.Fatal(err)
	}
	if cur != "USD" || total != "50.00" || unit != "25.0000" {
		t.Fatalf("repriced order = %s %s %s", cur, total, unit)
	}
	// Same currency again is a blocker; unknown option and code are invalid.
	if _, err := svc.ChangeCurrency(f.ctx, f.brand, ChangeInput{Currency: "USD"}); !errors.As(err, &blocked) ||
		!slices.Contains(blocked.Blockers, BlockerSameCurrency) {
		t.Fatalf("same currency err = %v", err)
	}
	if _, err := svc.ChangeCurrency(f.ctx, f.brand, ChangeInput{Currency: "XX"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad code err = %v", err)
	}
	if _, err := svc.ChangeCurrency(f.ctx, f.brand, ChangeInput{Currency: "TRY", OpenOrders: "drop"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad option err = %v", err)
	}
	if _, err := svc.ChangeCurrency(f.ctx, uuid.New(), ChangeInput{Currency: "TRY", DryRun: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown brand err = %v", err)
	}
}
