package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type serviceView struct {
	UUID         string `json:"uuid"`
	ServiceNo    string `json:"service_no"`
	Status       string `json:"status"`
	StatusLabel  string `json:"status_label"`
	Plate        string `json:"plate"`
	VehicleUUID  string `json:"vehicle_uuid"`
	CancelReason string `json:"cancel_reason"`
	Organization struct {
		UUID string `json:"uuid"`
	} `json:"organization"`
	Customer struct {
		UUID string `json:"uuid"`
	} `json:"customer"`
	CarBrand struct {
		UUID string `json:"uuid"`
	} `json:"car_brand"`
	ItemsEditable        bool     `json:"items_editable"`
	AvailableTransitions []string `json:"available_transitions"`
	Items                []struct {
		UUID    string `json:"uuid"`
		Barcode string `json:"barcode"`
		Kind    string `json:"kind"`
	} `json:"items"`
	StatusLogs []struct {
		FromStatus          *string `json:"from_status"`
		ToStatus            string  `json:"to_status"`
		ByOtherOrganization bool    `json:"by_other_organization"`
	} `json:"status_logs"`
}

type servicePage struct {
	Items []serviceView `json:"items"`
	Total int64         `json:"total"`
}

func (it *itest) svcCall(method, path, token string, body any, want int) (serviceView, string) {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, token, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
	}
	var v serviceView
	if code < 300 && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &v); err != nil {
			it.t.Fatal(err)
		}
	}
	return v, errCode(env)
}

func (it *itest) svcList(token, query string) servicePage {
	it.t.Helper()
	code, env := it.do("GET", "/v1/services"+query, hostOlex, token, nil)
	if code != http.StatusOK {
		it.t.Fatalf("list = %d %s", code, errCode(env))
	}
	var p servicePage
	if err := json.Unmarshal(env.Data, &p); err != nil {
		it.t.Fatal(err)
	}
	return p
}

func (p servicePage) has(id string) bool {
	for _, s := range p.Items {
		if s.UUID == id {
			return true
		}
	}
	return false
}

// svcCustomer creates a customer linked to org with one vehicle carrying a
// car brand / model and a plate.
func (it *itest) svcCustomer(org db.Organization, name, plate string) (db.User, db.Vehicle) {
	it.t.Helper()
	ctx := context.Background()
	cust, _ := it.user(name)
	if _, err := it.q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
		UserID: cust.ID, OrganizationID: org.ID, BrandID: org.BrandID,
	}); err != nil {
		it.t.Fatalf("link customer: %v", err)
	}
	cb, err := it.q.CreateCarBrand(ctx, db.CreateCarBrandParams{Name: "T179 " + name + " " + it.suffix, ShowName: true, Active: true})
	if err != nil {
		it.t.Fatalf("car brand: %v", err)
	}
	cm, err := it.q.CreateCarModel(ctx, db.CreateCarModelParams{CarBrandID: cb.ID, Name: "Model " + it.suffix, Active: true})
	if err != nil {
		it.t.Fatalf("car model: %v", err)
	}
	v, err := it.q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: cust.ID, OrganizationID: pgtype.Int8{Int64: org.ID, Valid: true}, BrandID: org.BrandID,
		CarBrandID: pgtype.Int8{Int64: cb.ID, Valid: true}, CarModelID: pgtype.Int8{Int64: cm.ID, Valid: true},
		ModelYear: pgtype.Int2{Int16: 2024, Valid: true},
		Plate:     pgtype.Text{String: plate, Valid: true}, PlateNormalized: pgtype.Text{String: plate, Valid: true},
		PlateCountry: pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		it.t.Fatalf("vehicle: %v", err)
	}
	return cust, v
}

// TEC-179 acceptance: a dealer opens a service, adds an item and moves it
// pending -> processing -> ready; another dealer gets 404; the same serial
// unit in two open services is 409; a dealer cannot cancel (403), the
// center can; a read-only dealer (K23, no contract) cannot write.
func TestIntegrationServicesFlow(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t179-dist", "distributor", center)
	dealerA := it.org("t179-dealer-a", "dealer", dist)
	dealerB := it.org("t179-dealer-b", "dealer", dist)
	dealerC := it.org("t179-dealer-c", "dealer", dist)

	staff, spw := it.user("t179-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	ownerA, apw := it.user("t179-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, bpw := it.user("t179-owner-b")
	it.member(dealerB, ownerB, "owner")
	ownerC, cpw := it.user("t179-owner-c")
	it.member(dealerC, ownerC, "owner")

	cust, veh := it.svcCustomer(dealerA, "t179-cust", "34T179"+it.suffix[len(it.suffix)-4:])
	custC, vehC := it.svcCustomer(dealerC, "t179-cust-c", "06T179"+it.suffix[len(it.suffix)-4:])

	// Stock: u1 and u2 reach dealer A, u3 stays at the distributor.
	p := it.product(center, "T179")
	c := it.stockChain()
	cLoc := c.location(center, "C179")
	dLoc := c.location(dist, "D179")
	dealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID}
	units := make([]db.Unit, 3)
	for i := range units {
		units[i] = c.unit(center, p, 1790+i)
		c.post(ledger.TypeEntry, units[i], c.nextRef(), cLoc)
		c.ship(units[i], ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
		if i < 2 {
			c.ship(units[i], ledger.TypeOrderOut, ledger.TypeReceived, dealerA, dealerOrg)
		}
	}

	staffTok := it.loginOrg(staff, spw, center)
	aTok := it.loginOrg(ownerA, apw, dealerA)
	bTok := it.loginOrg(ownerB, bpw, dealerB)
	cTok := it.loginOrg(ownerC, cpw, dealerC)

	// 1. Dealer A opens a draft: vehicle snapshot, DS number.
	body := map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String(), "km": 1200, "package": "Full body"}
	s1, _ := it.svcCall("POST", "/v1/services", aTok, body, http.StatusCreated)
	if s1.Status != "draft" || len(s1.ServiceNo) != 10 || s1.ServiceNo[:2] != "DS" || s1.Plate != veh.Plate.String ||
		s1.Organization.UUID != dealerA.Uuid.String() || s1.Customer.UUID != cust.Uuid.String() ||
		s1.VehicleUUID != veh.Uuid.String() || s1.StatusLabel == "" || len(s1.StatusLogs) != 1 {
		t.Fatalf("created = %+v", s1)
	}
	// Dealer B cannot open a service for dealer A's customer.
	if _, ec := it.svcCall("POST", "/v1/services", bTok, body, http.StatusBadRequest); ec != "VALIDATION_ERROR" {
		t.Fatalf("other dealer create = %s", ec)
	}

	// 2. Items: u1 is added; u3 (still at the distributor) is refused.
	s1, _ = it.svcCall("POST", "/v1/services/"+s1.UUID+"/items", aTok,
		map[string]any{"barcode": units[0].Barcode, "kind": "full"}, http.StatusCreated)
	if len(s1.Items) != 1 || s1.Items[0].Barcode != units[0].Barcode || !s1.ItemsEditable {
		t.Fatalf("items = %+v", s1.Items)
	}
	if _, ec := it.svcCall("POST", "/v1/services/"+s1.UUID+"/items", aTok,
		map[string]any{"barcode": units[2].Barcode}, http.StatusConflict); ec != "SERVICE_UNIT_NOT_AVAILABLE" {
		t.Fatalf("distributor unit = %s", ec)
	}

	// 3. Another dealer gets 404 everywhere.
	it.svcCall("GET", "/v1/services/"+s1.UUID, bTok, nil, http.StatusNotFound)
	it.svcCall("POST", "/v1/services/"+s1.UUID+"/transitions", bTok, map[string]string{"status": "pending"}, http.StatusNotFound)
	it.svcCall("POST", "/v1/services/"+s1.UUID+"/items", bTok, map[string]any{"barcode": units[1].Barcode}, http.StatusNotFound)
	if it.svcList(bTok, "").has(s1.UUID) {
		t.Fatal("dealer B lists dealer A's service")
	}
	// The center (brand scope) sees it.
	it.svcCall("GET", "/v1/services/"+s1.UUID, staffTok, nil, http.StatusOK)

	// 4. The same serial unit in a second open service is 409 (and twice in
	// the same service too).
	s2, _ := it.svcCall("POST", "/v1/services", aTok, body, http.StatusCreated)
	if _, ec := it.svcCall("POST", "/v1/services/"+s2.UUID+"/items", aTok,
		map[string]any{"barcode": units[0].Barcode}, http.StatusConflict); ec != "SERVICE_UNIT_IN_USE" {
		t.Fatalf("double unit = %s", ec)
	}
	if _, ec := it.svcCall("POST", "/v1/services/"+s1.UUID+"/items", aTok,
		map[string]any{"barcode": units[0].Barcode}, http.StatusConflict); ec != "SERVICE_UNIT_IN_USE" {
		t.Fatalf("same unit twice = %s", ec)
	}
	s2, _ = it.svcCall("POST", "/v1/services/"+s2.UUID+"/items", aTok,
		map[string]any{"barcode": units[1].Barcode}, http.StatusCreated)
	s2, _ = it.svcCall("DELETE", "/v1/services/"+s2.UUID+"/items/"+s2.Items[0].UUID, aTok, nil, http.StatusOK)
	if len(s2.Items) != 0 {
		t.Fatalf("item not removed: %+v", s2.Items)
	}

	// 5. Dealer transitions: draft -> pending -> processing -> ready; a
	// skipped step is 409, completion is not available yet.
	tr := func(tok, id, status string, want int) (serviceView, string) {
		return it.svcCall("POST", "/v1/services/"+id+"/transitions", tok, map[string]string{"status": status}, want)
	}
	if _, ec := tr(aTok, s1.UUID, "ready", http.StatusConflict); ec != "SERVICE_INVALID_TRANSITION" {
		t.Fatalf("skip = %s", ec)
	}
	for _, st := range []string{"pending", "processing", "ready"} {
		v, _ := tr(aTok, s1.UUID, st, http.StatusOK)
		if v.Status != st {
			t.Fatalf("status = %s want %s", v.Status, st)
		}
	}
	if _, ec := tr(aTok, s1.UUID, "completed", http.StatusConflict); ec != "SERVICE_COMPLETION_UNAVAILABLE" {
		t.Fatalf("complete = %s", ec)
	}
	if _, ec := it.svcCall("POST", "/v1/services/"+s1.UUID+"/items", aTok,
		map[string]any{"barcode": units[1].Barcode}, http.StatusConflict); ec != "SERVICE_NOT_EDITABLE" {
		t.Fatalf("item on ready = %s", ec)
	}
	ready, _ := it.svcCall("GET", "/v1/services/"+s1.UUID, aTok, nil, http.StatusOK)
	if len(ready.StatusLogs) != 4 || ready.StatusLogs[3].ToStatus != "ready" || ready.ItemsEditable {
		t.Fatalf("ready = %+v", ready)
	}

	// 6. A dealer cannot cancel (403); the center can; a cancelled service
	// is final.
	if _, ec := tr(aTok, s2.UUID, "cancelled", http.StatusForbidden); ec == "" {
		t.Fatal("dealer cancel must be 403")
	}
	cancelled, _ := it.svcCall("POST", "/v1/services/"+s2.UUID+"/transitions", staffTok,
		map[string]string{"status": "cancelled", "note": "duplicate"}, http.StatusOK)
	last := cancelled.StatusLogs[len(cancelled.StatusLogs)-1]
	if cancelled.Status != "cancelled" || cancelled.CancelReason != "duplicate" || !last.ByOtherOrganization {
		t.Fatalf("cancelled = %+v", cancelled)
	}
	if _, ec := tr(staffTok, s2.UUID, "pending", http.StatusConflict); ec != "SERVICE_INVALID_TRANSITION" {
		t.Fatalf("cancelled -> pending = %s", ec)
	}
	// A cancelled service frees its units: u1 is still in s1, u2 is free.
	s3, _ := it.svcCall("POST", "/v1/services", aTok, body, http.StatusCreated)
	it.svcCall("POST", "/v1/services/"+s3.UUID+"/items", aTok, map[string]any{"barcode": units[1].Barcode}, http.StatusCreated)

	// 7. Search by plate, customer name and number; outbox events written.
	if !it.svcList(aTok, "?q="+veh.Plate.String).has(s1.UUID) ||
		!it.svcList(aTok, "?q="+cust.Name).has(s1.UUID) ||
		!it.svcList(aTok, "?q="+s1.ServiceNo).has(s1.UUID) ||
		it.svcList(aTok, "?q="+s1.ServiceNo).has(s2.UUID) {
		t.Fatal("search misses the service")
	}
	var n int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events
		WHERE event_name LIKE 'service.%' AND payload::text LIKE '%' || $1 || '%'`, s1.UUID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 5 { // created, item, pending, processing, ready
		t.Fatalf("outbox events for s1 = %d", n)
	}

	// 8. K23: a read-only dealer (no contract) reads but cannot write.
	if _, err := it.pool.Exec(ctx, "UPDATE organizations SET status = 'read_only' WHERE id = $1", dealerC.ID); err != nil {
		t.Fatal(err)
	}
	cBody := map[string]any{"customer_uuid": custC.Uuid.String(), "vehicle_uuid": vehC.Uuid.String()}
	if _, ec := it.svcCall("POST", "/v1/services", cTok, cBody, http.StatusForbidden); ec != "ORGANIZATION_READ_ONLY" {
		t.Fatalf("read-only create = %s", ec)
	}
	it.svcList(cTok, "")
}
