package usecase

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricingrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-506: /v1/dealer-prices and its catalog carry the recommended block
// (the dealer's country, else currency-wide) and the deviation only for a
// caller holding pricing.recommended.read.
func TestDealerPricesRecommendedBlock(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("t506da-%d", time.Now().UnixNano())
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: prefix, AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: center.ID, BrandID: brand.ID, CategoryID: cat.ID, Sku: prefix, Name: prefix,
		Images: []byte("[]"), UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var de int64
	if err := tx.QueryRow(ctx, `SELECT id FROM countries WHERE iso2 = 'DE'`).Scan(&de); err != nil {
		t.Fatal(err)
	}
	dealer, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: prefix + "-dealer", Name: prefix, Status: "active", Type: OrgDealer,
		ParentID: pgtype.Int8{Int64: center.ID, Valid: true}, BrandID: brand.ID, Currency: "EUR", Locale: "de",
		Timezone: "Europe/Berlin", Settings: []byte("{}"), CountryID: pgtype.Int8{Int64: de, Valid: true},
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	today := pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -1), Valid: true}
	store := pricingrepo.FromQueries(q)
	for _, v := range []struct {
		country int64
		price   string
	}{{0, "100.00"}, {de, "80.00"}} {
		var n pgtype.Numeric
		_ = n.Scan(v.price)
		if _, err := store.Publish(ctx, db.InsertRecommendedPriceVersionParams{
			OrganizationID: center.ID, BrandID: brand.ID, ProductID: p.ID,
			CountryID: pgtype.Int8{Int64: v.country, Valid: v.country != 0}, Currency: "EUR", Price: n,
			EffectiveFrom: today, Source: "publish",
		}, today); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(tx, q, nil, nil, nil)
	caller := func(read bool) Caller {
		return Caller{
			Org:             orgctx.Scope{InternalID: dealer.ID, BrandID: brand.ID, OrgType: OrgDealer},
			Filter:          scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{dealer.ID}},
			RecommendedRead: read,
		}
	}
	v, err := svc.SetDealerPrice(ctx, caller(true), DealerPriceInput{ProductUUID: p.Uuid, SalePrice: "92"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Recommended == nil || v.Recommended.Price != "80.00" || v.Recommended.CountryISO2 != "DE" || v.DeviationPct == nil || *v.DeviationPct != "15.00" {
		t.Fatalf("set view = %+v / %v", v.Recommended, v.DeviationPct)
	}
	list, err := svc.ListDealerPrices(ctx, caller(true))
	if err != nil || len(list) != 1 || list[0].Recommended == nil || *list[0].DeviationPct != "15.00" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	cat2, err := svc.ListPriceCatalog(ctx, caller(true), PriceCatalogFilter{ListPage: ListPage{Limit: 10, Q: prefix}})
	if err != nil || len(cat2.Items) != 1 || cat2.Items[0].Recommended == nil || *cat2.Items[0].DeviationPct != "15.00" {
		t.Fatalf("catalog = %+v, %v", cat2.Items, err)
	}
	noPerm, err := svc.ListDealerPrices(ctx, caller(false))
	if err != nil || len(noPerm) != 1 || noPerm[0].Recommended != nil || noPerm[0].DeviationPct != nil {
		t.Fatalf("without pricing.recommended.read = %+v, %v", noPerm, err)
	}
	cat3, err := svc.ListPriceCatalog(ctx, caller(false), PriceCatalogFilter{ListPage: ListPage{Limit: 10, Q: prefix}})
	if err != nil || len(cat3.Items) != 1 || cat3.Items[0].Recommended != nil {
		t.Fatalf("catalog without permission = %+v, %v", cat3.Items, err)
	}
}
