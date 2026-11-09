package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-1090 (TEC-1069): the product totals of one warehouse sum its
// locations only (another warehouse of the organization stays out), count
// the rolls in stock with meters left and honour the status filter; a
// warehouse of another organization is 404.
func TestWarehouseStockTotals(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	id := func(sql string, args ...any) int64 {
		t.Helper()
		var v int64
		if err := tx.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return v
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	sfx := fmt.Sprintf("t1090-%d", time.Now().UnixNano())
	brand := id(`INSERT INTO brands (slug, name) VALUES ($1, $1) RETURNING id`, sfx)
	org := id(`INSERT INTO organizations (slug, name, status, type, brand_id, currency, locale, timezone, settings, access_starts_at)
VALUES ($1, $1, 'active', 'center', $2, 'TRY', 'tr', 'Europe/Istanbul', '{}', NOW() - interval '1 hour') RETURNING id`, sfx+"-c", brand)
	other := id(`INSERT INTO organizations (slug, name, status, type, parent_id, brand_id, currency, locale, timezone, settings, access_starts_at)
VALUES ($1, $1, 'active', 'dealer', $3, $2, 'TRY', 'tr', 'Europe/Istanbul', '{}', NOW() - interval '1 hour') RETURNING id`, sfx+"-d", brand, org)
	cat := id(`INSERT INTO product_categories (organization_id, brand_id, name, available_parts) VALUES ($1, $2, $3, '[]') RETURNING id`, org, brand, sfx)
	roll := id(`INSERT INTO products (organization_id, brand_id, category_id, sku, name, unit_type) VALUES ($1, $2, $3, $4, 'A roll', 'roll_meter') RETURNING id`, org, brand, cat, sfx+"-r")
	piece := id(`INSERT INTO products (organization_id, brand_id, category_id, sku, name, unit_type) VALUES ($1, $2, $3, $4, 'B piece', 'piece') RETURNING id`, org, brand, cat, sfx+"-p")
	empty := id(`INSERT INTO products (organization_id, brand_id, category_id, sku, name, unit_type) VALUES ($1, $2, $3, $4, 'C empty', 'piece') RETURNING id`, org, brand, cat, sfx+"-e")

	location := func(orgID int64, code string) (whID, locID int64, whUUID uuid.UUID) {
		t.Helper()
		if err := tx.QueryRow(ctx, `INSERT INTO warehouses (organization_id, code, name) VALUES ($1, $2, $2) RETURNING id, uuid`, orgID, code).Scan(&whID, &whUUID); err != nil {
			t.Fatal(err)
		}
		room := id(`INSERT INTO rooms (organization_id, warehouse_id, code, name) VALUES ($1, $2, 'R1', 'R1') RETURNING id`, orgID, whID)
		locID = id(`INSERT INTO warehouse_locations (organization_id, room_id, warehouse_id, type, code, name, full_code)
VALUES ($1, $2, $3, 'aisle', 'A', 'A', 'x') RETURNING id`, orgID, room, whID)
		return whID, locID, whUUID
	}
	_, locA, whA := location(org, "MAIN")
	_, locB, _ := location(org, "SIDE")
	_, _, whOther := location(other, "DLR")

	bin := func(loc, product int64, qty int, meters string) {
		exec(`INSERT INTO bin_product_stocks (location_id, organization_id, brand_id, product_id, quantity, meters) VALUES ($1, $2, $3, $4, $5, $6)`,
			loc, org, brand, product, qty, meters)
	}
	bin(locA, roll, 2, "75.50")
	bin(locA, piece, 4, "0")
	bin(locA, empty, 0, "0")
	bin(locB, piece, 10, "0")
	unit := func(loc int64, remaining string) {
		u := id(`INSERT INTO units (organization_id, brand_id, product_id, barcode, status, initial_meters, remaining_meters)
VALUES ($1, $2, $3, $4, 'placed', 50, $5) RETURNING id`, org, brand, roll, sfx+"-"+uuid.NewString()[:8], remaining)
		exec(`INSERT INTO unit_current_state (unit_id, brand_id, owner_type, owner_id, holder_org_id, status) VALUES ($1, $2, 'warehouse_location', $3, $4, 'placed')`,
			u, brand, loc, org)
	}
	unit(locA, "50")
	unit(locA, "25.50")
	unit(locA, "0") // used up: not a roll in stock
	unit(locB, "40")

	s := stockusecase.New(q)
	f := scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{org}, OrgID: org}
	in := model.StockFilter{Limit: 50, Sort: apiquery.ResolvedSort{Key: "product"}}
	rows, total, err := s.WarehouseStock(ctx, f, org, whA, in)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("rows = %d total = %d", len(rows), total)
	}
	if r := rows[0]; r.Product.Name != "A roll" || r.Quantity != 2 || r.Meters != "75.50" || r.RollCount != 2 {
		t.Fatalf("roll row = %+v", r)
	}
	if r := rows[1]; r.Product.Name != "B piece" || r.Quantity != 4 || r.RollCount != 0 {
		t.Fatalf("piece row = %+v (the other warehouse leaked?)", r)
	}
	in.Status = model.StatusInStock
	rows, total, err = s.WarehouseStock(ctx, f, org, whA, in)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("in_stock rows = %d total = %d err = %v", len(rows), total, err)
	}
	in.Status = model.StatusOutOfStock
	rows, _, err = s.WarehouseStock(ctx, f, org, whA, in)
	if err != nil || len(rows) != 1 || rows[0].Product.Name != "C empty" {
		t.Fatalf("out_of_stock rows = %+v err = %v", rows, err)
	}
	if _, _, err := s.WarehouseStock(ctx, f, org, whOther, model.StockFilter{Limit: 10}); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("other organization's warehouse err = %v", err)
	}
}
