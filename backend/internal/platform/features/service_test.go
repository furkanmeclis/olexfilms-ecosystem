package features

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Database tests use their own module keys (leads, appointments,
// stock_forecast) so they never race the HTTP integration tests, which
// switch other modules system wide.

type dbtest struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *db.Queries
	svc    *Service
	mr     *miniredis.Miniredis
	center db.Organization
	suffix string
}

func newDBTest(t *testing.T) *dbtest {
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
	q := db.New(pool)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := New(pool, q, NewRedisCache(rdb, "test", nil), log)
	ctx := context.Background()
	b, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &dbtest{t: t, pool: pool, q: q, svc: svc, mr: mr, center: center, suffix: fmt.Sprintf("%d", time.Now().UnixNano())}
}

func (d *dbtest) org(name, typ string, parent db.Organization) db.Organization {
	d.t.Helper()
	row, err := d.q.CreateOrganization(context.Background(), db.CreateOrganizationParams{
		Slug: "t86-" + name + "-" + d.suffix, Name: name + " " + d.suffix, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent.ID, Valid: true},
		BrandID: parent.BrandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		d.t.Fatalf("create org %s: %v", name, err)
	}
	d.t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", row.ID)
	})
	return row
}

func (d *dbtest) closeSystem(key string) {
	d.t.Helper()
	off := false
	if _, err := d.svc.UpdatePlatformModule(context.Background(), 0, key, PlatformModuleInput{Enabled: &off}); err != nil {
		d.t.Fatalf("close %s: %v", key, err)
	}
	d.t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), "DELETE FROM module_flags WHERE scope = 'system' AND module_key = $1", key)
	})
}

func (d *dbtest) enabled(org db.Organization, key string) bool {
	d.t.Helper()
	on, err := d.svc.Enabled(context.Background(), org.ID, key)
	if err != nil {
		d.t.Fatal(err)
	}
	return on
}

// The migration seed equals the Go catalog.
func TestMigrationMatchesModuleCatalog(t *testing.T) {
	d := newDBTest(t)
	rows, err := d.q.ListModules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(Modules) {
		t.Fatalf("modules table has %d rows, catalog %d", len(rows), len(Modules))
	}
	for i, m := range Modules {
		r := rows[i]
		if r.Key != m.Key || r.Level != string(m.Level) || r.SortOrder != m.SortOrder() {
			t.Fatalf("row %d = %+v, catalog %+v", i, r, m)
		}
	}
	if err := d.svc.SyncCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Acceptance 1: a standard module closed system wide cannot be opened by
// any distributor (nor by the admin per organization), and the cached
// "on" disappears without waiting for the TTL.
func TestSystemClosedBlocksEveryone(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()
	dist := d.org("dist", OrgDistributor, d.center)
	dealer := d.org("dealer", OrgDealer, dist)

	if !d.enabled(dealer, ModuleLeads) {
		t.Fatal("standard module starts on")
	}
	// Snapshot is now cached; closing must still take effect at once.
	d.closeSystem(ModuleLeads)
	if d.enabled(dealer, ModuleLeads) || d.enabled(dist, ModuleLeads) {
		t.Fatal("closed system wide must be off without waiting for the TTL")
	}
	if err := d.svc.SetDealerStandard(ctx, 0, dist.ID, ModuleLeads, true); !errors.Is(err, ErrUpstreamDisabled) {
		t.Fatalf("distributor standard = %v", err)
	}
	if err := d.svc.SetForDealers(ctx, 0, dist.ID, []int64{dealer.ID}, ModuleLeads, true); !errors.Is(err, ErrUpstreamDisabled) {
		t.Fatalf("distributor dealer = %v", err)
	}
	if _, err := d.svc.SetByAdmin(ctx, 0, dealer.ID, ModuleLeads, true); !errors.Is(err, ErrUpstreamDisabled) {
		t.Fatalf("admin below a closed system = %v", err)
	}
	// Switching off stays allowed.
	if err := d.svc.SetForDealers(ctx, 0, dist.ID, []int64{dealer.ID}, ModuleLeads, false); err != nil {
		t.Fatal(err)
	}
}

func TestCoreCannotBeSwitched(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()
	dist := d.org("dist", OrgDistributor, d.center)
	dealer := d.org("dealer", OrgDealer, dist)
	off := false
	if _, err := d.svc.UpdatePlatformModule(ctx, 0, ModuleServices, PlatformModuleInput{Enabled: &off}); !errors.Is(err, ErrCoreModule) {
		t.Fatalf("system close core = %v", err)
	}
	if _, err := d.svc.UpdatePlatformModule(ctx, 0, ModuleServices, PlatformModuleInput{DefaultEnabled: &off}); !errors.Is(err, ErrCoreModule) {
		t.Fatalf("core default off = %v", err)
	}
	if _, err := d.svc.SetByAdmin(ctx, 0, dealer.ID, ModuleServices, false); !errors.Is(err, ErrCoreModule) {
		t.Fatalf("admin core = %v", err)
	}
	if err := d.svc.SetForDealers(ctx, 0, dist.ID, []int64{dealer.ID}, ModuleServices, false); !errors.Is(err, ErrCoreModule) {
		t.Fatalf("distributor core = %v", err)
	}
	if err := d.svc.SetDealerStandard(ctx, 0, dist.ID, ModuleServices, false); !errors.Is(err, ErrCoreModule) {
		t.Fatalf("standard core = %v", err)
	}
	if !d.enabled(dealer, ModuleServices) {
		t.Fatal("core stays on")
	}
}

// Acceptance 2: a distributor opens an add-on for three dealers at once; a
// dealer of another distributor in the batch rejects the whole batch.
func TestBulkAddonForThreeDealers(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()
	dist := d.org("dist", OrgDistributor, d.center)
	other := d.org("other", OrgDistributor, d.center)
	a, b, c := d.org("a", OrgDealer, dist), d.org("b", OrgDealer, dist), d.org("c", OrgDealer, dist)
	foreign := d.org("foreign", OrgDealer, other)
	ids := []int64{a.ID, b.ID, c.ID}

	// The distributor has no add-on yet: cannot pass it down.
	if err := d.svc.SetForDealers(ctx, 0, dist.ID, ids, ModuleStockForecast, true); !errors.Is(err, ErrUpstreamDisabled) {
		t.Fatalf("add-on the distributor lacks = %v", err)
	}
	if _, err := d.svc.SetByAdmin(ctx, 0, dist.ID, ModuleStockForecast, true); err != nil {
		t.Fatal(err)
	}
	if err := d.svc.SetForDealers(ctx, 0, dist.ID, append(ids, foreign.ID), ModuleStockForecast, true); !errors.Is(err, ErrNotOwnDealer) {
		t.Fatalf("foreign dealer in batch = %v", err)
	}
	if d.enabled(a, ModuleStockForecast) {
		t.Fatal("rejected batch must not change anything")
	}
	if err := d.svc.SetForDealers(ctx, 0, dist.ID, ids, ModuleStockForecast, true); err != nil {
		t.Fatal(err)
	}
	for _, o := range []db.Organization{a, b, c} {
		if !d.enabled(o, ModuleStockForecast) {
			t.Fatalf("%s: add-on not on", o.Slug)
		}
	}
	if d.enabled(foreign, ModuleStockForecast) {
		t.Fatal("other distributor's dealer untouched")
	}
	matrix, err := d.svc.DealerMatrix(ctx, dist.ID)
	if err != nil || len(matrix) != 3 {
		t.Fatalf("matrix = %d %v", len(matrix), err)
	}
	// Closing the distributor closes its dealers at once (cache dropped).
	if _, err := d.svc.SetByAdmin(ctx, 0, dist.ID, ModuleStockForecast, false); err != nil {
		t.Fatal(err)
	}
	if d.enabled(a, ModuleStockForecast) {
		t.Fatal("distributor closed -> dealer closed")
	}
}

// A new dealer gets the dealer standard; changing the standard reaches
// existing dealers (live inheritance, no copies).
func TestNewDealerGetsStandard(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()
	dist := d.org("dist", OrgDistributor, d.center)
	old := d.org("old", OrgDealer, dist)
	if err := d.svc.SetDealerStandard(ctx, 0, dist.ID, ModuleAppointments, false); err != nil {
		t.Fatal(err)
	}
	fresh := d.org("fresh", OrgDealer, dist)
	if d.enabled(fresh, ModuleAppointments) || d.enabled(old, ModuleAppointments) {
		t.Fatal("standard off must apply to old and new dealers")
	}
	if err := d.svc.SetDealerStandard(ctx, 0, dist.ID, ModuleAppointments, true); err != nil {
		t.Fatal(err)
	}
	if !d.enabled(fresh, ModuleAppointments) || !d.enabled(old, ModuleAppointments) {
		t.Fatal("standard change must reach dealers at once")
	}
	var n int
	if err := d.pool.QueryRow(ctx, "SELECT count(*) FROM module_flags WHERE organization_id = ANY($1)",
		[]int64{old.ID, fresh.ID}).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no rows copied to dealers: %d %v", n, err)
	}
	std, err := d.svc.DealerStandard(ctx, dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := lookupStd(std, ModuleAppointments); !ok || !e.Explicit || !e.Enabled {
		t.Fatalf("standard entry = %+v", e)
	}
}

func lookupStd(items []StandardEntry, key string) (StandardEntry, bool) {
	for _, e := range items {
		if e.Key == key {
			return e, true
		}
	}
	return StandardEntry{}, false
}

// The admin opens an add-on for a dealer although the distributor lacks it;
// the distributor cannot override the admin's value.
func TestAdminExceptionBelowClosedDistributor(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()
	dist := d.org("dist", OrgDistributor, d.center)
	dealer := d.org("dealer", OrgDealer, dist)
	if _, err := d.svc.SetByAdmin(ctx, 0, dealer.ID, ModuleStockForecast, true); err != nil {
		t.Fatal(err)
	}
	if !d.enabled(dealer, ModuleStockForecast) {
		t.Fatal("admin exception")
	}
	if err := d.svc.SetForDealers(ctx, 0, dist.ID, []int64{dealer.ID}, ModuleStockForecast, false); !errors.Is(err, ErrAdminOverride) {
		t.Fatalf("distributor over admin = %v", err)
	}
	if _, err := d.svc.ClearByAdmin(ctx, dealer.ID, ModuleStockForecast); err != nil {
		t.Fatal(err)
	}
	if d.enabled(dealer, ModuleStockForecast) {
		t.Fatal("cleared admin value falls back to the closed distributor")
	}
}

func TestUnknownKeyIsOff(t *testing.T) {
	d := newDBTest(t)
	dist := d.org("dist", OrgDistributor, d.center)
	if d.enabled(dist, "no_such_module") {
		t.Fatal("unknown key must be off")
	}
	if _, err := d.svc.SetByAdmin(context.Background(), 0, dist.ID, "no_such_module", true); !errors.Is(err, ErrUnknownModule) {
		t.Fatalf("unknown write = %v", err)
	}
}

// Redis down: snapshots come from the database (never fail-open).
func TestRedisDownFallsBackToDatabase(t *testing.T) {
	d := newDBTest(t)
	dist := d.org("dist", OrgDistributor, d.center)
	d.mr.Close()
	if d.enabled(dist, ModuleAIAssistant) {
		t.Fatal("add-on default off with Redis down")
	}
	if !d.enabled(dist, ModuleAppointments) {
		t.Fatal("standard default on with Redis down")
	}
}
