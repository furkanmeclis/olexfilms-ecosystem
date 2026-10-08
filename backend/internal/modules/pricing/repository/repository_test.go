package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	store    *Store
	prefix   string
	seq      int
	brandID  int64
	centerID int64
	catID    int64
	today    time.Time
	tr, de   int64
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
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	f := &fixture{
		ctx: ctx, tx: tx, q: q, store: FromQueries(q),
		prefix:  fmt.Sprintf("t505-%d", time.Now().UnixNano()),
		brandID: brand.ID, centerID: center.ID,
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: f.prefix + "-cat",
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	f.catID = cat.ID
	if err := tx.QueryRow(ctx, `SELECT CURRENT_DATE`).Scan(&f.today); err != nil {
		t.Fatalf("today: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT (SELECT id FROM countries WHERE iso2 = 'TR'), (SELECT id FROM countries WHERE iso2 = 'DE')`).Scan(&f.tr, &f.de); err != nil {
		t.Fatalf("countries: %v", err)
	}
	return f
}

func (f *fixture) product(t *testing.T, name string) db.Product {
	t.Helper()
	f.seq++
	p, err := f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: f.centerID, BrandID: f.brandID, CategoryID: f.catID,
		Sku: fmt.Sprintf("%s-%d", f.prefix, f.seq), Name: f.prefix + " " + name, Images: []byte("[]"),
		UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatalf("product %s: %v", name, err)
	}
	return p
}

func (f *fixture) org(t *testing.T, typ, name string, parent int64, country int64) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, typ, f.seq), Name: f.prefix + " " + name,
		Status: "active", Type: typ, ParentID: pgtype.Int8{Int64: parent, Valid: true}, BrandID: f.brandID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		CountryID:      pgtype.Int8{Int64: country, Valid: country != 0},
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *fixture) day(offset int) pgtype.Date {
	return pgtype.Date{Time: f.today.AddDate(0, 0, offset), Valid: true}
}

func (f *fixture) publish(t *testing.T, productID, country int64, currency, price string, offset int) db.RecommendedPriceVersion {
	t.Helper()
	v, err := f.store.Publish(f.ctx, db.InsertRecommendedPriceVersionParams{
		OrganizationID: f.centerID, BrandID: f.brandID, ProductID: productID,
		CountryID: pgtype.Int8{Int64: country, Valid: country != 0}, Currency: currency,
		Price: numeric(t, price), EffectiveFrom: f.day(offset), Source: model.SourcePublish,
	}, f.day(0))
	if err != nil {
		t.Fatalf("publish %d/%d/%s %s: %v", productID, country, currency, price, err)
	}
	return v
}

// savepoint runs fn in a nested transaction and rolls it back, returning
// fn's error, so an expected failure does not abort the fixture tx.
func (f *fixture) savepoint(t *testing.T, fn func(q *db.Queries) error) error {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	defer func() { _ = sp.Rollback(f.ctx) }()
	return fn(db.New(sp))
}

func (f *fixture) recommended(t *testing.T, productID int64, currency string) string {
	t.Helper()
	var v pgtype.Numeric
	err := f.tx.QueryRow(f.ctx, `SELECT recommended_sale_price FROM product_prices WHERE product_id = $1 AND currency = $2`,
		productID, currency).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "none"
	}
	if err != nil {
		t.Fatalf("product_prices: %v", err)
	}
	return nstr(t, v)
}

func (f *fixture) version(t *testing.T, id int64) db.RecommendedPriceVersion {
	t.Helper()
	v, err := f.q.GetRecommendedPriceVersion(f.ctx, db.GetRecommendedPriceVersionParams{ID: id, BrandID: f.brandID})
	if err != nil {
		t.Fatalf("version %d: %v", id, err)
	}
	return v
}

func (f *fixture) versionsOf(t *testing.T, productID int64) []db.ListRecommendedPriceVersionsRow {
	t.Helper()
	rows, err := f.store.ListVersions(f.ctx, db.ListRecommendedPriceVersionsParams{
		BrandID: f.brandID, ProductIds: []int64{productID}, RowLimit: 100,
	}, []apiquery.SortField{{Field: "published_at"}})
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	return rows
}

func numeric(t *testing.T, raw string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(raw); err != nil {
		t.Fatalf("numeric %s: %v", raw, err)
	}
	return n
}

func nstr(t *testing.T, n pgtype.Numeric) string {
	t.Helper()
	if !n.Valid {
		return "null"
	}
	v, err := n.Float64Value()
	if err != nil {
		t.Fatalf("numeric: %v", err)
	}
	return strconv.FormatFloat(v.Float64, 'f', -1, 64)
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Versions are append-only: the only UPDATE the database accepts sets
// superseded_at once; any other column change, a second supersede and a
// direct DELETE are refused (restrict_violation 23001).
func TestVersionsAppendOnly(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "film")
	v := f.publish(t, p.ID, 0, "TRY", "100.00", 5) // scheduled, not current

	refused := map[string]string{
		"price":           `UPDATE recommended_price_versions SET price = 1 WHERE id = $1`,
		"effective_from":  `UPDATE recommended_price_versions SET effective_from = effective_from + 1 WHERE id = $1`,
		"note":            `UPDATE recommended_price_versions SET note = 'x' WHERE id = $1`,
		"price+supersede": `UPDATE recommended_price_versions SET price = 1, superseded_at = NOW() WHERE id = $1`,
		"unset":           `UPDATE recommended_price_versions SET superseded_at = NULL WHERE id = $1`,
		"delete":          `DELETE FROM recommended_price_versions WHERE id = $1`,
	}
	for name, stmt := range refused {
		err := f.savepoint(t, func(*db.Queries) error {
			_, err := f.tx.Exec(f.ctx, stmt, v.ID)
			return err
		})
		if sqlState(err) != "23001" {
			t.Fatalf("%s: err = %v, want restrict_violation", name, err)
		}
	}

	n, err := f.q.SupersedeRecommendedPriceVersion(f.ctx, db.SupersedeRecommendedPriceVersionParams{ID: v.ID, BrandID: f.brandID})
	if err != nil || n != 1 {
		t.Fatalf("supersede = %d, %v", n, err)
	}
	got := f.version(t, v.ID)
	if !got.SupersededAt.Valid || nstr(t, got.Price) != "100" || got.Note != "" {
		t.Fatalf("superseded version = %+v", got)
	}
	// Set once: a second supersede is refused by the database and skipped
	// by the query.
	err = f.savepoint(t, func(*db.Queries) error {
		_, err := f.tx.Exec(f.ctx, `UPDATE recommended_price_versions SET superseded_at = NOW() WHERE id = $1`, v.ID)
		return err
	})
	if sqlState(err) != "23001" {
		t.Fatalf("second supersede: err = %v, want restrict_violation", err)
	}
	if n, err := f.q.SupersedeRecommendedPriceVersion(f.ctx, db.SupersedeRecommendedPriceVersionParams{ID: v.ID, BrandID: f.brandID}); err != nil || n != 0 {
		t.Fatalf("second supersede query = %d, %v", n, err)
	}

	// A current version cannot be cancelled through the scheduled path.
	cur := f.publish(t, p.ID, 0, "TRY", "90.00", 0)
	if n, err := f.q.SupersedeRecommendedPriceVersion(f.ctx, db.SupersedeRecommendedPriceVersionParams{ID: cur.ID, BrandID: f.brandID}); err != nil || n != 0 {
		t.Fatalf("supersede current = %d, %v", n, err)
	}

	// The owner must be the brand center.
	dealer := f.org(t, "dealer", "dealer", f.centerID, 0)
	err = f.savepoint(t, func(q *db.Queries) error {
		_, err := q.InsertRecommendedPriceVersion(f.ctx, db.InsertRecommendedPriceVersionParams{
			OrganizationID: dealer.ID, BrandID: f.brandID, ProductID: p.ID, Currency: "TRY",
			Price: numeric(t, "1"), EffectiveFrom: f.day(9), Source: model.SourcePublish,
		})
		return err
	})
	if sqlState(err) != "23514" {
		t.Fatalf("dealer-owned version: err = %v, want check_violation", err)
	}

	// Deleting the product cascades through the history (not a direct delete).
	if _, err := f.q.DeleteProduct(f.ctx, db.DeleteProductParams{ID: p.ID, BrandID: f.brandID}); err != nil {
		t.Fatalf("delete product: %v", err)
	}
	if rows := f.versionsOf(t, p.ID); len(rows) != 0 {
		t.Fatalf("versions after product delete = %d", len(rows))
	}
}

// Publication, schedule, the effective-date input, same-day republish,
// country fallback and withdrawal keep the projection and
// product_prices.recommended_sale_price (country NULL only) in sync.
func TestPublishKeepsProjectionAndProductPricesInSync(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "ppf")

	v1 := f.publish(t, p.ID, 0, "TRY", "100.00", 0)
	if got := f.recommended(t, p.ID, "TRY"); got != "100" {
		t.Fatalf("product_prices after v1 = %s", got)
	}
	cur, err := f.q.GetApplicableRecommendedPrice(f.ctx, db.GetApplicableRecommendedPriceParams{ProductID: p.ID, Currency: "TRY"})
	if err != nil || cur.VersionID != v1.ID {
		t.Fatalf("current after v1 = %+v, %v", cur, err)
	}

	// Same-day republish supersedes v1 and becomes current.
	v2 := f.publish(t, p.ID, 0, "TRY", "110.00", 0)
	if !f.version(t, v1.ID).SupersededAt.Valid || f.version(t, v2.ID).SupersededAt.Valid {
		t.Fatal("same-day republish must supersede v1 only")
	}
	if got := f.recommended(t, p.ID, "TRY"); got != "110" {
		t.Fatalf("product_prices after v2 = %s", got)
	}

	// A future version waits; the job input lists it once its day came.
	v3 := f.publish(t, p.ID, 0, "TRY", "120.00", 3)
	if got := f.recommended(t, p.ID, "TRY"); got != "110" {
		t.Fatalf("scheduled version leaked into product_prices: %s", got)
	}
	due := func(offset int) []int64 {
		rows, err := f.q.ListDueRecommendedPriceVersions(f.ctx, db.ListDueRecommendedPriceVersionsParams{AsOf: f.day(offset), RowLimit: 1000})
		if err != nil {
			t.Fatalf("due: %v", err)
		}
		var out []int64
		for _, r := range rows {
			if r.ProductID == p.ID {
				out = append(out, r.ID)
			}
		}
		return out
	}
	if got := due(0); len(got) != 0 {
		t.Fatalf("due today = %v", got)
	}
	if got := due(3); !sameIDs(got, []int64{v3.ID}) {
		t.Fatalf("due in 3 days = %v, want [%d]", got, v3.ID)
	}
	if err := f.q.MakeRecommendedPriceCurrent(f.ctx, v3.ID); err != nil {
		t.Fatalf("make current: %v", err)
	}
	if !f.version(t, v2.ID).SupersededAt.Valid || f.recommended(t, p.ID, "TRY") != "120" {
		t.Fatal("v3 must supersede v2 and reach product_prices")
	}
	if got := due(3); len(got) != 0 {
		t.Fatalf("due after apply = %v", got)
	}
	if err := f.savepoint(t, func(q *db.Queries) error { return q.MakeRecommendedPriceCurrent(f.ctx, v2.ID) }); err == nil {
		t.Fatal("a superseded version became current")
	}

	// Country rows do not touch product_prices; lookup falls back to the
	// currency-wide price.
	vTR := f.publish(t, p.ID, f.tr, "TRY", "130.00", 0)
	if got := f.recommended(t, p.ID, "TRY"); got != "120" {
		t.Fatalf("country price leaked into product_prices: %s", got)
	}
	for country, want := range map[int64]int64{f.tr: vTR.ID, f.de: v3.ID, 0: v3.ID} {
		cur, err := f.q.GetApplicableRecommendedPrice(f.ctx, db.GetApplicableRecommendedPriceParams{
			ProductID: p.ID, Currency: "TRY", CountryID: pgtype.Int8{Int64: country, Valid: country != 0},
		})
		if err != nil || cur.VersionID != want {
			t.Fatalf("applicable for country %d = %d, %v; want %d", country, cur.VersionID, err, want)
		}
	}
	// A currency without a product_prices row gets one.
	f.publish(t, p.ID, 0, "EUR", "9.50", 0)
	if got := f.recommended(t, p.ID, "EUR"); got != "9.5" {
		t.Fatalf("EUR product_prices = %s", got)
	}

	// A projection row must mirror its version.
	err = f.savepoint(t, func(*db.Queries) error {
		_, err := f.tx.Exec(f.ctx, `UPDATE recommended_prices_current SET price = price + 1 WHERE version_id = $1`, v3.ID)
		return err
	})
	if sqlState(err) != "23514" {
		t.Fatalf("diverging projection: err = %v", err)
	}

	// Withdrawal: projection row gone, version superseded, column NULL.
	n, err := f.q.WithdrawRecommendedPrice(f.ctx, db.WithdrawRecommendedPriceParams{BrandID: f.brandID, ProductID: p.ID, Currency: "TRY"})
	if err != nil || n != 1 {
		t.Fatalf("withdraw = %d, %v", n, err)
	}
	if !f.version(t, v3.ID).SupersededAt.Valid || f.recommended(t, p.ID, "TRY") != "null" {
		t.Fatal("withdrawal must supersede v3 and clear product_prices")
	}
	if cur, err := f.q.GetApplicableRecommendedPrice(f.ctx, db.GetApplicableRecommendedPriceParams{
		ProductID: p.ID, Currency: "TRY", CountryID: pgtype.Int8{Int64: f.tr, Valid: true},
	}); err != nil || cur.VersionID != vTR.ID {
		t.Fatalf("country price after withdrawal = %d, %v", cur.VersionID, err)
	}
}

// A direct write of product_prices.recommended_sale_price (the existing
// price list API) is recorded as a 'price_list' version in force today; an
// unchanged value adds nothing; 4-decimal values are not rewritten; clearing
// or deleting the price withdraws the country-wide recommendation.
func TestPriceListWriteIsRecordedAsVersion(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "legacy")
	upsert := func(recommended, purchase string) {
		t.Helper()
		arg := db.UpsertProductPriceParams{ProductID: p.ID, BrandID: f.brandID, Currency: "TRY",
			PurchasePrice: pgtype.Text{String: purchase, Valid: true}}
		if recommended != "" {
			arg.RecommendedSalePrice = pgtype.Text{String: recommended, Valid: true}
		}
		if _, err := f.q.UpsertProductPrice(f.ctx, arg); err != nil {
			t.Fatalf("upsert price: %v", err)
		}
	}

	upsert("99.9999", "10")
	rows := f.versionsOf(t, p.ID)
	if len(rows) != 1 || rows[0].Source != model.SourcePriceList || nstr(t, rows[0].Price) != "100" ||
		!rows[0].IsCurrent || rows[0].PublishedByUserID.Valid || !rows[0].EffectiveFrom.Time.Equal(f.today) {
		t.Fatalf("price list version = %+v", rows)
	}
	if got := f.recommended(t, p.ID, "TRY"); got != "99.9999" {
		t.Fatalf("product_prices rewritten to %s", got)
	}

	upsert("99.9999", "11") // purchase change only
	if rows := f.versionsOf(t, p.ID); len(rows) != 1 {
		t.Fatalf("unchanged recommended price added a version: %d", len(rows))
	}

	// A published version still wins and is written back.
	f.publish(t, p.ID, 0, "TRY", "105.00", 0)
	if got := f.recommended(t, p.ID, "TRY"); got != "105" {
		t.Fatalf("published price not synced: %s", got)
	}

	upsert("", "11") // cleared
	rows = f.versionsOf(t, p.ID)
	for _, r := range rows {
		if r.IsCurrent || !r.SupersededAt.Valid {
			t.Fatalf("cleared price left a live version: %+v", r)
		}
	}

	upsert("80", "11")
	if n, err := f.q.DeleteProductPrice(f.ctx, db.DeleteProductPriceParams{ProductID: p.ID, BrandID: f.brandID, Currency: "TRY"}); err != nil || n != 1 {
		t.Fatalf("delete price = %d, %v", n, err)
	}
	if _, err := f.q.GetApplicableRecommendedPrice(f.ctx, db.GetApplicableRecommendedPriceParams{ProductID: p.ID, Currency: "TRY"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("current after price delete: %v", err)
	}
}

// The migration's carry-over block turns an existing
// product_prices.recommended_sale_price into a 'migration' version effective
// on the migration day, without changing product_prices.
func TestMigrationCarriesOverRecommendedPrices(t *testing.T) {
	f := newFixture(t)
	raw, err := os.ReadFile("../../../../migrations/000124_recommended_price_versions.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	src := string(raw)
	start := strings.Index(src, "-- 4. Carry over")
	end := strings.Index(src, "-- 5. Price discipline")
	if start < 0 || end < start {
		t.Fatal("carry-over block not found in migration")
	}
	carry := src[start:end]

	p := f.product(t, "carried")
	other := f.product(t, "no-price")
	// A legacy row as it exists before the migration: written with the sync
	// trigger off (ALTER TABLE is transactional, rolled back with the tx).
	if _, err := f.tx.Exec(f.ctx, `ALTER TABLE product_prices DISABLE TRIGGER trg_product_prices_sync_recommended`); err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `INSERT INTO product_prices (product_id, brand_id, currency, recommended_sale_price, purchase_price)
		VALUES ($1, $3, 'TRY', 123.4567, 10), ($1, $3, 'EUR', 12, NULL), ($2, $3, 'TRY', NULL, 5)`, p.ID, other.ID, f.brandID); err != nil {
		t.Fatalf("legacy prices: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `ALTER TABLE product_prices ENABLE TRIGGER trg_product_prices_sync_recommended`); err != nil {
		t.Fatalf("enable trigger: %v", err)
	}
	if rows := f.versionsOf(t, p.ID); len(rows) != 0 {
		t.Fatalf("versions before carry-over = %d", len(rows))
	}

	if _, err := f.tx.Exec(f.ctx, carry); err != nil {
		t.Fatalf("carry-over: %v", err)
	}
	rows := f.versionsOf(t, p.ID)
	if len(rows) != 2 {
		t.Fatalf("carried versions = %d, want 2", len(rows))
	}
	want := map[string]string{"TRY": "123.46", "EUR": "12"}
	for _, r := range rows {
		if r.Source != model.SourceMigration || !r.IsCurrent || r.CountryID.Valid ||
			!r.EffectiveFrom.Time.Equal(f.today) || r.OrganizationID != f.centerID || nstr(t, r.Price) != want[r.Currency] {
			t.Fatalf("carried version = %+v", r)
		}
	}
	if got := f.recommended(t, p.ID, "TRY"); got != "123.4567" {
		t.Fatalf("product_prices changed by the carry-over: %s", got)
	}
	if rows := f.versionsOf(t, other.ID); len(rows) != 0 {
		t.Fatalf("NULL recommended price carried over: %d", len(rows))
	}
	// Idempotent: a second run adds nothing.
	if _, err := f.tx.Exec(f.ctx, carry); err != nil {
		t.Fatalf("carry-over again: %v", err)
	}
	if rows := f.versionsOf(t, p.ID); len(rows) != 2 {
		t.Fatalf("carry-over not idempotent: %d", len(rows))
	}
}

// Version history: every sort key in both directions with the id tiebreak
// (asc: id asc, desc: id desc), the default and the list filters.
func TestVersionListSortTiebreakAndFilters(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "hist")
	// a, b share effective_from and price; published_at differs by insert
	// order inside one transaction (NOW() is equal), so it ties too.
	a := f.publish(t, p.ID, f.tr, "TRY", "50.00", 2)
	b := f.publish(t, p.ID, f.de, "TRY", "50.00", 2)
	c := f.publish(t, p.ID, 0, "EUR", "70.00", 4)
	d := f.publish(t, p.ID, 0, "TRY", "10.00", 1)

	list := func(sort []apiquery.SortField, mod func(*db.ListRecommendedPriceVersionsParams)) []int64 {
		t.Helper()
		arg := db.ListRecommendedPriceVersionsParams{BrandID: f.brandID, ProductIds: []int64{p.ID}}
		if mod != nil {
			mod(&arg)
		}
		rows, err := f.store.ListVersions(f.ctx, arg, sort)
		if err != nil {
			t.Fatalf("list %v: %v", sort, err)
		}
		out := make([]int64, len(rows))
		for i, r := range rows {
			out[i] = r.ID
			if r.TotalCount != int64(len(rows)) {
				t.Fatalf("total = %d, rows %d", r.TotalCount, len(rows))
			}
		}
		return out
	}
	cases := map[string][2][]int64{
		"effective_from": {{d.ID, a.ID, b.ID, c.ID}, {c.ID, b.ID, a.ID, d.ID}},
		"price":          {{d.ID, a.ID, b.ID, c.ID}, {c.ID, b.ID, a.ID, d.ID}},
		"published_at":   {{a.ID, b.ID, c.ID, d.ID}, {d.ID, c.ID, b.ID, a.ID}},
	}
	if len(cases) != len(VersionSort.Columns) {
		t.Fatalf("sort keys = %d, tested %d", len(VersionSort.Columns), len(cases))
	}
	for key, want := range cases {
		for i, desc := range []bool{false, true} {
			if got := list([]apiquery.SortField{{Field: key, Desc: desc}}, nil); !sameIDs(got, want[i]) {
				t.Fatalf("%s desc=%v = %v, want %v", key, desc, got, want[i])
			}
		}
	}
	if got := list(nil, nil); !sameIDs(got, cases["effective_from"][1]) {
		t.Fatalf("default (-effective_from) = %v", got)
	}
	if _, err := f.store.ListVersions(f.ctx, db.ListRecommendedPriceVersionsParams{BrandID: f.brandID}, []apiquery.SortField{{Field: "note"}}); err == nil {
		t.Fatal("unknown sort key accepted")
	}

	asc := []apiquery.SortField{{Field: "effective_from"}}
	filters := map[string]struct {
		mod  func(*db.ListRecommendedPriceVersionsParams)
		want []int64
	}{
		"country general": {func(a *db.ListRecommendedPriceVersionsParams) { a.CountryIds = []int64{model.CountryAll} }, []int64{d.ID, c.ID}},
		"country TR+DE":   {func(a *db.ListRecommendedPriceVersionsParams) { a.CountryIds = []int64{f.tr, f.de} }, []int64{a.ID, b.ID}},
		"currency EUR":    {func(a *db.ListRecommendedPriceVersionsParams) { a.Currencies = []string{"EUR"} }, []int64{c.ID}},
		"currency multi":  {func(a *db.ListRecommendedPriceVersionsParams) { a.Currencies = []string{"EUR", "TRY"} }, []int64{d.ID, a.ID, b.ID, c.ID}},
		"effective range": {func(a *db.ListRecommendedPriceVersionsParams) {
			a.EffectiveFromFrom, a.EffectiveFromBefore = f.day(2), f.day(4)
		}, []int64{a.ID, b.ID}},
		"source": {func(a *db.ListRecommendedPriceVersionsParams) { a.Sources = []string{model.SourceMigration} }, nil},
		"q sku": {func(a *db.ListRecommendedPriceVersionsParams) {
			a.Q = pgtype.Text{String: " " + p.Sku + " ", Valid: true}
		}, []int64{d.ID, a.ID, b.ID, c.ID}},
	}
	for name, fc := range filters {
		if got := list(asc, fc.mod); !sameIDs(got, fc.want) {
			t.Fatalf("filter %s = %v, want %v", name, got, fc.want)
		}
	}
}

// Current list: sort keys with the id tiebreak.
func TestCurrentListSortTiebreak(t *testing.T) {
	f := newFixture(t)
	p1 := f.product(t, "b-same")
	p2 := f.product(t, "a-other")
	x := f.publish(t, p1.ID, 0, "TRY", "20.00", 0)
	y := f.publish(t, p1.ID, f.tr, "TRY", "20.00", 0)
	z := f.publish(t, p2.ID, 0, "TRY", "30.00", -1)
	list := func(sort []apiquery.SortField) []int64 {
		t.Helper()
		rows, err := f.store.ListCurrent(f.ctx, db.ListRecommendedPricesCurrentParams{
			BrandID: f.brandID, ProductIds: []int64{p1.ID, p2.ID},
		}, sort)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		out := make([]int64, len(rows))
		for i, r := range rows {
			out[i] = r.VersionID
		}
		return out
	}
	cases := map[string][2][]int64{
		"product_name":   {{z.ID, x.ID, y.ID}, {y.ID, x.ID, z.ID}},
		"price":          {{x.ID, y.ID, z.ID}, {z.ID, y.ID, x.ID}},
		"effective_from": {{z.ID, x.ID, y.ID}, {y.ID, x.ID, z.ID}},
	}
	if len(cases) != len(CurrentSort.Columns) {
		t.Fatalf("sort keys = %d, tested %d", len(CurrentSort.Columns), len(cases))
	}
	for key, want := range cases {
		for i, desc := range []bool{false, true} {
			if got := list([]apiquery.SortField{{Field: key, Desc: desc}}); !sameIDs(got, want[i]) {
				t.Fatalf("%s desc=%v = %v, want %v", key, desc, got, want[i])
			}
		}
	}
	if got := list(nil); !sameIDs(got, cases["product_name"][0]) {
		t.Fatalf("default (product_name) = %v", got)
	}
}

// Deviation list: sort keys (deviation NULLS LAST both ways) with the id
// tiebreak, and the deviation range, over_threshold, country, distributor,
// scope and q filters.
func TestDisciplineListSortTiebreakAndFilters(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "disc")
	dist := f.org(t, "distributor", "dist", f.centerID, f.tr)
	d1 := f.org(t, "dealer", "same", dist.ID, f.tr)
	d2 := f.org(t, "dealer", "same", dist.ID, f.tr)
	d3 := f.org(t, "dealer", "alpha", f.centerID, f.de)
	d4 := f.org(t, "dealer", "zulu", f.centerID, f.de)
	v := f.publish(t, p.ID, 0, "TRY", "100.00", 0)
	snap := func(org db.Organization, list, dev string) int64 {
		t.Helper()
		arg := db.UpsertPriceDisciplineSnapshotParams{
			SnapshotDate: f.day(0), OrganizationID: org.ID, BrandID: f.brandID, ProductID: p.ID,
			CountryID: org.CountryID, Currency: "TRY", RecommendedVersionID: pgtype.Int8{Int64: v.ID, Valid: true},
			RecommendedPrice: numeric(t, "100"), ListPrice: numeric(t, list), SalesQuantity: numeric(t, "0"),
		}
		if dev != "" {
			arg.DeviationPct = numeric(t, dev)
		}
		s, err := f.q.UpsertPriceDisciplineSnapshot(f.ctx, arg)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		return s.ID
	}
	s1 := snap(d1, "80", "-20")
	s2 := snap(d2, "80", "-20")
	s3 := snap(d3, "110", "10")
	s4 := snap(d4, "0", "") // deviation unknown
	sd := snap(dist, "116", "16")
	// The daily upsert replaces the row of the day.
	if again := snap(d3, "110", "10"); again != s3 {
		t.Fatalf("upsert created a second row: %d vs %d", again, s3)
	}
	all := []int64{dist.ID, d1.ID, d2.ID, d3.ID, d4.ID}

	list := func(sort []apiquery.SortField, mod func(*db.ListPriceDisciplineSnapshotsParams)) []int64 {
		t.Helper()
		arg := db.ListPriceDisciplineSnapshotsParams{
			BrandID: f.brandID, SnapshotDate: f.day(0), OrgIds: all, ThresholdPct: numeric(t, "15"),
		}
		if mod != nil {
			mod(&arg)
		}
		rows, err := f.store.ListDiscipline(f.ctx, arg, sort)
		if err != nil {
			t.Fatalf("list %v: %v", sort, err)
		}
		out := make([]int64, len(rows))
		for i, r := range rows {
			out[i] = r.ID
		}
		return out
	}
	// Names: "dist" < "same"(d1,d2) < "alpha"? Names carry the prefix, so
	// order is alpha < dist < same < same < zulu.
	cases := map[string][2][]int64{
		"deviation_pct": {{s1, s2, s3, sd, s4}, {sd, s3, s2, s1, s4}},
		"org_name":      {{s3, sd, s1, s2, s4}, {s4, s2, s1, sd, s3}},
		"product_name":  {{s1, s2, s3, s4, sd}, {sd, s4, s3, s2, s1}},
	}
	if len(cases) != len(DisciplineSort.Columns) {
		t.Fatalf("sort keys = %d, tested %d", len(DisciplineSort.Columns), len(cases))
	}
	for key, want := range cases {
		for i, desc := range []bool{false, true} {
			if got := list([]apiquery.SortField{{Field: key, Desc: desc}}, nil); !sameIDs(got, want[i]) {
				t.Fatalf("%s desc=%v = %v, want %v", key, desc, got, want[i])
			}
		}
	}
	if got := list(nil, nil); !sameIDs(got, cases["deviation_pct"][1]) {
		t.Fatalf("default (-deviation_pct) = %v", got)
	}
	if _, err := f.store.ListDiscipline(f.ctx, db.ListPriceDisciplineSnapshotsParams{BrandID: f.brandID, SnapshotDate: f.day(0)},
		[]apiquery.SortField{{Field: "list_price"}}); err == nil {
		t.Fatal("unknown sort key accepted")
	}

	asc := []apiquery.SortField{{Field: "deviation_pct"}}
	yes, no := pgtype.Bool{Bool: true, Valid: true}, pgtype.Bool{Bool: false, Valid: true}
	filters := map[string]struct {
		mod  func(*db.ListPriceDisciplineSnapshotsParams)
		want []int64
	}{
		"over threshold":  {func(a *db.ListPriceDisciplineSnapshotsParams) { a.OverThreshold = yes }, []int64{s1, s2, sd}},
		"under threshold": {func(a *db.ListPriceDisciplineSnapshotsParams) { a.OverThreshold = no }, []int64{s3, s4}},
		"deviation range": {func(a *db.ListPriceDisciplineSnapshotsParams) {
			a.DeviationPctMin, a.DeviationPctMax = numeric(t, "-20"), numeric(t, "10")
		}, []int64{s1, s2, s3}},
		"country DE":  {func(a *db.ListPriceDisciplineSnapshotsParams) { a.CountryIds = []int64{f.de} }, []int64{s3, s4}},
		"distributor": {func(a *db.ListPriceDisciplineSnapshotsParams) { a.DistributorIds = []int64{dist.ID} }, []int64{s1, s2, sd}},
		"scope":       {func(a *db.ListPriceDisciplineSnapshotsParams) { a.OrgIds = []int64{d1.ID, d4.ID} }, []int64{s1, s4}},
		"q":           {func(a *db.ListPriceDisciplineSnapshotsParams) { a.Q = pgtype.Text{String: "zulu", Valid: true} }, []int64{s4}},
		"other day":   {func(a *db.ListPriceDisciplineSnapshotsParams) { a.SnapshotDate = f.day(-1) }, []int64{}},
	}
	for name, fc := range filters {
		if got := list(asc, fc.mod); !sameIDs(got, fc.want) {
			t.Fatalf("filter %s = %v, want %v", name, got, fc.want)
		}
	}

	if latest, err := f.q.GetLatestPriceDisciplineSnapshotDate(f.ctx, f.brandID); err != nil || !latest.Time.Equal(f.today) {
		t.Fatalf("latest snapshot date = %v, %v", latest, err)
	}

	// Only dealers and distributors sell to end customers.
	err := f.savepoint(t, func(q *db.Queries) error {
		_, err := q.UpsertPriceDisciplineSnapshot(f.ctx, db.UpsertPriceDisciplineSnapshotParams{
			SnapshotDate: f.day(0), OrganizationID: f.centerID, BrandID: f.brandID, ProductID: p.ID,
			Currency: "TRY", RecommendedPrice: numeric(t, "100"), ListPrice: numeric(t, "100"), SalesQuantity: numeric(t, "0"),
		})
		return err
	})
	if sqlState(err) != "23514" {
		t.Fatalf("center snapshot: err = %v, want check_violation", err)
	}
	// Deviation is undefined against a zero recommendation.
	err = f.savepoint(t, func(q *db.Queries) error {
		_, err := q.UpsertPriceDisciplineSnapshot(f.ctx, db.UpsertPriceDisciplineSnapshotParams{
			SnapshotDate: f.day(-2), OrganizationID: d1.ID, BrandID: f.brandID, ProductID: p.ID,
			Currency: "TRY", RecommendedPrice: numeric(t, "0"), ListPrice: numeric(t, "10"),
			DeviationPct: numeric(t, "5"), SalesQuantity: numeric(t, "0"),
		})
		return err
	})
	if sqlState(err) != "23514" {
		t.Fatalf("deviation against zero: err = %v, want check_violation", err)
	}
}
