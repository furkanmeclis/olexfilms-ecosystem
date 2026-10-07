package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-472 acceptance. Every test runs in one rolled-back transaction.

type fixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	store   *Store
	brandID int64
	dist    db.Organization
	dealer  db.Organization
	dealer2 db.Organization
	suffix  string
	seq     int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &fixture{ctx: ctx, tx: tx, q: db.New(tx), store: New(tx), suffix: fmt.Sprintf("%d", time.Now().UnixNano())}
	brand, err := f.q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := f.q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	f.brandID = brand.ID
	f.dist = f.org(t, "dist", "distributor", center.ID)
	f.dealer = f.org(t, "dealer", "dealer", f.dist.ID)
	f.dealer2 = f.org(t, "dealer2", "dealer", f.dist.ID)
	return f
}

func (f *fixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	f.seq++
	arg := db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t472-%s-%s-%d", name, f.suffix, f.seq), Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, BrandID: f.brandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	}
	if parent != 0 {
		arg.ParentID = pgtype.Int8{Int64: parent, Valid: true}
	}
	o, err := f.q.CreateOrganization(f.ctx, arg)
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

// fleet opens a fleet organization with its profile.
func (f *fixture) fleet(t *testing.T, name, taxNumber string) db.Organization {
	t.Helper()
	o := f.org(t, name, "fleet", 0)
	if _, err := f.store.CreateProfile(f.ctx, db.CreateFleetProfileParams{
		OrganizationID: o.ID, BrandID: o.BrandID, TaxNumber: taxNumber, LegalName: name + " A.Ş.",
		ReportFrequency: model.ReportMonthly, ReportLocale: "tr",
	}); err != nil {
		t.Fatalf("profile %s: %v", name, err)
	}
	return o
}

func (f *fixture) link(t *testing.T, fleet, dealer db.Organization, status string) db.FleetDealerLink {
	t.Helper()
	l, err := f.store.CreateLink(f.ctx, linkParams(fleet, dealer, status))
	if err != nil {
		t.Fatalf("link %s-%s: %v", fleet.Name, dealer.Name, err)
	}
	return l
}

func linkParams(fleet, dealer db.Organization, status string) db.CreateFleetDealerLinkParams {
	return db.CreateFleetDealerLinkParams{
		FleetOrgID: fleet.ID, DealerOrgID: dealer.ID, BrandID: fleet.BrandID, Status: status,
		CreatedByOrgID: dealer.ID,
	}
}

func (f *fixture) vehicle(t *testing.T, fleet db.Organization, owner db.User, plate string) db.Vehicle {
	t.Helper()
	v, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: owner.ID, BrandID: fleet.BrandID,
		Plate: pgtype.Text{String: plate, Valid: true}, PlateNormalized: pgtype.Text{String: plate, Valid: true},
		PlateCountry: pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle %s: %v", plate, err)
	}
	v, err = f.q.SetVehicleFleet(f.ctx, db.SetVehicleFleetParams{ID: v.ID, FleetOrgID: pgtype.Int8{Int64: fleet.ID, Valid: true}})
	if err != nil {
		t.Fatalf("vehicle fleet %s: %v", plate, err)
	}
	return v
}

func (f *fixture) user(t *testing.T) db.User {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Filo", Surname: "Sahibi", Status: "active",
		Email: pgtype.Text{String: fmt.Sprintf("t472-%s-%d@example.test", f.suffix, f.seq), Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return u
}

// savepoint runs fn in a nested transaction that is always rolled back, so
// an expected failure leaves the outer transaction usable.
func (f *fixture) savepoint(t *testing.T, fn func(s *Store) error) error {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sp.Rollback(f.ctx) }()
	return fn(New(sp))
}

// Acceptance: a second open (pending or active) link of the same (fleet,
// dealer) fails; another dealer, or the same dealer after the link ended,
// links fine.
func TestSecondOpenLinkFails(t *testing.T) {
	f := newFixture(t)
	fl := f.fleet(t, "Filo A", "1089325650")
	first := f.link(t, fl, f.dealer, model.LinkActive)
	if !first.StartedAt.Valid || first.Status != model.LinkActive {
		t.Fatalf("active link = %+v", first)
	}
	for _, status := range []string{model.LinkActive, model.LinkPending} {
		err := f.savepoint(t, func(s *Store) error {
			_, err := s.CreateLink(f.ctx, linkParams(fl, f.dealer, status))
			return err
		})
		if !errors.Is(err, ErrLinkExists) {
			t.Fatalf("second %s link: err = %v, want ErrLinkExists", status, err)
		}
	}
	// Another dealer of the same fleet is a separate pair.
	f.link(t, fl, f.dealer2, model.LinkPending)

	// Ending the link frees the pair; the ended row stays as history.
	ended, err := f.store.TransitionLink(f.ctx, first.Uuid, model.LinkActive, model.LinkEnded)
	if err != nil || ended.Status != model.LinkEnded || !ended.EndedAt.Valid {
		t.Fatalf("end link = %+v, %v", ended, err)
	}
	f.link(t, fl, f.dealer, model.LinkActive)
	// ended is final; the new link replaces it.
	if _, err := f.store.TransitionLink(f.ctx, ended.Uuid, model.LinkEnded, model.LinkActive); !errors.Is(err, ErrLinkStale) {
		t.Fatalf("reopen ended link: err = %v, want ErrLinkStale", err)
	}
}

// The link status moves with a compare-and-set: a stale from_status moves
// nothing.
func TestLinkTransitionCAS(t *testing.T) {
	f := newFixture(t)
	fl := f.fleet(t, "Filo CAS", "9249799759")
	l := f.link(t, fl, f.dealer, model.LinkPending)
	if l.StartedAt.Valid {
		t.Fatalf("pending link has started_at: %+v", l)
	}
	got, err := f.store.TransitionLink(f.ctx, l.Uuid, model.LinkPending, model.LinkActive)
	if err != nil || got.Status != model.LinkActive || !got.StartedAt.Valid {
		t.Fatalf("accept = %+v, %v", got, err)
	}
	if _, err := f.store.TransitionLink(f.ctx, l.Uuid, model.LinkPending, model.LinkActive); !errors.Is(err, ErrLinkStale) {
		t.Fatalf("stale accept: err = %v, want ErrLinkStale", err)
	}
}

// A tax number opens one fleet per brand.
func TestDuplicateTaxNumber(t *testing.T) {
	f := newFixture(t)
	fl := f.fleet(t, "Filo VKN", "6768656307")
	other := f.org(t, "Filo VKN 2", "fleet", 0)
	err := f.savepoint(t, func(s *Store) error {
		_, err := s.CreateProfile(f.ctx, db.CreateFleetProfileParams{
			OrganizationID: other.ID, BrandID: other.BrandID, TaxNumber: "6768656307", LegalName: "x",
			ReportFrequency: model.ReportMonthly, ReportLocale: "tr",
		})
		return err
	})
	if !errors.Is(err, ErrFleetExists) {
		t.Fatalf("duplicate tax number: err = %v, want ErrFleetExists", err)
	}
	got, err := f.store.FindByTaxNumber(f.ctx, f.brandID, "6768656307")
	if err != nil || got.Organization.ID != fl.ID || got.FleetProfile.ReportFrequency != model.ReportMonthly {
		t.Fatalf("find by tax number = %+v, %v", got, err)
	}
	// Exact match only.
	if _, err := f.store.FindByTaxNumber(f.ctx, f.brandID, "676865630"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("prefix lookup: err = %v, want no rows", err)
	}
}

// List contract of the dealer fleet list: scope by dealer ids, q on name
// and tax number, multi-value status, vehicle count range, sort keys with
// the default name.
func TestListDealerFleets(t *testing.T) {
	f := newFixture(t)
	owner := f.user(t)
	a := f.fleet(t, "Alfa Lojistik "+f.suffix, "1234567890")
	b := f.fleet(t, "Beta Kargo "+f.suffix, "10000000146")
	c := f.fleet(t, "Gama Taksi "+f.suffix, "1089325650")
	f.link(t, a, f.dealer, model.LinkActive)
	f.link(t, b, f.dealer, model.LinkPending)
	f.link(t, c, f.dealer2, model.LinkActive)
	f.vehicle(t, b, owner, "34ABC"+f.suffix[len(f.suffix)-3:])
	f.vehicle(t, b, owner, "34ABD"+f.suffix[len(f.suffix)-3:])
	f.vehicle(t, a, owner, "34ABE"+f.suffix[len(f.suffix)-3:])

	names := func(rows []db.ListDealerFleetsRow) []string {
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.Name)
		}
		return out
	}
	list := func(flt FleetFilter) ([]db.ListDealerFleetsRow, int64) {
		t.Helper()
		flt.BrandID, flt.Limit = f.brandID, 50
		rows, total, err := f.store.ListDealerFleets(f.ctx, flt)
		if err != nil {
			t.Fatalf("list %+v: %v", flt, err)
		}
		if int64(len(rows)) != total {
			t.Fatalf("total %d != rows %d", total, len(rows))
		}
		return rows, total
	}

	rows, _ := list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID}})
	if got := names(rows); len(got) != 2 || got[0] != a.Name || got[1] != b.Name {
		t.Fatalf("default sort (name) = %v", got)
	}
	if rows[1].VehicleCount != 2 || rows[0].VehicleCount != 1 || rows[1].LinkStatus != model.LinkPending {
		t.Fatalf("rows = %+v", rows)
	}
	rows, _ = list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID}, Sort: []apiquery.SortField{{Field: "vehicle_count", Desc: true}}})
	if got := names(rows); got[0] != b.Name {
		t.Fatalf("-vehicle_count = %v", got)
	}
	rows, _ = list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID, f.dealer2.ID}, Statuses: []string{model.LinkActive}})
	if got := names(rows); len(got) != 2 || got[0] != a.Name || got[1] != c.Name {
		t.Fatalf("status=active = %v", got)
	}
	rows, _ = list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID, f.dealer2.ID}, Q: "1000000"})
	if got := names(rows); len(got) != 1 || got[0] != b.Name {
		t.Fatalf("q by tax number = %v", got)
	}
	rows, _ = list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID, f.dealer2.ID}, Q: "gama taksi"})
	if got := names(rows); len(got) != 1 || got[0] != c.Name {
		t.Fatalf("q by name = %v", got)
	}
	one, two := int64(1), int64(2)
	rows, _ = list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID, f.dealer2.ID}, VehicleCountMin: &one, VehicleCountMax: &one})
	if got := names(rows); len(got) != 1 || got[0] != a.Name {
		t.Fatalf("vehicle_count 1..1 = %v", got)
	}
	rows, _ = list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID, f.dealer2.ID}, VehicleCountMin: &two})
	if got := names(rows); len(got) != 1 || got[0] != b.Name {
		t.Fatalf("vehicle_count_min=2 = %v", got)
	}
	for _, key := range []string{"last_service_at", "created_at", "-name"} {
		sf := apiquery.SortField{Field: key}
		if key[0] == '-' {
			sf = apiquery.SortField{Field: key[1:], Desc: true}
		}
		if rows, _ = list(FleetFilter{DealerOrgIDs: []int64{f.dealer.ID}, Sort: []apiquery.SortField{sf}}); len(rows) != 2 {
			t.Fatalf("sort %s = %d rows", key, len(rows))
		}
	}
	var ve *apiquery.ValidationError
	if _, _, err := f.store.ListDealerFleets(f.ctx, FleetFilter{BrandID: f.brandID, Sort: []apiquery.SortField{{Field: "tax_number"}}}); !errors.As(err, &ve) {
		t.Fatalf("unknown sort: err = %v", err)
	}
	if rows, total, err := f.store.ListDealerFleets(f.ctx, FleetFilter{BrandID: f.brandID, DealerOrgIDs: []int64{}}); err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("empty scope = %d, %d, %v", len(rows), total, err)
	}
}

// List contract of the fleet vehicles: plate default sort, q on plate,
// last service and active warranties per vehicle.
func TestListFleetVehicles(t *testing.T) {
	f := newFixture(t)
	owner := f.user(t)
	fl := f.fleet(t, "Filo Araç", "1089325650")
	tail := f.suffix[len(f.suffix)-3:]
	f.vehicle(t, fl, owner, "34ZZ"+tail)
	f.vehicle(t, fl, owner, "06AA"+tail)
	other := f.fleet(t, "Filo Başka", "9249799759")
	f.vehicle(t, other, owner, "35BB"+tail)

	rows, total, err := f.store.ListFleetVehicles(f.ctx, VehicleFilter{FleetOrgID: fl.ID, Limit: 50})
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("vehicles = %d/%d, %v", len(rows), total, err)
	}
	if rows[0].Plate.String != "06AA"+tail || rows[1].Plate.String != "34ZZ"+tail {
		t.Fatalf("default sort (plate) = %s, %s", rows[0].Plate.String, rows[1].Plate.String)
	}
	if rows[0].LastServiceAt.Valid || rows[0].ActiveWarrantyCount != 0 {
		t.Fatalf("vehicle without services = %+v", rows[0])
	}
	rows, total, err = f.store.ListFleetVehicles(f.ctx, VehicleFilter{FleetOrgID: fl.ID, Q: "34 zz", Limit: 50})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Plate.String != "34ZZ"+tail {
		t.Fatalf("q plate = %+v (%d), %v", rows, total, err)
	}
	for _, key := range []string{"car_brand", "last_service_at", "active_warranty_count", "created_at"} {
		if _, _, err := f.store.ListFleetVehicles(f.ctx, VehicleFilter{
			FleetOrgID: fl.ID, Limit: 50, Sort: []apiquery.SortField{{Field: key, Desc: true}},
			ServiceOrgIDs: []int64{f.dealer.ID},
		}); err != nil {
			t.Fatalf("sort %s: %v", key, err)
		}
	}
}
