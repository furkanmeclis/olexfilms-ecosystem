package migrator

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
)

// ownRow is one ownership row of a migrated unit (serial state or fixed
// holding), without the movement link.
type ownRow struct {
	Barcode, Kind, OwnerType string
	OwnerID, Holder          int64
	Status                   string
	Quantity                 int64
}

// TEC-258 acceptance over the legacy fixture. The whole run happens in one
// transaction that is rolled back: stock_movements is append-only, so a
// committed run could not be cleaned up from the shared test database.
//
//   - after the rebuild the ownership equals the TEC-257 ownership exactly;
//   - movements = imported legacy movements + opening corrections;
//   - no outbox event is written;
//   - a second run adds no movement.
func TestOlexLedgerOpening(t *testing.T) {
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	e.runner.Pool = tx
	e.runner.Profiles["olexfx-units"] = Profile{
		Name: "olexfx-units", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step {
			return []Step{
				OrganizationsStep{System: hubSys},
				CatalogStep{System: hubSys, WHSystem: whSys},
				WarehousesStep{WHSystem: whSys},
				UnitsStep{System: hubSys, WHSystem: whSys},
			}
		},
	}
	e.runner.Profiles["olexfx-ledger"] = Profile{
		Name: "olexfx-ledger", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step { return []Step{LedgerStep{System: hubSys, WHSystem: whSys}} },
	}

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	const migratedUnits = `SELECT x.id FROM units x JOIN migration_map m ON m.target_uuid = x.uuid
		AND m.target_table = 'units' AND m.source_system IN ($1, $2)`
	ownership := func() []ownRow {
		t.Helper()
		rows, err := tx.Query(ctx, `
			SELECT u.barcode, 'serial', s.owner_type, s.owner_id, s.holder_org_id, s.status, 1
			FROM unit_current_state s JOIN units u ON u.id = s.unit_id WHERE u.id IN (`+migratedUnits+`)
			UNION ALL
			SELECT u.barcode, 'fixed', h.owner_type, h.owner_id, h.holder_org_id, '', h.quantity_on_hand
			FROM fixed_barcode_holdings h JOIN units u ON u.id = h.unit_id WHERE u.id IN (`+migratedUnits+`)
			  AND h.quantity_on_hand <> 0
			ORDER BY 1, 2, 3, 4`, hubSys, whSys)
		if err != nil {
			t.Fatal(err)
		}
		out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ownRow])
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	movements := func() int64 {
		return scalar(`SELECT COUNT(*) FROM stock_movements WHERE unit_id IN (`+migratedUnits+`)`, hubSys, whSys)
	}
	outboxInserts := func() int64 {
		// Rows this transaction inserted into outbox_events so far.
		return scalar(`SELECT COALESCE((SELECT n_tup_ins FROM pg_stat_xact_user_tables WHERE relname = 'outbox_events'), 0)`)
	}

	e.run("olexfx-units", Options{})
	before := ownership()
	if len(before) != 6 {
		t.Fatalf("TEC-257 ownership rows = %d, want 6: %+v", len(before), before)
	}
	if n := movements(); n != 0 {
		t.Fatalf("movements before the ledger step = %d", n)
	}
	outboxBefore := outboxInserts()

	first := e.run("olexfx-ledger", Options{})
	lc := stepCounts(t, first, "ledger")
	for k, want := range map[string]int64{
		"hub_read": 16, "wh_read": 5, "wh_glorian_skipped": 3,
		"movements_created": 18, "movements_created:" + hubSys: 16, "movements_created:" + whSys: 2,
		"movements_without_owner": 9, "opening_corrections": 1, "units_corrected": 1,
		"units_opened": 6, "units_without_owner:used": 3,
	} {
		if lc[k] != want {
			t.Errorf("ledger.%s = %d, want %d (%v)", k, lc[k], want, lc)
		}
	}
	for k := range lc {
		if strings.HasPrefix(k, "ownership_mismatch") || strings.HasPrefix(k, "opening_unreachable") ||
			strings.Contains(k, "_skipped_unit") || strings.Contains(k, "unknown") {
			t.Errorf("unexpected report %s (%v)", k, lc)
		}
	}

	// Ownership after the rebuild = TEC-257 ownership.
	after := ownership()
	if !equalOwn(before, after) {
		t.Errorf("ownership changed:\n before %+v\n after  %+v", before, after)
	}
	if n := scalar(`SELECT COUNT(*) FROM unit_current_state WHERE last_movement_id IS NULL AND unit_id IN (`+migratedUnits+`)`,
		hubSys, whSys); n != 0 {
		t.Errorf("%d states still without a movement", n)
	}

	// Movements = source + corrections.
	source := scalar(`SELECT (SELECT COUNT(*) FROM legacy_hub.stock_movements)
		+ (SELECT COUNT(*) FROM legacy_wh.stock_movements m JOIN legacy_wh.products p ON p.id = m.product_id
		   JOIN legacy_wh.brands b ON b.id = p.brand_id WHERE b.name NOT ILIKE '%glorian%')`)
	corrections := scalar(`SELECT COUNT(*) FROM stock_movements WHERE reason = $3 AND unit_id IN (`+migratedUnits+`)`,
		hubSys, whSys, openingReason)
	if total := movements(); source != 18 || corrections != 1 || total != source+corrections {
		t.Errorf("movements = %d, source = %d, corrections = %d", total, source, corrections)
	}
	if n := scalar(`SELECT COUNT(*) FROM stock_movements sm JOIN migration_map m ON m.target_uuid = sm.uuid
		AND m.target_table = 'stock_movements' AND m.source_system IN ($1, $2)
		WHERE sm.idempotency_key LIKE 'legacy:%'`, hubSys, whSys); n != 18 {
		t.Errorf("mapped legacy movements = %d, want 18", n)
	}
	// The legacy time is kept.
	if n := scalar(`SELECT COUNT(*) FROM stock_movements sm
		JOIN migration_map m ON m.target_uuid = sm.uuid AND m.source_system = $1 AND m.source_table = 'stock_movements'
		JOIN legacy_hub.stock_movements l ON l.id::text = m.source_id
		WHERE sm.created_at = l.created_at AT TIME ZONE 'UTC'`, hubSys); n != 16 {
		t.Errorf("hub movements with the legacy time = %d, want 16", n)
	}

	// No outbox event.
	if n := outboxInserts() - outboxBefore; n != 0 {
		t.Errorf("outbox_events inserted = %d, want 0", n)
	}
	if n := scalar(`SELECT COUNT(*) FROM outbox_events WHERE event_name LIKE 'stock.%'
		AND (payload->>'unit_id')::bigint IN (`+migratedUnits+`)`, hubSys, whSys); n != 0 {
		t.Errorf("stock outbox events of migrated units = %d", n)
	}

	// Product stock projections follow the ledger.
	center := scalar(`SELECT o.id FROM organizations o JOIN brands b ON b.id = o.brand_id AND b.slug = 'olex'
		WHERE o.type = 'center' AND o.deleted_at IS NULL`)
	if q := scalar(`SELECT COALESCE(SUM(s.quantity), 0) FROM organization_product_stocks s JOIN products p ON p.id = s.product_id
		WHERE s.organization_id = $1 AND p.sku = 'SYN-PPF-GLOSS'`, center); q != 3 {
		t.Errorf("center SYN-PPF-GLOSS stock = %d, want 3", q)
	}
	if q := scalar(`SELECT COALESCE(SUM(s.quantity), 0) FROM bin_product_stocks s JOIN products p ON p.id = s.product_id
		JOIN warehouse_locations l ON l.id = s.location_id
		WHERE l.organization_id = $1 AND l.full_code = 'SYN_WH_1-DEF-A-01-01' AND p.sku = 'SYN-PPF-GLOSS'`, center); q != 2 {
		t.Errorf("bin stock = %d, want 2", q)
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT holder_org_id FROM unit_current_state WHERE unit_id IN (`+migratedUnits+`)`, hubSys, whSys)
	if err != nil {
		t.Fatal(err)
	}
	orgs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		t.Fatal(err)
	}
	for _, org := range orgs {
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rep, err := rebuild.ApplyTx(ctx, sp, rebuild.Options{OrganizationID: org})
		_ = sp.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// Only the fixture's units: the test database is shared.
		for _, d := range rep.Diffs {
			if strings.HasPrefix(d.Barcode, "SYN-") {
				t.Errorf("organization %d drifts after the run: %+v", org, d)
			}
		}
	}

	// Second run: nothing new.
	total := movements()
	second := e.run("olexfx-ledger", Options{})
	sc := stepCounts(t, second, "ledger")
	if sc["movements_created"] != 0 || sc["opening_corrections"] != 0 || sc["movements_unchanged"] != 18 || sc["rebuild_organizations"] != 0 {
		t.Errorf("rerun counts = %v", sc)
	}
	if n := movements(); n != total {
		t.Errorf("rerun movements %d -> %d", total, n)
	}
	if again := ownership(); !equalOwn(before, again) {
		t.Errorf("rerun ownership = %+v", again)
	}
	// TEC-257 rerun keeps the ledger's units.
	units := e.run("olexfx-units", Options{})
	if c := stepCounts(t, units, "units"); c["units_unchanged"] != 9 {
		t.Errorf("units rerun counts = %v", c)
	}
	if n := movements(); n != total {
		t.Errorf("units rerun movements %d -> %d", total, n)
	}
}

func equalOwn(a, b []ownRow) bool {
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
