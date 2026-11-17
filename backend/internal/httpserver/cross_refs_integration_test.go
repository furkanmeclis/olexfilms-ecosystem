package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/google/uuid"
)

// TEC-1216 (cross-record refs, backend contract): the reverse relation
// filters of the list endpoints (RelatedList on the web and mobile detail
// pages) select the related rows and never leave the caller's scope; the
// organization and staff user summaries answer inside the caller's
// organizations.read / members.read scope only.

type crossRefNet struct {
	it                   *itest
	center, dist, dA, dB db.Organization
	tokA, tokB, tokD     string
	tokCenter            string
	custA, custB         db.User
	vehA, vehB           db.Vehicle
	staffA1, staffA2     db.User
	productP, productQ   db.Product
	svcA, svcA2, svcB    string // service uuids
	warA, warB           string
	orderA, orderB       string
	leadA, leadB         string
	apptA, apptB         string
	measA, measB         string
	contractA            string
	taskA, taskB         string
	centerStaff          db.User
}

func (n *crossRefNet) one(sql string, args ...any) int64 {
	n.it.t.Helper()
	var id int64
	if err := n.it.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		n.it.t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

func (n *crossRefNet) uuidOf(table string, id int64) string {
	n.it.t.Helper()
	var out string
	if err := n.it.pool.QueryRow(context.Background(), "SELECT uuid::text FROM "+table+" WHERE id = $1", id).Scan(&out); err != nil {
		n.it.t.Fatalf("uuid of %s: %v", table, err)
	}
	return out
}

func newCrossRefNet(t *testing.T) *crossRefNet {
	it := newIntegration(t)
	n := &crossRefNet{it: it}
	n.center = it.brandCenter("olex")
	n.dist = it.org("t1216-dist", "distributor", n.center)
	n.dA = it.org("t1216-a", "dealer", n.dist)
	n.dB = it.org("t1216-b", "dealer", n.dist)
	for _, org := range []db.Organization{n.dist, n.dA, n.dB} {
		for _, key := range []string{features.ModuleLeads, features.ModuleAppointments, features.ModuleMeasurements,
			features.ModuleIntakeContracts} {
			it.setOrgModule(org, key, true)
		}
	}
	ownerA, pwA := it.user("t1216-owner-a")
	it.member(n.dA, ownerA, "owner")
	ownerB, pwB := it.user("t1216-owner-b")
	it.member(n.dB, ownerB, "owner")
	ownerD, pwD := it.user("t1216-owner-d")
	it.member(n.dist, ownerD, "owner")
	centerOwner, pwC := it.user("t1216-center")
	it.member(n.center, centerOwner, "owner")
	n.centerStaff, _ = it.user("t1216-center-staff")
	it.member(n.center, n.centerStaff, "staff")
	n.staffA1, _ = it.user("t1216-staff-a1")
	it.member(n.dA, n.staffA1, "staff")
	n.staffA2, _ = it.user("t1216-staff-a2")
	it.member(n.dA, n.staffA2, "staff")

	sfx := it.suffix[len(it.suffix)-4:]
	n.custA, n.vehA = it.svcCustomer(n.dA, "t1216-cust-a", "34X"+sfx)
	n.custB, n.vehB = it.svcCustomer(n.dB, "t1216-cust-b", "06X"+sfx)
	n.productP = it.product(n.center, "T1216P-"+it.suffix)
	n.productQ = it.product(n.center, "T1216Q-"+it.suffix)

	ctx := context.Background()
	var cleanup []string
	t.Cleanup(func() {
		for i := len(cleanup) - 1; i >= 0; i-- {
			_, _ = it.pool.Exec(context.Background(), cleanup[i])
		}
	})
	del := func(sql string, args ...any) {
		cleanup = append(cleanup, fmt.Sprintf(sql, args...))
	}

	// Services: A1 by staffA1 performed by staffA2 with product P; A2 by
	// staffA2 with product Q; B (dealer B) with product P.
	service := func(org db.Organization, cust db.User, veh db.Vehicle, createdBy, performedBy int64, product db.Product, no string) (int64, int64, int64) {
		id := n.one(`INSERT INTO services (service_no, organization_id, brand_id, customer_user_id, vehicle_id,
			car_brand_id, car_model_id, status, created_by_user_id, performed_by_user_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'draft', $8, NULLIF($9, 0)) RETURNING id`,
			no+"-"+it.suffix, org.ID, org.BrandID, cust.ID, veh.ID, veh.CarBrandID, veh.CarModelID, createdBy, performedBy)
		unit := n.one(`INSERT INTO units (organization_id, brand_id, product_id, barcode, status)
			VALUES ($1, $2, $3, $4, 'used') RETURNING id`, n.center.ID, n.center.BrandID, product.ID, "T1216-"+no+"-"+it.suffix)
		item := n.one(`INSERT INTO service_items (service_id, organization_id, brand_id, product_id, unit_id, kind)
			VALUES ($1, $2, $3, $4, $5, 'full') RETURNING id`, id, org.ID, org.BrandID, product.ID, unit)
		if _, err := it.pool.Exec(ctx, `UPDATE services SET status = 'completed', completed_at = NOW() WHERE id = $1`, id); err != nil {
			t.Fatalf("complete service: %v", err)
		}
		del("DELETE FROM warranties WHERE service_id = %d", id)
		del("DELETE FROM service_items WHERE service_id = %d", id)
		del("DELETE FROM contract_instances WHERE subject_type = 'service' AND subject_id = %d", id)
		del("DELETE FROM services WHERE id = %d", id)
		del("DELETE FROM units WHERE id = %d", unit)
		return id, item, unit
	}
	sA, itemA, unitA := service(n.dA, n.custA, n.vehA, n.staffA1.ID, n.staffA2.ID, n.productP, "A1")
	sA2, _, _ := service(n.dA, n.custA, n.vehA, n.staffA2.ID, 0, n.productQ, "A2")
	sB, itemB, unitB := service(n.dB, n.custB, n.vehB, ownerB.ID, 0, n.productP, "B")
	n.svcA, n.svcA2, n.svcB = n.uuidOf("services", sA), n.uuidOf("services", sA2), n.uuidOf("services", sB)

	now := time.Now().UTC()
	warranty := func(org db.Organization, svc, item, unit int64, product db.Product, veh db.Vehicle, cust db.User) string {
		id := n.one(`INSERT INTO warranties (organization_id, brand_id, service_id, service_item_id, product_id, unit_id,
			item_kind, vehicle_id, holder_user_id, start_at, end_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'full', $7, $8, $9, $10) RETURNING id`,
			org.ID, org.BrandID, svc, item, product.ID, unit, veh.ID, cust.ID, now.AddDate(0, 0, -1), now.AddDate(1, 0, 0))
		return n.uuidOf("warranties", id)
	}
	n.warA = warranty(n.dA, sA, itemA, unitA, n.productP, n.vehA, n.custA)
	n.warB = warranty(n.dB, sB, itemB, unitB, n.productP, n.vehB, n.custB)

	order := func(buyer db.Organization, createdBy int64, product db.Product) string {
		id := n.one(`INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
			rate_snapshot, try_rate, subtotal, total, created_by_user_id, approved_at)
			VALUES ($1, $2, $1, $3, 'approved', 'TRY', '{}', 1, 100, 100, $4, NOW()) RETURNING id`,
			n.dist.ID, n.dist.BrandID, buyer.ID, createdBy)
		if _, err := it.pool.Exec(ctx, `INSERT INTO order_items (order_id, organization_id, brand_id, product_id, quantity,
			unit_price, price_source, line_total) VALUES ($1, $2, $3, $4, 1, 100, 'list', 100)`,
			id, n.dist.ID, n.dist.BrandID, product.ID); err != nil {
			t.Fatalf("order item: %v", err)
		}
		del("DELETE FROM order_items WHERE order_id = %d", id)
		del("DELETE FROM orders WHERE id = %d", id)
		return n.uuidOf("orders", id)
	}
	n.orderA = order(n.dA, n.staffA1.ID, n.productP)
	n.orderB = order(n.dB, ownerB.ID, n.productQ)

	lead := func(org db.Organization, cust db.User) string {
		id := n.one(`INSERT INTO leads (organization_id, brand_id, target_type, source, customer_user_id)
			VALUES ($1, $2, 'customer', 'walk_in', $3) RETURNING id`, org.ID, org.BrandID, cust.ID)
		del("DELETE FROM leads WHERE id = %d", id)
		return n.uuidOf("leads", id)
	}
	n.leadA = lead(n.dA, n.custA)
	n.leadB = lead(n.dB, n.custB)

	appt := func(org db.Organization, cust db.User, veh db.Vehicle) string {
		start := now.Add(48 * time.Hour)
		id := n.one(`INSERT INTO appointments (organization_id, brand_id, customer_user_id, vehicle_id, starts_at, ends_at,
			estimated_minutes, source) VALUES ($1, $2, $3, $4, $5, $6, 60, 'panel') RETURNING id`,
			org.ID, org.BrandID, cust.ID, veh.ID, start, start.Add(time.Hour))
		del("DELETE FROM appointments WHERE id = %d", id)
		return n.uuidOf("appointments", id)
	}
	n.apptA = appt(n.dA, n.custA, n.vehA)
	n.apptB = appt(n.dB, n.custB, n.vehB)

	meas := func(org db.Organization, veh db.Vehicle, vin string) string {
		id := n.one(`INSERT INTO measurement_results (organization_id, brand_id, vehicle_id, vin, status, raw, created_by)
			VALUES ($1, $2, $3, $4, 'accepted', '{}', $5) RETURNING id`, org.ID, org.BrandID, veh.ID, vin, n.staffA1.ID)
		del("DELETE FROM measurement_results WHERE id = %d", id)
		return n.uuidOf("measurement_results", id)
	}
	n.measA = meas(n.dA, n.vehA, "T1216A"+strings.ToUpper(sfx)+"0000001")
	n.measB = meas(n.dB, n.vehB, "T1216B"+strings.ToUpper(sfx)+"0000001")

	tpl := n.one(`INSERT INTO contract_templates (organization_id, brand_id, name, kind)
		VALUES ($1, $2, $3, 'vehicle_intake') RETURNING id`, n.center.ID, n.center.BrandID, "T1216 "+it.suffix)
	del("DELETE FROM contract_templates WHERE id = %d", tpl)
	contract := n.one(`INSERT INTO contract_instances (organization_id, brand_id, contract_no, subject_type,
		subject_id, template_id, kind, locale, template_version, otp_required, signature_required, status)
		VALUES ($1, $2, 1, 'service', $3, $4, 'vehicle_intake', 'tr', 1, false, false, 'pending') RETURNING id`,
		n.dA.ID, n.dA.BrandID, sA, tpl)
	n.contractA = n.uuidOf("contract_instances", contract)

	task := func(subject db.Organization, assignee int64) string {
		id := n.one(`INSERT INTO tasks (organization_id, brand_id, subject_org_id, title, assignee_user_id)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`, n.center.ID, n.center.BrandID, subject.ID, "T1216 "+subject.Name, assignee)
		del("DELETE FROM tasks WHERE id = %d", id)
		return n.uuidOf("tasks", id)
	}
	n.taskA = task(n.dA, n.centerStaff.ID)
	n.taskB = task(n.dB, centerOwner.ID)

	n.tokA = it.loginOrg(ownerA, pwA, n.dA)
	n.tokB = it.loginOrg(ownerB, pwB, n.dB)
	n.tokD = it.loginOrg(ownerD, pwD, n.dist)
	n.tokCenter = it.loginOrg(centerOwner, pwC, n.center)
	return n
}

// ids lists the item uuids of GET path?params (fails on a non-200).
func (n *crossRefNet) ids(tok, path string, params url.Values) []string {
	n.it.t.Helper()
	code, env := n.it.do("GET", path+"?"+params.Encode(), hostOlex, tok, nil)
	if code != http.StatusOK {
		n.it.t.Fatalf("GET %s?%s = %d %s", path, params.Encode(), code, errCode(env))
	}
	var page struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		n.it.t.Fatal(err)
	}
	if int(page.Total) != len(page.Items) {
		n.it.t.Fatalf("GET %s?%s total %d != items %d", path, params.Encode(), page.Total, len(page.Items))
	}
	out := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, it.UUID)
	}
	slices.Sort(out)
	return out
}

func (n *crossRefNet) expect(tok, path string, params url.Values, want ...string) {
	n.it.t.Helper()
	got := n.ids(tok, path, params)
	slices.Sort(want)
	if want == nil {
		want = []string{}
	}
	if !slices.Equal(got, want) {
		n.it.t.Fatalf("GET %s?%s = %v, want %v", path, params.Encode(), got, want)
	}
}

func (n *crossRefNet) expect400(tok, path string, params url.Values, field string) {
	n.it.t.Helper()
	code, env := n.it.do("GET", path+"?"+params.Encode(), hostOlex, tok, nil)
	if code != http.StatusBadRequest {
		n.it.t.Fatalf("GET %s?%s = %d, want 400 (%s)", path, params.Encode(), code, field)
	}
	if errCode(env) != "VALIDATION_ERROR" {
		n.it.t.Fatalf("GET %s?%s error = %s", path, params.Encode(), errCode(env))
	}
}

func qv(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func TestIntegrationCrossRefFilters(t *testing.T) {
	n := newCrossRefNet(t)
	a1, a2 := n.staffA1.Uuid.String(), n.staffA2.Uuid.String()
	custA, custB := n.custA.Uuid.String(), n.custB.Uuid.String()
	pP, pQ := n.productP.Uuid.String(), n.productQ.Uuid.String()

	// Services: customer / vehicle / organization (existing), created_by,
	// performer (performed_by, else created_by) and product.
	n.expect(n.tokD, "/v1/services", qv("customer_uuid", custA), n.svcA, n.svcA2)
	n.expect(n.tokD, "/v1/services", qv("organization_uuid", n.dB.Uuid.String()), n.svcB)
	n.expect(n.tokD, "/v1/services", qv("created_by_uuid", a1), n.svcA)
	n.expect(n.tokD, "/v1/services", qv("created_by_uuid", a2), n.svcA2)
	n.expect(n.tokD, "/v1/services", qv("performer_uuid", a2), n.svcA, n.svcA2)
	n.expect(n.tokD, "/v1/services", qv("performer_uuid", a1))
	n.expect(n.tokD, "/v1/services", qv("product_uuid", pP), n.svcA, n.svcB)
	n.expect(n.tokD, "/v1/services", qv("product_uuid", pP, "customer_uuid", custA), n.svcA)
	n.expect(n.tokD, "/v1/services", qv("product_uuid", uuid.NewString()))
	n.expect400(n.tokD, "/v1/services", qv("performer_uuid", "x"), "performer_uuid")
	// Scope: dealer A asking for dealer B's records gets nothing.
	n.expect(n.tokA, "/v1/services", qv("product_uuid", pP), n.svcA)
	n.expect(n.tokA, "/v1/services", qv("customer_uuid", custB))
	n.expect(n.tokA, "/v1/services", qv("organization_uuid", n.dB.Uuid.String()))

	// Warranties: customer (holder), product, organization.
	n.expect(n.tokD, "/v1/warranties", qv("customer_uuid", custA), n.warA)
	n.expect(n.tokD, "/v1/warranties", qv("product_uuid", pP), n.warA, n.warB)
	n.expect(n.tokD, "/v1/warranties", qv("organization_uuid", n.dB.Uuid.String()), n.warB)
	n.expect(n.tokA, "/v1/warranties", qv("customer_uuid", custB))
	n.expect400(n.tokD, "/v1/warranties", qv("customer_uuid", "x"), "customer_uuid")

	// Orders: organization (seller or buyer), product, created_by.
	n.expect(n.tokD, "/v1/orders", qv("organization_uuid", n.dA.Uuid.String()), n.orderA)
	n.expect(n.tokD, "/v1/orders", qv("organization_uuid", n.dist.Uuid.String()), n.orderA, n.orderB)
	n.expect(n.tokD, "/v1/orders", qv("product_uuid", pQ), n.orderB)
	n.expect(n.tokD, "/v1/orders", qv("created_by_uuid", a1), n.orderA)
	n.expect(n.tokA, "/v1/orders", qv("organization_uuid", n.dB.Uuid.String()))
	n.expect(n.tokA, "/v1/orders", qv("product_uuid", pQ))
	n.expect400(n.tokD, "/v1/orders", qv("product_uuid", "x"), "product_uuid")

	// Appointments (no from/to needed with customer_uuid), leads,
	// measurements and contracts: customer.
	n.expect(n.tokD, "/v1/appointments", qv("customer_uuid", custA), n.apptA)
	n.expect(n.tokA, "/v1/appointments", qv("customer_uuid", custB))
	n.expect400(n.tokD, "/v1/appointments", qv(), "from")
	n.expect(n.tokB, "/v1/leads", qv("customer_uuid", custB), n.leadB) // leads.read is managed
	n.expect(n.tokA, "/v1/leads", qv("customer_uuid", custA), n.leadA)
	n.expect(n.tokA, "/v1/leads", qv("customer_uuid", custB))
	n.expect(n.tokD, "/v1/measurements", qv("customer_uuid", custA), n.measA)
	n.expect(n.tokA, "/v1/measurements", qv("customer_uuid", custB))
	n.expect(n.tokA, "/v1/contracts", qv("customer_uuid", custA), n.contractA)
	n.expect(n.tokB, "/v1/contracts", qv("customer_uuid", custA))
	n.expect(n.tokB, "/v1/contracts", qv())
	n.expect400(n.tokA, "/v1/contracts", qv("status", "nope"), "status")

	// Tasks (center): assignee_uuid / organization_uuid aliases.
	n.expect(n.tokCenter, "/v1/tasks", qv("organization_uuid", n.dA.Uuid.String()), n.taskA)
	n.expect(n.tokCenter, "/v1/tasks", qv("assignee_uuid", n.centerStaff.Uuid.String()), n.taskA)
	n.expect(n.tokCenter, "/v1/tasks", qv("organization_uuid", n.dA.Uuid.String()+","+n.dB.Uuid.String()), n.taskA, n.taskB)
}

func TestIntegrationCrossRefSummaries(t *testing.T) {
	n := newCrossRefNet(t)
	it := n.it

	type orgSummary struct {
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
		Ref struct {
			Kind string `json:"kind"`
			UUID string `json:"uuid"`
			Sub  string `json:"sub"`
		} `json:"ref"`
		Parent *struct {
			Kind string `json:"kind"`
			UUID string `json:"uuid"`
		} `json:"parent"`
		Counts map[string]*int64 `json:"counts"`
	}
	path := "/v1/tenant/organizations/" + n.dA.Uuid.String() + "/summary"
	s := decodeData[orgSummary](t, it.accDo("GET", path, n.tokD, nil, http.StatusOK))
	if s.Ref.Kind != "organization" || s.Ref.UUID != n.dA.Uuid.String() || s.Ref.Sub != "dealer" ||
		s.Parent == nil || s.Parent.UUID != n.dist.Uuid.String() {
		t.Fatalf("summary refs = %+v parent %+v", s.Ref, s.Parent)
	}
	if c := s.Counts["services"]; c == nil || *c != 2 {
		t.Fatalf("services count = %v", c)
	}
	if c := s.Counts["orders"]; c == nil || *c != 1 {
		t.Fatalf("orders count = %v", c)
	}
	if c := s.Counts["members"]; c == nil || *c != 3 {
		t.Fatalf("members count = %v", c)
	}
	// Outside the scope: dealer B cannot read dealer A's summary.
	it.accDo("GET", path, n.tokB, nil, http.StatusNotFound)
	it.accDo("GET", "/v1/tenant/organizations/"+uuid.NewString()+"/summary", n.tokD, nil, http.StatusNotFound)

	type memberSummary struct {
		User struct {
			UUID string `json:"uuid"`
		} `json:"user"`
		Ref struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"ref"`
		Memberships []struct {
			Organization struct {
				Kind string `json:"kind"`
				UUID string `json:"uuid"`
			} `json:"organization"`
			Role string `json:"role"`
		} `json:"memberships"`
		Counts map[string]*int64 `json:"counts"`
	}
	upath := "/v1/tenant/users/" + n.staffA2.Uuid.String() + "/summary"
	m := decodeData[memberSummary](t, it.accDo("GET", upath, n.tokA, nil, http.StatusOK))
	if m.User.UUID != n.staffA2.Uuid.String() || m.Ref.Kind != "user" || len(m.Memberships) != 1 ||
		m.Memberships[0].Organization.UUID != n.dA.Uuid.String() || m.Memberships[0].Role != "staff" {
		t.Fatalf("member summary = %+v", m)
	}
	if c := m.Counts["services_performed"]; c == nil || *c != 2 {
		t.Fatalf("services_performed = %v", c)
	}
	if c := m.Counts["services_created"]; c == nil || *c != 1 {
		t.Fatalf("services_created = %v", c)
	}
	// Dealer B does not see dealer A's staff; a customer account is not a
	// member.
	it.accDo("GET", upath, n.tokB, nil, http.StatusNotFound)
	it.accDo("GET", "/v1/tenant/users/"+n.custA.Uuid.String()+"/summary", n.tokA, nil, http.StatusNotFound)
	// The distributor sees the staff of its dealers.
	it.accDo("GET", upath, n.tokD, nil, http.StatusOK)
}

// TEC-1216 (PR-2): detail responses carry refs for every foreign key and no
// longer expose the internal ids that had no client.
func TestIntegrationCrossRefDetailRefs(t *testing.T) {
	n := newCrossRefNet(t)
	it := n.it
	type ref struct {
		Kind string `json:"kind"`
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}
	check := func(what string, got *ref, kind, id string) {
		t.Helper()
		if got == nil || got.Kind != kind || got.UUID != id || got.Name == "" {
			t.Fatalf("%s ref = %+v, want %s %s", what, got, kind, id)
		}
	}
	raw := func(path, tok string) map[string]json.RawMessage {
		t.Helper()
		code, env := it.do("GET", path, hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, code, errCode(env))
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(env.Data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	refOf := func(m map[string]json.RawMessage, key string) *ref {
		t.Helper()
		var r *ref
		if v, ok := m[key]; ok {
			if err := json.Unmarshal(v, &r); err != nil {
				t.Fatalf("%s: %v", key, err)
			}
		}
		return r
	}

	svc := raw("/v1/services/"+n.svcA, n.tokA)
	check("service.created_by", refOf(svc, "created_by"), "user", n.staffA1.Uuid.String())
	check("service.performed_by", refOf(svc, "performed_by"), "user", n.staffA2.Uuid.String())
	check("service.vehicle", refOf(svc, "vehicle"), "vehicle", n.vehA.Uuid.String())
	svc2 := raw("/v1/services/"+n.svcA2, n.tokA)
	check("service2.performed_by (fallback)", refOf(svc2, "performed_by"), "user", n.staffA2.Uuid.String())

	veh := raw("/v1/vehicles/"+n.vehA.Uuid.String(), n.tokA)
	check("vehicle.customer", refOf(veh, "customer"), "customer", n.custA.Uuid.String())

	lead := raw("/v1/leads/"+n.leadA, n.tokA)
	check("lead.customer", refOf(lead, "customer"), "customer", n.custA.Uuid.String())
	for _, gone := range []string{"won_ref_id", "created_by_user_id"} {
		if _, ok := lead[gone]; ok {
			t.Fatalf("lead still exposes %s", gone)
		}
	}

	appt := raw("/v1/appointments/"+n.apptA, n.tokA)
	check("appointment.customer", refOf(appt, "customer"), "customer", n.custA.Uuid.String())
	check("appointment.vehicle", refOf(appt, "vehicle"), "vehicle", n.vehA.Uuid.String())
	check("appointment.organization", refOf(appt, "organization"), "organization", n.dA.Uuid.String())
	for _, gone := range []string{"lead_id", "created_by_user_id", "plan_id"} {
		if _, ok := appt[gone]; ok {
			t.Fatalf("appointment still exposes %s", gone)
		}
	}

	contract := raw("/v1/contracts/"+n.contractA, n.tokA)
	check("contract.subject", refOf(contract, "subject"), "service", n.svcA)

	// Leads: assignee_uuid filter (uuid form of the deprecated
	// assignee_user_id).
	it.exec("UPDATE leads SET assignee_user_id = $1 WHERE uuid = $2", n.staffA1.ID, n.leadA)
	n.expect(n.tokA, "/v1/leads", qv("assignee_uuid", n.staffA1.Uuid.String()), n.leadA)
	n.expect(n.tokA, "/v1/leads", qv("assignee_uuid", uuid.NewString()))
	n.expect(n.tokA, "/v1/leads", qv("assignee_uuid", "none"))
	n.expect400(n.tokA, "/v1/leads", qv("assignee_uuid", "x"), "assignee_uuid")
	lead = raw("/v1/leads/"+n.leadA, n.tokA)
	check("lead.assignee", refOf(lead, "assignee"), "user", n.staffA1.Uuid.String())
}
