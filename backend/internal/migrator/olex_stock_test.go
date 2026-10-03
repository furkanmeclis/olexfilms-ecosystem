package migrator

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// stockCounts are the rows the TEC-257 steps own, for the rerun check.
type stockCounts struct{ Warehouses, Rooms, Locations, Units, States, Bins, Maps int64 }

// TEC-257 acceptance over the legacy fixture: one unit per distinct Olex
// barcode (the barcode in both systems is one unit), dealer / center /
// location / trash ownership as expected, the location tree and bin stock
// mapped, Glorian skipped, and a second run writes nothing.
func TestOlexWarehousesAndUnits(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool
	hubSys := e.system
	whSys := hubSys + "w"

	var taken int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM units u JOIN brands b ON b.id = u.brand_id AND b.slug = 'olex'
		         WHERE u.barcode LIKE 'SYN-%')
		     + (SELECT COUNT(*) FROM products p JOIN brands b ON b.id = p.brand_id AND b.slug = 'olex' WHERE p.sku = ANY($1))`,
		fixtureSKUs).Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture units / products already exist in the test database", taken)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		exec := func(sql string, args ...any) {
			if _, err := pool.Exec(ctx, sql, args...); err != nil {
				t.Logf("cleanup %q: %v", sql, err)
			}
		}
		systems := []string{hubSys, whSys}
		mapped := func(table string) string {
			return `SELECT target_uuid FROM migration_map WHERE source_system = ANY($1) AND target_table = '` + table + `'`
		}
		units := `SELECT id FROM units WHERE uuid IN (` + mapped("units") + `)`
		locs := `SELECT id FROM warehouse_locations WHERE uuid IN (` + mapped("warehouse_locations") + `)`
		exec(`DELETE FROM unit_current_state WHERE unit_id IN (`+units+`)`, systems)
		exec(`DELETE FROM fixed_barcode_holdings WHERE unit_id IN (`+units+`)`, systems)
		exec(`DELETE FROM bin_product_stocks WHERE location_id IN (`+locs+`)`, systems)
		exec(`DELETE FROM units WHERE uuid IN (`+mapped("units")+`)`, systems)
		for _, typ := range []string{"bin", "shelf", "aisle"} {
			exec(`DELETE FROM warehouse_locations WHERE type = $2 AND uuid IN (`+mapped("warehouse_locations")+`)`, systems, typ)
		}
		exec(`DELETE FROM rooms WHERE uuid IN (`+mapped("rooms")+`)`, systems)
		exec(`DELETE FROM warehouses WHERE uuid IN (`+mapped("warehouses")+`)`, systems)
		exec(`DELETE FROM products WHERE uuid IN (`+mapped("products")+`)`, systems)
		exec(`DELETE FROM product_categories WHERE uuid IN (`+mapped("product_categories")+`)`, systems)
		exec(`DELETE FROM migration_map WHERE source_system = $1`, whSys)
	})

	e.runner.Profiles["olexfx-stock"] = Profile{
		Name: "olexfx-stock", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step {
			return []Step{
				OrganizationsStep{System: hubSys},
				CatalogStep{System: hubSys, WHSystem: whSys},
				WarehousesStep{WHSystem: whSys},
				UnitsStep{System: hubSys, WHSystem: whSys},
			}
		},
	}

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	tableCounts := func() stockCounts {
		t.Helper()
		var c stockCounts
		if err := pool.QueryRow(ctx, `
			WITH m AS (SELECT DISTINCT target_table, target_uuid FROM migration_map WHERE source_system IN ($1, $2))
			SELECT (SELECT COUNT(*) FROM warehouses x JOIN m ON m.target_uuid = x.uuid AND m.target_table = 'warehouses'),
			       (SELECT COUNT(*) FROM rooms x JOIN m ON m.target_uuid = x.uuid AND m.target_table = 'rooms'),
			       (SELECT COUNT(*) FROM warehouse_locations x JOIN m ON m.target_uuid = x.uuid AND m.target_table = 'warehouse_locations'),
			       (SELECT COUNT(*) FROM units x JOIN m ON m.target_uuid = x.uuid AND m.target_table = 'units'),
			       (SELECT COUNT(*) FROM unit_current_state s JOIN units x ON x.id = s.unit_id
			          JOIN m ON m.target_uuid = x.uuid AND m.target_table = 'units'),
			       (SELECT COUNT(*) FROM bin_product_stocks s JOIN warehouse_locations x ON x.id = s.location_id
			          JOIN m ON m.target_uuid = x.uuid AND m.target_table = 'warehouse_locations'),
			       (SELECT COUNT(*) FROM migration_map WHERE source_system IN ($1, $2))`, hubSys, whSys).
			Scan(&c.Warehouses, &c.Rooms, &c.Locations, &c.Units, &c.States, &c.Bins, &c.Maps); err != nil {
			t.Fatal(err)
		}
		return c
	}

	first := e.run("olexfx-stock", Options{})
	wc := stepCounts(t, first, "warehouses")
	for k, want := range map[string]int64{
		"warehouses_read": 2, "warehouses_created": 2, "rooms_read": 2, "rooms_created": 2,
		"locations_read": 5, "locations_created": 5, "locations_aisle": 2, "locations_shelf": 2, "locations_bin": 1,
		"location_bin_as_shelf:00000003-0000-4000-8000-000000000005": 1,
	} {
		if wc[k] != want {
			t.Errorf("warehouses.%s = %d, want %d (%v)", k, wc[k], want, wc)
		}
	}
	uc := stepCounts(t, first, "units")
	for k, want := range map[string]int64{
		"hub_read": 8, "wh_read": 4, "wh_glorian_skipped": 2, "barcodes_merged": 1, "owner_conflicts": 1,
		"units_created": 9, "hub_reserved_as_available": 1,
		"ownership_center": 1, "ownership_dealer": 2, "ownership_location": 2, "ownership_trash": 1,
		"ownership_pending:service": 3,
		"bin_stocks_read":           3, "bin_stocks_created": 1, "bin_stock_glorian_skipped": 2,
	} {
		if uc[k] != want {
			t.Errorf("units.%s = %d, want %d (%v)", k, uc[k], want, uc)
		}
	}
	for k := range uc {
		if strings.HasPrefix(k, "bin_stock_mismatch") || strings.HasPrefix(k, "hub_skipped") || strings.HasPrefix(k, "wh_skipped") {
			t.Errorf("unexpected report %s (%v)", k, uc)
		}
	}

	// units = distinct Olex barcodes of both legacy systems.
	distinct := scalar(`
		SELECT COUNT(*) FROM (
			SELECT barcode FROM legacy_hub.stock_items
			UNION
			SELECT pb.code FROM legacy_wh.product_barcodes pb
			JOIN legacy_wh.products p ON p.id = pb.product_id
			JOIN legacy_wh.brands b ON b.id = p.brand_id
			WHERE b.name NOT ILIKE '%glorian%' AND pb.deleted_at IS NULL
		) x`)
	before := tableCounts()
	if before.Units != distinct || distinct != 9 {
		t.Errorf("units = %d, distinct legacy barcodes = %d, want 9", before.Units, distinct)
	}
	if before != (stockCounts{Warehouses: 2, Rooms: 2, Locations: 5, Units: 9, States: 6, Bins: 1, Maps: before.Maps}) {
		t.Errorf("rows after first run = %+v", before)
	}

	// The shared barcode is one unit both legacy rows map to.
	var dupTargets int64
	var dupUUID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT target_uuid), MIN(target_uuid::text)::uuid FROM migration_map
		WHERE (source_system = $1 AND source_table = 'stock_items' AND source_id = '8')
		   OR (source_system = $2 AND source_table = 'product_barcodes' AND source_id = '00000008-0000-4000-8000-000000000001')`,
		hubSys, whSys).Scan(&dupTargets, &dupUUID); err != nil {
		t.Fatal(err)
	}
	if dupTargets != 1 || scalar(`SELECT COUNT(*) FROM units WHERE barcode = 'SYN-DUP-0001' AND uuid = $1`, dupUUID) != 1 {
		t.Errorf("SYN-DUP-0001: %d targets", dupTargets)
	}

	// Location tree and the warehouse codes.
	rows, err := pool.Query(ctx, `
		SELECT x.full_code, x.type FROM warehouse_locations x
		JOIN migration_map m ON m.target_uuid = x.uuid AND m.source_system = $1 AND m.target_table = 'warehouse_locations'
		JOIN organizations o ON o.id = x.organization_id AND o.type = 'center'
		ORDER BY x.full_code`, whSys)
	if err != nil {
		t.Fatal(err)
	}
	var tree []string
	for rows.Next() {
		var code, typ string
		if err := rows.Scan(&code, &typ); err != nil {
			t.Fatal(err)
		}
		tree = append(tree, code+":"+typ)
	}
	rows.Close()
	wantTree := []string{
		"SYN_WH_1-DEF-A:aisle", "SYN_WH_1-DEF-A-01:shelf", "SYN_WH_1-DEF-A-01-01:bin",
		"SYN_WH_2-DEF-B:aisle", "SYN_WH_2-DEF-B-01:shelf",
	}
	if strings.Join(tree, " ") != strings.Join(wantTree, " ") {
		t.Errorf("location tree = %v, want %v", tree, wantTree)
	}

	// Ownership per barcode: owner kind, holder and status.
	dealerOrg := func(id string) int64 {
		return scalar(`SELECT o.id FROM organizations o JOIN migration_map m ON m.target_uuid = o.uuid
			WHERE m.source_system = $1 AND m.source_table = 'dealers' AND m.source_id = $2`, hubSys, id)
	}
	center := scalar(`SELECT o.id FROM organizations o JOIN brands b ON b.id = o.brand_id AND b.slug = 'olex'
		WHERE o.type = 'center' AND o.deleted_at IS NULL`)
	bin := scalar(`SELECT id FROM warehouse_locations WHERE organization_id = $1 AND full_code = 'SYN_WH_1-DEF-A-01-01'`, center)
	type own struct {
		Type          string
		Owner, Holder int64
		Status        string
	}
	want := map[string]own{
		"SYN-HUB-0001": {"organization", center, center, "available"},
		"SYN-HUB-0002": {"organization", dealerOrg("1"), dealerOrg("1"), "available"},
		"SYN-HUB-0006": {"organization", dealerOrg("2"), dealerOrg("2"), "available"},
		"SYN-HUB-0007": {"trash", center, center, "used"},
		"SYN-DUP-0001": {"warehouse_location", bin, center, "placed"},
		"SYN-WH-0002":  {"warehouse_location", bin, center, "placed"},
		"SYN-HUB-0003": {"", 0, 0, "used"},
		"SYN-HUB-0004": {"", 0, 0, "used"},
		"SYN-HUB-0005": {"", 0, 0, "used"},
	}
	for code, w := range want {
		var got own
		var unitStatus string
		if err := pool.QueryRow(ctx, `
			SELECT COALESCE(s.owner_type, ''), COALESCE(s.owner_id, 0), COALESCE(s.holder_org_id, 0), COALESCE(s.status, u.status), u.status
			FROM units u JOIN brands b ON b.id = u.brand_id AND b.slug = 'olex'
			LEFT JOIN unit_current_state s ON s.unit_id = u.id
			WHERE u.barcode = $1 AND u.organization_id = $2 AND u.source = 'imported'`, code, center).
			Scan(&got.Type, &got.Owner, &got.Holder, &got.Status, &unitStatus); err != nil {
			t.Errorf("%s: %v", code, err)
			continue
		}
		if got != w || unitStatus != w.Status {
			t.Errorf("%s ownership = %+v (unit status %s), want %+v", code, got, unitStatus, w)
		}
	}
	if n := scalar(`SELECT COUNT(*) FROM units WHERE barcode IN ('SYN-WH-0003', 'SYN-WH-0004')`); n != 0 {
		t.Errorf("Glorian units imported: %d", n)
	}

	// Bin stock of the Olex product in A-01-01.
	if q := scalar(`SELECT s.quantity FROM bin_product_stocks s JOIN products p ON p.id = s.product_id
		WHERE s.location_id = $1 AND p.sku = 'SYN-PPF-GLOSS'`, bin); q != 2 {
		t.Errorf("bin stock = %d, want 2", q)
	}

	// Second run: nothing new.
	second := e.run("olexfx-stock", Options{})
	for _, step := range []string{"warehouses", "units"} {
		for k, v := range stepCounts(t, second, step) {
			if v != 0 && (strings.HasSuffix(k, "_created") || strings.HasSuffix(k, "_updated") ||
				strings.HasSuffix(k, "_linked_existing") || strings.HasPrefix(k, "ownership_")) {
				t.Errorf("rerun %s.%s = %d, want 0", step, k, v)
			}
		}
	}
	if c := stepCounts(t, second, "units"); c["units_unchanged"] != 9 || c["bin_stocks_unchanged"] != 1 {
		t.Errorf("rerun units counts = %v", c)
	}
	if c := stepCounts(t, second, "warehouses"); c["locations_unchanged"] != 5 {
		t.Errorf("rerun warehouses counts = %v", c)
	}
	if after := tableCounts(); after != before {
		t.Errorf("rerun changed rows: %+v -> %+v", before, after)
	}
}
