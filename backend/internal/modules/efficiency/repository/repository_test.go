package repository

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx       context.Context
	tx        pgx.Tx
	q         *db.Queries
	store     *Store
	prefix    string
	seq       int
	brandID   int64
	centerID  int64
	dealer    db.Organization
	staff     db.User
	customer  db.User
	vehicle   db.Vehicle
	carBrand  int64
	sedan     int64
	suv       int64
	category  db.ProductCategory
	film      db.Product
	otherFilm db.Product
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

	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	f := &fixture{
		ctx: ctx, tx: tx, q: q, store: FromQueries(q),
		prefix:  fmt.Sprintf("t487-%d", time.Now().UnixNano()),
		brandID: brand.ID, centerID: center.ID,
	}
	f.dealer = f.org(t, "dealer", center.ID)
	f.staff = f.user(t, "staff")
	f.customer = f.user(t, "customer")
	f.carBrand, f.sedan, f.suv = f.carCatalog(t)
	f.vehicle, err = q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: f.customer.ID, BrandID: f.brandID,
		CarBrandID: i8(f.carBrand), CarModelID: i8(f.sedan),
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	f.category, err = q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: f.centerID, BrandID: f.brandID, Name: f.prefix + "-films",
		AvailableParts: []byte(`["hood","roof"]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	f.film = f.product(t, "film")
	f.otherFilm = f.product(t, "other")
	return f
}

func (f *fixture) org(t *testing.T, name string, parent int64) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, name, f.seq), Name: f.prefix + " " + name,
		Status: "active", Type: "dealer", ParentID: i8(parent), BrandID: f.brandID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *fixture) user(t *testing.T, name string) db.User {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: name, Surname: "T487", Status: "active",
		Email: text(fmt.Sprintf("%s-%s-%d@example.test", f.prefix, name, f.seq)),
	})
	if err != nil {
		t.Fatalf("user %s: %v", name, err)
	}
	return u
}

func (f *fixture) carCatalog(t *testing.T) (int64, int64, int64) {
	t.Helper()
	var brandID, sedanID, suvID int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, f.prefix+" brand").Scan(&brandID); err != nil {
		t.Fatalf("car brand: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name, body_type) VALUES ($1, $2, 'sedan') RETURNING id`, brandID, f.prefix+" sedan").Scan(&sedanID); err != nil {
		t.Fatalf("sedan model: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name, body_type) VALUES ($1, $2, 'suv') RETURNING id`, brandID, f.prefix+" suv").Scan(&suvID); err != nil {
		t.Fatalf("suv model: %v", err)
	}
	return brandID, sedanID, suvID
}

func (f *fixture) product(t *testing.T, sku string) db.Product {
	t.Helper()
	f.seq++
	p, err := f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: f.centerID, BrandID: f.brandID, CategoryID: f.category.ID,
		Sku: sku + "-" + f.prefix, Name: sku + " " + f.prefix,
		Images: []byte("[]"), UnitType: "roll_meter", Active: true,
	})
	if err != nil {
		t.Fatalf("product %s: %v", sku, err)
	}
	return p
}

func (f *fixture) unit(t *testing.T, p db.Product, meters string) db.Unit {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
		OrganizationID: f.centerID, BrandID: f.brandID, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T487-%d-%d", time.Now().UnixNano(), f.seq),
		UnitKind: "serial", Source: "generated", Status: "available",
		InitialMeters: numeric(t, meters), RemainingMeters: numeric(t, meters),
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	return u
}

func (f *fixture) expectation(t *testing.T, productID, categoryID *int64, body, part, meters string) db.PartConsumptionExpectation {
	t.Helper()
	row, err := f.q.CreatePartConsumptionExpectation(f.ctx, db.CreatePartConsumptionExpectationParams{
		OrganizationID: f.centerID, BrandID: f.brandID,
		ProductID: int8N(productID), CategoryID: int8N(categoryID), BodyType: text(body),
		PartKey: part, ExpectedMeters: numeric(t, meters), Source: "manual", SampleSize: 1,
	})
	if err != nil {
		t.Fatalf("expectation %s/%s: %v", part, meters, err)
	}
	return row
}

func (f *fixture) service(t *testing.T, modelID int64) db.Service {
	t.Helper()
	f.seq++
	svc, err := f.q.CreateService(f.ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T487-%d-%d", time.Now().UnixNano(), f.seq),
		OrganizationID: f.dealer.ID, BrandID: f.brandID, CustomerUserID: f.customer.ID,
		VehicleID: f.vehicle.ID, CarBrandID: f.carBrand, CarModelID: modelID,
		Status: "draft", CreatedByUserID: i8(f.staff.ID), PerformedByUserID: i8(f.staff.ID),
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return svc
}

func (f *fixture) serviceItem(t *testing.T, p db.Product, unit db.Unit, parts string, meters string) db.ServiceItem {
	t.Helper()
	item, err := f.q.CreateServiceItem(f.ctx, db.CreateServiceItemParams{
		ServiceID: f.service(t, f.sedan).ID, ProductID: p.ID, UnitID: unit.ID,
		Kind: "partial", Meters: numeric(t, meters), AppliedParts: []byte(parts),
	})
	if err != nil {
		t.Fatalf("service item: %v", err)
	}
	return item
}

func TestExpectationPriority(t *testing.T) {
	f := newFixture(t)
	catID := f.category.ID
	productID := f.film.ID

	f.expectation(t, nil, &catID, "", "hood", "2.00")
	f.expectation(t, nil, &catID, "sedan", "hood", "3.00")
	f.expectation(t, &productID, nil, "", "hood", "4.00")
	f.expectation(t, &productID, nil, "sedan", "hood", "5.00")

	cases := []struct {
		name    string
		product int64
		body    string
		want    string
	}{
		{"product body", f.film.ID, "sedan", "5.00"},
		{"product all bodies", f.film.ID, "suv", "4.00"},
		{"category body", f.otherFilm.ID, "sedan", "3.00"},
		{"category all bodies", f.otherFilm.ID, "suv", "2.00"},
	}
	for _, c := range cases {
		got, err := f.store.BestExpectation(f.ctx, db.GetBestPartExpectationParams{
			ProductID: c.product, BrandID: f.brandID, PartKey: "hood", BodyType: text(c.body),
		})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if ntext(got.ExpectedMeters) != c.want {
			t.Fatalf("%s: expected_meters = %s, want %s", c.name, ntext(got.ExpectedMeters), c.want)
		}
	}
}

func TestRefreshFactsAllocatesByExpectedRatio(t *testing.T) {
	f := newFixture(t)
	productID := f.film.ID
	f.expectation(t, &productID, nil, "sedan", "hood", "4.00")
	f.expectation(t, &productID, nil, "sedan", "roof", "2.00")

	unit := f.unit(t, f.film, "20.00")
	item := f.serviceItem(t, f.film, unit, `["hood","roof"]`, "12.00")
	if err := f.store.RefreshServiceItem(f.ctx, item.ID); err != nil {
		t.Fatalf("refresh facts: %v", err)
	}
	rows, err := f.tx.Query(f.ctx, `SELECT part_key, actual_meters, expected_meters, waste_ratio FROM efficiency_facts WHERE service_item_id = $1 ORDER BY part_key`, item.ID)
	if err != nil {
		t.Fatalf("facts query: %v", err)
	}
	defer rows.Close()
	got := map[string][]string{}
	for rows.Next() {
		var part string
		var actual, expected, waste pgtype.Numeric
		if err := rows.Scan(&part, &actual, &expected, &waste); err != nil {
			t.Fatal(err)
		}
		got[part] = []string{ntext(actual), ntext(expected), ntext(waste)}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got["hood"]) != "[8.00 4.00 1.000000]" || fmt.Sprint(got["roof"]) != "[4.00 2.00 1.000000]" {
		t.Fatalf("facts = %#v", got)
	}
}

func TestRollSortTiebreak(t *testing.T) {
	f := newFixture(t)
	u1 := f.unit(t, f.film, "20.00")
	u2 := f.unit(t, f.film, "20.00")
	insert := func(u db.Unit, consumed, expected string) {
		t.Helper()
		if _, err := f.tx.Exec(f.ctx, `
			INSERT INTO roll_efficiency (
				organization_id, brand_id, unit_id, product_id, initial_meters,
				consumed_meters, expected_meters, remaining_meters, service_count, last_used_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1,NOW())`,
			f.centerID, f.brandID, u.ID, f.film.ID, numeric(t, "20.00"),
			numeric(t, consumed), numeric(t, expected), numeric(t, "8.00")); err != nil {
			t.Fatalf("insert roll: %v", err)
		}
	}
	insert(u1, "12.00", "10.00")
	insert(u2, "12.00", "10.00")

	rows, err := f.store.ListRolls(f.ctx, db.ListRollEfficiencyParams{BrandID: f.brandID, RowLimit: 10}, []apiquery.SortField{{Field: "waste_ratio", Desc: true}})
	if err != nil {
		t.Fatalf("list rolls: %v", err)
	}
	if len(rows) < 2 || rows[0].UnitID != u2.ID || rows[1].UnitID != u1.ID {
		t.Fatalf("roll order unit ids = %v", unitIDs(rows))
	}
}

func unitIDs(rows []db.ListRollEfficiencyRow) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.UnitID)
	}
	return out
}

func text(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: v != ""}
}

func i8(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

func int8N(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return i8(*v)
}

func numeric(t *testing.T, raw string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(raw); err != nil {
		t.Fatalf("numeric %s: %v", raw, err)
	}
	return n
}

func ntext(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return ""
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp > 0 {
		r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)))
	} else if n.Exp < 0 {
		r.Quo(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil)))
	}
	decimals := 2
	if n.Exp < 0 {
		decimals = int(math.Min(float64(-n.Exp), 6))
	}
	return r.FloatString(decimals)
}
