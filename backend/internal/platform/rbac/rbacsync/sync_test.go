package rbacsync

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// The migration seed equals the Go catalog: a dry run on a migrated database
// reports nothing to do.
func TestMigrationMatchesCatalog(t *testing.T) {
	pool := testPool(t)
	rep, err := Sync(context.Background(), pool, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changes() != 0 || len(rep.PermissionsUnknown) != 0 {
		t.Fatalf("migration drifted from catalog: %+v", rep)
	}
}

// roles-sync repairs drift and a second run reports no diff. Everything runs
// inside an outer transaction that is rolled back, so concurrent tests never
// see the tampered packages.
func TestSyncIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	outer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = outer.Rollback(ctx) }()

	tamper := []string{
		// widen a grant, drop a grant, add a foreign grant, rename a role.
		`UPDATE role_permissions SET scope = 'subtree'
		 WHERE role_id = (SELECT id FROM roles WHERE slug = 'dealer_owner')
		   AND permission_id = (SELECT id FROM permissions WHERE slug = 'services.read')`,
		`DELETE FROM role_permissions
		 WHERE role_id = (SELECT id FROM roles WHERE slug = 'center_social')
		   AND permission_id = (SELECT id FROM permissions WHERE slug = 'campaigns.read')`,
		`INSERT INTO role_permissions (role_id, permission_id, scope)
		 SELECT r.id, p.id, 'managed' FROM roles r, permissions p
		 WHERE r.slug = 'dealer_staff' AND p.slug = 'pricing.sale.read'`,
		`UPDATE roles SET name = 'tampered' WHERE slug = 'fleet'`,
		`UPDATE permissions SET is_sensitive = false WHERE slug = 'pricing.purchase.read'`,
	}
	for _, sql := range tamper {
		if _, err := outer.Exec(ctx, sql); err != nil {
			t.Fatalf("tamper %q: %v", sql, err)
		}
	}
	first, err := Sync(ctx, outer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.GrantsUpdated) != 1 || len(first.GrantsAdded) != 1 || len(first.GrantsRemoved) != 1 ||
		len(first.RolesUpdated) != 1 || len(first.PermissionsUpdated) != 1 {
		t.Fatalf("first run report: %+v", first)
	}
	second, err := Sync(ctx, outer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second run must be a no-op: %+v", second)
	}
}

// The database refuses impersonation for any role other than super_admin and
// scopes a permission does not allow.
func TestGrantTriggers(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SAVEPOINT a`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id, scope)
		SELECT r.id, p.id, 'all' FROM roles r, permissions p
		WHERE r.slug = 'center_staff' AND p.slug = 'platform.users.impersonate'`)
	if err == nil {
		t.Fatal("impersonation grant to center_staff must fail")
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT a`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE role_permissions SET scope = 'all'
		WHERE role_id = (SELECT id FROM roles WHERE slug = 'dealer_staff')
		  AND permission_id = (SELECT id FROM permissions WHERE slug = 'auth.session')`)
	if err == nil {
		t.Fatal("scope outside the permission's scopes must fail")
	}
}
