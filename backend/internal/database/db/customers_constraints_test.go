package db_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-159: database-level guards of the customer schema (migration 000048).
// Everything runs inside one transaction that is rolled back; each failing
// statement runs in its own savepoint.

type customerFixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	olex    db.Organization
	glorian db.Organization
	user    db.User
}

func newCustomerFixture(t *testing.T) *customerFixture {
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
	f := &customerFixture{ctx: ctx, tx: tx, q: q,
		olex: brandCenter(t, ctx, q, "olex"), glorian: brandCenter(t, ctx, q, "glorian")}
	f.user = f.newUser(t, "+905551590001")
	return f
}

func (f *customerFixture) newUser(t *testing.T, e164 string) db.User {
	t.Helper()
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Musteri", Surname: "T159", Status: "active",
		PhoneE164: pgtype.Text{String: e164, Valid: true},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func (f *customerFixture) expectCode(t *testing.T, name string, fn func(sp pgx.Tx) error, codes ...string) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = fn(sp)
	_ = sp.Rollback(f.ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("%s: want SQLSTATE %v, got %v", name, codes, err)
	}
	for _, c := range codes {
		if pgErr.Code == c {
			return
		}
	}
	t.Fatalf("%s: want SQLSTATE %v, got %s (%s)", name, codes, pgErr.Code, pgErr.Message)
}

func (f *customerFixture) expectOK(t *testing.T, name string, fn func(sp pgx.Tx) error) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sp.Rollback(f.ctx) }()
	if err := fn(sp); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func TestCustomerOrganizationsConstraints(t *testing.T) {
	f := newCustomerFixture(t)
	link := db.InsertCustomerOrganizationParams{UserID: f.user.ID, OrganizationID: f.olex.ID, BrandID: f.olex.BrandID}
	first, err := f.q.InsertCustomerOrganization(f.ctx, link)
	if err != nil {
		t.Fatalf("first link: %v", err)
	}

	// Same (user, org) twice is rejected.
	f.expectCode(t, "duplicate user x org", func(sp pgx.Tx) error {
		_, err := db.New(sp).InsertCustomerOrganization(f.ctx, link)
		return err
	}, "23505")

	// The idempotent upsert returns the existing row.
	again, err := f.q.LinkCustomerOrganization(f.ctx, db.LinkCustomerOrganizationParams{
		UserID: f.user.ID, OrganizationID: f.olex.ID, BrandID: f.olex.BrandID,
	})
	if err != nil || again.ID != first.ID {
		t.Fatalf("idempotent link = %d, %v; want %d", again.ID, err, first.ID)
	}

	// brand_id must be the organization's brand (K20).
	f.expectCode(t, "brand mismatch", func(sp pgx.Tx) error {
		_, err := db.New(sp).InsertCustomerOrganization(f.ctx, db.InsertCustomerOrganizationParams{
			UserID: f.user.ID, OrganizationID: f.glorian.ID, BrandID: f.olex.BrandID,
		})
		return err
	}, "23514")

	// Scope: an organization of another scope does not see the customer.
	in, err := f.q.CustomerInScope(f.ctx, db.CustomerInScopeParams{UserID: f.user.ID, OrgIds: []int64{f.olex.ID}})
	if err != nil || !in {
		t.Fatalf("in scope = %v, %v", in, err)
	}
	in, err = f.q.CustomerInScope(f.ctx, db.CustomerInScopeParams{UserID: f.user.ID, OrgIds: []int64{f.glorian.ID}})
	if err != nil || in {
		t.Fatalf("other org in scope = %v, %v", in, err)
	}
	in, err = f.q.CustomerInScope(f.ctx, db.CustomerInScopeParams{UserID: f.user.ID, OrgIds: []int64{}})
	if err != nil || in {
		t.Fatalf("empty org set (customer scope) in scope = %v, %v", in, err)
	}
	rows, err := f.q.ListOrganizationCustomers(f.ctx, db.ListOrganizationCustomersParams{
		OrgIds: []int64{f.glorian.ID}, LimitCount: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == f.user.ID {
			t.Fatal("customer of olex listed for glorian")
		}
	}
	rows, err = f.q.ListOrganizationCustomers(f.ctx, db.ListOrganizationCustomersParams{
		OrgIds: []int64{f.olex.ID}, Q: text("T159"), LimitCount: 50,
	})
	if err != nil || len(rows) != 1 || rows[0].ID != f.user.ID {
		t.Fatalf("olex customers = %+v, %v", rows, err)
	}
	orgs, err := f.q.ListCustomerOrganizationsByUser(f.ctx, db.ListCustomerOrganizationsByUserParams{UserID: f.user.ID})
	if err != nil || len(orgs) != 1 || orgs[0].OrganizationID != f.olex.ID {
		t.Fatalf("user orgs = %+v, %v", orgs, err)
	}
}

func TestVehiclesConstraints(t *testing.T) {
	f := newCustomerFixture(t)
	var carBrand, otherBrand, model int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ('T159 Brand A') RETURNING id`).Scan(&carBrand); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ('T159 Brand B') RETURNING id`).Scan(&otherBrand); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, 'T159 Model') RETURNING id`, carBrand).Scan(&model); err != nil {
		t.Fatal(err)
	}
	base := func() db.CreateVehicleParams {
		return db.CreateVehicleParams{
			UserID: f.user.ID, BrandID: f.olex.BrandID,
			OrganizationID: pgtype.Int8{Int64: f.olex.ID, Valid: true},
			CarBrandID:     pgtype.Int8{Int64: carBrand, Valid: true},
			CarModelID:     pgtype.Int8{Int64: model, Valid: true},
			ModelYear:      pgtype.Int2{Int16: 2024, Valid: true},
			Plate:          text("34 ABC 123"), PlateNormalized: text("34ABC123"), PlateCountry: text("TR"),
		}
	}
	create := func(arg db.CreateVehicleParams) func(sp pgx.Tx) error {
		return func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateVehicle(f.ctx, arg)
			return err
		}
	}

	// VIN: 17 characters, no I/O/Q, upper case; NULL is allowed.
	for _, vin := range []string{"WVWZZZ1JZXW000001", "1HGCM82633A004352"} {
		arg := base()
		arg.Vin = text(vin)
		f.expectOK(t, "valid vin "+vin, create(arg))
	}
	f.expectOK(t, "no vin", create(base()))
	for _, vin := range []string{"WVWZZZ1JZXW00000", "WVWZZZ1JZXW0000012", "WVWZZZ1JZXI000001", "WVWZZZ1JZXO000001", "wvwzzz1jzxw000001", ""} {
		arg := base()
		arg.Vin = text(vin)
		// An 18-character VIN hits VARCHAR(17) (22001) before the CHECK (23514).
		f.expectCode(t, "invalid vin "+vin, create(arg), "23514", "22001")
	}

	// Same plate twice is allowed (plates change hands): no unique key.
	if _, err := f.q.CreateVehicle(f.ctx, base()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.CreateVehicle(f.ctx, base()); err != nil {
		t.Fatalf("second vehicle with the same plate: %v", err)
	}
	found, err := f.q.FindVehiclesByPlate(f.ctx, db.FindVehiclesByPlateParams{PlateCountry: text("TR"), PlateNormalized: text("34ABC123")})
	if err != nil || len(found) != 2 {
		t.Fatalf("plate lookup = %d, %v", len(found), err)
	}

	// Plate needs its normalized form and a country.
	arg := base()
	arg.PlateNormalized = pgtype.Text{}
	f.expectCode(t, "plate without normalized", create(arg), "23514")
	arg = base()
	arg.PlateCountry = pgtype.Text{}
	f.expectCode(t, "plate without country", create(arg), "23514")
	arg = base()
	arg.PlateCountry = text("tr")
	f.expectCode(t, "lower-case plate country", create(arg), "23514")

	// The model must belong to the car brand; a model needs a brand.
	arg = base()
	arg.CarBrandID = pgtype.Int8{Int64: otherBrand, Valid: true}
	f.expectCode(t, "model of another car brand", create(arg), "23514")
	arg = base()
	arg.CarBrandID = pgtype.Int8{}
	f.expectCode(t, "model without car brand", create(arg), "23514")
	arg = base()
	arg.ModelYear = pgtype.Int2{Int16: 1800, Valid: true}
	f.expectCode(t, "model year", create(arg), "23514")

	// brand_id must be the registering organization's brand (K20).
	arg = base()
	arg.OrganizationID = pgtype.Int8{Int64: f.glorian.ID, Valid: true}
	f.expectCode(t, "vehicle brand mismatch", create(arg), "23514")

	// Owner users are protected: a user with a vehicle cannot be deleted.
	f.expectCode(t, "delete vehicle owner", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `DELETE FROM users WHERE id = $1`, f.user.ID)
		return err
	}, "23001", "23503")
}

func TestCustomerUsersAndProfile(t *testing.T) {
	f := newCustomerFixture(t)

	// users.status accepts anonymized and still rejects unknown values.
	f.expectOK(t, "anonymized status", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE users SET status = 'anonymized' WHERE id = $1`, f.user.ID)
		return err
	})
	f.expectCode(t, "unknown status", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE users SET status = 'deleted' WHERE id = $1`, f.user.ID)
		return err
	}, "23514")

	// merged_into_user_id points to another user, never to itself.
	target := f.newUser(t, "+905551590002")
	f.expectOK(t, "merge pointer", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE users SET merged_into_user_id = $2 WHERE id = $1`, f.user.ID, target.ID)
		return err
	})
	f.expectCode(t, "merge into self", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE users SET merged_into_user_id = id WHERE id = $1`, f.user.ID)
		return err
	}, "23514")

	// Profile: ciphertext and mask go together; JSON columns are objects.
	prefs := []byte(`{"whatsapp": true}`)
	profile := db.CreateCustomerProfileParams{
		UserID: f.user.ID, Type: "individual", Address: []byte(`{}`), NotificationPrefs: prefs,
		NationalIDEnc: []byte{1, 2, 3}, NationalIDLast4: text("8901"),
	}
	f.expectOK(t, "profile", func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateCustomerProfile(f.ctx, profile)
		return err
	})
	bad := profile
	bad.NationalIDLast4 = pgtype.Text{}
	f.expectCode(t, "ciphertext without mask", func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateCustomerProfile(f.ctx, bad)
		return err
	}, "23514")
	bad = profile
	bad.Type = "company"
	f.expectCode(t, "unknown type", func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateCustomerProfile(f.ctx, bad)
		return err
	}, "23514")
	bad = profile
	bad.Address = []byte(`[]`)
	f.expectCode(t, "address not an object", func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateCustomerProfile(f.ctx, bad)
		return err
	}, "23514")
	if err := f.q.EnsureCustomerProfile(f.ctx, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.q.EnsureCustomerProfile(f.ctx, f.user.ID); err != nil {
		t.Fatalf("ensure twice: %v", err)
	}
	got, err := f.q.GetCustomerProfile(f.ctx, f.user.ID)
	if err != nil || got.Type != "individual" || string(got.NotificationPrefs) == "" {
		t.Fatalf("default profile = %+v, %v", got, err)
	}
}

// K29: organizations.phone is E.164 or empty.
func TestOrganizationPhoneE164Check(t *testing.T) {
	f := newCustomerFixture(t)
	f.expectOK(t, "e164 phone", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE organizations SET phone = '+902125551234' WHERE id = $1`, f.olex.ID)
		return err
	})
	f.expectOK(t, "empty phone", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE organizations SET phone = '' WHERE id = $1`, f.olex.ID)
		return err
	})
	f.expectCode(t, "national phone", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE organizations SET phone = '0212 555 12 34' WHERE id = $1`, f.olex.ID)
		return err
	}, "23514")

	// The normalizer query keeps a phone written after the migration.
	if _, err := f.tx.Exec(f.ctx, `UPDATE organizations SET phone = '+902125551234', phone_raw = '0212' WHERE id = $1`, f.olex.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.ResolveOrganizationRawPhone(f.ctx, db.ResolveOrganizationRawPhoneParams{ID: f.olex.ID, PhoneE164: "+905321234567"}); err != nil {
		t.Fatal(err)
	}
	var phone string
	var raw pgtype.Text
	if err := f.tx.QueryRow(f.ctx, `SELECT phone, phone_raw FROM organizations WHERE id = $1`, f.olex.ID).Scan(&phone, &raw); err != nil {
		t.Fatal(err)
	}
	if phone != "+902125551234" || raw.Valid {
		t.Fatalf("after resolve phone=%q raw=%+v", phone, raw)
	}
}
