package usecase

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

func rowFor(env Envelope, org uuid.UUID) map[string]any {
	for _, r := range env.Rows {
		if r["organization_uuid"] == org {
			return r
		}
	}
	return nil
}

func hasColumn(env Envelope, key string) bool {
	for _, c := range env.Columns {
		if c.Key == key {
			return true
		}
	}
	return false
}

// TestNetworkReportsTEC1090: orders.by_buyer (counted orders, amounts only
// with a pricing read permission), orders.inactive_buyers (period and
// all_time) and services.by_org (the center's service point row).
func TestNetworkReportsTEC1090(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	// A cancelled order does not count.
	f.exec(t, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
 rate_snapshot, try_rate, subtotal, total, created_at, cancelled_at, approved_at)
VALUES ($1, $2, $1, $3, 'cancelled', 'TRY', '{}', 1, 999, 999, $4, $4, $4)`, f.distA.ID, f.brandID, f.dealer1.ID, testNow.AddDate(0, 0, -1))
	// dealer4 never ordered, dealer5 ordered 100 days ago, dealer6 is suspended.
	dealer4 := f.org(t, "dealer", "dealer-4", f.distA.ID)
	dealer5 := f.org(t, "dealer", "dealer-5", f.distA.ID)
	dealer6 := f.org(t, "dealer", "dealer-6", f.distA.ID)
	f.exec(t, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, dealer6.ID)
	f.exec(t, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
 rate_snapshot, try_rate, subtotal, total, created_at, approved_at)
VALUES ($1, $2, $1, $3, 'received', 'TRY', '{}', 1, 50, 50, $4, $4)`, f.distA.ID, f.brandID, dealer5.ID, testNow.AddDate(0, 0, -100))

	staff := f.caller(t, f.center, rbac.RoleCenterStaff)
	accounting := f.caller(t, f.center, rbac.RoleCenterAccounting)
	in := in30
	in.Limit = "50"

	// orders.by_buyer: dealer3 (3 orders) first; total hidden without pricing.
	env, err := s.Report(f.ctx, staff, ReportOrdersByBuyer, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Rows) != 3 || env.Rows[0]["organization_uuid"] != f.dealer3.Uuid || env.Rows[0]["order_count"] != int64(3) {
		t.Fatalf("by_buyer rows = %v", env.Rows)
	}
	if r := rowFor(env, f.dealer1.Uuid); r == nil || r["order_count"] != int64(1) || r["total"] != nil || hasColumn(env, "total") {
		t.Fatalf("by_buyer without pricing = %v %v", r, env.Columns)
	}
	env, err = s.Report(f.ctx, accounting, ReportOrdersByBuyer, in)
	if err != nil {
		t.Fatal(err)
	}
	if r := rowFor(env, f.dealer2.Uuid); r == nil || r["total"] != "200.00" || r["currency"] != "TRY" || !hasColumn(env, "total") {
		t.Fatalf("by_buyer with pricing = %v", r)
	}

	// orders.inactive_buyers, 30 days: dealer4 (never) and dealer5 (old).
	env, err = s.Report(f.ctx, staff, ReportOrdersInactiveBuyer, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Rows) != 2 || env.Rows[0]["organization_uuid"] != dealer4.Uuid || env.Rows[0]["last_order_at"] != nil ||
		env.Rows[1]["organization_uuid"] != dealer5.Uuid || env.Rows[1]["last_order_at"] == nil || card(env, "inactive_dealers") != 2 {
		t.Fatalf("inactive 30d = %v", env.Rows)
	}
	if rowFor(env, dealer6.Uuid) != nil {
		t.Fatal("suspended dealer listed")
	}
	all := in
	all.AllTime = "true"
	env, err = s.Report(f.ctx, staff, ReportOrdersInactiveBuyer, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Rows) != 1 || env.Rows[0]["organization_uuid"] != dealer4.Uuid || !env.AllTime || env.Period != nil {
		t.Fatalf("inactive all_time = %v period=%v", env.Rows, env.Period)
	}
	// The distributor B subtree has no inactive dealer.
	envB, err := s.Report(f.ctx, f.caller(t, f.distB, rbac.RoleDistributorOwner), ReportOrdersInactiveBuyer, in)
	if err != nil || len(envB.Rows) != 0 {
		t.Fatalf("distB inactive = %v %v", envB.Rows, err)
	}
	// all_time is only for inactive_buyers and excludes a period.
	bad := all
	bad.Period = "7d"
	var ve *ValidationError
	if _, err := s.Report(f.ctx, staff, ReportOrdersInactiveBuyer, bad); !errors.As(err, &ve) || ve.Field != "all_time" {
		t.Fatalf("all_time + period err = %v", err)
	}
	if _, err := s.Report(f.ctx, staff, ReportOrdersByBuyer, all); !errors.As(err, &ve) || ve.Field != "all_time" {
		t.Fatalf("all_time on by_buyer err = %v", err)
	}

	// services.by_org: the center's own services are the service point row.
	f.exec(t, `UPDATE organizations SET service_point_enabled = true, service_point_name = 'AFM Florya' WHERE id = $1`, f.center.ID)
	customer := f.id(t, `INSERT INTO users (password_hash, name, surname, status, email)
VALUES ('x', 'c', 'T1090', 'active', $1) RETURNING id`, f.prefix+"-sp@example.test")
	vehicle := f.id(t, `INSERT INTO vehicles (user_id, organization_id, brand_id, car_brand_id, car_model_id, vin)
VALUES ($1, $2, $3, $4, $5, 'A1090ABCDEFG00001') RETURNING id`, customer, f.center.ID, f.brandID, f.carB, f.carM)
	at := testNow.AddDate(0, 0, -2)
	f.exec(t, `INSERT INTO services (service_no, organization_id, brand_id, customer_user_id, vehicle_id,
 car_brand_id, car_model_id, status, created_at, completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'completed', $8, $8)`, "S-"+f.prefix+"-sp", f.center.ID, f.brandID,
		customer, vehicle, f.carB, f.carM, at)
	env, err = s.Report(f.ctx, staff, ReportServicesByOrg, in)
	if err != nil {
		t.Fatal(err)
	}
	r := rowFor(env, f.center.Uuid)
	if r == nil || r["organization"] != "AFM Florya" || r["is_service_point"] != true || r["services_count"] != int64(1) {
		t.Fatalf("center row = %v", r)
	}
	if d := rowFor(env, f.dealer3.Uuid); d == nil || d["services_count"] != int64(3) || d["is_service_point"] != false {
		t.Fatalf("dealer3 row = %v", d)
	}
}
