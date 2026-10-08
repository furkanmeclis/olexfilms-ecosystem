package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakePlates accepts every plate (the geo formats are tested in geo).
type fakePlates struct{}

func (fakePlates) ValidatePlate(_ context.Context, iso2, plate string) (geo.PlateCheck, error) {
	return geo.PlateCheck{Country: iso2, Normalized: strings.ReplaceAll(plate, " ", ""), Valid: true}, nil
}

// apiFixture is a brand with two dealers under one distributor, inside a
// rolled back transaction.
type apiFixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	svc      *Service
	suffix   string
	brand    db.Brand
	dist     db.Organization
	d1, d2   db.Organization
	carBrand db.CarBrand
	carModel db.CarModel
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
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
	f := &apiFixture{ctx: ctx, tx: tx, q: db.New(tx), suffix: fmt.Sprintf("%d", time.Now().UnixNano())}
	if f.brand, err = f.q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatal(err)
	}
	center, err := f.q.GetBrandCenter(ctx, f.brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.dist = f.org(t, "dist", "distributor", center.ID)
	f.d1 = f.org(t, "d1", "dealer", f.dist.ID)
	f.d2 = f.org(t, "d2", "dealer", f.dist.ID)
	if f.carBrand, err = f.q.CreateCarBrand(ctx, db.CreateCarBrandParams{Name: "T473 " + f.suffix, ShowName: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if f.carModel, err = f.q.CreateCarModel(ctx, db.CreateCarModelParams{CarBrandID: f.carBrand.ID, Name: "M " + f.suffix, Active: true}); err != nil {
		t.Fatal(err)
	}
	f.svc = New(tx)
	f.svc.now = func() time.Time { return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC) }
	f.svc.SetPlates(fakePlates{})
	return f
}

func (f *apiFixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: "t473-" + name + "-" + f.suffix, Name: "T473 " + name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true}, BrandID: f.brand.ID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

// dealer is a dealer caller with the managed scope (dealer_owner).
func (f *apiFixture) dealer(o db.Organization) Caller {
	return Caller{Principal: authctx.Principal{PermissionScopes: map[string]rbac.Scope{
		rbac.PermFleetsPlan: rbac.ScopeManaged, rbac.PermAppointmentsWrite: rbac.ScopeManaged,
		rbac.PermServicesRead: rbac.ScopeManaged, rbac.PermServicesWrite: rbac.ScopeManaged,
	}}, Org: orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, OrgType: o.Type, BrandID: o.BrandID},
		OrgID: o.ID, BrandID: o.BrandID, OrgType: o.Type, Filter: scopefilter.Filter{
			Scope: rbac.ScopeManaged, OrgID: o.ID, OrgIDs: []int64{o.ID},
		}}
}

// plate is a plate unique to the run.
func (f *apiFixture) plate(n int) string {
	return fmt.Sprintf("34 T%s %d", f.suffix[len(f.suffix)-4:], n)
}

func (f *apiFixture) settings(t *testing.T, org db.Organization, capacity int32) {
	t.Helper()
	_, err := f.q.UpsertAppointmentSettings(f.ctx, db.UpsertAppointmentSettingsParams{
		OrganizationID: org.ID, BrandID: org.BrandID, DailyVehicleCapacity: capacity,
		DefaultEstimatedMinutes: 60, SlotIntervalMinutes: 60,
		WorkingHours: []byte(`{
			"monday":[{"start":"09:00","end":"17:00"}],
			"tuesday":[{"start":"09:00","end":"17:00"}],
			"wednesday":[{"start":"09:00","end":"17:00"}],
			"thursday":[{"start":"09:00","end":"17:00"}],
			"friday":[{"start":"09:00","end":"17:00"}]
		}`),
	})
	if err != nil {
		t.Fatalf("appointment settings: %v", err)
	}
}

func (f *apiFixture) fleetWithVehicles(t *testing.T, n int) (FleetView, []VehicleView) {
	t.Helper()
	opened, err := f.svc.Open(f.ctx, f.dealer(f.d1), OpenInput{LegalName: "T475 Filo A.Ş.", TaxNumber: validVKN(t, f.suffix)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.InviteUser(f.ctx, f.dealer(f.d1), opened.UUID, InviteInput{
		Email: "t475-" + f.suffix + "@example.test", Name: "Filo", Surname: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	vehicles := make([]VehicleView, 0, n)
	for i := 0; i < n; i++ {
		v, _, err := f.svc.AddVehicle(f.ctx, f.dealer(f.d1), opened.UUID, AddVehicleInput{
			Plate: f.plate(100 + i), CarBrandUUID: &f.carBrand.Uuid, CarModelUUID: &f.carModel.Uuid,
		})
		if err != nil {
			t.Fatalf("vehicle %d: %v", i, err)
		}
		vehicles = append(vehicles, v)
	}
	return opened, vehicles
}

// completedService inserts a completed service of org on the vehicle.
func (f *apiFixture) completedService(t *testing.T, org db.Organization, vehicleUUID uuid.UUID) db.Service {
	t.Helper()
	v, err := f.q.GetVehicleByUUID(f.ctx, vehicleUUID)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO services (service_no, organization_id, brand_id, customer_user_id,
		vehicle_id, car_brand_id, car_model_id, plate, plate_country, status, completed_at)
		VALUES ('T473' || right(gen_random_uuid()::text, 8), $1, $2, $3, $4, $5, $6, $7, 'TR', 'completed', NOW())
		RETURNING id`, org.ID, org.BrandID, v.UserID, v.ID, f.carBrand.ID, f.carModel.ID, v.Plate.String).Scan(&id); err != nil {
		t.Fatalf("service: %v", err)
	}
	s, err := f.q.GetService(f.ctx, db.GetServiceParams{ID: id, BrandID: org.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// income books the service income of org on the fleet cari, like the
// services income bridge does for a fleet vehicle.
func (f *apiFixture) income(t *testing.T, org db.Organization, fleetOrgID int64, s db.Service, amount string) {
	t.Helper()
	cari, err := ensureCariForTest(f.ctx, f.q, org, fleetOrgID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := posting.New(f.q, nil, nil).PostIncome(f.ctx, f.tx, posting.Entry{
		OrganizationID: org.ID, Source: posting.Source{Type: model.SourceServiceIncome, UUID: s.Uuid},
		Category: accounting.CategoryServiceIncome, Amount: amount, Currency: org.Currency, CariID: cari,
	}); err != nil {
		t.Fatalf("income: %v", err)
	}
}

// Acceptance (TEC-473): the same VKN from a second dealer opens no second
// fleet; the second dealer's link is pending (404 on the card) until the
// fleet user accepts it in the portal; afterwards the second dealer sees
// its own services and cari only, never the first dealer's.
func TestSecondDealerLinkPendingThenOwnDataOnly(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	tax := validVKN(t, f.suffix)
	opened, err := f.svc.Open(ctx, f.dealer(f.d1), OpenInput{LegalName: "T473 Filo A.Ş.", TaxNumber: tax})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Link == nil || opened.Link.Status != model.LinkActive {
		t.Fatalf("opener link = %+v", opened.Link)
	}

	// Second dealer, same VKN: no new fleet, the existing one is named.
	_, err = f.svc.Open(ctx, f.dealer(f.d2), OpenInput{LegalName: "Kopya", TaxNumber: tax})
	var fe *FleetExistsError
	if !errors.As(err, &fe) || fe.FleetUUID != opened.UUID || fe.LinkStatus != "" {
		t.Fatalf("second open: err = %v (%+v)", err, fe)
	}
	var fleets int
	if err := f.tx.QueryRow(ctx, `SELECT COUNT(*) FROM fleet_profiles WHERE brand_id = $1 AND tax_number = $2`,
		f.brand.ID, tax).Scan(&fleets); err != nil || fleets != 1 {
		t.Fatalf("fleets with the VKN = %d, %v", fleets, err)
	}
	req, err := f.svc.RequestLink(ctx, f.dealer(f.d2), opened.UUID)
	if err != nil || req.Link == nil || req.Link.Status != model.LinkPending {
		t.Fatalf("link request = %+v, %v", req.Link, err)
	}
	if _, err := f.svc.RequestLink(ctx, f.dealer(f.d2), opened.UUID); !errors.Is(err, ErrLinkExists) {
		t.Fatalf("second request: err = %v", err)
	}
	if _, err := f.svc.Card(ctx, f.dealer(f.d2), opened.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pending card: err = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.Open(ctx, f.dealer(f.d2), OpenInput{LegalName: "Kopya", TaxNumber: tax}); !errors.As(err, &fe) || fe.LinkStatus != model.LinkPending {
		t.Fatalf("open after request: %v", err)
	}

	// The first user is the primary user (the vehicle owner).
	user, err := f.svc.InviteUser(ctx, f.dealer(f.d1), opened.UUID, InviteInput{
		Email: "t473-" + f.suffix + "@example.test", Name: "Filo", Surname: "Yönetici",
	})
	if err != nil || !user.IsPrimary {
		t.Fatalf("invite = %+v, %v", user, err)
	}
	u, err := f.q.GetUserByUUID(ctx, user.UserUUID)
	if err != nil {
		t.Fatal(err)
	}
	veh, created, err := f.svc.AddVehicle(ctx, f.dealer(f.d1), opened.UUID, AddVehicleInput{
		Plate: f.plate(1), CarBrandUUID: &f.carBrand.Uuid, CarModelUUID: &f.carModel.Uuid,
	})
	if err != nil || !created {
		t.Fatalf("add vehicle = %v, %v", created, err)
	}

	// The fleet user accepts in the portal; a second decision is 409.
	if _, err := f.svc.DecideLink(ctx, u.ID, req.Link.UUID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.DecideLink(ctx, u.ID, req.Link.UUID, false); !errors.Is(err, ErrLinkNotPending) {
		t.Fatalf("second decision: err = %v", err)
	}

	fleet, err := f.q.GetFleetByUUID(ctx, opened.UUID)
	if err != nil {
		t.Fatal(err)
	}
	s1 := f.completedService(t, f.d1, veh.UUID)
	s2 := f.completedService(t, f.d2, veh.UUID)
	f.income(t, f.d1, fleet.Organization.ID, s1, "1000.00")
	f.income(t, f.d2, fleet.Organization.ID, s2, "250.00")

	card2, err := f.svc.Card(ctx, f.dealer(f.d2), opened.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if card2.VehicleCount != 1 || card2.ServiceCount != 1 || len(card2.RecentServices) != 1 || card2.RecentServices[0].UUID != s2.Uuid {
		t.Fatalf("second dealer card services = %d %+v", card2.ServiceCount, card2.RecentServices)
	}
	if card2.Cari == nil || card2.Cari.Balance != "250.00" {
		t.Fatalf("second dealer cari = %+v", card2.Cari)
	}
	if len(card2.Links) != 1 || card2.Links[0].DealerUUID != f.d2.Uuid {
		t.Fatalf("second dealer sees links %+v", card2.Links)
	}
	card1, err := f.svc.Card(ctx, f.dealer(f.d1), opened.UUID)
	if err != nil || card1.Cari == nil || card1.Cari.Balance != "1000.00" || card1.ServiceCount != 1 {
		t.Fatalf("first dealer card = %+v, %v", card1, err)
	}
	vehicles, _, err := f.svc.ListVehicles(ctx, f.dealer(f.d2), opened.UUID, VehicleFilter{Limit: 20})
	if err != nil || len(vehicles) != 1 || vehicles[0].LastServiceAt == nil {
		t.Fatalf("second dealer vehicles = %+v, %v", vehicles, err)
	}

	today := time.Now().In(time.UTC)
	p, err := ParsePeriod(today.AddDate(0, 0, -1).Format(time.DateOnly), today.AddDate(0, 0, 1).Format(time.DateOnly))
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.Statement(ctx, f.dealer(f.d2), opened.UUID, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Lines) != 1 || st.Lines[0].Kind != LineServiceIncome || st.Lines[0].Service == nil ||
		st.Lines[0].Service.UUID != s2.Uuid || st.ServiceIncomeTotal != "250.00" || st.ClosingBalance != "250.00" {
		t.Fatalf("second dealer statement = %+v", st)
	}
	if st.Dealer.UUID != f.d2.Uuid || st.ServiceCount != 1 {
		t.Fatalf("statement parties = %+v", st)
	}

	// The distributor (subtree) reaches the fleet through its dealers' links
	// but has no own link: no statement.
	distCaller := Caller{OrgID: f.dist.ID, BrandID: f.brand.ID, OrgType: "distributor", Filter: scopefilter.Filter{
		Scope: rbac.ScopeSubtree, OrgID: f.dist.ID, OrgIDs: []int64{f.dist.ID, f.d1.ID, f.d2.ID},
	}}
	if card, err := f.svc.Card(ctx, distCaller, opened.UUID); err != nil || card.ServiceCount != 2 || card.Cari != nil {
		t.Fatalf("distributor card = %+v, %v", card, err)
	}
	if _, err := f.svc.Statement(ctx, distCaller, opened.UUID, p); !errors.Is(err, ErrNoDealerLink) {
		t.Fatalf("distributor statement: err = %v", err)
	}

	// An unrelated dealer reaches nothing.
	d3 := f.org(t, "d3", "dealer", f.dist.ID)
	if _, err := f.svc.Card(ctx, f.dealer(d3), opened.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked dealer card: err = %v", err)
	}
	if items, total, err := f.svc.List(ctx, f.dealer(d3), ListFilter{Limit: 20}); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("unlinked dealer list = %d, %v", total, err)
	}
}

func TestFleetServicePlanPreviewCapacityClosureAndWarnings(t *testing.T) {
	f := newAPIFixture(t)
	f.settings(t, f.d1, 3)
	opened, vehicles := f.fleetWithVehicles(t, 7)
	if _, err := f.q.CreateAppointmentClosure(f.ctx, db.CreateAppointmentClosureParams{
		OrganizationID: f.d1.ID, BrandID: f.d1.BrandID,
		ClosedOn: pgtype.Date{Time: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   "closed",
	}); err != nil {
		t.Fatal(err)
	}
	input := ServicePlanInput{
		VehicleUUIDs: uuidsOf(vehicles), ServiceType: "PPF kontrol",
		StartDate: "2026-10-05", PreferredTimes: []string{"10:00", "11:00", "12:00"},
	}
	got, err := f.svc.PreviewServicePlan(f.ctx, f.dealer(f.d1), opened.UUID, input)
	if err != nil {
		t.Fatal(err)
	}
	byDay := map[string]int{}
	for _, a := range got.Appointments {
		byDay[a.StartsAt.In(loadLocation(f.d1.Timezone)).Format(time.DateOnly)]++
	}
	if byDay["2026-10-05"] != 3 || byDay["2026-10-06"] != 0 || byDay["2026-10-07"] != 3 || byDay["2026-10-08"] != 1 {
		t.Fatalf("distribution = %+v, want 3 + closure + 3 + 1", byDay)
	}

	// Same vehicle already has another active appointment on the first day:
	// the preview still proposes the plan row but flags the conflict.
	firstVehicle, err := f.q.GetVehicleByUUID(f.ctx, vehicles[0].UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.CreateAppointment(f.ctx, db.CreateAppointmentParams{
		OrganizationID: f.d1.ID, BrandID: f.d1.BrandID, CustomerUserID: firstVehicle.UserID,
		VehicleID:        pgtype.Int8{Int64: firstVehicle.ID, Valid: true},
		StartsAt:         tsArg(time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC)),
		EndsAt:           tsArg(time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)),
		EstimatedMinutes: 60, Source: "panel", Status: appointmentsuc.StatusScheduled,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = f.svc.PreviewServicePlan(f.ctx, f.dealer(f.d1), opened.UUID, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].VehicleUUID != vehicles[0].UUID {
		t.Fatalf("warnings = %+v", got.Warnings)
	}
}

func TestFleetServicePlanCreateRollbackOnStaleCapacityAndIdempotency(t *testing.T) {
	f := newAPIFixture(t)
	f.settings(t, f.d1, 3)
	opened, vehicles := f.fleetWithVehicles(t, 3)
	input := ServicePlanInput{VehicleUUIDs: uuidsOf(vehicles), ServiceType: "PPF", StartDate: "2026-10-05"}
	preview, err := f.svc.PreviewServicePlan(f.ctx, f.dealer(f.d1), opened.UUID, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vehicles {
		row, err := f.q.GetVehicleByUUID(f.ctx, v.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.q.CreateAppointment(f.ctx, db.CreateAppointmentParams{
			OrganizationID: f.d1.ID, BrandID: f.d1.BrandID, CustomerUserID: row.UserID,
			VehicleID:        pgtype.Int8{Int64: row.ID, Valid: true},
			StartsAt:         tsArg(time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC)),
			EndsAt:           tsArg(time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)),
			EstimatedMinutes: 60, Source: "panel", Status: appointmentsuc.StatusScheduled,
		}); err != nil {
			t.Fatal(err)
		}
	}
	input.Appointments = preview.Appointments
	if _, err := f.svc.CreateServicePlan(f.ctx, f.dealer(f.d1), opened.UUID, input, "stale-"+f.suffix); !errors.Is(err, ErrServicePlanStale) {
		t.Fatalf("create with stale preview err = %v", err)
	}
	if got := countPlans(t, f.tx, f.d1.ID); got != 0 {
		t.Fatalf("plans after stale create = %d", got)
	}

	f2 := newAPIFixture(t)
	f2.settings(t, f2.d1, 3)
	opened2, vehicles2 := f2.fleetWithVehicles(t, 2)
	okInput := ServicePlanInput{VehicleUUIDs: uuidsOf(vehicles2), ServiceType: "PPF", StartDate: "2026-10-05"}
	one, err := f2.svc.CreateServicePlan(f2.ctx, f2.dealer(f2.d1), opened2.UUID, okInput, "idem-"+f2.suffix)
	if err != nil {
		t.Fatal(err)
	}
	two, err := f2.svc.CreateServicePlan(f2.ctx, f2.dealer(f2.d1), opened2.UUID, okInput, "idem-"+f2.suffix)
	if err != nil {
		t.Fatal(err)
	}
	if one.UUID != two.UUID || countPlans(t, f2.tx, f2.d1.ID) != 1 {
		t.Fatalf("idempotent plans = %s %s count=%d", one.UUID, two.UUID, countPlans(t, f2.tx, f2.d1.ID))
	}
}

func TestFleetServicePlanStartIntakeIsRowWise(t *testing.T) {
	f := newAPIFixture(t)
	f.settings(t, f.d1, 3)
	opened, vehicles := f.fleetWithVehicles(t, 2)
	plan, err := f.svc.CreateServicePlan(f.ctx, f.dealer(f.d1), opened.UUID,
		ServicePlanInput{VehicleUUIDs: uuidsOf(vehicles), ServiceType: "PPF", StartDate: "2026-10-05"}, "")
	if err != nil {
		t.Fatal(err)
	}
	fail := plan.Appointments[0].UUID
	f.svc.SetAppointments(fakeAppointmentStarter{fail: fail})
	got, err := f.svc.StartServicePlanIntake(f.ctx, f.dealer(f.d1), opened.UUID, plan.UUID, IntakeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 {
		t.Fatalf("results = %+v", got.Results)
	}
	var ok, bad int
	for _, r := range got.Results {
		if r.OK {
			ok++
			if r.ServiceUUID == nil {
				t.Fatalf("success row missing service uuid: %+v", r)
			}
		} else {
			bad++
			if r.AppointmentUUID != fail || r.Code == "" {
				t.Fatalf("error row = %+v", r)
			}
		}
	}
	if ok != 1 || bad != 1 {
		t.Fatalf("row results ok=%d bad=%d: %+v", ok, bad, got.Results)
	}
}

type fakeAppointmentStarter struct{ fail uuid.UUID }

func (f fakeAppointmentStarter) StartIntake(_ context.Context, _ appointmentsuc.Caller, id uuid.UUID) (appointmentsuc.Appointment, error) {
	if id == f.fail {
		return appointmentsuc.Appointment{}, appointmentsuc.ErrInvalidTransition
	}
	svc := uuid.New()
	return appointmentsuc.Appointment{UUID: id, ServiceUUID: &svc}, nil
}

func uuidsOf(vs []VehicleView) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.UUID)
	}
	return out
}

func countPlans(t *testing.T, tx pgx.Tx, orgID int64) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(context.Background(), `SELECT COUNT(*) FROM fleet_service_plans WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Acceptance (TEC-473): another customer's vehicle never joins a fleet
// (409 FLEET_VEHICLE_OTHER_OWNER), by plate or by vehicle_uuid; a vehicle of
// the fleet's own user is offered for linking (FLEET_VEHICLE_EXISTS).
func TestAddVehicleOtherCustomer409(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	opened, err := f.svc.Open(ctx, f.dealer(f.d1), OpenInput{LegalName: "T473 Filo B", TaxNumber: validVKN(t, f.suffix)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.AddVehicle(ctx, f.dealer(f.d1), opened.UUID, AddVehicleInput{Plate: f.plate(5)}); !errors.Is(err, ErrPrimaryUserRequired) {
		t.Fatalf("no primary user: err = %v", err)
	}
	user, err := f.svc.InviteUser(ctx, f.dealer(f.d1), opened.UUID, InviteInput{Email: "t473b-" + f.suffix + "@example.test", Name: "Filo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.InviteUser(ctx, f.dealer(f.d1), opened.UUID, InviteInput{Email: "t473b-" + f.suffix + "@example.test", Name: "X"}); !errors.Is(err, ErrUserEmailTaken) {
		t.Fatalf("same e-mail: err = %v", err)
	}
	owner, err := f.q.GetUserByUUID(ctx, user.UserUUID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: "t473-other-" + f.suffix + "@example.test", Valid: true}, PasswordHash: "x",
		Name: "Başka", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	newVehicle := func(userID int64, plate string) db.Vehicle {
		v, err := f.q.CreateVehicle(ctx, db.CreateVehicleParams{
			UserID: userID, BrandID: f.brand.ID, Plate: optText(plate),
			PlateNormalized: optText(strings.ReplaceAll(plate, " ", "")), PlateCountry: optText("TR"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	foreign := newVehicle(other.ID, f.plate(6))
	_, _, err = f.svc.AddVehicle(ctx, f.dealer(f.d1), opened.UUID, AddVehicleInput{Plate: f.plate(6)})
	var re *RuleError
	if !errors.Is(err, ErrVehicleOtherOwner) || !errors.As(err, &re) || re.Status() != http.StatusConflict {
		t.Fatalf("other customer's plate: err = %v", err)
	}
	if _, _, err := f.svc.AddVehicle(ctx, f.dealer(f.d1), opened.UUID, AddVehicleInput{VehicleUUID: &foreign.Uuid}); !errors.Is(err, ErrVehicleOtherOwner) {
		t.Fatalf("other customer's vehicle_uuid: err = %v", err)
	}
	if v, err := f.q.GetVehicleByUUID(ctx, foreign.Uuid); err != nil || v.FleetOrgID.Valid {
		t.Fatalf("foreign vehicle joined a fleet: %+v %v", v.FleetOrgID, err)
	}

	// The owner's own vehicle: a link suggestion, then the link.
	own := newVehicle(owner.ID, f.plate(7))
	_, _, err = f.svc.AddVehicle(ctx, f.dealer(f.d1), opened.UUID, AddVehicleInput{Plate: f.plate(7)})
	var vx *VehicleExistsError
	if !errors.As(err, &vx) || vx.VehicleUUID != own.Uuid || vx.InFleet {
		t.Fatalf("own plate: err = %v", err)
	}
	linked, created, err := f.svc.AddVehicle(ctx, f.dealer(f.d1), opened.UUID, AddVehicleInput{VehicleUUID: &own.Uuid})
	if err != nil || created || linked.UUID != own.Uuid {
		t.Fatalf("link own vehicle = %v, %v", created, err)
	}
	// Removing keeps the vehicle.
	if err := f.svc.RemoveVehicle(ctx, f.dealer(f.d1), opened.UUID, own.Uuid); err != nil {
		t.Fatal(err)
	}
	if v, err := f.q.GetVehicleByUUID(ctx, own.Uuid); err != nil || v.FleetOrgID.Valid {
		t.Fatalf("removed vehicle = %+v, %v", v.FleetOrgID, err)
	}
	// The primary user is not disabled.
	if err := f.svc.DisableUser(ctx, f.dealer(f.d1), opened.UUID, user.UUID); !errors.Is(err, ErrPrimaryUserLocked) {
		t.Fatalf("disable primary: err = %v", err)
	}
}

func ensureCariForTest(ctx context.Context, q *db.Queries, org db.Organization, fleetOrgID int64) (int64, error) {
	c, err := q.GetCariAccountByCounterpartyOrg(ctx, db.GetCariAccountByCounterpartyOrgParams{
		OrganizationID: org.ID, CounterpartyOrgID: pgtype.Int8{Int64: fleetOrgID, Valid: true},
	})
	return c.ID, err
}
