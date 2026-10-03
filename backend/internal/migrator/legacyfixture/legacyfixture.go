// Package legacyfixture loads the synthetic legacy (Laravel hub + warehouse)
// schemas into a test PostgreSQL so migrator tests can run without touching
// the real legacy databases (TEC-253).
//
// The fixture lives in backend/internal/migrator/testdata/legacy_hub.sql and
// legacy_wh.sql. Each file drops and recreates its own schema (legacy_hub,
// legacy_wh), so loading is idempotent. The fixture is deliberately outside
// golang-migrate: it never reaches the migrations directory or production.
package legacyfixture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema names created by the fixture.
const (
	HubSchema = "legacy_hub"
	WHSchema  = "legacy_wh"
)

// advisoryLockKey serialises concurrent loads (several test packages may load
// the fixture against the same CI database at once).
const advisoryLockKey int64 = 0x7465633235330001 // "tec253" + 1

// files are loaded in this order inside one transaction.
var files = []string{"legacy_hub.sql", "legacy_wh.sql"}

// ExpectedRowCounts is the row count of every fixture table, keyed by
// "schema.table". The fixture creates exactly these tables and no others.
var ExpectedRowCounts = map[string]int64{
	"legacy_hub.dealers":                    3,
	"legacy_hub.users":                      5,
	"legacy_hub.roles":                      4,
	"legacy_hub.model_has_roles":            5,
	"legacy_hub.customers":                  8,
	"legacy_hub.car_brands":                 2,
	"legacy_hub.car_models":                 3,
	"legacy_hub.product_categories":         2,
	"legacy_hub.products":                   4,
	"legacy_hub.stock_items":                8,
	"legacy_hub.stock_movements":            16,
	"legacy_hub.orders":                     3,
	"legacy_hub.order_items":                4,
	"legacy_hub.order_item_stock":           4,
	"legacy_hub.services":                   3,
	"legacy_hub.service_items":              3,
	"legacy_hub.service_images":             3,
	"legacy_hub.service_status_logs":        3,
	"legacy_hub.warranties":                 4,
	"legacy_hub.nexptg_api_users":           2,
	"legacy_hub.nexptg_reports":             2,
	"legacy_hub.nexptg_report_measurements": 6,
	"legacy_hub.service_nexptg_report":      1,
	"legacy_hub.service_customer_transfers": 1,
	"legacy_hub.short_urls":                 4,
	"legacy_hub.sms_logs":                   3,
	"legacy_hub.notifications":              2,

	"legacy_wh.warehouses":          2,
	"legacy_wh.warehouse_sites":     2,
	"legacy_wh.warehouse_locations": 5,
	"legacy_wh.brands":              2,
	"legacy_wh.product_categories":  1,
	"legacy_wh.products":            2,
	"legacy_wh.customers":           2,
	"legacy_wh.product_barcodes":    4,
	"legacy_wh.bin_product_stocks":  3,
	"legacy_wh.stock_movements":     5,
	"legacy_wh.orders":              2,
	"legacy_wh.order_items":         2,
	"legacy_wh.order_item_barcodes": 1,
}

// TestdataDir returns the absolute path of backend/internal/migrator/testdata.
func TestdataDir() (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("legacyfixture: cannot resolve source path")
	}
	return filepath.Join(filepath.Dir(self), "..", "testdata"), nil
}

// Load drops and recreates legacy_hub and legacy_wh inside tx and seeds them.
// search_path is pinned to pg_catalog for the transaction so an unqualified
// name in the fixture fails instead of silently landing in public.
func Load(ctx context.Context, tx pgx.Tx) error {
	dir, err := TestdataDir()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("legacyfixture: advisory lock: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO pg_catalog"); err != nil {
		return fmt.Errorf("legacyfixture: search_path: %w", err)
	}
	for _, name := range files {
		sql, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("legacyfixture: read %s: %w", name, err)
		}
		// No arguments -> simple protocol, so the multi-statement file runs as is.
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("legacyfixture: load %s: %w", name, err)
		}
	}
	return nil
}

// LoadLegacyFixture loads the legacy fixture into pool in a single
// transaction, failing the test on error. Calling it again resets the
// legacy schemas to the same state (DROP SCHEMA ... CASCADE + reload).
func LoadLegacyFixture(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("legacyfixture: begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := Load(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("legacyfixture: commit: %v", err)
	}
}

// LoadAndHold loads the fixture like LoadLegacyFixture, then holds a shared
// advisory lock on the load key until the test ends, so a reload started by
// another test package (DROP SCHEMA ... CASCADE) waits instead of pulling the
// tables away from a test that is still reading them (TEC-252).
func LoadAndHold(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	LoadLegacyFixture(t, pool)
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("legacyfixture: acquire: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock_shared($1)", advisoryLockKey); err != nil {
		conn.Release()
		t.Fatalf("legacyfixture: shared lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock_shared($1)", advisoryLockKey)
		conn.Release()
	})
}
