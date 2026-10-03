package migrator

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// fixtureJPEG is a JPEG signature plus filler: enough for content sniffing.
var fixtureJPEG = append([]byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), bytes.Repeat([]byte{0}, 32)...)

// serviceRows are the rows the services step owns, for the rerun check.
type serviceRows struct{ Services, Items, Images, Logs, Vehicles, Maps int64 }

// TEC-259 acceptance over the legacy fixture. Like the ledger test the whole
// run happens in one transaction that is rolled back: service_status_logs is
// append-only, so a committed run could not be cleaned up.
//
//   - services, items, images and logs match the fixture;
//   - the services of the two dealers belong to the same (merged) user;
//   - the images land in the (fake) object store;
//   - no warranty and no outbox row is written;
//   - a second run creates nothing.
func TestOlexServices(t *testing.T) {
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
	store := storage.NewMemory()
	e.runner.Pool = tx
	e.runner.Storage = store
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
			}
		},
	}
	e.runner.Profiles["olexfx-services"] = Profile{
		Name: "olexfx-services", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step { return []Step{ServicesStep{System: hubSys}} },
	}

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	const migrated = `SELECT s.id FROM services s JOIN migration_map m ON m.target_uuid = s.uuid
		AND m.source_system = $1 AND m.source_table = 'services'`
	rowCounts := func() serviceRows {
		t.Helper()
		var c serviceRows
		if err := tx.QueryRow(ctx, `
			SELECT (SELECT COUNT(*) FROM services WHERE id IN (`+migrated+`)),
			       (SELECT COUNT(*) FROM service_items WHERE service_id IN (`+migrated+`)),
			       (SELECT COUNT(*) FROM service_images WHERE service_id IN (`+migrated+`)),
			       (SELECT COUNT(*) FROM service_status_logs WHERE service_id IN (`+migrated+`)),
			       (SELECT COUNT(DISTINCT vehicle_id) FROM services WHERE id IN (`+migrated+`)),
			       (SELECT COUNT(*) FROM migration_map WHERE source_system = $1)`, hubSys).
			Scan(&c.Services, &c.Items, &c.Images, &c.Logs, &c.Vehicles, &c.Maps); err != nil {
			t.Fatal(err)
		}
		return c
	}
	outboxInserts := func() int64 {
		return scalar(`SELECT COALESCE((SELECT n_tup_ins FROM pg_stat_xact_user_tables WHERE schemaname = 'public' AND relname = 'outbox_events'), 0)`)
	}
	warrantyInserts := func() int64 {
		return scalar(`SELECT COALESCE((SELECT n_tup_ins FROM pg_stat_xact_user_tables WHERE schemaname = 'public' AND relname = 'warranties'), 0)`)
	}

	e.run("olexfx-base", Options{})
	outboxBefore, warrantiesBefore := outboxInserts(), warrantyInserts()

	first := e.run("olexfx-services", Options{})
	sc := stepCounts(t, first, "services")
	for k, want := range map[string]int64{
		"services_read": 3, "services_created": 3,
		"items_read": 3, "items_created": 3, "item_partial_as_full": 1,
		"images_read": 3, "images_created": 3, "images_uploaded": 3,
		"logs_read": 3, "logs_created": 3,
		"vehicles_created": 3,
		// The fixture VINs contain an I: not a VIN here.
		"vin_invalid": 2,
	} {
		if sc[k] != want {
			t.Errorf("services.%s = %d, want %d (%v)", k, sc[k], want, sc)
		}
	}
	for k := range sc {
		if strings.Contains(k, "skipped") || strings.Contains(k, "unmapped") || strings.HasPrefix(k, "status_unknown") ||
			strings.HasPrefix(k, "service_no_") || strings.HasPrefix(k, "item_parts_dropped") {
			t.Errorf("unexpected report %s (%v)", k, sc)
		}
	}

	before := rowCounts()
	if before.Services != 3 || before.Items != 3 || before.Images != 3 || before.Logs != 3 || before.Vehicles != 3 {
		t.Errorf("rows after first run = %+v", before)
	}

	// Service numbers, statuses and completion times are the legacy ones.
	type svc struct {
		No, Status string
		Completed  *time.Time
		Customer   int64
		Org        int64
		Items      int64
	}
	byNo := map[string]svc{}
	rows, err := tx.Query(ctx, `
		SELECT s.service_no, s.status, s.completed_at, s.customer_user_id, s.organization_id,
		       (SELECT COUNT(*) FROM service_items i WHERE i.service_id = s.id)
		FROM services s WHERE s.id IN (`+migrated+`)`, hubSys)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s svc
		if err := rows.Scan(&s.No, &s.Status, &s.Completed, &s.Customer, &s.Org, &s.Items); err != nil {
			t.Fatal(err)
		}
		byNo[s.No] = s
	}
	rows.Close()
	s1, s2, s3 := byNo["SYN-S-0001"], byNo["SYN-S-0002"], byNo["SYN-S-0003"]
	if s1.Status != "completed" || s2.Status != "completed" || s3.Status != "processing" {
		t.Errorf("statuses = %+v", byNo)
	}
	wantDone := time.Date(2025, 3, 10, 17, 0, 0, 0, time.UTC)
	if s1.Completed == nil || !s1.Completed.Equal(wantDone) || s3.Completed != nil {
		t.Errorf("completed_at = %v / %v", s1.Completed, s3.Completed)
	}
	if s1.Items != 1 || s2.Items != 1 || s3.Items != 1 {
		t.Errorf("items per service = %+v", byNo)
	}

	// Customer 1 was served by dealers 1 and 2: one user, two organizations.
	dealerOrg := func(id string) int64 {
		return scalar(`SELECT o.id FROM organizations o JOIN migration_map m ON m.target_uuid = o.uuid
			WHERE m.source_system = $1 AND m.source_table = 'dealers' AND m.source_id = $2`, hubSys, id)
	}
	customer1 := scalar(`SELECT u.id FROM users u JOIN migration_map m ON m.target_uuid = u.uuid
		WHERE m.source_system = $1 AND m.source_table = 'customers' AND m.source_id = '1'`, hubSys)
	if s1.Customer != customer1 || s2.Customer != customer1 {
		t.Errorf("customers = %d / %d, want %d", s1.Customer, s2.Customer, customer1)
	}
	if s1.Org != dealerOrg("1") || s2.Org != dealerOrg("2") || s1.Org == s2.Org {
		t.Errorf("organizations = %d / %d", s1.Org, s2.Org)
	}
	if n := scalar(`SELECT COUNT(*) FROM customer_organizations WHERE user_id = $1 AND organization_id IN ($2, $3)`,
		customer1, s1.Org, s2.Org); n != 2 {
		t.Errorf("customer 1 organization links = %d, want 2", n)
	}
	// Each service's vehicle belongs to its customer.
	if n := scalar(`SELECT COUNT(*) FROM services s JOIN vehicles v ON v.id = s.vehicle_id
		WHERE s.id IN (`+migrated+`) AND v.user_id = s.customer_user_id AND v.plate_normalized IS NOT NULL`, hubSys); n != 3 {
		t.Errorf("services with the customer's vehicle = %d, want 3", n)
	}
	// Items point at the TEC-257 units of the legacy stock items.
	if n := scalar(`SELECT COUNT(*) FROM service_items i JOIN units u ON u.id = i.unit_id
		WHERE i.service_id IN (`+migrated+`) AND u.barcode IN ('SYN-HUB-0003', 'SYN-HUB-0004', 'SYN-HUB-0005')
		  AND i.kind = 'full' AND i.product_id = u.product_id`, hubSys); n != 3 {
		t.Errorf("items on the legacy units = %d, want 3", n)
	}
	if n := scalar(`SELECT COUNT(*) FROM service_items i JOIN services s ON s.id = i.service_id
		WHERE s.service_no = 'SYN-S-0001' AND i.applied_parts = '["hood", "roof"]'::jsonb AND s.id IN (`+migrated+`)`, hubSys); n != 1 {
		t.Errorf("SYN-S-0001 item parts not carried over")
	}
	// Logs are notes in the service status, the author's organization as actor.
	if n := scalar(`SELECT COUNT(*) FROM service_status_logs l JOIN services s ON s.id = l.service_id
		WHERE s.id IN (`+migrated+`) AND l.from_status IS NULL AND l.to_status = s.status
		  AND l.actor_org_id IS NOT NULL AND l.metadata->>'source' = 'legacy_hub.service_status_logs'`, hubSys); n != 3 {
		t.Errorf("status logs as notes = %d, want 3", n)
	}

	// Images: one object per row, in the store.
	objs, err := store.List(ctx, storage.ListObjectsInput{Prefix: "services/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(objs.Objects) != 3 {
		t.Errorf("stored images = %d, want 3", len(objs.Objects))
	}
	keys, err := tx.Query(ctx, `SELECT storage_key FROM service_images WHERE service_id IN (`+migrated+`)`, hubSys)
	if err != nil {
		t.Fatal(err)
	}
	for keys.Next() {
		var key string
		if err := keys.Scan(&key); err != nil {
			t.Fatal(err)
		}
		if ok, err := store.Exists(ctx, key); err != nil || !ok {
			t.Errorf("image %s not in the store (%v)", key, err)
		}
	}
	keys.Close()

	// No warranty, no outbox event.
	if n := warrantyInserts() - warrantiesBefore; n != 0 {
		t.Errorf("warranties inserted = %d, want 0", n)
	}
	if n := scalar(`SELECT COUNT(*) FROM warranties WHERE service_id IN (`+migrated+`)`, hubSys); n != 0 {
		t.Errorf("warranties of migrated services = %d", n)
	}
	if n := outboxInserts() - outboxBefore; n != 0 {
		t.Errorf("outbox_events inserted = %d, want 0", n)
	}

	// Second run: nothing new.
	second := e.run("olexfx-services", Options{})
	rc := stepCounts(t, second, "services")
	for k, v := range rc {
		if v != 0 && (strings.HasSuffix(k, "_created") || strings.HasSuffix(k, "_updated") ||
			strings.HasSuffix(k, "_uploaded") || strings.HasPrefix(k, "vehicles_")) {
			t.Errorf("rerun services.%s = %d, want 0", k, v)
		}
	}
	if rc["services_unchanged"] != 3 || rc["items_unchanged"] != 3 || rc["images_unchanged"] != 3 || rc["logs_unchanged"] != 3 {
		t.Errorf("rerun counts = %v", rc)
	}
	if after := rowCounts(); after != before {
		t.Errorf("rerun changed rows: %+v -> %+v", before, after)
	}
	if objs, _ := store.List(ctx, storage.ListObjectsInput{Prefix: "services/"}); len(objs.Objects) != 3 {
		t.Errorf("rerun stored images = %d", len(objs.Objects))
	}
	if n := outboxInserts() - outboxBefore; n != 0 {
		t.Errorf("rerun outbox_events inserted = %d", n)
	}
}
