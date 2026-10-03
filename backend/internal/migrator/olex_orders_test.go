package migrator

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// orderRow is one migrated order with its parties, for the fixture checks.
type orderRow struct {
	Ref, Status, Seller, Buyer string
	Lines                      int64
	Units                      string
	Approved, Rate             bool
}

// TEC-261 acceptance over the legacy fixture. The run happens in one
// transaction that is rolled back (like TEC-258): the shared test database
// keeps nothing.
//
//   - hub order 2 and warehouse order SYN-WH-REF-0001 are one order;
//   - lines and their units point at the right products and barcodes;
//   - no finance_entries, outbox_events or order_status_history row is added;
//   - a second run creates nothing.
func TestOlexOrders(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool
	hubSys := e.system
	whSys := hubSys + "w"

	var taken int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM units u JOIN brands b ON b.id = u.brand_id AND b.slug = 'olex'
		         WHERE u.barcode LIKE 'SYN-%')
		     + (SELECT COUNT(*) FROM products p JOIN brands b ON b.id = p.brand_id AND b.slug = 'olex' WHERE p.sku = ANY($1))
		     + (SELECT COUNT(*) FROM orders WHERE external_reference LIKE 'SYN-WH-REF-%')`,
		fixtureSKUs).Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture units / products / orders already exist in the test database", taken)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	e.runner.Pool = tx
	e.runner.Profiles["olexfx-before-orders"] = Profile{
		Name: "olexfx-before-orders", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step {
			return []Step{
				OrganizationsStep{System: hubSys},
				UsersStep{System: hubSys},
				CatalogStep{System: hubSys, WHSystem: whSys},
				WarehousesStep{WHSystem: whSys},
				UnitsStep{System: hubSys, WHSystem: whSys},
			}
		},
	}
	e.runner.Profiles["olexfx-orders"] = Profile{
		Name: "olexfx-orders", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step { return []Step{OrdersStep{System: hubSys, WHSystem: whSys}} },
	}

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	inserted := func(table string) int64 {
		// Rows this transaction inserted into table so far (the table of
		// this database: the fixture schemas have orders tables too).
		return scalar(`SELECT COALESCE((SELECT n_tup_ins FROM pg_stat_xact_user_tables WHERE relid = to_regclass($1::text)), 0)`, table)
	}
	const migratedOrders = `SELECT o.id FROM orders o JOIN migration_map m ON m.target_uuid = o.uuid
		AND m.target_table = 'orders' AND m.source_system IN ($1, $2)`
	orders := func() []orderRow {
		t.Helper()
		rows, err := tx.Query(ctx, `
			SELECT COALESCE(o.external_reference, ''), o.status, s.type, b.slug,
			       (SELECT COUNT(*) FROM order_items i WHERE i.order_id = o.id),
			       COALESCE((SELECT string_agg(p.sku || '=' || u.barcode, ',' ORDER BY p.sku, u.barcode)
			                 FROM order_item_units iu JOIN order_items i ON i.id = iu.order_item_id
			                 JOIN units u ON u.id = iu.unit_id JOIN products p ON p.id = i.product_id
			                 WHERE i.order_id = o.id), ''),
			       o.approved_at IS NOT NULL, o.try_rate IS NOT NULL
			FROM orders o JOIN organizations s ON s.id = o.seller_org_id JOIN organizations b ON b.id = o.buyer_org_id
			WHERE o.id IN (`+migratedOrders+`)
			ORDER BY o.created_at, o.id`, hubSys, whSys)
		if err != nil {
			t.Fatal(err)
		}
		out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[orderRow])
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	e.run("olexfx-before-orders", Options{})
	finance, outbox, history := inserted("finance_entries"), inserted("outbox_events"), inserted("order_status_history")

	first := e.run("olexfx-orders", Options{})
	oc := stepCounts(t, first, "orders")
	for k, want := range map[string]int64{
		"hub_read": 3, "wh_read": 2, "orders_matched": 1, "orders_created": 4,
		"lines_created": 5, "units_assigned": 4, "lines_glorian_skipped": 1,
		"orders_status:received": 1, "orders_status:shipped": 1, "orders_status:submitted": 1, "orders_status:draft": 1,
	} {
		if oc[k] != want {
			t.Errorf("orders.%s = %d, want %d (%v)", k, oc[k], want, oc)
		}
	}
	for k := range oc {
		if strings.HasPrefix(k, "skipped_") || strings.Contains(k, "unmapped") || strings.Contains(k, "conflict") ||
			strings.Contains(k, "mismatch") || k == "rate_defaulted" {
			t.Errorf("unexpected report %s (%v)", k, oc)
		}
	}

	// Matched hub + warehouse order is one record, mapped from both sides.
	if n := scalar(`SELECT COUNT(*) FROM orders WHERE external_reference = 'SYN-WH-REF-0001'`); n != 1 {
		t.Errorf("orders with SYN-WH-REF-0001 = %d, want 1", n)
	}
	if n := scalar(`SELECT COUNT(DISTINCT target_uuid) FROM migration_map
		WHERE target_table = 'orders' AND ((source_system = $1 AND source_id = '2')
		   OR (source_system = $2 AND source_id = '0000000b-0000-4000-8000-000000000001'))`, hubSys, whSys); n != 1 {
		t.Errorf("hub order 2 and its warehouse order map to %d orders, want 1", n)
	}

	want := []orderRow{
		// hub 1: delivered -> received, two lines, three units.
		{"", "received", "distributor", "", 2, "SYN-PPF-GLOSS=SYN-HUB-0002,SYN-PPF-GLOSS=SYN-HUB-0003,SYN-PPF-MATTE=SYN-HUB-0004", true, true},
		// hub 2 + warehouse: the warehouse (updated later) decides: shipped.
		// The Glorian warehouse line is skipped; the hub line keeps its unit.
		{"SYN-WH-REF-0001", "shipped", "distributor", "", 1, "SYN-WIN-01=SYN-HUB-0006", true, true},
		// hub 3: pending -> submitted, not approved yet.
		{"", "submitted", "distributor", "", 1, "", false, false},
		// warehouse-only draft.
		{"SYN-WH-REF-0002", "draft", "distributor", "", 1, "", false, false},
	}
	got := orders()
	if len(got) != len(want) {
		t.Fatalf("orders = %+v", got)
	}
	for i := range want {
		got[i].Buyer = "" // dealer slugs are generated
		if got[i] != want[i] {
			t.Errorf("order %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if n := scalar(`SELECT COUNT(*) FROM orders o JOIN organizations b ON b.id = o.buyer_org_id
		WHERE o.id IN (`+migratedOrders+`) AND (b.type <> 'dealer' OR b.parent_id <> o.seller_org_id)`, hubSys, whSys); n != 0 {
		t.Errorf("%d orders with a buyer that is not a dealer of the seller", n)
	}
	// Buyers: hub dealer 1 for orders 1, 3 and the warehouse draft; dealer 2
	// for the matched order.
	if n := scalar(`SELECT COUNT(DISTINCT o.buyer_org_id) FROM orders o WHERE o.id IN (`+migratedOrders+`)`, hubSys, whSys); n != 2 {
		t.Errorf("distinct buyers = %d, want 2", n)
	}
	if n := scalar(`SELECT COUNT(*) FROM orders o JOIN migration_map m ON m.target_uuid = o.uuid AND m.source_system = $1
		AND m.source_table = 'orders' WHERE o.id IN (`+migratedOrders+`) AND o.created_by_user_id IS NOT NULL`, hubSys, whSys); n != 3 {
		t.Errorf("hub orders with a creator = %d, want 3", n)
	}
	if n := scalar(`SELECT COUNT(*) FROM orders WHERE id IN (`+migratedOrders+`) AND (currency <> 'TRY' OR total <> 0)`, hubSys, whSys); n != 0 {
		t.Errorf("%d orders not in TRY or with a total", n)
	}

	// No accounting, no outbox, no status history.
	if n := inserted("finance_entries") - finance; n != 0 {
		t.Errorf("finance_entries inserted = %d, want 0", n)
	}
	if n := inserted("outbox_events") - outbox; n != 0 {
		t.Errorf("outbox_events inserted = %d, want 0", n)
	}
	if n := inserted("order_status_history") - history; n != 0 {
		t.Errorf("order_status_history inserted = %d, want 0", n)
	}

	// Second run: nothing new.
	tables := func() [3]int64 {
		return [3]int64{inserted("orders"), inserted("order_items"), inserted("order_item_units")}
	}
	before := tables()
	second := e.run("olexfx-orders", Options{})
	sc := stepCounts(t, second, "orders")
	if sc["orders_unchanged"] != 4 || sc["orders_created"] != 0 || sc["orders_updated"] != 0 ||
		sc["lines_created"] != 0 || sc["units_assigned"] != 0 {
		t.Errorf("rerun counts = %v", sc)
	}
	if after := tables(); after != before {
		t.Errorf("rerun inserted rows: %v -> %v", before, after)
	}
	if again := orders(); len(again) != len(want) {
		t.Errorf("rerun orders = %+v", again)
	}
}
