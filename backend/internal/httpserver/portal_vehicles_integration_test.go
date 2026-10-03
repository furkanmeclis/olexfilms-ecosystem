package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type portalServiceRow struct {
	UUID         string `json:"uuid"`
	ServiceNo    string `json:"service_no"`
	Status       string `json:"status"`
	VehicleUUID  string `json:"vehicle_uuid"`
	Organization struct {
		UUID string `json:"uuid"`
	} `json:"organization"`
}

type portalServicePage struct {
	Items []portalServiceRow `json:"items"`
	Total int64              `json:"total"`
}

type portalVehicleRow struct {
	UUID                string `json:"uuid"`
	ServiceCount        int64  `json:"service_count"`
	ActiveWarrantyCount int64  `json:"active_warranty_count"`
}

type portalVehiclePage struct {
	Items []portalVehicleRow `json:"items"`
	Total int64              `json:"total"`
}

type portalVehicleDetail struct {
	UUID           string `json:"uuid"`
	ServiceSummary struct {
		Total             int64 `json:"total"`
		Completed         int64 `json:"completed"`
		OrganizationCount int64 `json:"organization_count"`
	} `json:"service_summary"`
	Services         []portalServiceRow `json:"services"`
	ActiveWarranties []struct {
		UUID        string `json:"uuid"`
		DaysLeft    int    `json:"days_left"`
		PercentLeft int    `json:"percent_left"`
	} `json:"active_warranties"`
}

func noMeasurementField(t *testing.T, label string, env envelope) {
	t.Helper()
	if strings.Contains(strings.ToLower(string(env.Data)), "measurement") {
		t.Fatalf("%s carries measurement data: %s", label, env.Data)
	}
}

// TEC-238 acceptance: a customer served by two dealers sees both services
// in one list and the vehicle with its summary and active warranties;
// another customer's vehicle is 404; Glorian rows and drafts never come
// back; no measurement field in any response.
func TestIntegrationPortalVehicles(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	sfx := it.suffix[len(it.suffix)-6:]
	center := it.brandCenter("olex")
	dist := it.org("t238-dist", "distributor", center)
	dealerA := it.org("t238-dealer-a", "dealer", dist)
	dealerB := it.org("t238-dealer-b", "dealer", dist)
	gCenter := it.brandCenter("glorian")
	gDealer := it.org("t238-g", "dealer", gCenter)

	p := it.product(center, "T238")
	it.setWarrantyMonths(p, 12)

	custA, vehA := it.svcCustomer(dealerA, "t238-cust-a", "34T238"+sfx)
	custB, vehB := it.svcCustomer(dealerB, "t238-cust-b", "06T238"+sfx)
	for _, o := range []db.Organization{dealerB, gDealer} {
		if _, err := it.q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
			UserID: custA.ID, OrganizationID: o.ID, BrandID: o.BrandID,
		}); err != nil {
			t.Fatalf("link: %v", err)
		}
	}
	// Customer A: a warranty service at dealer A, a completed service at
	// dealer B, a draft at dealer B (hidden) and a Glorian vehicle + service.
	wA := it.warrantyFor(dealerA, center, p, custA, vehA, 238)
	svcAB := it.directService(dealerB, custA, vehA, 238101)
	draft, err := it.q.CreateService(ctx, db.CreateServiceParams{
		ServiceNo:      "T238D-" + sfx,
		OrganizationID: dealerB.ID, BrandID: dealerB.BrandID, CustomerUserID: custA.ID, VehicleID: vehA.ID,
		CarBrandID: vehA.CarBrandID.Int64, CarModelID: vehA.CarModelID.Int64, Status: "draft",
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	vehG, err := it.q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: custA.ID, OrganizationID: pgtype.Int8{Int64: gDealer.ID, Valid: true}, BrandID: gDealer.BrandID,
		CarBrandID: vehA.CarBrandID, CarModelID: vehA.CarModelID,
		Plate: pgtype.Text{String: "35G238" + sfx, Valid: true}, PlateNormalized: pgtype.Text{String: "35G238" + sfx, Valid: true},
		PlateCountry: pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		t.Fatalf("glorian vehicle: %v", err)
	}
	svcG := it.directService(gDealer, custA, vehG, 238102)
	// Customer B: one service at dealer B.
	svcB := it.directService(dealerB, custB, vehB, 238103)

	var svcAA string
	if err := it.pool.QueryRow(ctx, `SELECT uuid::text FROM services WHERE id = $1`, wA.ServiceID).Scan(&svcAA); err != nil {
		t.Fatal(err)
	}

	portalTok := func(u db.User) string {
		tok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
			UserID: u.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
		})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	tokA, tokB := portalTok(custA), portalTok(custB)

	// 1. Services: both dealers in one list; no draft, no Glorian, nothing
	// of customer B.
	env := it.custDo("GET", "/v1/portal/services?limit=100", tokA, nil, http.StatusOK)
	noMeasurementField(t, "services", env)
	sp := decodeData[portalServicePage](t, env)
	got := map[string]bool{}
	orgs := map[string]bool{}
	for _, s := range sp.Items {
		got[s.UUID] = true
		orgs[s.Organization.UUID] = true
	}
	if sp.Total != 2 || len(sp.Items) != 2 || !got[svcAA] || !got[svcAB.Uuid.String()] {
		t.Fatalf("customer A services = %+v", sp)
	}
	if !orgs[dealerA.Uuid.String()] || !orgs[dealerB.Uuid.String()] {
		t.Fatalf("customer A service organizations = %v", orgs)
	}
	if got[draft.Uuid.String()] || got[svcG.Uuid.String()] || got[svcB.Uuid.String()] {
		t.Fatalf("customer A sees a draft / Glorian / foreign service: %+v", sp)
	}
	spB := decodeData[portalServicePage](t, it.custDo("GET", "/v1/portal/services", tokB, nil, http.StatusOK))
	if spB.Total != 1 || spB.Items[0].UUID != svcB.Uuid.String() {
		t.Fatalf("customer B services = %+v", spB)
	}

	// 2. Vehicles: only the own Olex vehicle.
	env = it.custDo("GET", "/v1/portal/vehicles", tokA, nil, http.StatusOK)
	noMeasurementField(t, "vehicles", env)
	vp := decodeData[portalVehiclePage](t, env)
	if vp.Total != 1 || len(vp.Items) != 1 || vp.Items[0].UUID != vehA.Uuid.String() ||
		vp.Items[0].ServiceCount != 2 || vp.Items[0].ActiveWarrantyCount != 1 {
		t.Fatalf("customer A vehicles = %+v", vp)
	}

	// 3. Detail: summary across both dealers, the active warranty with time
	// left; another customer's vehicle, the Glorian vehicle and a bad uuid
	// are 404.
	env = it.custDo("GET", "/v1/portal/vehicles/"+vehA.Uuid.String(), tokA, nil, http.StatusOK)
	noMeasurementField(t, "vehicle detail", env)
	det := decodeData[portalVehicleDetail](t, env)
	if det.ServiceSummary.Total != 2 || det.ServiceSummary.Completed != 2 || det.ServiceSummary.OrganizationCount != 2 ||
		len(det.Services) != 2 {
		t.Fatalf("detail = %+v", det)
	}
	if len(det.ActiveWarranties) != 1 || det.ActiveWarranties[0].UUID != wA.Uuid.String() ||
		det.ActiveWarranties[0].DaysLeft < 300 || det.ActiveWarranties[0].PercentLeft < 95 {
		t.Fatalf("active warranties = %+v", det.ActiveWarranties)
	}
	it.custDo("GET", "/v1/portal/vehicles/"+vehB.Uuid.String(), tokA, nil, http.StatusNotFound)
	it.custDo("GET", "/v1/portal/vehicles/"+vehA.Uuid.String(), tokB, nil, http.StatusNotFound)
	it.custDo("GET", "/v1/portal/vehicles/"+vehG.Uuid.String(), tokA, nil, http.StatusNotFound)
	it.custDo("GET", "/v1/portal/vehicles/nope", tokA, nil, http.StatusNotFound)

	// 4. Glorian host: Glorian rows stay closed there too.
	if code, env := it.do("GET", "/v1/portal/vehicles/"+vehG.Uuid.String(), hostGlorian, tokA, nil); code == http.StatusOK {
		t.Fatalf("glorian host returned the glorian vehicle: %s", env.Data)
	}
	if code, env := it.do("GET", "/v1/portal/services", hostGlorian, tokA, nil); code == http.StatusOK {
		if gp := decodeData[portalServicePage](t, env); gp.Total != 0 {
			t.Fatalf("glorian host services = %+v", gp)
		}
	}

	// 5. TEC-239 ownership rule: a service is also the user's when the user
	// holds one of its warranties (a transferred warranty). Customer B's
	// warranty service moves its warranty to customer A: A now lists it, B
	// (the service's customer) still does.
	wB := it.warrantyFor(dealerB, center, p, custB, vehB, 239)
	if _, err := it.pool.Exec(ctx, `UPDATE warranties SET holder_user_id = $2 WHERE id = $1`, wB.ID, custA.ID); err != nil {
		t.Fatalf("transfer warranty: %v", err)
	}
	var svcWB string
	if err := it.pool.QueryRow(ctx, `SELECT uuid::text FROM services WHERE id = $1`, wB.ServiceID).Scan(&svcWB); err != nil {
		t.Fatal(err)
	}
	has := func(pg portalServicePage, id string) bool {
		for _, s := range pg.Items {
			if s.UUID == id {
				return true
			}
		}
		return false
	}
	spA := decodeData[portalServicePage](t, it.custDo("GET", "/v1/portal/services?limit=100", tokA, nil, http.StatusOK))
	if spA.Total != 3 || !has(spA, svcWB) {
		t.Fatalf("warranty holder services = %+v", spA)
	}
	spB = decodeData[portalServicePage](t, it.custDo("GET", "/v1/portal/services?limit=100", tokB, nil, http.StatusOK))
	if spB.Total != 2 || !has(spB, svcWB) {
		t.Fatalf("service customer services = %+v", spB)
	}

	// 6. Realm: a panel token is refused on the portal routes.
	u, pw := it.user("t238-a-owner")
	it.member(dealerA, u, "owner")
	panelTok := it.loginOrg(u, pw, dealerA)
	for _, path := range []string{"/v1/portal/vehicles", "/v1/portal/services"} {
		if code, env := it.do("GET", path, hostOlex, panelTok, nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
			t.Fatalf("panel token on %s: %d %s", path, code, errCode(env))
		}
	}
}
