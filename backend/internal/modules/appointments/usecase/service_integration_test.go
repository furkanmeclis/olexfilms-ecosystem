package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
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
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
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
	_, err := f.q.UpsertAppointmentSettings(f.ctx, db.UpsertAppointmentSettingsParams{
		OrganizationID: org.ID, BrandID: org.BrandID, DailyVehicleCapacity: capacity,
		DefaultEstimatedMinutes: 60, SlotIntervalMinutes: 60,
		WorkingHours: []byte(`{"monday":[{"start":"09:00","end":"17:00"}]}`),
	})
	if err != nil {
		f.t.Fatalf("settings: %v", err)
	}
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
