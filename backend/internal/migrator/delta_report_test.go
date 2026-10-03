package migrator

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// deltaFixture builds its rows in a transaction that is rolled back.
type deltaFixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	center   db.Organization
	carBrand int64
	carModel int64
	seq      int
}

func newDeltaFixture(t *testing.T) *deltaFixture {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
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
	f := &deltaFixture{ctx: ctx, tx: tx, q: q, center: center}
	suffix := fmt.Sprint(time.Now().UnixNano())
	if err := tx.QueryRow(ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "T276 "+suffix).Scan(&f.carBrand); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, 'T276 Model') RETURNING id`, f.carBrand).Scan(&f.carModel); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *deltaFixture) user(t *testing.T) db.User {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Musteri", Surname: "T276", Status: "active",
		Email: pgtype.Text{String: fmt.Sprintf("t276-%d-%d@example.test", time.Now().UnixNano(), f.seq), Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return u
}

func (f *deltaFixture) service(t *testing.T, customer db.User) db.Service {
	t.Helper()
	v, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: customer.ID, BrandID: f.center.BrandID,
		CarBrandID: pgtype.Int8{Int64: f.carBrand, Valid: true},
		CarModelID: pgtype.Int8{Int64: f.carModel, Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	f.seq++
	s, err := f.q.CreateService(f.ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T276-%d-%d", time.Now().UnixNano(), f.seq),
		OrganizationID: f.center.ID, BrandID: f.center.BrandID,
		CustomerUserID: customer.ID, VehicleID: v.ID,
		CarBrandID: f.carBrand, CarModelID: f.carModel, Status: "draft",
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return s
}

func (f *deltaFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// mapTo records a migration_map row whose target is (table, target).
func (f *deltaFixture) mapTo(t *testing.T, table string, target uuid.UUID, migratedAt time.Time) {
	t.Helper()
	f.seq++
	f.exec(t, `INSERT INTO migration_map (source_system, source_table, source_id, target_table, target_uuid, migrated_at)
		VALUES ('t276', $1, $2, $1, $3, $4)`,
		table, fmt.Sprintf("%d-%d", time.Now().UnixNano(), f.seq), target, migratedAt)
}

func TestDeltaReport(t *testing.T) {
	f := newDeltaFixture(t)
	ctx := f.ctx
	now := time.Now()
	since := now.Add(-time.Hour)

	// A service created after the cutover in this application: listed.
	customer := f.user(t)
	fresh := f.service(t, customer)

	// A record older than since: not listed.
	old := f.service(t, customer)
	f.exec(t, `UPDATE services SET created_at = $2 WHERE id = $1`, old.ID, now.Add(-2*time.Hour))

	// A migrator record (a migration_map target), even with a legacy
	// created_at after since: not listed.
	migrated := f.service(t, customer)
	f.mapTo(t, "services", migrated.Uuid, now)

	// A migrated customer: the user is a map target and its legacy link,
	// written by the migrator, is older than the mapping: neither listed.
	// A link created in this application after the mapping: listed.
	legacy := f.user(t)
	f.mapTo(t, "users", legacy.Uuid, now.Add(-50*time.Minute))
	f.exec(t, `INSERT INTO customer_organizations (user_id, organization_id, brand_id, created_at)
		VALUES ($1, $2, $3, $4)`, legacy.ID, f.center.ID, f.center.BrandID, now.Add(-55*time.Minute))
	appLinked := f.user(t)
	f.mapTo(t, "users", appLinked.Uuid, now.Add(-50*time.Minute))
	f.exec(t, `INSERT INTO customer_organizations (user_id, organization_id, brand_id, created_at)
		VALUES ($1, $2, $3, $4)`, appLinked.ID, f.center.ID, f.center.BrandID, now)

	rows, err := DeltaReport(ctx, f.tx, since)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]DeltaRow{}
	for _, r := range rows {
		if r.CreatedAt.Before(since) {
			t.Errorf("row before since listed: %+v", r)
		}
		got[r.Table+"/"+r.UUID.String()] = r
	}

	s, ok := got["services/"+fresh.Uuid.String()]
	if !ok {
		t.Fatalf("new service %s not listed", fresh.Uuid)
	}
	if s.Organization != f.center.Name || s.Summary == "" {
		t.Errorf("new service row = %+v", s)
	}
	if _, ok := got["services/"+old.Uuid.String()]; ok {
		t.Error("service created before since is listed")
	}
	if _, ok := got["services/"+migrated.Uuid.String()]; ok {
		t.Error("migrated service (migration_map target) is listed")
	}
	if _, ok := got["users/"+customer.Uuid.String()]; !ok {
		t.Error("new customer user not listed")
	}
	if _, ok := got["users/"+legacy.Uuid.String()]; ok {
		t.Error("migrated user is listed")
	}
	if _, ok := got["customer_organizations/"+legacy.Uuid.String()]; ok {
		t.Error("migrator customer link is listed")
	}
	if _, ok := got["customer_organizations/"+appLinked.Uuid.String()]; !ok {
		t.Error("customer link created after the mapping is not listed")
	}
	if _, ok := got["vehicles/"+fresh.Uuid.String()]; ok {
		t.Error("service uuid reported under vehicles")
	}

	// The CSV header row is fixed.
	var buf bytes.Buffer
	if err := WriteDeltaCSV(&buf, rows); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != len(rows)+1 || fmt.Sprint(recs[0]) != "[table uuid organization created_at summary]" {
		t.Fatalf("csv header = %v (rows %d, records %d)", recs[0], len(rows), len(recs))
	}
}

// The report reads only: it runs inside a READ ONLY transaction.
func TestDeltaReportReadOnly(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := DeltaReport(ctx, tx, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatalf("report in a read only transaction: %v", err)
	}
}
