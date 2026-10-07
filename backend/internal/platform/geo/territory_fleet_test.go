package geo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-472: a fleet organization never owns a territory, so the overlap
// check never sees it.
func TestTerritoryRefusesFleet(t *testing.T) {
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
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	fleet, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t472-geo-fleet-%d", time.Now().UnixNano()), Name: "Filo", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "fleet", BrandID: brand.ID, Currency: "TRY", Locale: "tr",
		Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The guard runs before any transaction of the service (pool unused).
	svc := New(nil, q)
	if _, err := svc.Assign(ctx, AssignInput{BrandID: brand.ID, DistributorID: fleet.ID, CountryID: 1}); !errors.Is(err, ErrNotDistributor) {
		t.Fatalf("assign territory to a fleet: err = %v, want ErrNotDistributor", err)
	}
}
