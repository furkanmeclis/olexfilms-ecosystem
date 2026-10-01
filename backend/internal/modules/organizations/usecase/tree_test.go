package usecase

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidChildType(t *testing.T) {
	cases := []struct {
		parent, child string
		ok            bool
	}{
		{TypeCenter, TypeDistributor, true},
		{TypeCenter, TypeDealer, true},
		{TypeDistributor, TypeDealer, true},
		{TypeDistributor, TypeDistributor, false},
		{TypeDealer, TypeDealer, false},
		{TypeDealer, TypeDistributor, false},
		{TypeCenter, TypeCenter, false},
	}
	for _, c := range cases {
		if got := validChildType(c.parent, c.child); got != c.ok {
			t.Fatalf("validChildType(%s, %s) = %v, want %v", c.parent, c.child, got, c.ok)
		}
	}
}

// testPool connects to TEST_DATABASE_URL (a migrated database) or skips.
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

// TestSupplierOfChain: dealer -> distributor -> center -> nil (K9).
func TestSupplierOfChain(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := db.New(pool)
	svc := New(pool, q)

	olex, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, olex.ID)
	if err != nil {
		t.Fatalf("olex center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	create := func(name, typ string, parentID int64) db.Organization {
		t.Helper()
		row, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: "t83-" + name + "-" + suffix, Name: name, Status: "active",
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type:           typ, ParentID: pgtype.Int8{Int64: parentID, Valid: true},
			BrandID: olex.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
			Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return row
	}
	dist := create("dist", TypeDistributor, center.ID)
	dealer := create("dealer", TypeDealer, dist.ID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id = $1", dealer.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id = $1", dist.ID)
	})

	got, err := svc.SupplierOf(ctx, dealer.ID)
	if err != nil || got == nil || got.ID != dist.ID {
		t.Fatalf("supplier of dealer: %+v %v", got, err)
	}
	got, err = svc.SupplierOf(ctx, dist.ID)
	if err != nil || got == nil || got.ID != center.ID {
		t.Fatalf("supplier of distributor: %+v %v", got, err)
	}
	got, err = svc.SupplierOf(ctx, center.ID)
	if err != nil || got != nil {
		t.Fatalf("supplier of center must be nil: %+v %v", got, err)
	}

	desc, err := svc.Descendants(ctx, dist.ID)
	if err != nil || len(desc) != 1 || desc[0].ID != dealer.ID {
		t.Fatalf("descendants of distributor: %+v %v", desc, err)
	}
	all, err := svc.Descendants(ctx, center.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, o := range all {
		seen[o.ID] = true
	}
	if !seen[dist.ID] || !seen[dealer.ID] {
		t.Fatalf("center descendants missing tree members")
	}

	// A center row with a parent, or a dealer without one, is rejected by the DB.
	if _, err := pool.Exec(ctx, "UPDATE organizations SET parent_id = NULL WHERE id = $1", dealer.ID); err == nil {
		t.Fatal("dealer without parent must violate chk_organizations_parent")
	}
}
