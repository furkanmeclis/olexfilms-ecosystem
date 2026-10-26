package migrator

import (
	"context"
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

// TEC-1003 over the legacy fixture, as on a database the old migrator
// filled: the dealers sit under a Türkiye distributor (with the TR
// territory) that sold them the orders. A run with the service point dealer
// set merges the distributor and the service point dealer (hub dealer 1)
// into the center; a rerun changes nothing; undoing both merges puts every
// row back. Everything runs in one rolled back transaction.
func TestOlexServicePointMerge(t *testing.T) {
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

	steps := func(servicePoint int64) []Step {
		return []Step{
			OrganizationsStep{System: hubSys, ServicePointDealerID: servicePoint},
			UsersStep{System: hubSys},
			CustomersStep{System: hubSys},
			VehicleCatalogStep{System: hubSys},
			CatalogStep{System: hubSys, WHSystem: whSys},
			WarehousesStep{WHSystem: whSys, PrimaryCode: "SYN-WH-1", PrimaryName: "Sentetik Ana Depo"},
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
	e.runner.Profiles["olexfx-sp-before"] = Profile{Name: "olexfx-sp-before", Enabled: true,
		Sources: []string{SourceHub, SourceWH}, Steps: func() []Step { return steps(0) }}
	e.runner.Profiles["olexfx-sp"] = Profile{Name: "olexfx-sp", Enabled: true,
		Sources: []string{SourceHub, SourceWH}, Steps: func() []Step { return steps(1) }}

	// 1. The old shape: a full run, then the dealers under a TR distributor
	//    that sold them their orders.
	first := e.run("olexfx-sp-before", Options{})
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

	type counts struct{ Services, Warranties, Customers, Units, Members, Orders int64 }
	of := func(org int64) counts {
		t.Helper()
		return counts{
			Services:   scalar(`SELECT count(*) FROM services WHERE organization_id = $1`, org),
			Warranties: scalar(`SELECT count(*) FROM warranties WHERE organization_id = $1`, org),
			Customers:  scalar(`SELECT count(*) FROM customer_organizations WHERE organization_id = $1`, org),
			Units:      scalar(`SELECT count(*) FROM unit_current_state WHERE holder_org_id = $1`, org),
			Members:    scalar(`SELECT count(*) FROM organization_members WHERE organization_id = $1`, org),
			Orders:     scalar(`SELECT count(*) FROM orders WHERE buyer_org_id = $1`, org),
		}
	}
	dealerBefore, centerBefore := of(dealer1), of(centerID)
	if dealerBefore.Services == 0 || dealerBefore.Warranties == 0 || dealerBefore.Members == 0 || dealerBefore.Orders == 0 {
		t.Fatalf("fixture dealer 1 has too little to merge: %+v", dealerBefore)
	}
	ordersOfDist := scalar(`SELECT count(*) FROM orders WHERE seller_org_id = $1`, distID)
	if ordersOfDist == 0 {
		t.Fatal("no distributor orders")
	}

	// 2. The run with the service point merges both into the center.
	rep := e.run("olexfx-sp", Options{})
	oc := stepCounts(t, rep, "organizations")
	if oc["distributor_merged"] != 1 || oc["service_point_merged"] != 1 {
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
	dealerAfter, centerAfter := of(dealer1), of(centerID)
	if dealerAfter.Services != 0 || dealerAfter.Warranties != 0 || dealerAfter.Units != 0 || dealerAfter.Members != 0 {
		t.Errorf("dealer 1 after merge = %+v", dealerAfter)
	}
	// The orders the service point bought stay its history.
	if dealerAfter.Orders != dealerBefore.Orders {
		t.Errorf("dealer 1 orders = %d, want %d kept", dealerAfter.Orders, dealerBefore.Orders)
	}
	if centerAfter.Services != centerBefore.Services+dealerBefore.Services ||
		centerAfter.Warranties != centerBefore.Warranties+dealerBefore.Warranties {
		t.Errorf("center after merge = %+v, before %+v + dealer %+v", centerAfter, centerBefore, dealerBefore)
	}
	var spName string
	var spOn bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(service_point_name, ''), service_point_enabled FROM organizations WHERE id = $1`, centerID).
		Scan(&spName, &spOn); err != nil {
		t.Fatal(err)
	}
	if spName != "Sentetik Bayi Bir" || !spOn {
		t.Errorf("center service point = %q %v", spName, spOn)
	}
	// Dealer 1's owner and staff are center members now, with the center
	// counterparts of their roles.
	for email, want := range map[string]string{"sahip1@example.test": "center_staff", "personel1@example.test": "center_service"} {
		var got string
		if err := tx.QueryRow(ctx, `SELECT string_agg(r.slug, ',') FROM organization_members om
			JOIN users u ON u.id = om.user_id
			JOIN organization_member_roles omr ON omr.member_id = om.id JOIN roles r ON r.id = omr.role_id
			WHERE u.email = $1 AND om.organization_id = $2`, email, centerID).Scan(&got); err != nil {
			t.Fatalf("%s: %v", email, err)
		}
		if got != want {
			t.Errorf("%s center roles = %q, want %s", email, got, want)
		}
	}

	// 3. A rerun resolves dealer 1 to the center and writes nothing new.
	servicesTotal := scalar(`SELECT count(*) FROM services`)
	warrantiesTotal := scalar(`SELECT count(*) FROM warranties`)
	rep = e.run("olexfx-sp", Options{})
	if n := stepCounts(t, rep, "unit_trace")["service_consumption_created"]; n != 0 {
		t.Errorf("rerun created %d service consumptions", n)
	}
	oc = stepCounts(t, rep, "organizations")
	if oc["distributor_merged"] != 0 || oc["service_point_merged"] != 0 {
		t.Errorf("rerun merged again: %v", oc)
	}
	if n := scalar(`SELECT count(*) FROM services`); n != servicesTotal {
		t.Errorf("rerun services %d -> %d", servicesTotal, n)
	}
	if n := scalar(`SELECT count(*) FROM warranties`); n != warrantiesTotal {
		t.Errorf("rerun warranties %d -> %d", warrantiesTotal, n)
	}
	if got := of(dealer1); got.Services != 0 || got.Members != 0 {
		t.Errorf("rerun wrote to the merged dealer: %+v", got)
	}

	// 4. Undo both merges (last first): dealer 1 has its rows back.
	rows, err := tx.Query(ctx, `SELECT uuid::text FROM org_merge_runs WHERE source_org_id = ANY ($1) ORDER BY id DESC`,
		[]int64{distID, dealer1})
	if err != nil {
		t.Fatal(err)
	}
	var runs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, s)
	}
	rows.Close()
	if len(runs) != 2 {
		t.Fatalf("merge runs = %v", runs)
	}
	for _, r := range runs {
		if _, err := orgmerge.Undo(ctx, tx, r); err != nil {
			t.Fatalf("undo %s: %v", r, err)
		}
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
	if n := scalar(`SELECT count(*) FROM organizations WHERE id = ANY ($1) AND (deleted_at IS NOT NULL OR merged_into_id IS NOT NULL)`,
		[]int64{distID, dealer1}); n != 0 {
		t.Errorf("%d organizations still merged after undo", n)
	}
	if _, err := orgmerge.Undo(ctx, tx, runs[0]); err == nil {
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
	all, total, err := svc.WarehouseUnits(ctx, orgID, whUUID, stockusecase.WarehouseUnitFilter{
		Sort: apiquery.ResolvedSort{Key: "barcode"}, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	left, leftTotal, err := svc.WarehouseUnits(ctx, orgID, whUUID, stockusecase.WarehouseUnitFilter{
		State: stockusecase.WarehouseUnitLeft, Sort: apiquery.ResolvedSort{Key: "barcode"}, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	page := func(n int64) int { return int(min(n, 100)) }
	if total == 0 || leftTotal == 0 || len(all) != page(total) || len(left) != page(leftTotal) || leftTotal >= total {
		t.Fatalf("warehouse units: all %d, left %d", total, leftTotal)
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
