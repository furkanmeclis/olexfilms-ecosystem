package features

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-472: a fleet organization (type fleet, no parent) is outside the
// features resolver: no snapshot, no admin switch, every non-core module
// off, and it never shows up in a distributor's dealer matrix.
func TestFleetOutsideResolver(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()
	fleet, err := d.q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t472-fleet-" + d.suffix, Name: "Fleet " + d.suffix, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           OrgFleet, BrandID: d.center.BrandID, Currency: "TRY", Locale: "tr",
		Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("create fleet: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", fleet.ID)
	})

	if _, err := d.svc.Snapshot(ctx, fleet.ID); !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("snapshot of a fleet: err = %v, want ErrOrganizationNotFound", err)
	}
	if _, err := d.svc.SetByAdmin(ctx, 0, fleet.ID, "stock_forecast", false); !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("admin switch on a fleet: err = %v, want ErrOrganizationNotFound", err)
	}
	if on, err := d.svc.Enabled(ctx, fleet.ID, "stock_forecast"); err != nil || on {
		t.Fatalf("fleet stock_forecast = %v, %v; want off without error", on, err)
	}

	dist := d.org("t472-dist", OrgDistributor, d.center)
	matrix, err := d.svc.DealerMatrix(ctx, dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matrix {
		if m.OrgID == fleet.ID {
			t.Fatal("fleet appears in the dealer matrix")
		}
	}
}
