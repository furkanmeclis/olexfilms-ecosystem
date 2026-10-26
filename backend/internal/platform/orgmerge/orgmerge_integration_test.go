package orgmerge_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgmerge"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-1003 fix (olexdemo dry run): the source and the target both number
// their documents from 1, and both registered the same measurement device.
// The merge keeps the source's document numbers (numbered_by_org), moves
// their children with them and points the source's results at the target's
// device; undo restores all of it.
func TestMergeNumberedDocumentsAndDuplicateDevices(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	one := func(sql string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	brand := one(`SELECT id FROM brands WHERE slug = 'olex'`)
	center := one(`SELECT id FROM organizations WHERE brand_id = $1 AND type = 'center' AND deleted_at IS NULL ORDER BY id LIMIT 1`, brand)
	dealer := one(`INSERT INTO organizations (slug, name, status, access_starts_at, type, parent_id, brand_id, currency, locale, timezone, settings)
		VALUES ($1, 'Florya test', 'active', NOW() - interval '1 hour', 'dealer', $2, $3, 'TRY', 'tr', 'Europe/Istanbul', '{}') RETURNING id`,
		"t1003-fix-"+suffix, center, brand)
	tpl := one(`INSERT INTO contract_templates (organization_id, brand_id, name, kind) VALUES ($1, $2, $3, 'service_sale') RETURNING id`,
		center, brand, "t1003-"+suffix)

	// Contract number 1 (or the center's current one) at both.
	no := one(`SELECT COALESCE(MIN(contract_no), 0) FROM contract_instances WHERE organization_id = $1 AND numbered_by_org IS NULL`, center)
	contract := func(org, no, subject int64) int64 {
		return one(`INSERT INTO contract_instances (organization_id, brand_id, contract_no, subject_type, subject_id,
			template_id, kind, locale, template_version, otp_required, signature_required)
			VALUES ($1, $2, $3, 'service_subscription', $4, $5, 'service_sale', 'tr', 1, false, false) RETURNING id`,
			org, brand, no, subject, tpl)
	}
	base := time.Now().UnixNano() % 1_000_000_000
	if no == 0 {
		no = 1
		contract(center, no, base+1)
	}
	dealerContract := contract(dealer, no, base+2)
	media := one(`INSERT INTO contract_media (instance_id, organization_id, brand_id, storage_key, mime_type, size_bytes, sha256)
		VALUES ($1, $2, $3, 'k/1.jpg', 'image/jpeg', 10, $4) RETURNING id`,
		dealerContract, dealer, brand, fmt.Sprintf("%064d", 1))

	// Quote number of the center at the dealer too, with a line.
	lead := func(org int64) int64 {
		return one(`INSERT INTO leads (organization_id, brand_id, target_type, source) VALUES ($1, $2, 'customer', 'walk_in') RETURNING id`, org, brand)
	}
	qno := one(`SELECT COALESCE(MAX(quote_no), 0) + 1 FROM quotes WHERE organization_id = $1`, center)
	one(`INSERT INTO quotes (organization_id, brand_id, lead_id, quote_no, currency) VALUES ($1, $2, $3, $4, 'TRY') RETURNING id`, center, brand, lead(center), qno)
	dealerQuote := one(`INSERT INTO quotes (organization_id, brand_id, lead_id, quote_no, currency) VALUES ($1, $2, $3, $4, 'TRY') RETURNING id`, dealer, brand, lead(dealer), qno)
	product := one(`SELECT id FROM products WHERE brand_id = $1 ORDER BY id LIMIT 1`, brand)
	line := one(`INSERT INTO quote_lines (quote_id, organization_id, brand_id, line_type, product_id, description_snapshot) VALUES ($1, $2, $3, 'product', $4, 'x') RETURNING id`, dealerQuote, dealer, brand, product)

	// The same device registered at both.
	serial := "T1003-" + suffix
	centerDevice := one(`INSERT INTO measurement_devices (organization_id, brand_id, serial) VALUES ($1, $2, $3) RETURNING id`, center, brand, serial)
	dealerDevice := one(`INSERT INTO measurement_devices (organization_id, brand_id, serial) VALUES ($1, $2, $3) RETURNING id`, dealer, brand, serial)
	result := one(`INSERT INTO measurement_results (organization_id, brand_id, device_id, status, raw, source) VALUES ($1, $2, $3, 'vin_pending', '{}', 'legacy_import') RETURNING id`,
		dealer, brand, dealerDevice)

	rep, err := orgmerge.Merge(ctx, tx, orgmerge.Options{SourceID: dealer, TargetID: center, Actor: "test"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if rep.Numbered["contract_instances"] != 1 || rep.Numbered["quotes"] != 1 || rep.Repointed["measurement_results.device_id"] != 1 {
		t.Fatalf("report numbered %v repointed %v", rep.Numbered, rep.Repointed)
	}
	var org, by, gotNo int64
	if err := tx.QueryRow(ctx, `SELECT organization_id, numbered_by_org, contract_no FROM contract_instances WHERE id = $1`, dealerContract).Scan(&org, &by, &gotNo); err != nil {
		t.Fatal(err)
	}
	if org != center || by != dealer || gotNo != no {
		t.Fatalf("contract org %d by %d no %d", org, by, gotNo)
	}
	if one(`SELECT organization_id FROM contract_media WHERE id = $1`, media) != center {
		t.Fatal("media did not follow its contract")
	}
	if one(`SELECT organization_id FROM quotes WHERE id = $1`, dealerQuote) != center || one(`SELECT organization_id FROM quote_lines WHERE id = $1`, line) != center {
		t.Fatal("quote did not move with its line")
	}
	if got := one(`SELECT device_id FROM measurement_results WHERE id = $1`, result); got != centerDevice {
		t.Fatalf("result device %d, want the center's %d", got, centerDevice)
	}
	// The center's next quote number stays in its own series.
	if next := one(`SELECT COALESCE(MAX(quote_no), 0) + 1 FROM quotes WHERE organization_id = $1 AND numbered_by_org IS NULL`, center); next != qno+1 {
		t.Fatalf("next quote no %d", next)
	}

	if _, err := orgmerge.Undo(ctx, tx, rep.RunUUID); err != nil {
		t.Fatalf("undo: %v", err)
	}
	var nullBy *int64
	if err := tx.QueryRow(ctx, `SELECT organization_id, numbered_by_org FROM contract_instances WHERE id = $1`, dealerContract).Scan(&org, &nullBy); err != nil {
		t.Fatal(err)
	}
	if org != dealer || nullBy != nil {
		t.Fatalf("after undo contract org %d by %v", org, nullBy)
	}
	if got := one(`SELECT device_id FROM measurement_results WHERE id = $1`, result); got != dealerDevice {
		t.Fatalf("after undo result device %d", got)
	}
	if one(`SELECT organization_id FROM contract_media WHERE id = $1`, media) != dealer {
		t.Fatal("media not back")
	}
}
