package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/google/uuid"
)

// fleetPortalUser invites a fleet user through dealer tok and signs them in
// to the portal.
func (it *itest) fleetPortalUser(fleetUUID, tok, tag string) string {
	it.t.Helper()
	ctx := context.Background()
	invited := decodeData[struct {
		UserUUID string `json:"user_uuid"`
	}](it.t, it.fleetDo("POST", "/v1/fleets/"+fleetUUID+"/users", tok, map[string]any{
		"email": tag + "-" + it.suffix + "@example.test", "name": "Filo " + tag,
	}, http.StatusCreated))
	u, err := it.q.GetUserByUUID(ctx, uuid.MustParse(invited.UserUUID))
	if err != nil {
		it.t.Fatal(err)
	}
	pw := "Fleet-Passw0rd!x"
	hash, _ := password.Hash(pw)
	if _, err := it.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, u.ID, hash); err != nil {
		it.t.Fatal(err)
	}
	return it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email.String, "password": pw, "realm": "portal",
	})).AccessToken
}

// TEC-474 acceptance (HTTP): the fleet portal reads with the real module
// gate; another fleet's vehicle is 404; a non-primary fleet user reads the
// fleet's service through the existing portal detail; the fleet session
// stays read only (403 PORTAL_READ_ONLY on portal writes) except the link
// decision; with the module off at every linked dealer the reads answer 403
// FEATURE_DISABLED while the link list stays open; list contracts are 400
// on unknown sorts and bad filters.
func TestIntegrationFleetPortal(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	n := it.fleetNet("t474")
	it.enableFleet(n.dealerA, n.dealerB)

	open := func(tag string) string {
		return decodeData[fleetView](t, it.fleetDo("POST", "/v1/fleets", n.tokA, map[string]any{
			"legal_name": "T474 Filo " + tag, "tax_number": tecVKN(t, it.suffix[:len(it.suffix)-1]+tag),
		}, http.StatusCreated)).UUID
	}
	fleetA, fleetB := open("1"), open("2")
	it.fleetPortalUser(fleetA, n.tokA, "t474-primary-a") // primary: owns the vehicles
	tokA := it.fleetPortalUser(fleetA, n.tokA, "t474-second-a")
	tokB := it.fleetPortalUser(fleetB, n.tokA, "t474-primary-b")
	vehicle := func(fleet string, i int) string {
		return decodeData[struct {
			UUID string `json:"uuid"`
		}](t, it.fleetDo("POST", "/v1/fleets/"+fleet+"/vehicles", n.tokA, map[string]any{
			"plate": it.fleetPlate(i), "car_brand_uuid": n.carBrand.Uuid.String(), "car_model_uuid": n.carModel.Uuid.String(),
		}, http.StatusCreated)).UUID
	}
	vA, vB := vehicle(fleetA, 1), vehicle(fleetB, 2)
	svcA := it.completedFleetService(n.dealerA, vA, n)
	svcB := it.completedFleetService(n.dealerA, vB, n)

	// 1. Reads of the own fleet.
	for _, p := range []string{
		"/v1/portal/fleet/overview", "/v1/portal/fleet/services?status=completed&date_from=2020-01-01",
		"/v1/portal/fleet/warranties?state=active,expired", "/v1/portal/fleet/accounting",
		"/v1/portal/fleet/reports?period_kind=monthly", "/v1/portal/fleet/vehicles/" + vA,
	} {
		it.fleetDo("GET", p, tokA, nil, http.StatusOK)
	}
	vl := decodeData[fleetPage](t, it.fleetDo("GET", "/v1/portal/fleet/vehicles?sort=-warranty_until&has_active_warranty=false&brand="+n.carBrand.Uuid.String(), tokA, nil, http.StatusOK))
	if vl.Total != 1 || len(vl.Items) != 1 || vl.Items[0].UUID != vA {
		t.Fatalf("fleet A vehicles = %+v", vl)
	}
	sl := decodeData[fleetPage](t, it.fleetDo("GET", "/v1/portal/fleet/services?dealer="+n.dealerA.Uuid.String(), tokA, nil, http.StatusOK))
	if sl.Total != 1 || sl.Items[0].UUID != svcA {
		t.Fatalf("fleet A services = %+v", sl)
	}
	// 2. Another fleet's vehicle and service are 404 (the existing portal
	// detail serves the non-primary user its own fleet's service).
	it.fleetDo("GET", "/v1/portal/fleet/vehicles/"+vB, tokA, nil, http.StatusNotFound)
	it.fleetDo("GET", "/v1/portal/fleet/vehicles/"+vA, tokB, nil, http.StatusNotFound)
	it.fleetDo("GET", "/v1/portal/services/"+svcA, tokA, nil, http.StatusOK)
	it.fleetDo("GET", "/v1/portal/services/"+svcB, tokA, nil, http.StatusNotFound)

	// 3. List contracts: unknown sort and bad filters are 400.
	for _, p := range []string{
		"/v1/portal/fleet/vehicles?sort=vin", "/v1/portal/fleet/vehicles?has_active_warranty=maybe",
		"/v1/portal/fleet/vehicles?brand=x", "/v1/portal/fleet/services?status=draft",
		"/v1/portal/fleet/services?sort=plate", "/v1/portal/fleet/services?date_from=2026-02-01&date_to=2026-01-01",
		"/v1/portal/fleet/warranties?state=bogus", "/v1/portal/fleet/accounting?date_from=2026-01-01",
		"/v1/portal/fleet/reports?sort=locale",
	} {
		if env := it.fleetDo("GET", p, tokA, nil, http.StatusBadRequest); errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("GET %s = %s, want VALIDATION_ERROR", p, errCode(env))
		}
	}

	// 4. Read only: portal writes are 403 PORTAL_READ_ONLY, the link
	// decision is allowed.
	for _, c := range []struct {
		path string
		body any
	}{
		{"/v1/portal/services/" + svcA + "/review", map[string]any{"rating": 5}},
		{"/v1/portal/appointments", map[string]any{}},
		{"/v1/portal/vehicles/" + vA + "/transfers", map[string]any{"phone": itPhone()}},
	} {
		if code, env := it.do("POST", c.path, hostOlex, tokA, c.body); code != http.StatusForbidden || errCode(env) != "PORTAL_READ_ONLY" {
			t.Fatalf("fleet POST %s = %d %s, want 403 PORTAL_READ_ONLY", c.path, code, errCode(env))
		}
	}
	req := decodeData[fleetView](t, it.fleetDo("POST", "/v1/fleets/"+fleetA+"/links", n.tokB, nil, http.StatusCreated))
	it.fleetDo("POST", "/v1/portal/fleet/links/"+req.Link.UUID+"/accept", tokA, nil, http.StatusOK)
	ov := decodeData[struct {
		Dealers []struct {
			UUID string `json:"uuid"`
		} `json:"dealers"`
	}](t, it.fleetDo("GET", "/v1/portal/fleet/overview", tokA, nil, http.StatusOK))
	if len(ov.Dealers) != 2 {
		t.Fatalf("overview dealers after accept = %+v", ov.Dealers)
	}

	// 5. Module off at dealer B: B drops out; off everywhere: 403
	// FEATURE_DISABLED, the links stay readable.
	admin, _ := it.user("t474-admin")
	setFleet := func(org int64, on bool) {
		if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, org, features.ModuleFleet, on); err != nil {
			t.Fatal(err)
		}
	}
	setFleet(n.dealerB.ID, false)
	ov = decodeData[struct {
		Dealers []struct {
			UUID string `json:"uuid"`
		} `json:"dealers"`
	}](t, it.fleetDo("GET", "/v1/portal/fleet/overview", tokA, nil, http.StatusOK))
	if len(ov.Dealers) != 1 || ov.Dealers[0].UUID != n.dealerA.Uuid.String() {
		t.Fatalf("overview dealers with B off = %+v", ov.Dealers)
	}
	setFleet(n.dealerA.ID, false)
	for _, p := range []string{"/v1/portal/fleet/overview", "/v1/portal/fleet/vehicles", "/v1/portal/fleet/accounting"} {
		if env := it.fleetDo("GET", p, tokA, nil, http.StatusForbidden); errCode(env) != "FEATURE_DISABLED" {
			t.Fatalf("module off GET %s = %s", p, errCode(env))
		}
	}
	it.fleetDo("GET", "/v1/portal/fleet/links", tokA, nil, http.StatusOK)
	it.fleetDo("GET", "/v1/portal/services/"+svcA, tokA, nil, http.StatusNotFound)

	// 6. A panel token never reaches the fleet portal.
	if code, _ := it.do("GET", "/v1/portal/fleet/overview", hostOlex, n.tokA, nil); code < 400 {
		t.Fatalf("panel token on the fleet portal = %d", code)
	}
}
