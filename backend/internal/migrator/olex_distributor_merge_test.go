package migrator

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgmerge"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// TEC-1003 / TEC-1202 over the legacy fixture, as on a database the old
// migrator filled: the dealers sit under a Türkiye distributor (with the TR
// territory) that sold them the orders. A run merges the distributor into
// the center (the center is the Türkiye distributor too) and nothing else:
// hub dealer 1 (AFM Florya in the Olex data) stays a normal dealer under the
// center with its services, warranties, customers, members (dealer roles)
// and orders, and the center's service point stays unset. A rerun changes
// nothing; undoing the merge puts every row back. Everything runs in one
// rolled back transaction.
func TestOlexDistributorMergeKeepsDealers(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool
	hubSys := e.system
	whSys := hubSys + "w"

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}

	steps := func() []Step {
		return []Step{
			OrganizationsStep{System: hubSys},
			UsersStep{System: hubSys},
			CustomersStep{System: hubSys},
			VehicleCatalogStep{System: hubSys},
			CatalogStep{System: hubSys, WHSystem: whSys},
			WarehousesStep{WHSystem: whSys, PrimaryCode: "SYN-WH-1", PrimaryNewCode: "SYN_CENTRAL", PrimaryName: "Sentetik Ana Depo"},
			UnitsStep{System: hubSys, WHSystem: whSys},
			LedgerStep{System: hubSys, WHSystem: whSys},
			OrdersStep{System: hubSys, WHSystem: whSys},
			ShortURLsStep{System: hubSys},
			LegacyMessagesStep{System: hubSys},
			ServicesStep{System: hubSys},
			WarrantiesStep{System: hubSys},
			MeasurementsStep{System: hubSys},
			UnitTraceStep{System: hubSys, WHSystem: whSys},
		}
	}
	e.runner.Pool = tx
	e.runner.Storage = storage.NewMemory()
	e.runner.Profiles["olexfx-dist"] = Profile{Name: "olexfx-dist", Enabled: true,
		Sources: []string{SourceHub, SourceWH}, Steps: steps}

	// 1. The old shape: a full run, then the dealers under a TR distributor
	//    that sold them their orders.
	first := e.run("olexfx-dist", Options{})
	// TEC-1006: the units of completed legacy services get the service as
	// owner, every legacy barcode is a unit. (The ownership counts cover the
	// whole brand of the shared test database, so they are not asserted.)
	trace := stepCounts(t, first, "unit_trace")
	// At least the two fixture units; units other tests left in the shared
	// database may be traced too.
	if trace["service_consumption_created"] < 2 || trace["legacy_barcodes_missing"] != 0 ||
		trace["legacy_barcodes"] != trace["legacy_barcodes_in_units"] {
		t.Fatalf("unit_trace counts = %v", trace)
	}
	if n := scalar(`SELECT count(*) FROM service_items si JOIN services s ON s.id = si.service_id
		JOIN units u ON u.id = si.unit_id
		JOIN brands b ON b.id = s.brand_id AND b.slug = 'olex'
		WHERE s.status = 'completed' AND u.unit_kind = 'serial'
		  AND NOT EXISTS (SELECT 1 FROM unit_current_state st WHERE st.unit_id = si.unit_id)`); n != 0 {
		t.Fatalf("%d units of completed services without an owner", n)
	}
	checkWarehouseTrace(t, ctx, tx)
	// TEC-1202: the primary warehouse carries its new code, not the legacy one.
	if n := scalar(`SELECT count(*) FROM warehouses WHERE is_primary AND name = 'Sentetik Ana Depo' AND code = 'SYN_CENTRAL'`); n != 1 {
		t.Errorf("primary warehouse with code SYN_CENTRAL = %d, want 1", n)
	}
	if n := scalar(`SELECT count(*) FROM warehouses WHERE name = 'Sentetik Ana Depo' AND code = 'SYN_WH_1'`); n != 0 {
		t.Errorf("primary warehouse kept its legacy code")
	}
	var centerID, dealer1, countryID int64
	if err := tx.QueryRow(ctx, `SELECT c.id, o.id, o.country_id
		FROM migration_map m JOIN organizations o ON o.uuid = m.target_uuid
		JOIN organizations c ON c.id = o.parent_id
		WHERE m.source_system = $1 AND m.source_table = 'dealers' AND m.source_id = '1'`, hubSys).
		Scan(&centerID, &dealer1, &countryID); err != nil {
		t.Fatal(err)
	}
	if scalar(`SELECT count(*) FROM territories t JOIN organizations o ON o.id = t.organization_id
		WHERE o.brand_id = (SELECT brand_id FROM organizations WHERE id = $1) AND t.country_id = $2 AND t.province_id IS NULL`,
		centerID, countryID) != 0 {
		t.Skip("the test database already has a TR country territory")
	}
	var distID int64
	if err := tx.QueryRow(ctx, `INSERT INTO organizations (uuid, slug, name, status, type, parent_id, brand_id, country_id)
		SELECT gen_random_uuid(), $2, 'TR Distribütörü', 'active', 'distributor', id, brand_id, $3 FROM organizations WHERE id = $1
		RETURNING id`, centerID, TRDistributorSlug, countryID).Scan(&distID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO territories (brand_id, organization_id, country_id) SELECT brand_id, id, $2 FROM organizations WHERE id = $1`, distID, countryID)
	exec(`UPDATE organizations SET parent_id = $1 WHERE uuid IN (SELECT target_uuid FROM migration_map
		WHERE source_system = $2 AND source_table = 'dealers')`, distID, hubSys)
	// The orders move to the distributor with their lines (composite keys,
	// append-only status history).
	exec(`SET CONSTRAINTS ALL DEFERRED`)
	exec(`SELECT set_config('olex.org_merge', 'on', true)`)
	exec(`CREATE TEMP TABLE sp_orders ON COMMIT DROP AS SELECT id FROM orders WHERE seller_org_id = $1 AND buyer_org_id IN (
		SELECT o.id FROM organizations o JOIN migration_map m ON m.target_uuid = o.uuid
		WHERE m.source_system = $2 AND m.source_table = 'dealers')`, centerID, hubSys)
	exec(`UPDATE orders SET seller_org_id = $1, organization_id = $1 WHERE id IN (SELECT id FROM sp_orders)`, distID)
	for _, table := range []string{"order_items", "order_status_history"} {
		exec(`UPDATE `+table+` SET organization_id = $1 WHERE order_id IN (SELECT id FROM sp_orders)`, distID)
	}
	for _, table := range []string{"order_item_units", "stock_reservations"} {
		exec(`UPDATE `+table+` SET organization_id = $1 WHERE order_item_id IN (
			SELECT id FROM order_items WHERE order_id IN (SELECT id FROM sp_orders))`, distID)
	}
	exec(`SELECT set_config('olex.org_merge', 'off', true)`)
	exec(`SET CONSTRAINTS ALL IMMEDIATE`)

	// Services and warranties are counted on the fixture rows only (legacy
	// service numbers "SYN-S-..."): the center is the shared brand center,
	// other packages commit services into it while this test runs.
	const fixtureServices = `SELECT id FROM services WHERE service_no LIKE 'SYN-%'`
	type counts struct{ Services, Warranties, Customers, Units, Members, Orders int64 }
	of := func(org int64) counts {
		t.Helper()
		return counts{
			Services: scalar(`SELECT count(*) FROM services WHERE organization_id = $1 AND id IN (`+fixtureServices+`)`, org),
			Warranties: scalar(`SELECT count(*) FROM warranties WHERE organization_id = $1 AND service_id IN (`+fixtureServices+`)`,
				org),
			Customers: scalar(`SELECT count(*) FROM customer_organizations WHERE organization_id = $1`, org),
			Units:     scalar(`SELECT count(*) FROM unit_current_state WHERE holder_org_id = $1`, org),
			Members:   scalar(`SELECT count(*) FROM organization_members WHERE organization_id = $1`, org),
			Orders:    scalar(`SELECT count(*) FROM orders WHERE buyer_org_id = $1`, org),
		}
	}
	dealerBefore, centerBefore := of(dealer1), of(centerID)
	if dealerBefore.Services == 0 || dealerBefore.Warranties == 0 || dealerBefore.Members == 0 || dealerBefore.Orders == 0 {
		t.Fatalf("fixture dealer 1 has too few records: %+v", dealerBefore)
	}
	ordersOfDist := scalar(`SELECT count(*) FROM orders WHERE seller_org_id = $1`, distID)
	if ordersOfDist == 0 {
		t.Fatal("no distributor orders")
	}

	// 2. The run merges the distributor into the center, and only it.
	rep := e.run("olexfx-dist", Options{})
	oc := stepCounts(t, rep, "organizations")
	if oc["distributor_merged"] != 1 || oc["service_point_merged"] != 0 {
		t.Fatalf("organizations counts = %v", oc)
	}
	if n := scalar(`SELECT count(*) FROM organizations WHERE parent_id = $1 AND deleted_at IS NULL`, distID); n != 0 {
		t.Errorf("%d dealers still under the distributor", n)
	}
	if n := scalar(`SELECT count(*) FROM orders WHERE seller_org_id = $1`, centerID) -
		scalar(`SELECT count(*) FROM orders WHERE seller_org_id = $1`, distID); n < ordersOfDist {
		t.Errorf("center sells %d orders, want at least %d", n, ordersOfDist)
	}
	if n := scalar(`SELECT count(*) FROM territories WHERE organization_id = $1`, distID); n != 0 {
		t.Errorf("distributor territory kept: %d", n)
	}
	// TEC-1202: dealer 1 is a live dealer under the center and keeps every
	// record; the center's own records do not grow.
	if n := scalar(`SELECT count(*) FROM organizations WHERE id = $1 AND type = 'dealer' AND parent_id = $2
		AND deleted_at IS NULL AND merged_into_id IS NULL`, dealer1, centerID); n != 1 {
		t.Errorf("dealer 1 is not a live dealer under the center")
	}
	if n := scalar(`SELECT count(*) FROM org_merge_runs WHERE source_org_id = $1`, dealer1); n != 0 {
		t.Errorf("dealer 1 merged %d times", n)
	}
	if got := of(dealer1); got != dealerBefore {
		t.Errorf("dealer 1 after the run = %+v, want %+v", got, dealerBefore)
	}
	if got := of(centerID); got.Services != centerBefore.Services || got.Warranties != centerBefore.Warranties {
		t.Errorf("center after the run = %+v, before %+v", got, centerBefore)
	}
	var spName string
	var spOn bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(service_point_name, ''), service_point_enabled FROM organizations WHERE id = $1`, centerID).
		Scan(&spName, &spOn); err != nil {
		t.Fatal(err)
	}
	if spName == "Sentetik Bayi Bir" {
		t.Errorf("center service point filled from dealer 1: %q %v", spName, spOn)
	}
	// Dealer 1's owner and staff keep their dealer roles at the dealer and
	// get no center membership (center_service is not used).
	for email, want := range map[string]string{"sahip1@example.test": "dealer_owner", "personel1@example.test": "dealer_staff"} {
		var got string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(string_agg(r.slug, ','), '') FROM organization_members om
			JOIN users u ON u.id = om.user_id
			JOIN organization_member_roles omr ON omr.member_id = om.id JOIN roles r ON r.id = omr.role_id
			WHERE u.email = $1 AND om.organization_id = $2`, email, dealer1).Scan(&got); err != nil {
			t.Fatalf("%s: %v", email, err)
		}
		if got != want {
			t.Errorf("%s dealer roles = %q, want %s", email, got, want)
		}
		if n := scalar(`SELECT count(*) FROM organization_members om JOIN users u ON u.id = om.user_id
			WHERE u.email = $1 AND om.organization_id = $2`, email, centerID); n != 0 {
			t.Errorf("%s is a center member", email)
		}
	}

	// 3. A rerun writes nothing new and merges nothing.
	servicesTotal := scalar(`SELECT count(*) FROM services WHERE id IN (` + fixtureServices + `)`)
	warrantiesTotal := scalar(`SELECT count(*) FROM warranties WHERE service_id IN (` + fixtureServices + `)`)
	rep = e.run("olexfx-dist", Options{})
	if n := stepCounts(t, rep, "unit_trace")["service_consumption_created"]; n != 0 {
		t.Errorf("rerun created %d service consumptions", n)
	}
	oc = stepCounts(t, rep, "organizations")
	if oc["distributor_merged"] != 0 {
		t.Errorf("rerun merged again: %v", oc)
	}
	if n := scalar(`SELECT count(*) FROM services WHERE id IN (` + fixtureServices + `)`); n != servicesTotal {
		t.Errorf("rerun services %d -> %d", servicesTotal, n)
	}
	if n := scalar(`SELECT count(*) FROM warranties WHERE service_id IN (` + fixtureServices + `)`); n != warrantiesTotal {
		t.Errorf("rerun warranties %d -> %d", warrantiesTotal, n)
	}
	if got := of(dealer1); got != dealerBefore {
		t.Errorf("rerun changed dealer 1: %+v, want %+v", got, dealerBefore)
	}

	// 4. Undo the distributor merge: the dealers are back under it.
	var run string
	if err := tx.QueryRow(ctx, `SELECT uuid::text FROM org_merge_runs WHERE source_org_id = $1`, distID).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if _, err := orgmerge.Undo(ctx, tx, run); err != nil {
		t.Fatalf("undo %s: %v", run, err)
	}
	if got := of(dealer1); got.Services != dealerBefore.Services || got.Warranties != dealerBefore.Warranties ||
		got.Members != dealerBefore.Members || got.Customers != dealerBefore.Customers {
		t.Errorf("dealer 1 after undo = %+v, want %+v", got, dealerBefore)
	}
	if n := scalar(`SELECT count(*) FROM organizations WHERE parent_id = $1`, distID); n == 0 {
		t.Error("dealers are not back under the distributor")
	}
	if n := scalar(`SELECT count(*) FROM territories WHERE organization_id = $1`, distID); n != 1 {
		t.Errorf("distributor territory after undo = %d", n)
	}
	if n := scalar(`SELECT count(*) FROM organizations WHERE id = $1 AND (deleted_at IS NOT NULL OR merged_into_id IS NOT NULL)`,
		distID); n != 0 {
		t.Error("distributor still merged after undo")
	}
	if _, err := orgmerge.Undo(ctx, tx, run); err == nil {
		t.Error("a second undo must fail")
	}
}

// checkWarehouseTrace (TEC-1006): the primary warehouse lists the units that
// left it with their order / service, and a used unit's trace names its
// service and customer.
func checkWarehouseTrace(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	q := db.New(tx)
	var whUUID uuid.UUID
	var orgID int64
	if err := tx.QueryRow(ctx, `SELECT uuid, organization_id FROM warehouses WHERE is_primary AND name = 'Sentetik Ana Depo'`).
		Scan(&whUUID, &orgID); err != nil {
		t.Fatalf("primary warehouse: %v", err)
	}
	svc := stockusecase.New(q)
	// The primary warehouse is the origin of every center unit without a
	// warehouse placement, so units other packages commit into the shared
	// Olex center concurrently show up in it, and between the count and the
	// list statement of one call (read committed: each statement takes a
	// new snapshot). Scope the check to the fixture units (barcodes "SYN-",
	// q = barcode prefix): no other test creates those.
	const fixturePrefix = "SYN-"
	all, total, err := svc.WarehouseUnits(ctx, orgID, whUUID, stockusecase.WarehouseUnitFilter{
		Q: fixturePrefix, Sort: apiquery.ResolvedSort{Key: "barcode"}, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	left, leftTotal, err := svc.WarehouseUnits(ctx, orgID, whUUID, stockusecase.WarehouseUnitFilter{
		Q: fixturePrefix, State: stockusecase.WarehouseUnitLeft, Sort: apiquery.ResolvedSort{Key: "barcode"}, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Fixture: SYN-DUP-0001 and SYN-WH-0002 are placed in the primary
	// warehouse; the seven SYN-HUB units left it (dealer, service, trash).
	if total != 9 || leftTotal != 7 || len(all) != int(total) || len(left) != int(leftTotal) {
		t.Fatalf("warehouse units: all %d (%d rows), left %d (%d rows); want 9 and 7",
			total, len(all), leftTotal, len(left))
	}
	for _, r := range all {
		if !strings.HasPrefix(r.Barcode, fixturePrefix) {
			t.Fatalf("unit %s is not a fixture unit", r.Barcode)
		}
	}
	var used *stockmodel.WarehouseUnitRow
	for i := range left {
		if left[i].InWarehouse {
			t.Fatalf("left row %s is in the warehouse", left[i].Barcode)
		}
		if left[i].Service != nil && used == nil {
			used = &left[i]
		}
	}
	if used == nil || used.Service.CustomerName == "" {
		t.Fatalf("no used unit with its service among %d left units", len(left))
	}
	u, err := q.GetUnitByUUID(ctx, used.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := svc.Trace(ctx, scopefilter.Filter{Scope: rbac.ScopeAll}, u)
	if err != nil || tr == nil || len(tr.Services) == 0 || tr.OriginWarehouse == nil || tr.OriginWarehouse.UUID != whUUID {
		t.Fatalf("trace of %s = %+v, %v", u.Barcode, tr, err)
	}
}
