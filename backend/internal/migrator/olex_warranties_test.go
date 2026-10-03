package migrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	warrantyhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/handler"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
)

// TEC-260 acceptance. The base steps run on the legacy fixture; services and
// warranties run on a copy of the hub's service tables with one extra
// service (SYN-S-0004: the same vehicle and stock item as SYN-S-0001, a
// later start), so the copy holds both duplicate kinds:
//
//   - warranties 1 and 2 (same service item) become one warranty;
//   - warranty 5 (same vehicle and unit, another service) is merged into it
//     and SYN-S-0004 stays resolvable as an alias;
//   - warranty 4 (service still processing) is not imported;
//   - the legacy service numbers answer GET /v1/public/warranties/{no};
//   - the completed transfer of SYN-S-0001 is a completed vehicle transfer
//     to the new customer, who now owns the vehicle and holds the warranty;
//   - nothing is written to the outbox and a second run creates nothing.
func TestOlexWarranties(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool
	hubSys := e.system
	whSys := hubSys + "w"

	var taken int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM units u JOIN brands b ON b.id = u.brand_id AND b.slug = 'olex'
		         WHERE u.barcode LIKE 'SYN-%')
		     + (SELECT COUNT(*) FROM services WHERE service_no LIKE 'SYN-S-%' OR service_no LIKE $1)
		     + (SELECT COUNT(*) FROM warranties WHERE public_code LIKE 'SYN-S-%')`,
		LegacyServiceNoPrefix+"%").Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture units / services / warranties already exist in the test database", taken)
	}

	// The copy of the service tables (committed: the source reads through
	// the pool), dropped at the end.
	schema := "legacy_hub_" + hubSys
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`CREATE SCHEMA ` + schema)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) })
	for _, table := range []string{"services", "service_items", "service_images", "service_status_logs",
		"warranties", "service_customer_transfers"} {
		exec(`CREATE TABLE ` + schema + `.` + table + ` AS TABLE legacy_hub.` + table)
	}
	exec(`INSERT INTO ` + schema + `.services (id, service_no, dealer_id, customer_id, user_id, car_brand_id, car_model_id,
		year, vin, plate, plate_country, km, package, applied_parts, notes, status, completed_at, review_request_sms_sent_at,
		created_at, updated_at)
		SELECT 4, 'SYN-S-0004', dealer_id, customer_id, user_id, car_brand_id, car_model_id, year, vin, plate, plate_country,
		       km, package, applied_parts, notes, 'completed', '2025-06-01 17:00:00', NULL, '2025-06-01 09:00:00',
		       '2025-06-01 17:00:00'
		FROM ` + schema + `.services WHERE id = 1`)
	exec(`INSERT INTO ` + schema + `.service_items (id, service_id, stock_item_id, usage_type, notes, created_at, updated_at)
		VALUES (4, 4, 3, 'full', NULL, '2025-06-01 09:30:00', '2025-06-01 09:30:00')`)
	exec(`INSERT INTO ` + schema + `.warranties (id, service_id, stock_item_id, start_date, end_date, is_active, created_at, updated_at)
		VALUES (5, 4, 3, '2025-06-01', '2030-06-01', true, '2025-06-01 17:00:00', '2025-06-01 17:00:00')`)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	e.runner.Pool = tx
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
			}
		},
	}
	e.runner.Profiles["olexfx-warranties"] = Profile{
		Name: "olexfx-warranties", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step { return []Step{ServicesStep{System: hubSys}, WarrantiesStep{System: hubSys}} },
	}
	e.run("olexfx-base", Options{})
	e.runner.Open = func(_ context.Context, name string) (source.LegacySource, error) {
		if name == SourceHub {
			return source.NewPostgres(name, pool, schema)
		}
		return source.NewPostgres(name, pool, source.FixtureSchemas[name])
	}

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	outboxInserts := func() int64 {
		return scalar(`SELECT COALESCE((SELECT n_tup_ins FROM pg_stat_xact_user_tables WHERE schemaname = 'public' AND relname = 'outbox_events'), 0)`)
	}
	const migrated = `SELECT s.id FROM services s JOIN migration_map m ON m.target_uuid = s.uuid
		AND m.source_system = $1 AND m.source_table = 'services'`
	type rows struct{ Warranties, Aliases, Transfers, Maps int64 }
	rowCounts := func() rows {
		t.Helper()
		var c rows
		if err := tx.QueryRow(ctx, `
			SELECT (SELECT COUNT(*) FROM warranties WHERE service_id IN (`+migrated+`)),
			       (SELECT COUNT(*) FROM warranty_public_code_aliases a JOIN warranties w ON w.id = a.warranty_id
			         WHERE w.service_id IN (`+migrated+`)),
			       (SELECT COUNT(*) FROM vehicle_transfers vt JOIN services s ON s.vehicle_id = vt.vehicle_id
			         WHERE s.id IN (`+migrated+`) AND s.service_no = 'SYN-S-0001'),
			       (SELECT COUNT(*) FROM migration_map WHERE source_system = $1)`, hubSys).
			Scan(&c.Warranties, &c.Aliases, &c.Transfers, &c.Maps); err != nil {
			t.Fatal(err)
		}
		return c
	}
	outboxBefore := outboxInserts()

	first := e.run("olexfx-warranties", Options{})
	wc := stepCounts(t, first, "warranties")
	for k, want := range map[string]int64{
		"warranties_read": 5, "warranties_created": 2, "warranties_active": 2, "public_codes_kept": 2,
		"warranties_merged": 2, "warranty_merged_unit": 1, "warranty_skipped_service_not_completed": 1,
		"aliases_created": 1,
		"transfers_read":  1, "transfers_created": 1, "vehicles_moved": 1, "warranty_holders_moved": 1,
	} {
		if wc[k] != want {
			t.Errorf("warranties.%s = %d, want %d (%v)", k, wc[k], want, wc)
		}
	}
	for k := range wc {
		if strings.HasPrefix(k, "transfer_skipped") || strings.HasPrefix(k, "public_code_") ||
			strings.HasPrefix(k, "warranty_skipped_item") || strings.HasPrefix(k, "warranty_linked") {
			t.Errorf("unexpected report %s (%v)", k, wc)
		}
	}
	before := rowCounts()
	if before.Warranties != 2 || before.Aliases != 1 || before.Transfers != 1 {
		t.Errorf("rows after first run = %+v", before)
	}

	// Every migrated warranty and transfer is a migration_map target (the
	// cutover delta report does not list it).
	if n := scalar(`SELECT COUNT(*) FROM warranties w WHERE w.service_id IN (`+migrated+`)
		AND NOT EXISTS (SELECT 1 FROM migration_map mm WHERE mm.target_table = 'warranties' AND mm.target_uuid = w.uuid)`, hubSys); n != 0 {
		t.Errorf("unmapped warranties = %d", n)
	}
	if n := scalar(`SELECT COUNT(*) FROM vehicle_transfers vt
		WHERE vt.uuid IN (SELECT target_uuid FROM migration_map WHERE source_system = $1 AND target_table = 'vehicle_transfers')`, hubSys); n != 1 {
		t.Errorf("mapped transfers = %d, want 1", n)
	}
	// Legacy warranties 1, 2 and 5 all map to the one kept warranty.
	if n := scalar(`SELECT COUNT(DISTINCT w.id) FROM migration_map mm JOIN warranties w ON w.uuid = mm.target_uuid
		WHERE mm.source_system = $1 AND mm.source_table = 'warranties' AND mm.source_id IN ('1', '2', '5')`, hubSys); n != 1 {
		t.Errorf("warranties of legacy 1/2/5 = %d, want 1", n)
	}

	// The kept warranty: legacy number as public code, the earliest start
	// (a day in the organization's zone), active, no reminders stamped.
	var (
		start            string
		status           string
		notified         bool
		holder, kept     int64
		vehicleOwner     int64
		customer1, cust7 int64
	)
	if err := tx.QueryRow(ctx, `
		SELECT w.id, to_char(w.start_at AT TIME ZONE o.timezone, 'YYYY-MM-DD HH24:MI'), w.status,
		       (w.notified_30_at IS NOT NULL OR w.notified_7_at IS NOT NULL), w.holder_user_id, v.user_id
		FROM warranties w JOIN organizations o ON o.id = w.organization_id JOIN vehicles v ON v.id = w.vehicle_id
		WHERE w.public_code = 'SYN-S-0001'`).Scan(&kept, &start, &status, &notified, &holder, &vehicleOwner); err != nil {
		t.Fatalf("warranty SYN-S-0001: %v", err)
	}
	if start != "2025-03-10 00:00" || status != "active" || notified {
		t.Errorf("SYN-S-0001 = start %s status %s notified %v", start, status, notified)
	}
	customerUser := func(id string) int64 {
		return scalar(`SELECT u.id FROM users u JOIN migration_map m ON m.target_uuid = u.uuid
			WHERE m.source_system = $1 AND m.source_table = 'customers' AND m.source_id = $2`, hubSys, id)
	}
	customer1, cust7 = customerUser("1"), customerUser("7")
	if holder != cust7 || vehicleOwner != cust7 {
		t.Errorf("holder %d / vehicle owner %d, want the new customer %d", holder, vehicleOwner, cust7)
	}
	if n := scalar(`SELECT COUNT(*) FROM warranty_public_code_aliases WHERE code = 'SYN-S-0004' AND warranty_id = $1
		AND reason = 'merged'`, kept); n != 1 {
		t.Errorf("alias SYN-S-0004 -> %d missing", kept)
	}

	// Transfer history: completed, from customer 1 to customer 7 (the
	// receiver), with the receiver's phone and no usable code.
	var from, to int64
	var phone, trStatus, hash string
	var verified bool
	if err := tx.QueryRow(ctx, `
		SELECT vt.from_user_id, vt.to_user_id, vt.to_phone, vt.status, vt.from_code_hash,
		       (vt.from_verified_at IS NOT NULL AND vt.to_verified_at IS NOT NULL AND vt.completed_at = '2025-04-01 12:00:00+00')
		FROM vehicle_transfers vt
		WHERE vt.uuid = (SELECT target_uuid FROM migration_map WHERE source_system = $1
		                 AND source_table = 'service_customer_transfers' AND source_id = '1')`, hubSys).
		Scan(&from, &to, &phone, &trStatus, &hash, &verified); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if from != customer1 || to != cust7 || phone != "+905419876543" || trStatus != "completed" || !verified ||
		hash != legacyTransferCodeHash {
		t.Errorf("transfer = from %d to %d phone %s status %s hash %s verified %v (customers %d -> %d)",
			from, to, phone, trStatus, hash, verified, customer1, cust7)
	}

	// The public endpoint answers the legacy numbers (kept and alias).
	brand, err := db.New(tx).GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h := warrantyhandler.NewPublic(warrantyusecase.NewPublicLookup(db.New(tx)), nil, 0, time.Minute)
	mux.HandleFunc("GET /v1/public/warranties/{public_code}", h.Get)
	get := func(code string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/public/warranties/"+code, nil)
		req = req.WithContext(brandctx.WithBrand(req.Context(),
			brandctx.Brand{ID: brand.ID, Slug: brand.Slug, Name: brand.Name, Status: "active"}))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var body struct {
			Data struct {
				PublicCode string `json:"public_code"`
				Status     string `json:"status"`
			} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body.Data.PublicCode
	}
	for code, want := range map[string]string{"SYN-S-0001": "SYN-S-0001", "SYN-S-0002": "SYN-S-0002", "SYN-S-0004": "SYN-S-0001"} {
		if st, got := get(code); st != http.StatusOK || got != want {
			t.Errorf("GET %s = %d %q, want 200 %q", code, st, got, want)
		}
	}
	for _, code := range []string{"SYN-S-0003", "SYN-S-9999"} {
		if st, _ := get(code); st != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", code, st)
		}
	}

	if n := outboxInserts() - outboxBefore; n != 0 {
		t.Errorf("outbox_events inserted = %d, want 0", n)
	}

	// Second run: nothing new.
	second := e.run("olexfx-warranties", Options{})
	rc := stepCounts(t, second, "warranties")
	for k, v := range rc {
		if v != 0 && (strings.HasSuffix(k, "_created") || strings.HasSuffix(k, "_updated") ||
			strings.HasSuffix(k, "_moved") || k == "warranties_merged") {
			t.Errorf("rerun warranties.%s = %d, want 0", k, v)
		}
	}
	if rc["warranties_unchanged"] != 3 || rc["aliases_unchanged"] != 1 || rc["transfers_unchanged"] != 1 {
		t.Errorf("rerun counts = %v", rc)
	}
	if after := rowCounts(); after != before {
		t.Errorf("rerun changed rows: %+v -> %+v", before, after)
	}
	if n := outboxInserts() - outboxBefore; n != 0 {
		t.Errorf("rerun outbox_events inserted = %d", n)
	}
}
