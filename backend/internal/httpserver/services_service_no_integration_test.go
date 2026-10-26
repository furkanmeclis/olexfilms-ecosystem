package httpserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-1004: a hand-entered service number (printed warranty card).
//
//   - create with service_no keeps it as entered; blank allocates DS + 8;
//   - a taken or malformed number is refused (409 SERVICE_NO_TAKEN / 400);
//   - the availability check answers valid / available, a service's own
//     number is available to it;
//   - PATCH changes the number until the service is cancelled or completed,
//     then 409 SERVICE_NO_LOCKED.
func TestIntegrationServiceNoManual(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t1004-dist", "distributor", center)
	dealer := it.org("t1004-dealer", "dealer", dist)
	staff, spw := it.user("t1004-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	owner, opw := it.user("t1004-owner")
	it.member(dealer, owner, "owner")
	cust, veh := it.svcCustomer(dealer, "t1004-cust", "34T1004"+it.suffix[len(it.suffix)-3:])
	tok := it.loginOrg(owner, opw, dealer)
	staffTok := it.loginOrg(staff, spw, center)

	no := "KRT" + it.suffix
	if len(no) > 32 {
		no = no[:32]
	}
	body := map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String(), "service_no": "  " + no + " "}
	s1, _ := it.svcCall("POST", "/v1/services", tok, body, http.StatusCreated)
	if s1.ServiceNo != no {
		t.Fatalf("service_no = %q, want %q", s1.ServiceNo, no)
	}
	if _, ec := it.svcCall("POST", "/v1/services", tok, body, http.StatusConflict); ec != "SERVICE_NO_TAKEN" {
		t.Fatalf("duplicate = %s", ec)
	}
	body["service_no"] = "bad no!"
	if _, ec := it.svcCall("POST", "/v1/services", tok, body, http.StatusBadRequest); ec != "VALIDATION_ERROR" {
		t.Fatalf("malformed = %s", ec)
	}
	body["service_no"] = ""
	s2, _ := it.svcCall("POST", "/v1/services", tok, body, http.StatusCreated)
	if len(s2.ServiceNo) != 10 || s2.ServiceNo[:2] != "DS" {
		t.Fatalf("generated = %q", s2.ServiceNo)
	}

	check := func(q url.Values) (valid, available bool) {
		t.Helper()
		code, env := it.do("GET", "/v1/services/service-no-availability?"+q.Encode(), hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("availability = %d %s", code, errCode(env))
		}
		var res struct {
			Valid     bool `json:"valid"`
			Available bool `json:"available"`
		}
		if err := json.Unmarshal(env.Data, &res); err != nil {
			t.Fatal(err)
		}
		return res.Valid, res.Available
	}
	if v, a := check(url.Values{"service_no": {no}}); !v || a {
		t.Fatalf("taken number = %v/%v", v, a)
	}
	if v, a := check(url.Values{"service_no": {no}, "service_uuid": {s1.UUID}}); !v || !a {
		t.Fatalf("own number = %v/%v", v, a)
	}
	if v, _ := check(url.Values{"service_no": {"a b"}}); v {
		t.Fatal("malformed number is valid")
	}
	if v, a := check(url.Values{"service_no": {no + "X"}}); !v || !a {
		t.Fatalf("free number = %v/%v", v, a)
	}

	// PATCH: another service's number is refused, a free one is taken.
	if _, ec := it.svcCall("PATCH", "/v1/services/"+s2.UUID, tok, map[string]any{"service_no": no}, http.StatusConflict); ec != "SERVICE_NO_TAKEN" {
		t.Fatalf("patch taken = %s", ec)
	}
	renamed := no[:len(no)-1] + "Z"
	s2, _ = it.svcCall("PATCH", "/v1/services/"+s2.UUID, tok, map[string]any{"service_no": renamed}, http.StatusOK)
	if s2.ServiceNo != renamed {
		t.Fatalf("renamed = %q", s2.ServiceNo)
	}

	// Cancelled: the number is final, also for the center.
	it.svcCall("POST", "/v1/services/"+s2.UUID+"/transitions", staffTok,
		map[string]string{"status": "cancelled", "note": "t1004"}, http.StatusOK)
	if _, ec := it.svcCall("PATCH", "/v1/services/"+s2.UUID, staffTok, map[string]any{"service_no": renamed + "Q"}, http.StatusConflict); ec != "SERVICE_NO_LOCKED" {
		t.Fatalf("locked = %s", ec)
	}
	// The same number is a no-op.
	it.svcCall("PATCH", "/v1/services/"+s2.UUID, staffTok, map[string]any{"service_no": renamed}, http.StatusOK)
}
