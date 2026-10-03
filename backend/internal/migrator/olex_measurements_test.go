package migrator

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// TEC-262 acceptance over the legacy fixture (2 NexPTG accounts, 2 reports
// with 3 + 3 measurement rows, report 1 linked to service 1, report 2
// without a VIN). The run happens in one transaction that is rolled back,
// like the services test it builds on.
//
//   - 1 report -> 1 measurement_results row; raw holds every measurement row;
//   - the VIN-less report is vin_pending, the linked one carries the
//     service and its vehicle;
//   - each account becomes a measurement_devices row;
//   - every legacy row is in migration_map, no outbox row is written;
//   - a second run creates nothing.
func TestOlexMeasurements(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool
	hubSys := e.system
	whSys := hubSys + "w"

	var taken int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM units u JOIN brands b ON b.id = u.brand_id AND b.slug = 'olex'
		         WHERE u.barcode LIKE 'SYN-%')
		     + (SELECT COUNT(*) FROM products p JOIN brands b ON b.id = p.brand_id AND b.slug = 'olex' WHERE p.sku = ANY($1))
		     + (SELECT COUNT(*) FROM services WHERE service_no LIKE 'SYN-S-%' OR service_no LIKE $2)`,
		fixtureSKUs, LegacyServiceNoPrefix+"%").Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture units / products / services already exist in the test database", taken)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	e.runner.Pool = tx
	e.runner.Storage = storage.NewMemory()
	e.runner.LegacyFiles = fstest.MapFS{
		"app/public/services/syn-s-0001/1.jpg": {Data: fixtureJPEG},
		"app/public/services/syn-s-0001/2.jpg": {Data: append([]byte{}, append(fixtureJPEG, 1)...)},
		"public/services/syn-s-0002/1.jpg":     {Data: append([]byte{}, append(fixtureJPEG, 2)...)},
	}
	e.runner.Profiles["olexfx-base"] = Profile{
		Name: "olexfx-base", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step {
			return []Step{
				OrganizationsStep{System: hubSys},
				UsersStep{System: hubSys},
				CustomersStep{System: hubSys},
				VehicleCatalogStep{System: hubSys},
				CatalogStep{System: hubSys, WHSystem: whSys},
				WarehousesStep{WHSystem: whSys},
				UnitsStep{System: hubSys, WHSystem: whSys},
				ServicesStep{System: hubSys},
			}
		},
	}
	e.runner.Profiles["olexfx-measurements"] = Profile{
		Name: "olexfx-measurements", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step { return []Step{MeasurementsStep{System: hubSys}} },
	}

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	// pg_stat counters of this schema's tables only: the legacy fixture has
	// tables of the same name in legacy_hub.
	inserts := func(table string) int64 {
		return scalar(`SELECT COALESCE((SELECT n_tup_ins FROM pg_stat_xact_user_tables
			WHERE schemaname = 'public' AND relname = $1), 0)`, table)
	}
	const results = `SELECT r.id FROM measurement_results r JOIN migration_map m ON m.target_uuid = r.uuid
		AND m.source_system = $1 AND m.source_table = 'nexptg_reports'`
	const devices = `SELECT d.id FROM measurement_devices d JOIN migration_map m ON m.target_uuid = d.uuid
		AND m.source_system = $1 AND m.source_table = 'nexptg_api_users'`
	type rowCounts struct{ Results, Devices, Maps int64 }
	countRows := func() rowCounts {
		t.Helper()
		var c rowCounts
		if err := tx.QueryRow(ctx, `
			SELECT (SELECT COUNT(*) FROM measurement_results WHERE id IN (`+results+`)),
			       (SELECT COUNT(*) FROM measurement_devices WHERE id IN (`+devices+`)),
			       (SELECT COUNT(*) FROM migration_map WHERE source_system = $1)`, hubSys).
			Scan(&c.Results, &c.Devices, &c.Maps); err != nil {
			t.Fatal(err)
		}
		return c
	}

	e.run("olexfx-base", Options{})
	outboxBefore := inserts("outbox_events")
	resultsBefore, devicesBefore := inserts("measurement_results"), inserts("measurement_devices")

	first := e.run("olexfx-measurements", Options{})
	mc := stepCounts(t, first, "measurements")
	for k, want := range map[string]int64{
		"devices_read": 2, "devices_created": 2, "devices_inactive": 1, "device_serial_from_username": 1,
		"reports_read": 2, "reports_created": 2, "measurements_read": 6, "vin_pending": 1,
	} {
		if mc[k] != want {
			t.Errorf("measurements.%s = %d, want %d (%v)", k, mc[k], want, mc)
		}
	}
	for k := range mc {
		if strings.Contains(k, "skipped") || strings.Contains(k, "unmapped") || strings.Contains(k, "taken") ||
			strings.Contains(k, "invalid") || strings.Contains(k, "multiple") {
			t.Errorf("unexpected report %s (%v)", k, mc)
		}
	}
	if n := inserts("measurement_results") - resultsBefore; n != 2 {
		t.Errorf("measurement_results inserted = %d, want 2", n)
	}
	before := countRows()
	if before.Results != 2 || before.Devices != 2 {
		t.Errorf("rows after first run = %+v", before)
	}

	dealerOrg := func(id string) int64 {
		return scalar(`SELECT o.id FROM organizations o JOIN migration_map m ON m.target_uuid = o.uuid
			WHERE m.source_system = $1 AND m.source_table = 'dealers' AND m.source_id = $2`, hubSys, id)
	}
	hubUser := func(id string) int64 {
		return scalar(`SELECT u.id FROM users u JOIN migration_map m ON m.target_uuid = u.uuid
			WHERE m.source_system = $1 AND m.source_table = 'users' AND m.source_id = $2`, hubSys, id)
	}
	result := func(reportID string) (status, vin, serial string, org, service, vehicle, createdBy, rows int64, source string) {
		t.Helper()
		var vinN, serialN *string
		var serviceN, vehicleN, createdByN *int64
		if err := tx.QueryRow(ctx, `
			SELECT r.status, r.vin, r.device_serial, r.organization_id, r.service_id, r.vehicle_id, r.created_by,
			       jsonb_array_length(r.raw->'measurements'), r.source
			FROM measurement_results r JOIN migration_map m ON m.target_uuid = r.uuid
			WHERE m.source_system = $1 AND m.source_table = 'nexptg_reports' AND m.source_id = $2`, hubSys, reportID).
			Scan(&status, &vinN, &serialN, &org, &serviceN, &vehicleN, &createdByN, &rows, &source); err != nil {
			t.Fatalf("report %s: %v", reportID, err)
		}
		deref := func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		}
		derefN := func(p *int64) int64 {
			if p == nil {
				return 0
			}
			return *p
		}
		return status, deref(vinN), deref(serialN), org, derefN(serviceN), derefN(vehicleN), derefN(createdByN), rows, source
	}

	// Report 1: VIN, linked to service 1 and its vehicle, three rows.
	service1 := scalar(`SELECT s.id FROM services s JOIN migration_map m ON m.target_uuid = s.uuid
		WHERE m.source_system = $1 AND m.source_table = 'services' AND m.source_id = '1'`, hubSys)
	vehicle1 := scalar(`SELECT vehicle_id FROM services WHERE id = $1`, service1)
	status, vin, serial, org, service, vehicle, _, rows, src := result("1")
	if status != "accepted" || vin != "SYNVIN00000000001" || serial != "SYN-SN-0001" || rows != 3 || src != "legacy_import" {
		t.Errorf("report 1 = %s %s %s rows=%d %s", status, vin, serial, rows, src)
	}
	if service != service1 || vehicle != vehicle1 || vehicle == 0 || org != dealerOrg("1") {
		t.Errorf("report 1 service/vehicle/org = %d/%d/%d, want %d/%d/%d", service, vehicle, org, service1, vehicle1, dealerOrg("1"))
	}
	// Report 2: no VIN -> vin_pending; no service; its user's dealer.
	status, vin, _, org, service, vehicle, createdBy, rows, _ := result("2")
	if status != "vin_pending" || vin != "" || rows != 3 {
		t.Errorf("report 2 = %s %q rows=%d", status, vin, rows)
	}
	if service != 0 || vehicle != 0 || org != dealerOrg("1") || createdBy != hubUser("3") {
		t.Errorf("report 2 service/vehicle/org/created_by = %d/%d/%d/%d", service, vehicle, org, createdBy)
	}
	// raw row count equals the source.
	if n := scalar(`SELECT SUM(jsonb_array_length(raw->'measurements')) FROM measurement_results WHERE id IN (`+results+`)`, hubSys); n != 6 {
		t.Errorf("raw measurement rows = %d, want 6", n)
	}

	// Devices: account 1 takes its reports' serial, account 2 (no reports)
	// its username; each in its user's dealer.
	if n := scalar(`SELECT COUNT(*) FROM measurement_devices WHERE id IN (`+devices+`)
		AND ((serial = 'SYN-SN-0001' AND label = 'syn-device-1' AND organization_id = $2)
		  OR (serial = 'syn-device-2' AND label = 'syn-device-2' AND organization_id = $3))`,
		hubSys, dealerOrg("1"), dealerOrg("2")); n != 2 {
		t.Errorf("devices as expected = %d, want 2", n)
	}

	// Every legacy row is mapped: 2 accounts, 2 reports, 6 rows, 1 link.
	for table, want := range map[string]int64{
		"nexptg_api_users": 2, "nexptg_reports": 2, "nexptg_report_measurements": 6, "service_nexptg_report": 1,
	} {
		if n := scalar(`SELECT COUNT(*) FROM migration_map WHERE source_system = $1 AND source_table = $2`, hubSys, table); n != want {
			t.Errorf("migration_map %s = %d, want %d", table, n, want)
		}
	}
	// The cutover delta report would not list them.
	if n := scalar(`SELECT COUNT(*) FROM measurement_results r WHERE r.source = 'legacy_import'
		AND r.organization_id IN ($1, $2) AND NOT EXISTS (SELECT 1 FROM migration_map mm
		WHERE mm.target_table = 'measurement_results' AND mm.target_uuid = r.uuid)`, dealerOrg("1"), dealerOrg("2")); n != 0 {
		t.Errorf("unmapped legacy results = %d", n)
	}
	if n := inserts("outbox_events") - outboxBefore; n != 0 {
		t.Errorf("outbox_events inserted = %d, want 0", n)
	}

	// Second run: nothing new.
	second := e.run("olexfx-measurements", Options{})
	rc := stepCounts(t, second, "measurements")
	for k, v := range rc {
		if v != 0 && (strings.HasSuffix(k, "_created") || strings.HasSuffix(k, "_updated") || strings.HasSuffix(k, "_linked")) {
			t.Errorf("rerun measurements.%s = %d, want 0", k, v)
		}
	}
	if rc["reports_unchanged"] != 2 || rc["devices_unchanged"] != 2 {
		t.Errorf("rerun counts = %v", rc)
	}
	if after := countRows(); after != before {
		t.Errorf("rerun changed rows: %+v -> %+v", before, after)
	}
	if n := inserts("measurement_results") - resultsBefore; n != 2 {
		t.Errorf("rerun measurement_results inserted = %d, want 2 in total", n)
	}
	if n := inserts("measurement_devices") - devicesBefore; n != 2 {
		t.Errorf("rerun measurement_devices inserted = %d, want 2 in total", n)
	}
	if n := inserts("outbox_events") - outboxBefore; n != 0 {
		t.Errorf("rerun outbox_events inserted = %d", n)
	}
}
