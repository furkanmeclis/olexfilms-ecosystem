// Command seed-demo fills a local development database with realistic Olex
// demo data. It is intentionally local-only and idempotent.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/legacyfixture"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	legacyDBName  = "olex_legacy_demo"
	credentials   = "/Volumes/DockerRuns/Projects/olexfilms-wt/_tools/DEV_CREDENTIALS.md"
	passwordEnv   = "SEED_DEMO_PASSWORD"
	defaultDBHost = "127.0.0.1"
)

var demoRoles = []struct {
	Email   string
	Role    string
	OrgSlug string
	Name    string
}{
	{"center_staff@demo.olexfilms.app", "center_staff", "olex-merkez", "Merkez Operasyon"},
	{"center_warehouse@demo.olexfilms.app", "center_warehouse", "olex-merkez", "Merkez Depo"},
	{"center_accounting@demo.olexfilms.app", "center_accounting", "olex-merkez", "Merkez Muhasebe"},
	{"center_social@demo.olexfilms.app", "center_social", "olex-merkez", "Merkez Pazarlama"},
	{"distributor_owner@demo.olexfilms.app", "distributor_owner", "demo-nl-distributor", "Distribütör Sahibi"},
	{"distributor_staff@demo.olexfilms.app", "distributor_staff", "demo-ua-distributor", "Distribütör Operasyon"},
	{"distributor_warehouse_staff@demo.olexfilms.app", "distributor_warehouse_staff", "demo-nl-distributor", "Distribütör Depo"},
	{"distributor_accounting@demo.olexfilms.app", "distributor_accounting", "demo-ua-distributor", "Distribütör Muhasebe"},
	{"dealer_owner@demo.olexfilms.app", "dealer_owner", "demo-istanbul-detailing", "Bayi Sahibi"},
	{"dealer_staff@demo.olexfilms.app", "dealer_staff", "demo-amsterdam-wraps", "Bayi Personel"},
	{"dealer_accounting@demo.olexfilms.app", "dealer_accounting", "demo-kyiv-studio", "Bayi Muhasebe"},
}

type app struct {
	cfg      config.Config
	pool     *pgxpool.Pool
	q        *db.Queries
	out      io.Writer
	pass     string
	passHash string

	brandID  int64
	center   int64
	users    map[string]int64
	orgs     map[string]int64
	products map[string]int64
	units    map[string]int64
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "seed-demo: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")); dsn != "" {
		if err := applyDatabaseURL(dsn, &cfg.DB); err != nil {
			return err
		}
	}
	if strings.EqualFold(cfg.App.Env, "production") {
		return errors.New("APP_ENV=production refused")
	}
	if !isLocalHost(cfg.DB.Host) {
		return fmt.Errorf("DB_HOST %q refused; seed-demo only runs against localhost/127.0.0.1", cfg.DB.Host)
	}

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := ensureLegacyLayer(ctx, cfg, out); err != nil {
		return err
	}

	pass := os.Getenv(passwordEnv)
	if pass == "" {
		pass, err = randomPassword()
		if err != nil {
			return err
		}
	}
	hash, err := password.Hash(pass)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}

	a := &app{
		cfg: cfg, pool: pool, q: db.New(pool), out: out,
		pass: pass, passHash: hash, users: map[string]int64{}, orgs: map[string]int64{},
		products: map[string]int64{}, units: map[string]int64{},
	}
	before, err := demoCounts(ctx, pool)
	if err != nil {
		return err
	}
	if err := a.seedDemo(ctx); err != nil {
		return err
	}
	after, err := demoCounts(ctx, pool)
	if err != nil {
		return err
	}
	if before.Total > 0 && before != after {
		return fmt.Errorf("idempotency check failed: before=%+v after=%+v", before, after)
	}
	if err := appendCredentials(pass); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Demo seed tamamlandı.\nŞifre: %s\n", pass)
	return nil
}

func ensureLegacyLayer(ctx context.Context, cfg config.Config, out io.Writer) error {
	legacyDSN, err := ensureLegacyDB(ctx, cfg.DB)
	if err != nil {
		return err
	}
	legacyPool, err := pgxpool.New(ctx, legacyDSN)
	if err != nil {
		return fmt.Errorf("legacy db connect: %w", err)
	}
	defer legacyPool.Close()
	tx, err := legacyPool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := legacyfixture.Load(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	oldHub, oldWH := os.Getenv("LEGACY_HUB_DSN"), os.Getenv("LEGACY_WH_DSN")
	defer func() {
		_ = os.Setenv("LEGACY_HUB_DSN", oldHub)
		_ = os.Setenv("LEGACY_WH_DSN", oldWH)
	}()
	_ = os.Setenv("LEGACY_HUB_DSN", legacyDSN)
	_ = os.Setenv("LEGACY_WH_DSN", legacyDSN)

	target, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer target.Close()
	r := &migrator.Runner{
		Pool: target, Open: migrator.OpenLegacyFromEnv,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Storage: storage.NewMemory(),
	}
	rep, runErr := r.Run(ctx, migrator.Options{Profile: "olex", Mode: migrator.ModeFull})
	if rep.RunID != 0 {
		_, _ = fmt.Fprintf(out, "Legacy migrator run #%d: %s\n", rep.RunID, rep.Status)
	}
	if runErr != nil {
		_, _ = fmt.Fprintf(out, "UYARI: legacy migrator hata döndürdü, demo katmanına devam ediliyor: %v\n", runErr)
	}
	p, err := migrator.Lookup(migrator.Profiles(), "olex")
	if err != nil {
		return err
	}
	report, err := migrator.BuildReportReadOnly(ctx, target, p, migrator.OpenLegacyFromEnv, migrator.ReportOptions{Profile: "olex"})
	if err != nil {
		_, _ = fmt.Fprintf(out, "UYARI: legacy rapor üretilemedi: %v\n", err)
		return nil
	}
	if err := report.Err(); err != nil {
		_, _ = fmt.Fprintf(out, "UYARI: legacy raporda mismatch var, demo katmanına devam ediliyor: %v\n", err)
	}
	return nil
}

func (a *app) seedDemo(ctx context.Context) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx, `SELECT id FROM brands WHERE slug = 'olex'`).Scan(&a.brandID); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = 'olex-merkez'`).Scan(&a.center); err != nil {
		return err
	}
	a.orgs["olex-merkez"] = a.center
	if err := a.seedOrganizations(ctx, tx); err != nil {
		return err
	}
	if err := a.seedUsers(ctx, tx); err != nil {
		return err
	}
	if err := a.seedModules(ctx, tx); err != nil {
		return err
	}
	if err := a.seedCatalogStock(ctx, tx); err != nil {
		return err
	}
	if err := a.seedCustomersServices(ctx, tx); err != nil {
		return err
	}
	if err := a.seedAccounting(ctx, tx); err != nil {
		return err
	}
	if err := a.seedF3(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *app) seedOrganizations(ctx context.Context, tx pgx.Tx) error {
	rows := []struct {
		Slug, Name, Typ, Parent, Currency, ISO2, Province, District, City, Address, Phone string
	}{
		{"demo-nl-distributor", "Olex Films Netherlands", "distributor", "olex-merkez", "EUR", "NL", "NL-NH", "Amsterdam", "Amsterdam", "Keizersgracht 127, Amsterdam", "+31201234567"},
		{"demo-amsterdam-wraps", "Amsterdam Wraps", "dealer", "demo-nl-distributor", "EUR", "NL", "NL-NH", "Amsterdam", "Amsterdam", "Aambeeldstraat 10, Amsterdam", "+31207654321"},
		{"demo-rotterdam-ppf", "Rotterdam PPF Lab", "dealer", "demo-nl-distributor", "EUR", "NL", "NL-NH", "Haarlem", "Haarlem", "Waarderweg 44, Haarlem", "+31235550100"},
		{"demo-ua-distributor", "Olex Films Ukraine", "distributor", "olex-merkez", "UAH", "UA", "", "", "Kyiv", "Volodymyrska Street 40, Kyiv", "+380441234567"},
		{"demo-kyiv-studio", "Kyiv Detailing Studio", "dealer", "demo-ua-distributor", "UAH", "UA", "", "", "Kyiv", "Peremohy Ave 12, Kyiv", "+380671112233"},
		{"demo-istanbul-detailing", "Istanbul Detailing", "dealer", "olex-merkez", "TRY", "TR", "34", "Kadıköy", "İstanbul", "Bağdat Cd. No:120 Kadıköy", "+902165551212"},
		{"demo-ankara-garage", "Ankara Garage", "dealer", "olex-merkez", "TRY", "TR", "06", "Çankaya", "Ankara", "Tunalı Hilmi Cd. No:55 Çankaya", "+903125551313"},
	}
	for _, r := range rows {
		parent := a.orgs[r.Parent]
		var id int64
		err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = $1 AND deleted_at IS NULL`, r.Slug).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
WITH geo AS (
  SELECT c.id country_id, p.id province_id, d.id district_id
  FROM countries c
  LEFT JOIN provinces p ON p.country_id = c.id AND ($7 = '' OR p.code = $7 OR p.name = $7)
  LEFT JOIN districts d ON d.province_id = p.id AND ($8 = '' OR d.name = $8)
  WHERE c.iso2 = $6
  LIMIT 1
), ins AS (
  INSERT INTO organizations (
    slug, name, city, district, phone, address, status, plan_code, type,
    parent_id, brand_id, currency, locale, timezone, country_id, province_id, district_id
  )
  SELECT $1, $2, $9, $8, $10, $11, 'active', NULL, $3, $4, $5, $12,
         CASE WHEN $6::text = 'UA' THEN 'uk' WHEN $6::text = 'NL' THEN 'en' ELSE 'tr' END,
         CASE WHEN $6::text = 'UA' THEN 'Europe/Kyiv' WHEN $6::text = 'NL' THEN 'Europe/Amsterdam' ELSE 'Europe/Istanbul' END,
         country_id, province_id, district_id
  FROM geo
  RETURNING id
)
SELECT id FROM ins`, r.Slug, r.Name, r.Typ, parent, a.brandID, r.ISO2, r.Province, r.District, r.City, r.Phone, r.Address, r.Currency).Scan(&id)
		}
		if err != nil {
			return fmt.Errorf("org %s: %w", r.Slug, err)
		}
		_, _ = tx.Exec(ctx, `UPDATE organizations SET name = $2, city = $3, district = $4, phone = $5, address = $6, currency = $7 WHERE id = $1`,
			id, r.Name, r.City, r.District, r.Phone, r.Address, r.Currency)
		a.orgs[r.Slug] = id
	}
	return nil
}

func (a *app) seedUsers(ctx context.Context, tx pgx.Tx) error {
	for i, r := range demoRoles {
		parts := strings.SplitN(r.Name, " ", 2)
		surname := "Demo"
		if len(parts) == 2 {
			surname = parts[1]
		}
		var id int64
		phone := fmt.Sprintf("+90555990%04d", i+1)
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1 AND deleted_at IS NULL`, r.Email).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
INSERT INTO users (email, password_hash, name, surname, status, email_verified_at, phone_e164, phone_verified_at)
VALUES ($1, $2, $3, $4, 'active', NOW(), $5, NOW())
RETURNING id`, r.Email, a.passHash, parts[0], surname, phone).Scan(&id)
		}
		if err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `UPDATE users SET password_hash = $2, status = 'active', email_verified_at = COALESCE(email_verified_at, NOW()) WHERE id = $1`, id, a.passHash)
		a.users[r.Email] = id
		if _, err := tx.Exec(ctx, `
INSERT INTO organization_members (organization_id, user_id, role)
VALUES ($1, $2, CASE WHEN $3 LIKE '%owner' THEN 'owner' ELSE 'staff' END)
ON CONFLICT (organization_id, user_id) DO NOTHING`, a.orgs[r.OrgSlug], id, r.Role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO user_roles (user_id, role_id)
SELECT $1, id FROM roles WHERE slug IN ($2, 'organization_user')
ON CONFLICT DO NOTHING`, id, r.Role); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) seedModules(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
INSERT INTO module_flags (scope, organization_id, module_key, enabled, source, note)
SELECT 'system', NULL, m.key, true, 'admin', 'seed-demo'
FROM modules m
WHERE NOT EXISTS (
  SELECT 1 FROM module_flags f WHERE f.scope = 'system' AND f.module_key = m.key
);
UPDATE module_flags SET enabled = true, source = 'admin', note = 'seed-demo' WHERE scope = 'system';`)
	return err
}

func (a *app) seedCatalogStock(ctx context.Context, tx pgx.Tx) error {
	cats := []string{"PPF Filmler", "Seramik Kaplama", "Aksesuar"}
	for i, name := range cats {
		var id int64
		if err := tx.QueryRow(ctx, `
INSERT INTO product_categories (organization_id, brand_id, name, available_parts, sort, active)
VALUES ($1, $2, $3, '["hood","bumper","door","fender","mirror","roof"]', $4, true)
ON CONFLICT (brand_id, name) DO UPDATE SET active = true
RETURNING id`, a.center, a.brandID, name, (i+1)*10).Scan(&id); err != nil {
			return err
		}
	}
	products := []struct {
		SKU, Name, Cat, Unit string
		Fixed                bool
		Months               int
		Micron               string
	}{
		{"DEMO-PPF-GLOSS-152", "Olex Pro Gloss PPF 152cm", "PPF Filmler", "roll_meter", false, 120, "190.00"},
		{"DEMO-PPF-MATTE-152", "Olex Matte PPF 152cm", "PPF Filmler", "roll_meter", false, 96, "185.00"},
		{"DEMO-CERAMIC-50ML", "Olex Ceramic Coat 50ml", "Seramik Kaplama", "piece", true, 24, "0.00"},
		{"DEMO-SQUEEGEE", "Olex Pro Squeegee", "Aksesuar", "piece", false, 0, "0.00"},
	}
	for _, p := range products {
		var id int64
		if err := tx.QueryRow(ctx, `
INSERT INTO products (
 organization_id, brand_id, category_id, sku, name, description_md,
 warranty_duration_months, micron_thickness, unit_type, uses_fixed_barcode, active
)
SELECT $1, $2, c.id, $3, $4, 'Demo seed ürünü', NULLIF($7, 0), NULLIF($8, '0.00')::numeric,
       $5, $6, true
FROM product_categories c WHERE c.brand_id = $2 AND c.name = $9
ON CONFLICT (brand_id, sku) DO UPDATE SET name = EXCLUDED.name, active = true
RETURNING id`, a.center, a.brandID, p.SKU, p.Name, p.Unit, p.Fixed, p.Months, p.Micron, p.Cat).Scan(&id); err != nil {
			return fmt.Errorf("product %s: %w", p.SKU, err)
		}
		a.products[p.SKU] = id
	}
	var loc int64
	err := tx.QueryRow(ctx, `SELECT id FROM warehouse_locations WHERE organization_id = $1 AND code = 'DEMO-A-01' AND room_id IS NULL`, a.center).Scan(&loc)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
INSERT INTO warehouse_locations (organization_id, code, name, active)
VALUES ($1, 'DEMO-A-01', 'Demo Ana Raf', true)
RETURNING id`, a.center).Scan(&loc)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	_, _ = tx.Exec(ctx, `UPDATE warehouse_locations SET name = 'Demo Ana Raf', active = true WHERE id = $1`, loc)
	qtx := a.q.WithTx(tx)
	l := ledger.New(a.q, nil)
	for i := 1; i <= 20; i++ {
		sku := "DEMO-PPF-GLOSS-152"
		if i%3 == 0 {
			sku = "DEMO-PPF-MATTE-152"
		}
		barcode := fmt.Sprintf("DEMO-ROLL-%03d", i)
		unit, err := ensureUnit(ctx, qtx, a.center, a.brandID, a.products[sku], barcode, "serial", "50.00")
		if err != nil {
			return err
		}
		a.units[barcode] = unit.ID
		if err := post(ctx, l, tx, ledger.Movement{Type: ledger.TypeEntry, UnitID: unit.ID, To: &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc, OrgID: a.center}, Source: "seed-demo", RefType: "unit", RefID: unit.ID}); err != nil {
			return err
		}
	}
	unit, err := ensureUnit(ctx, qtx, a.center, a.brandID, a.products["DEMO-CERAMIC-50ML"], "DEMO-CERAMIC-FIXED", "fixed", "")
	if err != nil {
		return err
	}
	a.units["DEMO-CERAMIC-FIXED"] = unit.ID
	if err := post(ctx, l, tx, ledger.Movement{Type: ledger.TypeEntry, UnitID: unit.ID, To: &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc, OrgID: a.center}, Quantity: 80, Source: "seed-demo", RefType: "unit", RefID: unit.ID}); err != nil {
		return err
	}
	return a.seedOrders(ctx, tx, l)
}

func (a *app) seedOrders(ctx context.Context, tx pgx.Tx, l *ledger.Ledger) error {
	orderRows := []struct {
		No            string
		Seller, Buyer string
		Status        string
		Amount        string
	}{
		{"DEMO-ORD-001", "olex-merkez", "demo-nl-distributor", "submitted", "4200.00"},
		{"DEMO-ORD-002", "olex-merkez", "demo-ua-distributor", "approved", "3100.00"},
		{"DEMO-ORD-003", "olex-merkez", "demo-nl-distributor", "received", "5200.00"},
		{"DEMO-ORD-004", "demo-nl-distributor", "demo-amsterdam-wraps", "approved", "1800.00"},
		{"DEMO-ORD-005", "demo-nl-distributor", "demo-rotterdam-ppf", "received", "2400.00"},
		{"DEMO-ORD-006", "demo-ua-distributor", "demo-kyiv-studio", "submitted", "90000.00"},
	}
	actor := a.users["center_warehouse@demo.olexfilms.app"]
	for i, o := range orderRows {
		var id int64
		uid := stableUUID("order:" + o.No)
		err := tx.QueryRow(ctx, `
INSERT INTO orders (
 uuid, organization_id, brand_id, seller_org_id, buyer_org_id, order_no, status,
 currency, rate_snapshot, try_rate, subtotal, total, note, submitted_at, approved_at, shipped_at, received_at,
 created_by_user_id, approved_by_user_id
) VALUES (
 $1, $4, $3, $4, $2, $5, $6::varchar, (SELECT currency FROM brands WHERE id = $3),
 CASE WHEN $6::text IN ('approved','preparing','ready','processing','shipped','delivered','received') THEN '{"base":"EUR","quote":"TRY","rate":"35.00000000","rate_date":"2026-10-04","source":"seed-demo"}'::jsonb END,
 CASE WHEN $6::text IN ('approved','preparing','ready','processing','shipped','delivered','received') THEN 35.0000000000 END,
 $8::numeric, $8::numeric, 'Demo sipariş', NOW(), CASE WHEN $6::text IN ('approved','received') THEN NOW() END,
	 CASE WHEN $6::text = 'received' THEN NOW() END, CASE WHEN $6::text = 'received' THEN NOW() END, $7::bigint,
	 CASE WHEN $6::text IN ('approved','received') THEN $7::bigint END
)
ON CONFLICT (uuid) DO UPDATE SET status = EXCLUDED.status
RETURNING id`, uid, a.orgs[o.Buyer], a.brandID, a.orgs[o.Seller], o.No, o.Status, actor, o.Amount).Scan(&id)
		if err != nil {
			return fmt.Errorf("order %s: %w", o.No, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO order_status_history (order_id, organization_id, brand_id, from_status, to_status, actor_user_id, reason)
SELECT id, organization_id, brand_id, 'draft', $2::varchar, $3::bigint, 'seed-demo'
FROM orders o
WHERE o.id = $1
  AND NOT EXISTS (
    SELECT 1 FROM order_status_history h
    WHERE h.order_id = o.id AND h.to_status = $2::varchar AND h.reason = 'seed-demo'
  )`, id, o.Status, actor); err != nil {
			return fmt.Errorf("order history %s: %w", o.No, err)
		}
		if false && o.Status == "received" && i < 5 {
			barcode := fmt.Sprintf("DEMO-ROLL-%03d", i+1)
			unitID := a.units[barcode]
			if err := post(ctx, l, tx, ledger.Movement{Type: ledger.TypeOrderOut, UnitID: unitID, From: &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: 0}, To: &ledger.Owner{Type: ledger.OwnerOrganization, ID: a.orgs[o.Buyer]}, Source: "seed-demo", RefType: "order", RefID: id}); err != nil {
				// Some legacy/demo reruns may already have moved this unit through a service;
				// orders are still useful as status data, so keep seeding.
				_, _ = fmt.Fprintf(a.out, "UYARI: order stock movement skipped for %s: %v\n", o.No, err)
			} else if err := post(ctx, l, tx, ledger.Movement{Type: ledger.TypeReceived, UnitID: unitID, To: &ledger.Owner{Type: ledger.OwnerOrganization, ID: a.orgs[o.Buyer]}, Source: "seed-demo", RefType: "order", RefID: id}); err != nil {
				_, _ = fmt.Fprintf(a.out, "UYARI: order receive movement skipped for %s: %v\n", o.No, err)
			}
		}
		_ = o.Amount
	}
	return nil
}

func (a *app) seedCustomersServices(ctx context.Context, tx pgx.Tx) error {
	carBrand, carModel, err := ensureCar(ctx, tx)
	if err != nil {
		return err
	}
	l := ledger.New(a.q, nil)
	for i := 1; i <= 18; i++ {
		orgSlug := []string{"demo-istanbul-detailing", "demo-amsterdam-wraps", "demo-rotterdam-ppf", "demo-kyiv-studio"}[i%4]
		orgID := a.orgs[orgSlug]
		email := fmt.Sprintf("customer%02d@demo.olexfilms.app", i)
		userID, vehicleID, err := a.ensureCustomerVehicle(ctx, tx, email, orgID, carBrand, carModel, i)
		if err != nil {
			return err
		}
		status := []string{"draft", "pending", "processing", "ready", "completed", "cancelled"}[i%6]
		if status == "completed" {
			orgID = a.center
		}
		insertStatus := status
		if status == "completed" {
			insertStatus = "ready"
		}
		serviceNo := fmt.Sprintf("DEMO-SVC-%03d", i)
		var svcID int64
		err = tx.QueryRow(ctx, `
INSERT INTO services (
 uuid, service_no, organization_id, brand_id, customer_user_id, vehicle_id, car_brand_id, car_model_id,
 model_year, plate, plate_country, vin, km, package, notes, status, created_by_user_id,
 completed_by_user_id, cancelled_by_user_id, completed_at, cancelled_at
) VALUES (
 $1, $2, $3, $4, $5, $6, $7, $8, 2022, $9, 'TR', $10, 42000 + $11,
 'Full front PPF + ceramic', 'Demo hizmet kaydı' , $12::varchar, $13::bigint,
 CASE WHEN $12::text = 'completed' THEN $13::bigint END, CASE WHEN $12::text = 'cancelled' THEN $13::bigint END,
 CASE WHEN $12::text = 'completed' THEN NOW() - INTERVAL '2 days' END,
 CASE WHEN $12::text = 'cancelled' THEN NOW() - INTERVAL '1 day' END
)
ON CONFLICT (service_no) DO UPDATE SET notes = EXCLUDED.notes
RETURNING id`, stableUUID("service:"+serviceNo), serviceNo, orgID, a.brandID, userID, vehicleID, carBrand, carModel, fmt.Sprintf("34DM%04d", i), fmt.Sprintf("WVWZZZ1KZAW%06d", i), i, insertStatus, a.users["dealer_staff@demo.olexfilms.app"]).Scan(&svcID)
		if err != nil {
			return fmt.Errorf("service %s: %w", serviceNo, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO service_status_logs (service_id, organization_id, brand_id, from_status, to_status, actor_user_id, actor_org_id, note)
SELECT id, organization_id, brand_id, 'draft', $2::varchar, $3::bigint, organization_id, 'seed-demo'
FROM services s
WHERE s.id = $1
  AND NOT EXISTS (
    SELECT 1 FROM service_status_logs l
    WHERE l.service_id = s.id AND l.to_status = $2::varchar AND l.note = 'seed-demo'
  )`, svcID, status, a.users["dealer_staff@demo.olexfilms.app"]); err != nil {
			return fmt.Errorf("service history %s: %w", serviceNo, err)
		}
		if status == "completed" {
			barcode := fmt.Sprintf("DEMO-ROLL-%03d", i)
			unitID := a.units[barcode]
			var itemID int64
			_ = tx.QueryRow(ctx, `SELECT id FROM service_items WHERE service_id = $1 LIMIT 1`, svcID).Scan(&itemID)
			if itemID == 0 {
				err = tx.QueryRow(ctx, `
INSERT INTO service_items (service_id, organization_id, brand_id, product_id, unit_id, kind, applied_parts, notes)
VALUES ($1, $2, $3, $4, $5, 'full', '["hood","bumper"]', 'Demo tüketim')
RETURNING id`, svcID, orgID, a.brandID, a.products["DEMO-PPF-GLOSS-152"], unitID).Scan(&itemID)
				if err != nil {
					return err
				}
			}
			if itemID != 0 {
				res, err := tryPost(ctx, l, tx, ledger.Movement{
					Type: ledger.TypeConsumption, UnitID: unitID,
					To:     &ledger.Owner{Type: ledger.OwnerService, ID: svcID, OrgID: orgID},
					Source: "seed-demo", RefType: "service_item", RefID: itemID,
				})
				if err == nil {
					_, _ = tx.Exec(ctx, `UPDATE service_items SET stock_movement_id = $2 WHERE id = $1 AND stock_movement_id IS NULL`, itemID, res.Movement.ID)
					_, _ = tx.Exec(ctx, `UPDATE services SET status = 'completed', completed_at = COALESCE(completed_at, NOW() - INTERVAL '2 days'), completed_by_user_id = COALESCE(completed_by_user_id, $2) WHERE id = $1 AND status <> 'completed'`, svcID, a.users["dealer_staff@demo.olexfilms.app"])
					_, _ = a.q.WithTx(tx).CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
						ServiceItemID: itemID, HolderUserID: userID,
						StartAt: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
						EndAt:   pgtype.Timestamptz{Time: time.Now().AddDate(5, 0, 0), Valid: true},
					})
				} else {
					_, _ = fmt.Fprintf(a.out, "UYARI: service consumption skipped for %s: %v\n", serviceNo, err)
				}
			}
		}
	}
	return nil
}

func (a *app) ensureCustomerVehicle(ctx context.Context, tx pgx.Tx, email string, orgID, carBrand, carModel int64, i int) (int64, int64, error) {
	var userID int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1 AND deleted_at IS NULL`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
INSERT INTO users (email, password_hash, name, surname, status, email_verified_at, phone_e164, phone_verified_at)
VALUES ($1, $2, 'Demo', $3, 'active', NOW(), $4, NOW())
RETURNING id`, email, a.passHash, fmt.Sprintf("Müşteri %02d", i), fmt.Sprintf("+9055588%05d", i)).Scan(&userID)
	}
	if err != nil {
		return 0, 0, err
	}
	_, _ = tx.Exec(ctx, `UPDATE users SET status = 'active' WHERE id = $1`, userID)
	_, _ = tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE slug = 'customer' ON CONFLICT DO NOTHING`, userID)
	_, _ = tx.Exec(ctx, `INSERT INTO customer_profiles (user_id, type) VALUES ($1, 'individual') ON CONFLICT (user_id) DO NOTHING`, userID)
	_, _ = tx.Exec(ctx, `INSERT INTO customer_organizations (user_id, organization_id, brand_id, first_service_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (user_id, organization_id) DO NOTHING`, userID, orgID, a.brandID)
	var vehicleID int64
	err = tx.QueryRow(ctx, `
INSERT INTO vehicles (uuid, user_id, organization_id, brand_id, car_brand_id, car_model_id, model_year, plate, plate_normalized, plate_country, vin)
VALUES ($9, $1, $2, $3, $4, $5, 2022, $6, $7, 'TR', $8)
ON CONFLICT (uuid) DO NOTHING
RETURNING id`, userID, orgID, a.brandID, carBrand, carModel, fmt.Sprintf("34 DM %04d", i), fmt.Sprintf("34DM%04d", i), fmt.Sprintf("WVWZZZ1KZAW%06d", i), stableUUID(fmt.Sprintf("vehicle:%s", email))).Scan(&vehicleID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM vehicles WHERE user_id = $1 AND vin = $2`, userID, fmt.Sprintf("WVWZZZ1KZAW%06d", i)).Scan(&vehicleID)
	}
	return userID, vehicleID, err
}

func (a *app) seedAccounting(ctx context.Context, tx pgx.Tx) error {
	p := posting.New(a.q, nil, nil)
	src := stableUUID("accounting:demo-sale")
	res, err := p.PostHierarchicalSaleTx(ctx, tx, posting.Sale{
		Source:      posting.Source{Type: "seed_demo", UUID: src},
		SellerOrgID: a.center, BuyerOrgID: a.orgs["demo-nl-distributor"],
		Amount: "1250.00", Currency: "EUR", RateDate: time.Now(),
		RateSnapshot: &fxrates.Snapshot{
			Base: "EUR", Quote: "TRY", Rate: "35.00000000", RateDate: time.Now().Format(time.DateOnly), Source: "seed-demo",
		},
		Description: "Demo cari hareketi",
	})
	if err != nil {
		return err
	}
	_, _ = res, err
	buyerEntry, err := a.q.WithTx(tx).GetFinanceEntryBySource(ctx, db.GetFinanceEntryBySourceParams{
		OrganizationID: a.orgs["demo-nl-distributor"], SourceType: "seed_demo",
		SourceUuid: src, Role: posting.RolePurchase, Revision: 1,
	})
	if err == nil {
		_, _ = a.q.WithTx(tx).InsertAccountingDispute(ctx, db.InsertAccountingDisputeParams{
			OrganizationID: a.orgs["demo-nl-distributor"], BrandID: a.brandID, CounterpartyOrgID: a.center,
			EntryID: buyerEntry.ID, SourceType: "seed_demo", SourceUuid: src, Reason: "Demo itiraz: iskonto bekleniyordu.",
			OpenedByUserID: pgtype.Int8{Int64: a.users["distributor_accounting@demo.olexfilms.app"], Valid: true},
		})
	}
	return nil
}

func (a *app) seedF3(ctx context.Context, tx pgx.Tx) error {
	var tpl int64
	err := tx.QueryRow(ctx, `SELECT id FROM contract_templates WHERE brand_id = $1 AND kind = 'vehicle_intake' AND is_default`, a.brandID).Scan(&tpl)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
INSERT INTO contract_templates (organization_id, brand_id, name, kind, is_default, is_active, created_by_user_id, updated_by_user_id)
VALUES ($1, $2, 'Demo Araç Kabul Sözleşmesi', 'vehicle_intake', true, true, $3, $3)
RETURNING id`, a.center, a.brandID, a.users["center_staff@demo.olexfilms.app"]).Scan(&tpl)
	}
	if err != nil {
		return err
	}
	_, _ = tx.Exec(ctx, `UPDATE contract_templates SET name = 'Demo Araç Kabul Sözleşmesi', is_active = true WHERE id = $1`, tpl)
	for _, loc := range []string{"tr", "en"} {
		html := "<p>Demo intake terms for Olex Films.</p>"
		if loc == "tr" {
			html = "<p>Olex Films demo araç kabul koşulları.</p>"
		}
		_, _ = tx.Exec(ctx, `
INSERT INTO contract_template_locales (template_id, organization_id, brand_id, locale, html, updated_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (template_id, locale) DO UPDATE SET html = EXCLUDED.html`, tpl, a.center, a.brandID, loc, html, a.users["center_staff@demo.olexfilms.app"])
	}
	var catalogItem int64
	if err := tx.QueryRow(ctx, `
INSERT INTO service_catalog_items (uuid, organization_id, brand_id, name, description, category, default_price, currency, recurrence, cancellation_fee, contract_template_id, is_active)
VALUES ($4, $1, $2, 'Demo Bayi Başlangıç Paketi', 'Eğitim, kurulum ve modül paketi', 'module_bundle', 750.00, 'EUR', 'yearly', 150.00, $3, true)
ON CONFLICT (uuid) DO NOTHING
RETURNING id`, a.center, a.brandID, tpl, stableUUID("service-catalog:starter")).Scan(&catalogItem); errors.Is(err, pgx.ErrNoRows) {
		_ = tx.QueryRow(ctx, `SELECT id FROM service_catalog_items WHERE brand_id = $1 AND name = 'Demo Bayi Başlangıç Paketi'`, a.brandID).Scan(&catalogItem)
	} else if err != nil {
		return err
	}
	_, _ = tx.Exec(ctx, `INSERT INTO service_catalog_modules (item_id, module_key) VALUES ($1, 'dealer_showcase'), ($1, 'reviews') ON CONFLICT DO NOTHING`, catalogItem)
	_, _ = tx.Exec(ctx, `
INSERT INTO service_price_overrides (item_id, organization_id, brand_id, price, currency)
VALUES ($1, $2, $3, 690.00, 'EUR')
ON CONFLICT (item_id, organization_id) DO UPDATE SET price = EXCLUDED.price`, catalogItem, a.orgs["demo-nl-distributor"], a.brandID)

	var ann int64
	if err := tx.QueryRow(ctx, `
INSERT INTO announcements (uuid, organization_id, brand_id, default_locale, title, body, status, pinned, notify, publish_at, author_user_id)
VALUES ($4, $1, $2, 'tr', 'Demo panel hazır', 'Yerel demo verisi yüklendi.', 'published', true, false, NOW(), $3)
ON CONFLICT (uuid) DO NOTHING
RETURNING id`, a.center, a.brandID, a.users["center_social@demo.olexfilms.app"], stableUUID("announcement:ready")).Scan(&ann); errors.Is(err, pgx.ErrNoRows) {
		_ = tx.QueryRow(ctx, `SELECT id FROM announcements WHERE title = 'Demo panel hazır' AND brand_id = $1`, a.brandID).Scan(&ann)
	} else if err != nil {
		return err
	}
	_, _ = tx.Exec(ctx, `INSERT INTO announcement_audiences (announcement_id, target_type) VALUES ($1, 'all_network') ON CONFLICT DO NOTHING`, ann)
	_, _ = tx.Exec(ctx, `INSERT INTO announcement_locales (announcement_id, locale, title, body) VALUES ($1, 'en', 'Demo panel is ready', 'Local demo data has been loaded.') ON CONFLICT (announcement_id, locale) DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body`, ann)

	var folder int64
	_ = tx.QueryRow(ctx, `
SELECT id FROM library_folders
WHERE organization_id = $1 AND parent_id IS NULL AND lower(name) = lower('Demo Materyaller') AND deleted_at IS NULL`,
		a.center).Scan(&folder)
	if folder == 0 {
		if err := tx.QueryRow(ctx, `
INSERT INTO library_folders (organization_id, brand_id, name, sort_order, created_by_user_id)
VALUES ($1, $2, 'Demo Materyaller', 10, $3)
RETURNING id`, a.center, a.brandID, a.users["center_social@demo.olexfilms.app"]).Scan(&folder); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(ctx, `
INSERT INTO library_items (uuid, organization_id, brand_id, folder_id, name, description, tags, access_level, created_by_user_id)
VALUES ($5, $1, $2, $3, 'PPF Uygulama Kontrol Listesi', 'Demo metadata kaydı', ARRAY['demo','ppf'], 'all_network', $4)
ON CONFLICT (uuid) DO NOTHING`, a.center, a.brandID, folder, a.users["center_social@demo.olexfilms.app"], stableUUID("library:item:ppf-checklist"))
	return nil
}

func ensureUnit(ctx context.Context, q *db.Queries, orgID, brandID, productID int64, barcode, kind, meters string) (db.Unit, error) {
	u, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: brandID, Barcode: barcode})
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Unit{}, err
	}
	var n pgtype.Numeric
	if meters != "" {
		if err := n.Scan(meters); err != nil {
			return db.Unit{}, err
		}
	}
	return q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: orgID, BrandID: brandID, ProductID: productID, Barcode: barcode,
		UnitKind: kind, Source: "generated", Status: "printed",
		InitialMeters: n, RemainingMeters: n,
	})
}

func post(ctx context.Context, l *ledger.Ledger, tx pgx.Tx, m ledger.Movement) error {
	_, err := l.Post(ctx, tx, m)
	return err
}

func tryPost(ctx context.Context, l *ledger.Ledger, tx pgx.Tx, m ledger.Movement) (ledger.Result, error) {
	if _, err := tx.Exec(ctx, `SAVEPOINT seed_demo_ledger_try`); err != nil {
		return ledger.Result{}, err
	}
	res, err := l.Post(ctx, tx, m)
	if err != nil {
		_, _ = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT seed_demo_ledger_try`)
		_, _ = tx.Exec(ctx, `RELEASE SAVEPOINT seed_demo_ledger_try`)
		return ledger.Result{}, err
	}
	if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT seed_demo_ledger_try`); err != nil {
		return ledger.Result{}, err
	}
	return res, nil
}

func ensureCar(ctx context.Context, tx pgx.Tx) (int64, int64, error) {
	var brand, model int64
	_ = tx.QueryRow(ctx, `SELECT id FROM car_brands WHERE lower(name) = lower('Tesla')`).Scan(&brand)
	if brand == 0 {
		if err := tx.QueryRow(ctx, `
INSERT INTO car_brands (name, active) VALUES ('Tesla', true)
RETURNING id`).Scan(&brand); err != nil {
			return 0, 0, err
		}
	}
	_ = tx.QueryRow(ctx, `SELECT id FROM car_models WHERE car_brand_id = $1 AND lower(name) = lower('Model Y')`, brand).Scan(&model)
	if model == 0 {
		if err := tx.QueryRow(ctx, `
INSERT INTO car_models (car_brand_id, name, body_type, active) VALUES ($1, 'Model Y', 'SUV', true)
RETURNING id`, brand).Scan(&model); err != nil {
			return 0, 0, err
		}
	}
	return brand, model, nil
}

type counts struct{ Total, Users, Orgs, Services, Warranties, Movements int64 }

func demoCounts(ctx context.Context, pool *pgxpool.Pool) (counts, error) {
	var c counts
	err := pool.QueryRow(ctx, `
SELECT
 (SELECT COUNT(*) FROM users WHERE email LIKE '%@demo.olexfilms.app') +
 (SELECT COUNT(*) FROM organizations WHERE slug LIKE 'demo-%') +
 (SELECT COUNT(*) FROM services WHERE service_no LIKE 'DEMO-SVC-%') +
 (SELECT COUNT(*) FROM warranties w JOIN services s ON s.id = w.service_id WHERE s.service_no LIKE 'DEMO-SVC-%') +
 (SELECT COUNT(*) FROM stock_movements WHERE idempotency_key LIKE 'seed-demo:%'),
 (SELECT COUNT(*) FROM users WHERE email LIKE '%@demo.olexfilms.app'),
 (SELECT COUNT(*) FROM organizations WHERE slug LIKE 'demo-%'),
 (SELECT COUNT(*) FROM services WHERE service_no LIKE 'DEMO-SVC-%'),
 (SELECT COUNT(*) FROM warranties w JOIN services s ON s.id = w.service_id WHERE s.service_no LIKE 'DEMO-SVC-%'),
 (SELECT COUNT(*) FROM stock_movements WHERE idempotency_key LIKE 'seed-demo:%')`).Scan(
		&c.Total, &c.Users, &c.Orgs, &c.Services, &c.Warranties, &c.Movements,
	)
	return c, err
}

func ensureLegacyDB(ctx context.Context, cfg config.DBConfig) (string, error) {
	admin := cfg
	admin.Name = "postgres"
	pool, err := pgxpool.New(ctx, admin.DSN())
	if err != nil {
		return "", err
	}
	defer pool.Close()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, legacyDBName).Scan(&exists); err != nil {
		return "", err
	}
	if !exists {
		if _, err := pool.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{legacyDBName}.Sanitize()); err != nil {
			return "", err
		}
	}
	cfg.Name = legacyDBName
	return cfg.DSN(), nil
}

func applyDatabaseURL(raw string, cfg *config.DBConfig) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("TEST_DATABASE_URL: %w", err)
	}
	cfg.Host = u.Hostname()
	if p := u.Port(); p != "" {
		_, _ = fmt.Sscan(p, &cfg.Port)
	}
	cfg.Name = strings.TrimPrefix(u.Path, "/")
	cfg.User = u.User.Username()
	if pass, ok := u.User.Password(); ok {
		cfg.Password = pass
	}
	if ssl := u.Query().Get("sslmode"); ssl != "" {
		cfg.SSLMode = ssl
	}
	return nil
}

func isLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == defaultDBHost || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func randomPassword() (string, error) {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "Demo1!" + hex.EncodeToString(b[:]), nil
}

func stableUUID(key string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("olexfilms-seed-demo:"+key))
}

func appendCredentials(pass string) error {
	if err := os.MkdirAll(filepath.Dir(credentials), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(credentials, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "\n## seed-demo %s\n- URL: http://localhost:13000\n- E-posta deseni: `<rol>@demo.olexfilms.app`\n- Şifre: `%s`\n", time.Now().Format(time.RFC3339), pass)
	return err
}
