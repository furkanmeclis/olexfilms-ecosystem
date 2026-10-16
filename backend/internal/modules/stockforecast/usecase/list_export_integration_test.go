package usecase

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-755: the screen's widget (critical count through the list), the
// threshold save and the list export read the dealer's own forecasts.
func TestService_DealerListWidgetThresholdsAndExport(t *testing.T) {
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
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dealer, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t755-dealer-" + suffix, Name: "t755 dealer", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
		BrandID: brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("dealer: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t755-cat-" + suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product := func(sku string) db.Product {
		t.Helper()
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: center.ID, BrandID: brand.ID, CategoryID: cat.ID,
			Sku: sku + "-" + suffix, Name: "t755 " + sku, Images: []byte("[]"),
			UnitType: "piece", UsesFixedBarcode: false, Active: true,
		})
		if err != nil {
			t.Fatalf("product: %v", err)
		}
		return p
	}
	critical, healthy := product("CRIT"), product("OK")
	seed := func(p db.Product, status string, days float64, suggested int32) {
		t.Helper()
		if _, err := q.UpsertStockForecastSnapshot(ctx, db.UpsertStockForecastSnapshotParams{
			OrganizationID: dealer.ID, BrandID: brand.ID, ProductID: p.ID, ComputedOn: date(time.Now()),
			IsLatest: true, OnHandQty: 3, OnHandMeters: num(0), AvgDaily30: num4(1), AvgDaily90: num4(1),
			SeasonalityFactor: num3(1), DaysLeft: num2(days), DataDays: 120, Status: status,
			SuggestedQty: pgtype.Int4{Int32: suggested, Valid: suggested > 0},
		}); err != nil {
			t.Fatalf("seed %s: %v", status, err)
		}
	}
	seed(critical, StatusCritical, 3, 27)
	seed(healthy, StatusOK, 90, 0)

	svc := New(pool, q, outbox.NewStore(pool, q), nil, fakeSettings{min: 90, warn: 14, crit: 7, cover: 30}, nil)
	c := Caller{
		Principal: authctx.Principal{PermissionScopes: map[string]rbac.Scope{
			rbac.PermStockForecastRead: rbac.ScopeManaged, rbac.PermStockForecastManage: rbac.ScopeManaged,
		}},
		Org: orgctx.Scope{InternalID: dealer.ID, UUID: dealer.Uuid, OrgType: dealer.Type, BrandID: brand.ID},
	}

	// Widget: GET /v1/stock-forecasts?status=critical&limit=1 → total.
	f, err := ParseListFilter(map[string][]string{"status": {"critical"}, "limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	items, total, err := svc.List(ctx, c, f)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Product.UUID != critical.Uuid {
		t.Fatalf("critical list total=%d items=%+v", total, items)
	}

	// Threshold save: PUT /v1/stock-forecasts/thresholds with one product override.
	cfg, err := svc.UpsertThresholds(ctx, c, ThresholdInput{
		WarningDays: 14, CoverDays: 30,
		Products: []ProductThresholdInput{{ProductUUID: critical.Uuid, WarningDays: 10, CoverDays: 45}},
	})
	if err != nil {
		t.Fatalf("thresholds: %v", err)
	}
	productOverrides := func(cfg ThresholdConfig) []ThresholdView {
		var out []ThresholdView
		for _, o := range cfg.Overrides {
			if o.Product != nil { // the organization default is listed without a product
				out = append(out, o)
			}
		}
		return out
	}
	if po := productOverrides(cfg); cfg.WarningDays != 14 || cfg.CoverDays != 30 || len(po) != 1 ||
		po[0].Product.UUID != critical.Uuid || po[0].WarningDays != 10 || po[0].CoverDays != 45 {
		t.Fatalf("thresholds = %+v", cfg)
	}
	meta, err := svc.Meta(ctx, c)
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if got := meta["thresholds"].(ThresholdConfig); len(productOverrides(got)) != 1 {
		t.Fatalf("meta thresholds = %+v", got)
	}

	// Export: the list parameters are validated at request time …
	if _, err := ListExportQuery(map[string]string{"status": "bogus"}); err == nil {
		t.Fatal("bad status must be rejected at request time")
	}
	query, err := ListExportQuery(map[string]string{"status": "critical,ok", "sort": "-days_left", "ignored": "x"})
	if err != nil {
		t.Fatalf("export query: %v", err)
	}
	if _, ok := query["ignored"]; ok {
		t.Fatalf("unknown keys must not reach the job: %v", query)
	}
	// … and the worker reads the job organization's list.
	query[ioengine.QueryOrganizationID] = strconv.FormatInt(dealer.ID, 10)
	ds, err := NewListExportAdapter(svc).Export(ctx, query, i18n.LocaleEN)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if ds.Resource != ResourceListExport || len(ds.Rows) != 2 {
		t.Fatalf("dataset resource=%s rows=%d", ds.Resource, len(ds.Rows))
	}
	if ds.Rows[0]["product_sku"] != healthy.Sku || ds.Rows[0]["status"] != StatusOK || ds.Rows[0]["suggested_qty"] != "" {
		t.Fatalf("first row (sorted by -days_left) = %+v", ds.Rows[0])
	}
	if ds.Rows[1]["product_sku"] != critical.Sku || ds.Rows[1]["suggested_qty"] != int32(27) || ds.Rows[1]["category_name"] != cat.Name {
		t.Fatalf("second row = %+v", ds.Rows[1])
	}
	if _, err := NewListExportAdapter(svc).Export(ctx, ioengine.ExportQuery{}, i18n.LocaleEN); err == nil {
		t.Fatal("export without organization must fail")
	}
}
