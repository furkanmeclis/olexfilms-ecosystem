package migrator

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/legacyfixture"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// fixtureLogo is a PNG signature plus filler: enough for content sniffing.
var fixtureLogo = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)

var (
	fixtureCarBrands = []string{"sentetik marka a", "sentetik marka b"}
	fixtureSKUs      = []string{"SYN-PPF-GLOSS", "SYN-PPF-MATTE", "SYN-WIN-01", "SYN-OLD-01"}
	fixtureCats      = []string{"Sentetik PPF", "Sentetik Cam Filmi"}
)

// catalogCounts are the rows the TEC-256 steps own, for the rerun check.
type catalogCounts struct{ Brands, Models, Categories, Products, Maps int64 }

// TEC-256 acceptance over the legacy fixture: brands and models match the
// fixture without duplicating a seeded brand, the logo lands in the (fake)
// object store, the warehouse product maps onto the hub product as one record,
// Glorian is skipped and a second run creates nothing.
func TestOlexVehicleAndProductCatalog(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	legacyfixture.LoadAndHold(t, pool)

	var taken int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM car_brands WHERE lower(name) = ANY($1))
		     + (SELECT COUNT(*) FROM products p JOIN brands b ON b.id = p.brand_id AND b.slug = 'olex' WHERE p.sku = ANY($2))
		     + (SELECT COUNT(*) FROM product_categories c JOIN brands b ON b.id = c.brand_id AND b.slug = 'olex' WHERE c.name = ANY($3))`,
		fixtureCarBrands, fixtureSKUs, fixtureCats).Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture catalog rows already exist in the test database", taken)
	}

	hubSys := "t" + uuid.NewString()[:8]
	whSys := hubSys + "w"

	// A brand the new app already has (seed / created by hand), spelled in
	// another case: the legacy brand must link to it, not duplicate it.
	var seedID int64
	if err := pool.QueryRow(ctx, `INSERT INTO car_brands (name) VALUES ('SENTETIK MARKA B') RETURNING id`).Scan(&seedID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		exec := func(sql string, args ...any) {
			if _, err := pool.Exec(ctx, sql, args...); err != nil {
				t.Logf("cleanup %q: %v", sql, err)
			}
		}
		mapped := `SELECT target_uuid FROM migration_map WHERE source_system = $1 AND target_table = `
		exec(`DELETE FROM products WHERE uuid IN (`+mapped+`'products')`, hubSys)
		exec(`DELETE FROM product_categories WHERE uuid IN (`+mapped+`'product_categories')`, hubSys)
		exec(`DELETE FROM car_models WHERE uuid IN (`+mapped+`'car_models')`, hubSys)
		exec(`DELETE FROM car_brands WHERE uuid IN (`+mapped+`'car_brands')`, hubSys)
		exec(`DELETE FROM car_brands WHERE id = $1`, seedID)
		exec(`DELETE FROM migration_map WHERE source_system = ANY($1)`, []string{hubSys, whSys})
	})

	store := storage.NewMemory()
	runner := &Runner{
		Pool: pool,
		Profiles: map[string]Profile{
			"olexfx": {Name: "olexfx", Enabled: true, Sources: []string{SourceHub, SourceWH}, Steps: func() []Step {
				return []Step{VehicleCatalogStep{System: hubSys}, CatalogStep{System: hubSys, WHSystem: whSys}}
			}},
		},
		Open: func(_ context.Context, name string) (source.LegacySource, error) {
			return source.NewPostgres(name, pool, source.FixtureSchemas[name])
		},
		Storage:     store,
		LegacyFiles: fstest.MapFS{"app/public/car-brands/syn-a.png": {Data: fixtureLogo}},
	}
	run := func() RunReport {
		t.Helper()
		rep, err := runner.Run(ctx, Options{Profile: "olexfx"})
		if rep.RunID != 0 {
			cleanupRun(t, pool, rep.RunID)
		}
		if err != nil {
			t.Fatalf("run: %v (report %+v)", err, rep)
		}
		return rep
	}
	tableCounts := func() catalogCounts {
		t.Helper()
		var c catalogCounts
		if err := pool.QueryRow(ctx, `
			SELECT (SELECT COUNT(*) FROM car_brands WHERE lower(name) = ANY($2)),
			       (SELECT COUNT(*) FROM car_models m JOIN migration_map mm ON mm.target_uuid = m.uuid
			          WHERE mm.source_system = $1 AND mm.target_table = 'car_models'),
			       (SELECT COUNT(*) FROM product_categories c JOIN brands b ON b.id = c.brand_id AND b.slug = 'olex'
			          WHERE c.name = ANY($4)),
			       (SELECT COUNT(*) FROM products p JOIN brands b ON b.id = p.brand_id AND b.slug = 'olex'
			          WHERE p.sku = ANY($3)),
			       (SELECT COUNT(*) FROM migration_map WHERE source_system IN ($1, $1 || 'w'))`,
			hubSys, fixtureCarBrands, fixtureSKUs, fixtureCats).
			Scan(&c.Brands, &c.Models, &c.Categories, &c.Products, &c.Maps); err != nil {
			t.Fatal(err)
		}
		return c
	}

	first := run()
	vc := stepCounts(t, first, "vehicle_catalog")
	for k, want := range map[string]int64{
		"brands_read": 2, "brands_created": 1, "brands_linked_existing": 1,
		"models_read": 3, "models_created": 3, "logos_uploaded": 1, "logos_set": 1, "logo_missing": 0,
	} {
		if vc[k] != want {
			t.Errorf("vehicle_catalog.%s = %d, want %d (%v)", k, vc[k], want, vc)
		}
	}
	cc := stepCounts(t, first, "catalog")
	for k, want := range map[string]int64{
		"categories_read": 2, "categories_created": 2, "products_read": 4, "products_created": 4,
		"wh_products_read": 2, "wh_matched_sku": 1, "wh_glorian_skipped": 1, "wh_unmatched": 0,
	} {
		if cc[k] != want {
			t.Errorf("catalog.%s = %d, want %d (%v)", k, cc[k], want, cc)
		}
	}

	// No duplicate of the seeded brand; its models hang under it.
	before := tableCounts()
	if before != (catalogCounts{Brands: 2, Models: 3, Categories: 2, Products: 4, Maps: 2 + 3 + 2 + 4 + 1}) {
		t.Errorf("rows after first run = %+v", before)
	}
	var b1Brand int64
	if err := pool.QueryRow(ctx, `SELECT m.car_brand_id FROM car_models m JOIN migration_map mm ON mm.target_uuid = m.uuid
		WHERE mm.source_system = $1 AND mm.source_table = 'car_models' AND mm.source_id = '3'`, hubSys).Scan(&b1Brand); err != nil {
		t.Fatal(err)
	}
	if b1Brand != seedID {
		t.Errorf("Model B1 brand = %d, want the seeded brand %d", b1Brand, seedID)
	}
	var extID, logoKey, brandUUID string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(external_id, ''), COALESCE(logo_object_key, ''), uuid::text
		FROM car_brands WHERE lower(name) = 'sentetik marka a'`).Scan(&extID, &logoKey, &brandUUID); err != nil {
		t.Fatal(err)
	}
	if extID != "syn-brand-a" {
		t.Errorf("brand A external_id = %q", extID)
	}

	// The logo is in the object store under the brand's key.
	if !strings.HasPrefix(logoKey, "vehicle-brands/"+brandUUID+"/logo-") || !strings.HasSuffix(logoKey, ".png") {
		t.Fatalf("brand A logo key = %q", logoKey)
	}
	body, _, err := store.Download(ctx, logoKey)
	if err != nil {
		t.Fatalf("logo not uploaded: %v", err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if !bytes.Equal(got, fixtureLogo) {
		t.Error("uploaded logo bytes differ from the legacy file")
	}
	if ct, _, _ := store.GetMeta(logoKey); ct != "image/png" {
		t.Errorf("logo content type = %q", ct)
	}

	// The warehouse product and the hub product are one catalog record of the
	// olex brand, owned by the center.
	var hubTarget, whTarget uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT target_uuid FROM migration_map WHERE source_system = $1 AND source_table = 'products' AND source_id = '1'`,
		hubSys).Scan(&hubTarget); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT target_uuid FROM migration_map WHERE source_system = $1 AND source_table = 'products'`,
		whSys).Scan(&whTarget); err != nil {
		t.Fatalf("warehouse product not mapped: %v", err)
	}
	if hubTarget != whTarget {
		t.Errorf("warehouse product maps to %s, hub product to %s", whTarget, hubTarget)
	}
	var orgType string
	var sameSKU int
	var oldActive bool
	if err := pool.QueryRow(ctx, `
		SELECT o.type,
		       (SELECT COUNT(*) FROM products x WHERE x.brand_id = p.brand_id AND x.sku = 'SYN-PPF-GLOSS'),
		       (SELECT active FROM products x WHERE x.brand_id = p.brand_id AND x.sku = 'SYN-OLD-01')
		FROM products p JOIN organizations o ON o.id = p.organization_id
		WHERE p.uuid = $1`, hubTarget).Scan(&orgType, &sameSKU, &oldActive); err != nil {
		t.Fatal(err)
	}
	if orgType != "center" || sameSKU != 1 || oldActive {
		t.Errorf("product owner = %s, SYN-PPF-GLOSS rows = %d, inactive legacy product active = %v", orgType, sameSKU, oldActive)
	}

	// Second run: nothing new.
	second := run()
	for _, step := range []string{"vehicle_catalog", "catalog"} {
		c := stepCounts(t, second, step)
		for k, v := range c {
			if v != 0 && (strings.HasSuffix(k, "_created") || strings.HasSuffix(k, "_updated") ||
				strings.HasSuffix(k, "_linked_existing") || strings.HasPrefix(k, "wh_matched") || k == "logos_uploaded" || k == "logos_set") {
				t.Errorf("rerun %s.%s = %d, want 0 (%v)", step, k, v, c)
			}
		}
	}
	if c := stepCounts(t, second, "catalog"); c["wh_unchanged"] != 1 || c["wh_glorian_skipped"] != 1 {
		t.Errorf("rerun catalog counts = %v", c)
	}
	if after := tableCounts(); after != before {
		t.Errorf("rerun changed rows: %+v -> %+v", before, after)
	}
	if objs, _ := store.List(ctx, storage.ListObjectsInput{Prefix: "vehicle-brands/"}); len(objs.Objects) != 1 {
		t.Errorf("objects after rerun = %+v", objs.Objects)
	}
}
