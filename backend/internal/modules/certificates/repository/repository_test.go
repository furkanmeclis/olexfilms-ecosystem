package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/certificates/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type certFixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	repo     *repository.Repository
	brandID  int64
	centerID int64
	dealer   db.Organization
	user     db.User
	customer db.User
	vehicle  db.Vehicle
	carBrand int64
	carModel int64
	category db.ProductCategory
	film     db.Product
	piece    db.Product
	seq      int
}

func newCertFixture(t *testing.T) *certFixture {
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
		t.Fatalf("olex brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("olex center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dealer, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t479-dealer-%s", suffix), Name: "T479 Dealer", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
		BrandID: brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("dealer: %v", err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Sertifika", Surname: "Personel", Status: "active",
		Email: text(fmt.Sprintf("t479-staff-%s@example.test", suffix)),
	})
	if err != nil {
		t.Fatalf("staff user: %v", err)
	}
	customer, err := q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Musteri", Surname: "T479", Status: "active",
		Email: text(fmt.Sprintf("t479-customer-%s@example.test", suffix)),
	})
	if err != nil {
		t.Fatalf("customer user: %v", err)
	}
	var carBrand int64
	if err := tx.QueryRow(ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "T479 "+suffix).Scan(&carBrand); err != nil {
		t.Fatalf("car brand: %v", err)
	}
	var carModel int64
	if err := tx.QueryRow(ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`, carBrand, "T479 Model").Scan(&carModel); err != nil {
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
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t479-cat-" + suffix,
		AvailableParts: []byte(`["hood"]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product := func(sku, unitType string) db.Product {
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: center.ID, BrandID: brand.ID, CategoryID: category.ID,
			Sku: sku + "-" + suffix, Name: sku, Images: []byte("[]"), UnitType: unitType, Active: true,
		})
		if err != nil {
			t.Fatalf("product %s: %v", sku, err)
		}
		return p
	}
	return &certFixture{
		ctx: ctx, tx: tx, q: q, repo: repository.New(q), brandID: brand.ID, centerID: center.ID,
		dealer: dealer, user: user, customer: customer, vehicle: vehicle, carBrand: carBrand, carModel: carModel,
		category: category, film: product("t479-film", "roll_meter"), piece: product("t479-piece", "piece"),
	}
}

func text(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: true}
}

func ts(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v, Valid: true}
}

func numeric(t *testing.T, raw string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(raw); err != nil {
		t.Fatalf("numeric %s: %v", raw, err)
	}
	return n
}

func i8(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

func (f *certFixture) certType(t *testing.T, label string) db.CertificateType {
	t.Helper()
	ct, err := f.q.CreateCertificateType(f.ctx, db.CreateCertificateTypeParams{
		OrganizationID: f.centerID, BrandID: f.brandID,
		Name:        []byte(fmt.Sprintf(`{"tr":%q,"en":%q}`, label, label)),
		Description: []byte(`{}`), Active: true,
	})
	if err != nil {
		t.Fatalf("certificate type %s: %v", label, err)
	}
	return ct
}

func (f *certFixture) unit(t *testing.T, p db.Product, meters string) db.Unit {
	t.Helper()
	f.seq++
	arg := db.CreateUnitParams{
		OrganizationID: f.centerID, BrandID: f.brandID, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T479-%d-%d", time.Now().UnixNano(), f.seq),
		UnitKind: "serial", Source: "generated", Status: "available",
	}
	if meters != "" {
		arg.InitialMeters = numeric(t, meters)
		arg.RemainingMeters = numeric(t, meters)
	}
	u, err := f.q.CreateUnit(f.ctx, arg)
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	return u
}

func (f *certFixture) service(t *testing.T) db.Service {
	t.Helper()
	f.seq++
	svc, err := f.q.CreateService(f.ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T479-%d-%d", time.Now().UnixNano(), f.seq),
		OrganizationID: f.dealer.ID, BrandID: f.brandID, CustomerUserID: f.customer.ID,
		VehicleID: f.vehicle.ID, CarBrandID: f.carBrand, CarModelID: f.carModel,
		Status: "draft", CreatedByUserID: i8(f.user.ID),
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return svc
}

func (f *certFixture) addServiceItem(t *testing.T, svc db.Service, p db.Product, unit db.Unit, kind string) {
	t.Helper()
	arg := db.CreateServiceItemParams{
		ServiceID: svc.ID, ProductID: p.ID, UnitID: unit.ID, Kind: kind, AppliedParts: []byte(`[]`),
	}
	if kind == "partial" {
		arg.Meters = numeric(t, "1")
	}
	if _, err := f.q.CreateServiceItem(f.ctx, arg); err != nil {
		t.Fatalf("service item: %v", err)
	}
}

func (f *certFixture) validCertificate(t *testing.T, typ db.CertificateType, issued, expires time.Time, salt int) db.Certificate {
	t.Helper()
	c, err := f.q.CreateCertificate(f.ctx, db.CreateCertificateParams{
		UserID: f.user.ID, OrganizationID: f.dealer.ID, BrandID: f.brandID, TypeID: typ.ID,
		StorageKey: fmt.Sprintf("certificates/t479/%d.pdf", salt),
		Sha256:     fmt.Sprintf("%064x", salt+1),
		IssuedAt:   ts(issued),
		ExpiresAt:  ts(expires),
		Status:     "pending",
	})
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	c, err = f.q.VerifyCertificate(f.ctx, db.VerifyCertificateParams{
		ID: c.ID, BrandID: f.brandID, VerifiedByUserID: i8(f.user.ID), VerifiedByOrgID: i8(f.centerID),
		VerifiedAt: ts(issued.Add(time.Hour)),
	})
	if err != nil {
		t.Fatalf("verify certificate: %v", err)
	}
	return c
}

func TestRepositoryValidCertificatesMatchProductAndCategory(t *testing.T) {
	f := newCertFixture(t)
	now := time.Now().UTC()

	categoryType := f.certType(t, "Kategori Sertifikasi")
	if _, err := f.q.AddCertificateTypeCategory(f.ctx, db.AddCertificateTypeCategoryParams{
		TypeID: categoryType.ID, OrganizationID: f.centerID, BrandID: f.brandID, CategoryID: f.category.ID,
	}); err != nil {
		t.Fatalf("category binding: %v", err)
	}
	productType := f.certType(t, "Urun Sertifikasi")
	if _, err := f.q.AddCertificateTypeProduct(f.ctx, db.AddCertificateTypeProductParams{
		TypeID: productType.ID, OrganizationID: f.centerID, BrandID: f.brandID, ProductID: f.piece.ID,
	}); err != nil {
		t.Fatalf("product binding: %v", err)
	}
	expiredType := f.certType(t, "Suresi Gecmis")
	if _, err := f.q.AddCertificateTypeCategory(f.ctx, db.AddCertificateTypeCategoryParams{
		TypeID: expiredType.ID, OrganizationID: f.centerID, BrandID: f.brandID, CategoryID: f.category.ID,
	}); err != nil {
		t.Fatalf("expired binding: %v", err)
	}

	categoryCert := f.validCertificate(t, categoryType, now.Add(-48*time.Hour), now.Add(24*time.Hour), 1)
	productCert := f.validCertificate(t, productType, now.Add(-48*time.Hour), now.Add(48*time.Hour), 2)
	expiredCert := f.validCertificate(t, expiredType, now.Add(-96*time.Hour), now.Add(-24*time.Hour), 3)

	svc := f.service(t)
	f.addServiceItem(t, svc, f.piece, f.unit(t, f.piece, ""), "full")
	f.addServiceItem(t, svc, f.film, f.unit(t, f.film, "15"), "partial")

	got, err := f.repo.ListValidForServiceUser(f.ctx, db.ListValidCertificatesForServiceUserParams{
		BrandID: f.brandID, OrganizationID: f.dealer.ID, UserID: f.user.ID,
		ServiceID: svc.ID, Now: ts(now),
	})
	if err != nil {
		t.Fatalf("valid certificates: %v", err)
	}
	ids := map[int64]bool{}
	for _, c := range got {
		ids[c.ID] = true
	}
	if !ids[categoryCert.ID] {
		t.Fatalf("category-bound certificate missing: %+v", got)
	}
	if !ids[productCert.ID] {
		t.Fatalf("product-bound certificate missing: %+v", got)
	}
	if ids[expiredCert.ID] {
		t.Fatalf("expired valid certificate counted as valid: %+v", got)
	}
}

func TestRepositoryCertificateListSortWhitelistAndTiebreak(t *testing.T) {
	f := newCertFixture(t)
	now := time.Now().UTC()
	typ := f.certType(t, "Liste")
	expires := now.Add(72 * time.Hour)
	first := f.validCertificate(t, typ, now.Add(-time.Hour), expires, 10)
	second := f.validCertificate(t, typ, now.Add(-time.Hour), expires, 11)

	_, err := f.repo.ListCertificates(f.ctx, db.ListCertificatesParams{
		BrandID: f.brandID, TypeIds: []int64{typ.ID}, RowLimit: 10,
	}, []apiquery.SortField{{Field: "not_allowed"}})
	var ve *apiquery.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("invalid sort error = %v, want ValidationError", err)
	}

	asc, err := f.repo.ListCertificates(f.ctx, db.ListCertificatesParams{
		BrandID: f.brandID, TypeIds: []int64{typ.ID}, RowLimit: 10,
	}, []apiquery.SortField{{Field: "expires_at"}})
	if err != nil {
		t.Fatalf("list asc: %v", err)
	}
	if len(asc) < 2 || asc[0].ID != first.ID || asc[1].ID != second.ID {
		t.Fatalf("asc tiebreak = %+v, want %d then %d", asc, first.ID, second.ID)
	}
	desc, err := f.repo.ListCertificates(f.ctx, db.ListCertificatesParams{
		BrandID: f.brandID, TypeIds: []int64{typ.ID}, RowLimit: 10,
	}, []apiquery.SortField{{Field: "expires_at", Desc: true}})
	if err != nil {
		t.Fatalf("list desc: %v", err)
	}
	if len(desc) < 2 || desc[0].ID != second.ID || desc[1].ID != first.ID {
		t.Fatalf("desc tiebreak = %+v, want %d then %d", desc, second.ID, first.ID)
	}
}
