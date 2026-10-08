package usecase

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeFeatures map[int64]bool

func (f fakeFeatures) Enabled(_ context.Context, organizationID int64, _ string) (bool, error) {
	return f[organizationID], nil
}

type fakeSettings struct {
	days            int
	requireApproval bool
}

func (f fakeSettings) CertificatesExpiryNoticeDays(context.Context) int { return f.days }
func (f fakeSettings) CertificatesRequireAdminApproval(context.Context) bool {
	return f.requireApproval
}

type cronFixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	cron     *CronService
	features fakeFeatures
	brandID  int64
	centerID int64
	dealer   db.Organization
	user     db.User
	customer db.User
	vehicle  db.Vehicle
	carBrand int64
	carModel int64
	category db.ProductCategory
	product  db.Product
	seq      int
}

func newCronFixture(t *testing.T) *cronFixture {
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
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dealer, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t481-dealer-%s", suffix), Name: "T481 Dealer", Status: "active",
		AccessStartsAt: ts(time.Now().Add(-time.Hour)), Type: "dealer",
		ParentID: pgtype.Int8{Int64: center.ID, Valid: true}, BrandID: brand.ID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("dealer: %v", err)
	}
	user := createUser(t, q, ctx, "staff", suffix)
	customer := createUser(t, q, ctx, "customer", suffix)
	if _, err := tx.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, 'owner')`, dealer.ID, user.ID); err != nil {
		t.Fatalf("member: %v", err)
	}
	var carBrand int64
	if err := tx.QueryRow(ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "T481 "+suffix).Scan(&carBrand); err != nil {
		t.Fatalf("car brand: %v", err)
	}
	var carModel int64
	if err := tx.QueryRow(ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`, carBrand, "Model").Scan(&carModel); err != nil {
		t.Fatalf("car model: %v", err)
	}
	vehicle, err := q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: customer.ID, BrandID: brand.ID,
		CarBrandID: pgtype.Int8{Int64: carBrand, Valid: true},
		CarModelID: pgtype.Int8{Int64: carModel, Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	category, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t481-cat-" + suffix,
		AvailableParts: []byte(`["hood"]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product, err := q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: center.ID, BrandID: brand.ID, CategoryID: category.ID,
		Sku: "t481-" + suffix, Name: "T481 PPF", Images: []byte("[]"), UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	features := fakeFeatures{dealer.ID: true}
	qtx := db.New(tx)
	cron := NewCron(tx, qtx, outbox.NewStore(nil, qtx), features, fakeSettings{days: 30, requireApproval: true}, nil)
	return &cronFixture{
		ctx: ctx, tx: tx, q: q, cron: cron, features: features, brandID: brand.ID, centerID: center.ID,
		dealer: dealer, user: user, customer: customer, vehicle: vehicle, carBrand: carBrand, carModel: carModel,
		category: category, product: product,
	}
}

func createUser(t *testing.T, q *db.Queries, ctx context.Context, kind, suffix string) db.User {
	t.Helper()
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "T481", Surname: kind, Status: "active",
		Email: text(fmt.Sprintf("t481-%s-%s@example.test", kind, suffix)),
	})
	if err != nil {
		t.Fatalf("%s user: %v", kind, err)
	}
	return u
}

func (f *cronFixture) certType(t *testing.T) db.CertificateType {
	t.Helper()
	ct, err := f.q.CreateCertificateType(f.ctx, db.CreateCertificateTypeParams{
		OrganizationID: f.centerID, BrandID: f.brandID,
		Name: []byte(`{"tr":"PPF Sertifika","en":"PPF certificate"}`), Description: []byte(`{}`), Active: true,
	})
	if err != nil {
		t.Fatalf("type: %v", err)
	}
	if _, err := f.q.AddCertificateTypeProduct(f.ctx, db.AddCertificateTypeProductParams{
		TypeID: ct.ID, OrganizationID: f.centerID, BrandID: f.brandID, ProductID: f.product.ID,
	}); err != nil {
		t.Fatalf("type product: %v", err)
	}
	return ct
}

func (f *cronFixture) certificate(t *testing.T, typ db.CertificateType, expires pgtype.Timestamptz, salt int) db.Certificate {
	t.Helper()
	issued := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	c, err := f.q.CreateCertificate(f.ctx, db.CreateCertificateParams{
		UserID: f.user.ID, OrganizationID: f.dealer.ID, BrandID: f.brandID, TypeID: typ.ID,
		StorageKey: fmt.Sprintf("certificates/t481/%d.pdf", salt), Sha256: fmt.Sprintf("%064x", salt+10),
		IssuedAt: ts(issued), ExpiresAt: expires, Status: "pending",
	})
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	c, err = f.q.VerifyCertificate(f.ctx, db.VerifyCertificateParams{
		ID: c.ID, BrandID: f.brandID, VerifiedByUserID: pgtype.Int8{Int64: f.user.ID, Valid: true},
		VerifiedByOrgID: pgtype.Int8{Int64: f.centerID, Valid: true}, VerifiedAt: ts(issued.Add(time.Hour)),
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	return c
}

func (f *cronFixture) openService(t *testing.T) db.Service {
	t.Helper()
	f.seq++
	svc, err := f.q.CreateService(f.ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T481-%d-%d", time.Now().UnixNano(), f.seq),
		OrganizationID: f.dealer.ID, BrandID: f.brandID, CustomerUserID: f.customer.ID,
		VehicleID: f.vehicle.ID, CarBrandID: f.carBrand, CarModelID: f.carModel,
		Status: "draft", CreatedByUserID: pgtype.Int8{Int64: f.user.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	unit, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
		OrganizationID: f.centerID, BrandID: f.brandID, ProductID: f.product.ID,
		Barcode: fmt.Sprintf("T481-UNIT-%d-%d", time.Now().UnixNano(), f.seq), UnitKind: "serial",
		Source: "generated", Status: "available",
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	if _, err := f.q.CreateServiceItem(f.ctx, db.CreateServiceItemParams{
		ServiceID: svc.ID, ProductID: f.product.ID, UnitID: unit.ID, Kind: "full",
		AppliedParts: []byte(`[]`),
	}); err != nil {
		t.Fatalf("service item: %v", err)
	}
	return svc
}

func (f *cronFixture) outboxCount(t *testing.T, name string, certificateID int64) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = $1 AND (payload->>'entity_id')::bigint = $2`, name, certificateID).Scan(&n); err != nil {
		t.Fatalf("outbox count: %v", err)
	}
	return n
}

func TestCronNotifyExpiringOnce(t *testing.T) {
	f := newCronFixture(t)
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	typ := f.certType(t)
	cert := f.certificate(t, typ, ts(now.Add(30*24*time.Hour)), 1)

	n, err := f.cron.NotifyExpiring(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || f.outboxCount(t, events.CertificatesExpiring, cert.ID) != 1 {
		t.Fatalf("first notice n/outbox = %d/%d", n, f.outboxCount(t, events.CertificatesExpiring, cert.ID))
	}
	n, err = f.cron.NotifyExpiring(f.ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || f.outboxCount(t, events.CertificatesExpiring, cert.ID) != 1 {
		t.Fatalf("second notice n/outbox = %d/%d", n, f.outboxCount(t, events.CertificatesExpiring, cert.ID))
	}
}

func TestCronExpiresAndRefreshesServiceWarning(t *testing.T) {
	f := newCronFixture(t)
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	typ := f.certType(t)
	cert := f.certificate(t, typ, ts(now.Add(-time.Minute)), 2)
	svc := f.openService(t)

	n, err := f.cron.ExpireDue(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || f.outboxCount(t, events.CertificateExpired, cert.ID) != 1 {
		t.Fatalf("expired n/outbox = %d/%d", n, f.outboxCount(t, events.CertificateExpired, cert.ID))
	}
	got, err := f.q.GetCertificate(f.ctx, db.GetCertificateParams{ID: cert.ID, BrandID: f.brandID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "expired" {
		t.Fatalf("status = %s", got.Status)
	}
	var reason, decision string
	if err := f.tx.QueryRow(f.ctx, `SELECT reason, decision FROM service_certificate_warnings WHERE service_id = $1`, svc.ID).Scan(&reason, &decision); err != nil {
		t.Fatalf("warning: %v", err)
	}
	if reason != "expired" || decision != "pending_approval" {
		t.Fatalf("warning = %s/%s", reason, decision)
	}
}

func TestCronSkipsNonExpiringAndDisabledOrganizations(t *testing.T) {
	f := newCronFixture(t)
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	typ := f.certType(t)
	never := f.certificate(t, typ, pgtype.Timestamptz{}, 3)
	due := f.certificate(t, typ, ts(now.Add(-time.Minute)), 4)
	f.features[f.dealer.ID] = false

	notices, err := f.cron.NotifyExpiring(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := f.cron.ExpireDue(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if notices != 0 || expired != 0 {
		t.Fatalf("disabled/non-expiring wrote notices=%d expired=%d", notices, expired)
	}
	for _, cert := range []db.Certificate{never, due} {
		got, err := f.q.GetCertificate(f.ctx, db.GetCertificateParams{ID: cert.ID, BrandID: f.brandID})
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "valid" || got.ExpiryNoticeSentAt.Valid {
			t.Fatalf("cert %d changed: status=%s notice=%v", cert.ID, got.Status, got.ExpiryNoticeSentAt.Valid)
		}
	}
}
