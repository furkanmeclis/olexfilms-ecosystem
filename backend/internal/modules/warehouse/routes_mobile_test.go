package warehouse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	whhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

type noLoader struct{}

func (noLoader) LoadPrincipal(*http.Request, jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{}, http.ErrNoCookie
}

const someUUID = "0b8c7c1e-5a52-4d8e-9c0a-6f1f0f7b2a11"

var mobileWarehouseRoutes = []struct{ method, path string }{
	{"GET", "/v1/mobile/warehouse/warehouses"},
	{"POST", "/v1/mobile/warehouse/scan"},
	{"GET", "/v1/mobile/warehouse/stock-counts"},
	{"POST", "/v1/mobile/warehouse/stock-counts"},
	{"GET", "/v1/mobile/warehouse/stock-counts/" + someUUID},
	{"POST", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/approve-start"},
	{"POST", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/start"},
	{"GET", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/scans"},
	{"POST", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/scans"},
	{"DELETE", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/scans/" + someUUID},
	{"POST", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/complete"},
	{"GET", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/report"},
	{"POST", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/approve"},
	{"POST", "/v1/mobile/warehouse/stock-counts/" + someUUID + "/cancel"},
	{"GET", "/v1/mobile/warehouse/transfers"},
	{"POST", "/v1/mobile/warehouse/transfers"},
	{"GET", "/v1/mobile/warehouse/transfers/" + someUUID},
	{"POST", "/v1/mobile/warehouse/transfers/" + someUUID + "/lines"},
	{"DELETE", "/v1/mobile/warehouse/transfers/" + someUUID + "/lines/" + someUUID},
	{"POST", "/v1/mobile/warehouse/transfers/" + someUUID + "/place"},
	{"POST", "/v1/mobile/warehouse/transfers/" + someUUID + "/ship"},
	{"POST", "/v1/mobile/warehouse/transfers/" + someUUID + "/complete"},
	{"POST", "/v1/mobile/warehouse/transfers/" + someUUID + "/cancel"},
}

// TEC-235: every mobile warehouse route is mounted, checks the version
// header before anything else (426) and refuses a panel (cookie/BFF)
// session with 403 REALM_FORBIDDEN. Scope and flows are covered by the
// DB integration test in httpserver.
func TestMobileWarehouseRoutesGate(t *testing.T) {
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterMobileRoutes(mux, MobileHandlers{
		Tree: &whhandler.Handler{}, Scan: &whhandler.ScanHandler{},
		Counts: &whhandler.Counts{}, Transfers: &whhandler.Transfers{},
	}, nil, tokens, noLoader{}, nil, 1, 1)

	oid := uuid.New()
	panel, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), OrganizationID: &oid, Audience: jwt.AudiencePanel})
	if err != nil {
		t.Fatal(err)
	}
	mobile, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), OrganizationID: &oid, Audience: jwt.AudienceMobile})
	if err != nil {
		t.Fatal(err)
	}

	call := func(method, path, bearer, version string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if version != "" {
			req.Header.Set("X-Mobile-Api-Version", version)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var env struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		code := ""
		if env.Error != nil {
			code = env.Error.Code
		}
		return rec.Code, code
	}

	for _, rt := range mobileWarehouseRoutes {
		name := rt.method + " " + rt.path
		for _, v := range []string{"", "0", "2"} {
			if st, code := call(rt.method, rt.path, mobile, v); st != http.StatusUpgradeRequired || code != "MOBILE_API_VERSION_UNSUPPORTED" {
				t.Fatalf("%s version %q = %d %s, want 426", name, v, st, code)
			}
		}
		if st, code := call(rt.method, rt.path, panel, "1"); st != http.StatusForbidden || code != "REALM_FORBIDDEN" {
			t.Fatalf("%s panel token = %d %s, want 403 REALM_FORBIDDEN", name, st, code)
		}
		if st, _ := call(rt.method, rt.path, "", "1"); st != http.StatusUnauthorized {
			t.Fatalf("%s without bearer = %d, want 401", name, st)
		}
		// A mobile token passes the realm gate and reaches the identity
		// loader (here: rejected, 401), never REALM_FORBIDDEN.
		if st, code := call(rt.method, rt.path, mobile, "1"); st != http.StatusUnauthorized || code == "REALM_FORBIDDEN" {
			t.Fatalf("%s mobile token = %d %s, want loader 401", name, st, code)
		}
	}
}
