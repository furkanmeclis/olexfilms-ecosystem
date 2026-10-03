package migrator

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/legacyfixture"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// TEC-264 acceptance: the whole olex profile, in profile order, over a copy
// of the F2-01b legacy fixture. The run happens in one transaction that is
// rolled back (append-only ledger and status logs); the copies of the legacy
// schemas are committed (the sources read through the pool) and dropped.
//
//  1. a full run, then the report has no mismatch, and the expected
//     differences are classified (K26, duplicate warranties, external
//     reference orders, measurement rows in raw, Glorian out of scope);
//  2. a second full run inserts no row anywhere;
//  3. a new service and an updated customer in the legacy copy: the delta
//     run carries exactly these (one service, its vehicle, one updated
//     customer) and the report stays clean;
//  4. a deleted target row makes the report fail.
func TestOlexEndToEndFullRerunDelta(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool
	hubSys := e.system
	whSys := hubSys + "w"

	var taken int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM units u JOIN brands b ON b.id = u.brand_id AND b.slug = 'olex'
		         WHERE u.barcode LIKE 'SYN-%')
		     + (SELECT COUNT(*) FROM products p JOIN brands b ON b.id = p.brand_id AND b.slug = 'olex' WHERE p.sku = ANY($1))
		     + (SELECT COUNT(*) FROM services WHERE service_no LIKE 'SYN-S-%' OR service_no LIKE $2)
		     + (SELECT COUNT(*) FROM warranties WHERE public_code LIKE 'SYN-S-%')`,
		fixtureSKUs, LegacyServiceNoPrefix+"%").Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture units / products / services / warranties already exist in the test database", taken)
	}

	// Copies of both legacy schemas: the delta scenario edits them.
	hubSchema, whSchema := "legacy_hub_"+hubSys, "legacy_wh_"+hubSys
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	for _, s := range []string{hubSchema, whSchema} {
		exec(`CREATE SCHEMA ` + s)
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+s+` CASCADE`) })
	}
	for qualified := range legacyfixture.ExpectedRowCounts {
		schema, table, _ := strings.Cut(qualified, ".")
		dst := hubSchema
		if schema == legacyfixture.WHSchema {
			dst = whSchema
		}
		exec(`CREATE TABLE ` + dst + `.` + table + ` AS TABLE ` + qualified)
	}
	open := func(_ context.Context, name string) (source.LegacySource, error) {
		if name == SourceHub {
			return source.NewPostgres(name, pool, hubSchema)
		}
		return source.NewPostgres(name, pool, whSchema)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	const profile = "olexfx-e2e"
	e.runner.Pool = tx
	e.runner.Open = open
	e.runner.Storage = storage.NewMemory()
	e.runner.LegacyFiles = fstest.MapFS{
		"app/public/services/syn-s-0001/1.jpg": {Data: fixtureJPEG},
		"app/public/services/syn-s-0001/2.jpg": {Data: append([]byte{}, append(fixtureJPEG, 1)...)},
		"public/services/syn-s-0002/1.jpg":     {Data: append([]byte{}, append(fixtureJPEG, 2)...)},
	}
	e.runner.Profiles[profile] = Profile{
		Name: profile, Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step {
			return []Step{
				OrganizationsStep{System: hubSys},
				UsersStep{System: hubSys},
				CustomersStep{System: hubSys},
				VehicleCatalogStep{System: hubSys},
				CatalogStep{System: hubSys, WHSystem: whSys},
				WarehousesStep{WHSystem: whSys},
				UnitsStep{System: hubSys, WHSystem: whSys},
				LedgerStep{System: hubSys, WHSystem: whSys},
				OrdersStep{System: hubSys, WHSystem: whSys},
				ShortURLsStep{System: hubSys},
				LegacyMessagesStep{System: hubSys},
				ServicesStep{System: hubSys},
				WarrantiesStep{System: hubSys},
				MeasurementsStep{System: hubSys},
			}
		},
	}
	// The e2e profile runs the olex steps in the olex order.
	var names, olexNames []string
	for _, s := range e.runner.Profiles[profile].Steps() {
		names = append(names, s.Name())
	}
	for _, s := range olexSteps() {
		olexNames = append(olexNames, s.Name())
	}
	if strings.Join(names, ",") != strings.Join(olexNames, ",") {
		t.Fatalf("e2e steps %v, olex steps %v", names, olexNames)
	}

	report := func() *Report {
		t.Helper()
		srcs := Sources{}
		for _, name := range []string{SourceHub, SourceWH} {
			s, err := open(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			srcs[name] = s
		}
		rep, err := BuildReport(ctx, srcs, tx, ReportOptions{
			Profile: profile, Systems: map[string]string{SourceHub: hubSys, SourceWH: whSys},
		})
		if err != nil {
			t.Fatalf("report: %v", err)
		}
		var buf bytes.Buffer
		_ = WriteReportText(&buf, rep)
		t.Logf("report:\n%s", buf.String())
		return rep
	}
	tableOf := func(rep *Report, src, table string) TableReport {
		t.Helper()
		tr, ok := rep.Table(src, table)
		if !ok {
			t.Fatalf("report has no %s.%s", src, table)
		}
		return tr
	}
	// Rows inserted / updated so far by this transaction, per public table
	// (legacy_hub has tables of the same names).
	type stat struct{ Ins, Upd int64 }
	stats := func() map[string]stat {
		t.Helper()
		rows, err := tx.Query(ctx, `SELECT relname, n_tup_ins, n_tup_upd FROM pg_stat_xact_user_tables
			WHERE schemaname = 'public'`)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]stat{}
		for rows.Next() {
			var name string
			var s stat
			if err := rows.Scan(&name, &s.Ins, &s.Upd); err != nil {
				t.Fatal(err)
			}
			out[name] = s
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	inserted := func(before, after map[string]stat) map[string]int64 {
		out := map[string]int64{}
		for name, s := range after {
			if n := s.Ins - before[name].Ins; n != 0 {
				out[name] = n
			}
		}
		return out
	}

	// 1. Full run, clean report with the expected differences classified.
	e.run(profile, Options{})
	rep := report()
	if err := rep.Err(); err != nil {
		t.Fatalf("full run report: %v: %+v", err, rep.Mismatches)
	}
	for _, c := range []struct {
		src, table                        string
		source, merged, outOfScope, skipd int64
	}{
		{SourceHub, "customers", 8, 4, 0, 0},                  // K26: two pairs
		{SourceHub, "warranties", 4, 2, 0, 1},                 // duplicate; 4: service not completed
		{SourceHub, "orders", 3, 1, 0, 0},                     // external_reference
		{SourceWH, "orders", 2, 1, 0, 0},                      // external_reference
		{SourceHub, "nexptg_report_measurements", 6, 6, 0, 0}, // rows in raw
		{SourceHub, "short_urls", 4, 0, 0, 1},                 // external target
		{SourceWH, "products", 2, 1, 1, 0},                    // SKU match; Glorian
		{SourceWH, "product_barcodes", 4, 1, 2, 0},            // barcode match; Glorian
		{SourceWH, "stock_movements", 5, 0, 3, 0},             // Glorian
		{SourceWH, "order_items", 2, 0, 1, 0},                 // Glorian
		{SourceHub, "service_status_logs", 3, 0, 0, 0},
	} {
		tr := tableOf(rep, c.src, c.table)
		if tr.SourceRows != c.source || tr.Merged != c.merged || tr.OutOfScope != c.outOfScope || tr.Skipped != c.skipd ||
			tr.Migrated+tr.OutOfScope+tr.Skipped != tr.SourceRows {
			t.Errorf("%s.%s = %+v, want source %d merged %d out_of_scope %d skipped %d",
				c.src, c.table, tr, c.source, c.merged, c.outOfScope, c.skipd)
		}
	}
	if tr := tableOf(rep, SourceHub, "customers"); tr.TargetRows != 6 {
		t.Errorf("customers target rows = %d, want 6 (8 merged into 6)", tr.TargetRows)
	}

	// 2. Second full run: no row inserted anywhere but the run log.
	before := stats()
	second := e.run(profile, Options{})
	for name, n := range inserted(before, stats()) {
		if name != "migration_runs" {
			t.Errorf("second full run inserted %d rows into %s", n, name)
		}
	}
	for _, s := range second.Steps {
		for k, v := range s.Counts {
			if v != 0 && (strings.HasSuffix(k, "_created") || k == cntCreated) {
				t.Errorf("second full run %s.%s = %d", s.Name, k, v)
			}
		}
	}

	// 3. Delta: one new legacy service, one updated customer.
	q := db.New(tx)
	watermark := func(step string) pgtype.Timestamptz {
		t.Helper()
		wm, err := q.LastMigrationWatermark(ctx, db.LastMigrationWatermarkParams{
			Profile: profile, Step: pgtype.Text{String: step, Valid: true}})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		return wm
	}
	servicesWM := watermark("services")
	if !servicesWM.Valid {
		t.Fatal("services step left no watermark")
	}
	exec(`INSERT INTO ` + hubSchema + `.services (id, service_no, dealer_id, customer_id, user_id, car_brand_id, car_model_id,
		year, vin, plate, plate_country, km, package, applied_parts, notes, status, completed_at, review_request_sms_sent_at,
		created_at, updated_at)
		SELECT 5, 'SYN-S-0005', dealer_id, customer_id, user_id, car_brand_id, car_model_id, year, 'SYNVIN00000000005',
		       '34 SYN 005', plate_country, km, package, applied_parts, NULL, 'processing', NULL, NULL,
		       LOCALTIMESTAMP(0), LOCALTIMESTAMP(0)
		FROM ` + hubSchema + `.services WHERE id = 1`)
	// A changed customer only fills what the account lacks (values set in
	// the new app win, TEC-255): customer 3 gets the e-mail it never had.
	const newEmail = "musteri3-tec264@example.test"
	exec(`UPDATE `+hubSchema+`.customers SET email = $1, updated_at = LOCALTIMESTAMP(0) WHERE id = 3`, newEmail)

	before = stats()
	delta := e.run(profile, Options{Mode: ModeDelta})
	after := stats()
	ins := inserted(before, after)
	for name, n := range ins {
		switch name {
		case "migration_runs", "migration_map":
		case "services", "vehicles":
			if n != 1 {
				t.Errorf("delta inserted %d rows into %s, want 1", n, name)
			}
		default:
			t.Errorf("delta inserted %d rows into %s", n, name)
		}
	}
	if ins["services"] != 1 || ins["vehicles"] != 1 {
		t.Errorf("delta inserts = %v, want one service and its vehicle", ins)
	}
	if n := after["users"].Upd - before["users"].Upd; n < 1 {
		t.Errorf("delta updated %d users, want the changed customer", n)
	}
	var created []string
	for _, s := range delta.Steps {
		for k, v := range s.Counts {
			if v != 0 && (strings.HasSuffix(k, "_created") || strings.HasSuffix(k, "_updated") ||
				k == cntCreated || k == cntUpdated) {
				created = append(created, s.Name+"."+k)
			}
		}
	}
	sort.Strings(created)
	if got := strings.Join(created, ","); got != "customers.updated,services.services_created,services.vehicles_created" {
		t.Errorf("delta created / updated = %s", got)
	}
	if wm := watermark("services"); !wm.Valid || !wm.Time.After(servicesWM.Time) {
		t.Errorf("services watermark %v did not move past %v", wm, servicesWM)
	}
	var email pgtype.Text
	if err := tx.QueryRow(ctx, `SELECT u.email FROM users u JOIN migration_map m ON m.target_uuid = u.uuid
		WHERE m.source_system = $1 AND m.source_table = 'customers' AND m.source_id = '3'`, hubSys).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email.String != newEmail {
		t.Errorf("customer 3 e-mail = %q, want %q", email.String, newEmail)
	}
	rep = report()
	if err := rep.Err(); err != nil {
		t.Fatalf("delta report: %v: %+v", err, rep.Mismatches)
	}
	if tr := tableOf(rep, SourceHub, "services"); tr.SourceRows != 4 || tr.Migrated != 4 {
		t.Errorf("services after delta = %+v, want 4 migrated", tr)
	}

	// A delta run with nothing new writes nothing.
	before = stats()
	e.run(profile, Options{Mode: ModeDelta})
	for name, n := range inserted(before, stats()) {
		if name != "migration_runs" {
			t.Errorf("idle delta inserted %d rows into %s", n, name)
		}
	}

	// 4. A deleted target row fails the report.
	exec2 := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec2(`DELETE FROM short_urls WHERE uuid = (SELECT target_uuid FROM migration_map
		WHERE source_system = $1 AND source_table = 'short_urls' AND source_id = '1')`, hubSys)
	rep = report()
	if err := rep.Err(); !errors.Is(err, ErrReportMismatch) {
		t.Fatalf("report after a deleted target = %v, want ErrReportMismatch", err)
	}
	if len(rep.Mismatches) != 1 || rep.Mismatches[0].Table != "short_urls" || rep.Mismatches[0].ID != "1" ||
		rep.Mismatches[0].Class != MismatchTargetMissing {
		t.Errorf("mismatches = %+v, want short_urls 1 target_missing", rep.Mismatches)
	}
}
