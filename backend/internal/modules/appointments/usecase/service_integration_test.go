package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	servicesuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	tx     pgx.Tx
	q      *db.Queries
	svc    *Service
	brand  int64
	center db.Organization
	dist   db.Organization
	dealer db.Organization
	other  db.Organization
	user   db.User
	out    *outbox.Memory
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	f := &fixture{t: t, ctx: ctx, pool: pool, tx: tx, q: q}
	if err := tx.QueryRow(ctx, `SELECT id FROM brands WHERE slug = 'olex'`).Scan(&f.brand); err != nil {
		t.Fatalf("brand: %v", err)
	}
	f.center = f.centerOrg()
	f.dist = f.org("dist", "distributor", f.center.ID, "Europe/Istanbul")
	f.dealer = f.org("dealer", "dealer", f.dist.ID, "Europe/Berlin")
	f.other = f.org("other", "dealer", f.center.ID, "Europe/Istanbul")
	f.user = f.userRow("appointment-user")
	out := outbox.NewMemory()
	f.out = out
	services := servicesuc.New(tx, q, out)
	f.svc = New(tx, q, out, services)
	f.svc.SetClock(func() time.Time { return time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC) })
	return f
}

func (f *fixture) org(prefix, typ string, parent int64, tz string) db.Organization {
	f.t.Helper()
	slug := fmt.Sprintf("tec323-%s-%d", prefix, time.Now().UnixNano())
	params := db.CreateOrganizationParams{
		Slug: slug, Name: slug, Status: "active", Type: typ, BrandID: f.brand,
		Currency: "TRY", Locale: "tr", Timezone: tz, Settings: []byte(`{}`),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour).UTC(), Valid: true},
	}
	if parent != 0 {
		params.ParentID = pgtype.Int8{Int64: parent, Valid: true}
	}
	o, err := f.q.CreateOrganization(f.ctx, params)
	if err != nil {
		f.t.Fatalf("org: %v", err)
	}
	return o
}

func (f *fixture) centerOrg() db.Organization {
	f.t.Helper()
	o, err := f.q.GetOrganizationBySlug(f.ctx, "olex-center")
	if err == nil {
		return o
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		f.t.Fatalf("center lookup: %v", err)
	}
	var row db.Organization
	err = f.tx.QueryRow(f.ctx, `SELECT id, uuid, slug, name, city, district, phone, address, logo_object_key, status, plan_code,
		access_starts_at, access_ends_at, created_at, updated_at, deleted_at, email, website, tagline, footer_text,
		paper_size, primary_color, type, parent_id, brand_id, currency, locale, timezone, country_id, contract_pdf_key,
		contract_valid_until, settings, province_id, district_id, phone_raw, google_business_url, latitude, longitude
		FROM organizations WHERE brand_id = $1 AND type = 'center' ORDER BY id LIMIT 1`, f.brand).Scan(
		&row.ID, &row.Uuid, &row.Slug, &row.Name, &row.City, &row.District, &row.Phone,
		&row.Address, &row.LogoObjectKey, &row.Status, &row.PlanCode, &row.AccessStartsAt,
		&row.AccessEndsAt, &row.CreatedAt, &row.UpdatedAt, &row.DeletedAt, &row.Email,
		&row.Website, &row.Tagline, &row.FooterText, &row.PaperSize, &row.PrimaryColor,
		&row.Type, &row.ParentID, &row.BrandID, &row.Currency, &row.Locale, &row.Timezone,
		&row.CountryID, &row.ContractPdfKey, &row.ContractValidUntil, &row.Settings, &row.ProvinceID,
		&row.DistrictID, &row.PhoneRaw, &row.GoogleBusinessUrl, &row.Latitude, &row.Longitude)
	if err != nil {
		f.t.Fatalf("center: %v", err)
	}
	return row
}

func (f *fixture) userRow(prefix string) db.User {
	f.t.Helper()
	email := fmt.Sprintf("%s-%d@example.test", prefix, time.Now().UnixNano())
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		Email: pgtype.Text{String: email, Valid: true}, PasswordHash: "x", Name: prefix, Surname: "User", Status: "active",
	})
	if err != nil {
		f.t.Fatalf("user: %v", err)
	}
	return u
}

func (f *fixture) vehicle(user db.User, org db.Organization) db.Vehicle {
	f.t.Helper()
	var brandID, modelID int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`,
		fmt.Sprintf("Brand %d", time.Now().UnixNano())).Scan(&brandID); err != nil {
		f.t.Fatalf("car brand: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`,
		brandID, fmt.Sprintf("Model %d", time.Now().UnixNano())).Scan(&modelID); err != nil {
		f.t.Fatalf("car model: %v", err)
	}
	v, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: user.ID, OrganizationID: pgtype.Int8{Int64: org.ID, Valid: true}, BrandID: org.BrandID,
		CarBrandID: pgtype.Int8{Int64: brandID, Valid: true}, CarModelID: pgtype.Int8{Int64: modelID, Valid: true},
		Plate: pgtype.Text{String: "34 TEC 323", Valid: true}, PlateNormalized: pgtype.Text{String: "34TEC323", Valid: true},
		PlateCountry: pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		f.t.Fatalf("vehicle: %v", err)
	}
	return v
}

func (f *fixture) link(user db.User, org db.Organization) {
	f.t.Helper()
	if _, err := f.q.LinkCustomerOrganization(f.ctx, db.LinkCustomerOrganizationParams{
		UserID: user.ID, OrganizationID: org.ID, BrandID: org.BrandID,
	}); err != nil {
		f.t.Fatalf("link customer: %v", err)
	}
}

func (f *fixture) settings(org db.Organization, capacity int32) {
	f.t.Helper()
	f.portalSettings(org, capacity, false)
}

func (f *fixture) portalSettings(org db.Organization, capacity int32, enabled bool) {
	f.t.Helper()
	_, err := f.q.UpsertAppointmentSettings(f.ctx, db.UpsertAppointmentSettingsParams{
		OrganizationID: org.ID, BrandID: org.BrandID, DailyVehicleCapacity: capacity,
		DefaultEstimatedMinutes: 60, SlotIntervalMinutes: 60,
		WorkingHours:              []byte(`{"monday":[{"start":"09:00","end":"17:00"}]}`),
		PortalAppointmentsEnabled: enabled,
	})
	if err != nil {
		f.t.Fatalf("settings: %v", err)
	}
}

func (f *fixture) portalCaller(user db.User) PortalCaller {
	return PortalCaller{UserID: user.ID, BrandID: f.brand}
}

func (f *fixture) caller(org db.Organization, scope rbac.Scope, orgIDs []int64) Caller {
	return Caller{
		Principal: authctx.Principal{UserInternal: f.user.ID, PermissionScopes: map[string]rbac.Scope{
			rbac.PermAppointmentsRead: scope, rbac.PermAppointmentsWrite: rbac.ScopeManaged,
			rbac.PermAppointmentSettingsManage: rbac.ScopeManaged,
			rbac.PermServicesRead:              rbac.ScopeManaged, rbac.PermServicesWrite: rbac.ScopeManaged,
		}},
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: org.BrandID},
		Filter: scopefilter.Filter{Permission: rbac.PermAppointmentsRead, Scope: scope, UserID: f.user.ID, OrgID: org.ID, OrgIDs: orgIDs, BrandID: org.BrandID},
	}
}

func TestCreateRejectsFourthAppointmentWhenCapacityIsThree(t *testing.T) {
	f := newFixture(t)
	f.settings(f.dealer, 3)
	f.link(f.user, f.dealer)
	c := f.caller(f.dealer, rbac.ScopeManaged, []int64{f.dealer.ID})
	start := time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := f.svc.Create(f.ctx, c, CreateInput{CustomerUserID: f.user.ID, StartsAt: start.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := f.svc.Create(f.ctx, c, CreateInput{CustomerUserID: f.user.ID, StartsAt: start.Add(3 * time.Hour)}); !errors.Is(err, ErrCapacityFull) {
		t.Fatalf("fourth appointment err = %v, want ErrCapacityFull", err)
	}
}

func TestStartIntakeCreatesDraftServiceAndSecondCallConflicts(t *testing.T) {
	f := newFixture(t)
	f.settings(f.dealer, 3)
	f.link(f.user, f.dealer)
	vehicle := f.vehicle(f.user, f.dealer)
	c := f.caller(f.dealer, rbac.ScopeManaged, []int64{f.dealer.ID})
	appt, err := f.svc.Create(f.ctx, c, CreateInput{
		CustomerUserID: f.user.ID, VehicleID: &vehicle.ID, StartsAt: time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, err := f.svc.StartIntake(f.ctx, c, appt.UUID)
	if err != nil {
		t.Fatalf("start intake: %v", err)
	}
	if started.ServiceID == nil || started.Status != StatusArrived {
		t.Fatalf("started appointment = %+v", started)
	}
	var status string
	if err := f.tx.QueryRow(f.ctx, `SELECT status FROM services WHERE id = $1 AND organization_id = $2`,
		*started.ServiceID, f.dealer.ID).Scan(&status); err != nil {
		t.Fatalf("service status lookup: %v", err)
	}
	if status != servicesuc.StatusDraft {
		t.Fatalf("service status = %s, want draft", status)
	}
	if _, err := f.svc.StartIntake(f.ctx, c, appt.UUID); !errors.Is(err, ErrIntakeStarted) {
		t.Fatalf("second start err = %v, want ErrIntakeStarted", err)
	}
}

func TestDistributorOccupancySeesOnlyOwnSubtree(t *testing.T) {
	f := newFixture(t)
	f.settings(f.dist, 5)
	f.settings(f.dealer, 4)
	f.settings(f.other, 7)
	start := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	for _, org := range []db.Organization{f.dealer, f.other} {
		if _, err := f.q.CreateAppointment(f.ctx, db.CreateAppointmentParams{
			OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: f.user.ID,
			StartsAt: tsArg(start), EndsAt: tsArg(start.Add(time.Hour)), EstimatedMinutes: 60, Source: "panel", Status: StatusScheduled,
		}); err != nil {
			t.Fatalf("seed appointment: %v", err)
		}
	}
	c := f.caller(f.dist, rbac.ScopeSubtree, []int64{f.dist.ID, f.dealer.ID})
	rows, err := f.svc.Occupancy(f.ctx, c, start)
	if err != nil {
		t.Fatalf("occupancy: %v", err)
	}
	seen := map[int64]bool{}
	for _, row := range rows {
		seen[row.OrganizationID] = true
		if row.OrganizationID == f.other.ID {
			t.Fatal("occupancy included dealer outside distributor subtree")
		}
	}
	if !seen[f.dealer.ID] || !seen[f.dist.ID] {
		t.Fatalf("occupancy rows = %+v, want distributor and own dealer", rows)
	}
}

func (f *fixture) closure(org db.Organization, day time.Time) {
	f.t.Helper()
	if _, err := f.q.CreateAppointmentClosure(f.ctx, db.CreateAppointmentClosureParams{
		OrganizationID: org.ID, BrandID: org.BrandID, ClosedOn: dateArg(day), Reason: "holiday",
	}); err != nil {
		f.t.Fatalf("closure: %v", err)
	}
}

func (f *fixture) eventNames() []string {
	var names []string
	for _, r := range f.out.All() {
		names = append(names, r.EventName)
	}
	sort.Strings(names) // Memory.All is unordered
	return names
}

func TestAvailabilityBerlinSlotsAndClosedDay(t *testing.T) {
	f := newFixture(t)
	f.settings(f.dealer, 3) // Europe/Berlin, monday 09:00-17:00
	c := f.caller(f.dealer, rbac.ScopeManaged, []int64{f.dealer.ID})
	monday := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) // parsed YYYY-MM-DD
	days, err := f.svc.Availability(f.ctx, c, monday, monday)
	if err != nil {
		t.Fatalf("availability: %v", err)
	}
	if len(days) != 1 || days[0].Date != "2026-10-05" || len(days[0].Slots) == 0 {
		t.Fatalf("availability = %+v", days)
	}
	if want := time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC); !days[0].Slots[0].Start.Equal(want) {
		t.Fatalf("first slot = %s, want %s (09:00 Berlin, CEST)", days[0].Slots[0].Start, want)
	}
	f.closure(f.dealer, monday)
	days, err = f.svc.Availability(f.ctx, c, monday, monday)
	if err != nil {
		t.Fatalf("availability closed: %v", err)
	}
	if !days[0].Closed || len(days[0].Slots) != 0 || days[0].RemainingCapacity != 0 {
		t.Fatalf("closed day availability = %+v, want empty", days[0])
	}
	if _, err := f.svc.Availability(f.ctx, c, monday, monday.AddDate(0, 0, maxRangeDays+1)); err == nil {
		t.Fatal("over-long range accepted")
	}
}

func TestCreateRejectsClosedDayAndForeignVehicle(t *testing.T) {
	f := newFixture(t)
	f.settings(f.dealer, 3)
	f.link(f.user, f.dealer)
	c := f.caller(f.dealer, rbac.ScopeManaged, []int64{f.dealer.ID})
	f.closure(f.dealer, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	// 23:30 UTC on Oct 4 is 01:30 on Oct 5 in Berlin: the closed local day.
	if _, err := f.svc.Create(f.ctx, c, CreateInput{CustomerUserID: f.user.ID, StartsAt: time.Date(2026, 10, 4, 23, 30, 0, 0, time.UTC)}); !errors.Is(err, ErrDayClosed) {
		t.Fatalf("closed day err = %v, want ErrDayClosed", err)
	}
	other := f.userRow("appointment-other")
	f.link(other, f.dealer)
	foreign := f.vehicle(other, f.dealer)
	var ve *ValidationError
	if _, err := f.svc.Create(f.ctx, c, CreateInput{CustomerUserID: f.user.ID, VehicleID: &foreign.ID, StartsAt: time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)}); !errors.As(err, &ve) || ve.Field != "vehicle_id" {
		t.Fatalf("foreign vehicle err = %v, want vehicle_id validation", err)
	}
	stranger := f.userRow("appointment-stranger")
	if _, err := f.svc.Create(f.ctx, c, CreateInput{CustomerUserID: stranger.ID, StartsAt: time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)}); !errors.As(err, &ve) || ve.Field != "customer_user_id" {
		t.Fatalf("non-customer err = %v, want customer_user_id validation", err)
	}
}

func TestPortalCreateRejectsForeignVehicleAsNotFound(t *testing.T) {
	f := newFixture(t)
	f.portalSettings(f.dealer, 3, true)
	f.link(f.user, f.dealer)
	other := f.userRow("portal-appointment-other")
	f.link(other, f.dealer)
	foreign := f.vehicle(other, f.dealer)
	if _, err := f.svc.PortalCreate(f.ctx, f.portalCaller(f.user), PortalCreateInput{
		DealerUUID: f.dealer.Uuid, VehicleUUID: foreign.Uuid, StartsAt: time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign vehicle err = %v, want ErrNotFound", err)
	}
}

func TestPortalCreateDisabledDealerNotFound(t *testing.T) {
	f := newFixture(t)
	f.portalSettings(f.dealer, 3, false)
	f.link(f.user, f.dealer)
	vehicle := f.vehicle(f.user, f.dealer)
	if _, err := f.svc.PortalCreate(f.ctx, f.portalCaller(f.user), PortalCreateInput{
		DealerUUID: f.dealer.Uuid, VehicleUUID: vehicle.Uuid, StartsAt: time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled dealer err = %v, want ErrNotFound", err)
	}
}

func TestPortalCreateRejectsFullDay(t *testing.T) {
	f := newFixture(t)
	f.portalSettings(f.dealer, 1, true)
	f.link(f.user, f.dealer)
	vehicle := f.vehicle(f.user, f.dealer)
	in := PortalCreateInput{
		DealerUUID: f.dealer.Uuid, VehicleUUID: vehicle.Uuid, StartsAt: time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC),
	}
	if _, err := f.svc.PortalCreate(f.ctx, f.portalCaller(f.user), in); err != nil {
		t.Fatalf("first portal create: %v", err)
	}
	in.StartsAt = in.StartsAt.Add(time.Hour)
	if _, err := f.svc.PortalCreate(f.ctx, f.portalCaller(f.user), in); !errors.Is(err, ErrCapacityFull) {
		t.Fatalf("full day err = %v, want ErrCapacityFull", err)
	}
}

func TestPortalCancelWindow(t *testing.T) {
	f := newFixture(t)
	f.portalSettings(f.dealer, 5, true)
	f.link(f.user, f.dealer)
	vehicle := f.vehicle(f.user, f.dealer)
	c := f.portalCaller(f.user)
	soon, err := f.svc.PortalCreate(f.ctx, c, PortalCreateInput{
		DealerUUID: f.dealer.Uuid, VehicleUUID: vehicle.Uuid, StartsAt: f.svc.nowFunc().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("soon create: %v", err)
	}
	if _, err := f.svc.PortalCancel(f.ctx, c, soon.UUID); !errors.Is(err, ErrCancelWindow) {
		t.Fatalf("one-hour cancel err = %v, want ErrCancelWindow", err)
	}
	later, err := f.svc.PortalCreate(f.ctx, c, PortalCreateInput{
		DealerUUID: f.dealer.Uuid, VehicleUUID: vehicle.Uuid, StartsAt: f.svc.nowFunc().Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("later create: %v", err)
	}
	cancelled, err := f.svc.PortalCancel(f.ctx, c, later.UUID)
	if err != nil {
		t.Fatalf("three-hour cancel: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("cancelled status = %s", cancelled.Status)
	}
}

func TestStatusTransitionsAndEvents(t *testing.T) {
	f := newFixture(t)
	f.settings(f.dealer, 5)
	f.link(f.user, f.dealer)
	c := f.caller(f.dealer, rbac.ScopeManaged, []int64{f.dealer.ID})
	start := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	a, err := f.svc.Create(f.ctx, c, CreateInput{CustomerUserID: f.user.ID, StartsAt: start})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.svc.SetStatus(f.ctx, c, a.UUID, StatusInput{Status: StatusArrived}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("scheduled->arrived err = %v", err)
	}
	if _, err := f.svc.Patch(f.ctx, c, a.UUID, PatchInput{CustomerUserID: f.user.ID, StartsAt: start.Add(time.Hour)}); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	for _, next := range []string{StatusConfirmed, StatusArrived} {
		if _, err := f.svc.SetStatus(f.ctx, c, a.UUID, StatusInput{Status: next}); err != nil {
			t.Fatalf("-> %s: %v", next, err)
		}
	}
	if _, err := f.svc.SetStatus(f.ctx, c, a.UUID, StatusInput{Status: StatusCancelled, CancelReason: strPtr("x")}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("arrived->cancelled err = %v", err)
	}
	if _, err := f.svc.Patch(f.ctx, c, a.UUID, PatchInput{CustomerUserID: f.user.ID, StartsAt: start}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reschedule arrived err = %v", err)
	}
	b, err := f.svc.Create(f.ctx, c, CreateInput{CustomerUserID: f.user.ID, StartsAt: start})
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	var ve *ValidationError
	if _, err := f.svc.SetStatus(f.ctx, c, b.UUID, StatusInput{Status: StatusCancelled}); !errors.As(err, &ve) {
		t.Fatalf("cancel without reason err = %v", err)
	}
	if _, err := f.svc.SetStatus(f.ctx, c, b.UUID, StatusInput{Status: StatusCancelled, CancelReason: strPtr("customer called")}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got := fmt.Sprint(f.eventNames())
	want := "[appointment.cancelled appointment.created appointment.created appointment.rescheduled]"
	if got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

func TestDistributorCannotWriteDealerAppointment(t *testing.T) {
	f := newFixture(t)
	f.settings(f.dealer, 3)
	f.link(f.user, f.dealer)
	dealerCaller := f.caller(f.dealer, rbac.ScopeManaged, []int64{f.dealer.ID})
	a, err := f.svc.Create(f.ctx, dealerCaller, CreateInput{CustomerUserID: f.user.ID, StartsAt: time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	dist := f.caller(f.dist, rbac.ScopeSubtree, []int64{f.dist.ID, f.dealer.ID})
	if _, err := f.svc.SetStatus(f.ctx, dist, a.UUID, StatusInput{Status: StatusConfirmed}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor write err = %v, want ErrForbidden", err)
	}
	items, total, err := f.svc.List(f.ctx, dist, ListFilter{From: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), Limit: 20})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("distributor list = %d/%d err=%v, want the dealer appointment", len(items), total, err)
	}
	center := f.caller(f.center, rbac.ScopeAll, nil)
	_, total, err = f.svc.List(f.ctx, center, ListFilter{From: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), Limit: 100})
	if err != nil || total < 1 {
		t.Fatalf("center (all) list total = %d err=%v, want >= 1", total, err)
	}
}

// TestCreateCapacityHoldsUnderConcurrentInserts runs on committed rows and
// real connections: the settings row lock must keep concurrent bookings of
// the same day within the capacity.
func TestCreateCapacityHoldsUnderConcurrentInserts(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var brand, center int64
	if err := pool.QueryRow(ctx, `SELECT b.id, o.id FROM brands b JOIN organizations o ON o.brand_id = b.id AND o.type = 'center'
		WHERE b.slug = 'olex' ORDER BY o.id LIMIT 1`).Scan(&brand, &center); err != nil {
		t.Fatalf("brand: %v", err)
	}
	slug := fmt.Sprintf("tec323-race-%d", time.Now().UnixNano())
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: slug, Name: slug, Status: "active", Type: "dealer", BrandID: brand, ParentID: pgtype.Int8{Int64: center, Valid: true},
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte(`{}`),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: slug + "@example.test", Valid: true}, PasswordHash: "x", Name: "race", Surname: "User", Status: "active",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{UserID: user.ID, OrganizationID: org.ID, BrandID: brand}); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := q.UpsertAppointmentSettings(ctx, db.UpsertAppointmentSettingsParams{
		OrganizationID: org.ID, BrandID: brand, DailyVehicleCapacity: 3, DefaultEstimatedMinutes: 60, SlotIntervalMinutes: 60,
		WorkingHours: []byte(`{}`),
	}); err != nil {
		t.Fatalf("settings: %v", err)
	}
	svc := New(pool, q, outbox.NewMemory(), nil)
	c := Caller{
		Principal: authctx.Principal{UserInternal: user.ID, PermissionScopes: map[string]rbac.Scope{
			rbac.PermAppointmentsRead: rbac.ScopeManaged, rbac.PermAppointmentsWrite: rbac.ScopeManaged,
		}},
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: brand},
		Filter: scopefilter.Filter{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeManaged, OrgID: org.ID, OrgIDs: []int64{org.ID}},
	}
	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Create(ctx, c, CreateInput{CustomerUserID: user.ID, StartsAt: time.Date(2030, 3, 4, 8, i, 0, 0, time.UTC)})
		}(i)
	}
	wg.Wait()
	ok, full := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrCapacityFull):
			full++
		default:
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if ok != 3 || full != workers-3 {
		t.Fatalf("created=%d full=%d, want 3/%d", ok, full, workers-3)
	}
}
