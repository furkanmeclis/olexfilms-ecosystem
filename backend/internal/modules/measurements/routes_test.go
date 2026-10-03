package measurements

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

type noLoader struct{}

func (noLoader) LoadPrincipal(*http.Request, jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{}, http.ErrNoCookie
}

// TEC-233: the upload checks the version header first (426), refuses a
// panel (cookie/BFF) token with 403 REALM_FORBIDDEN and needs a Bearer.
// The stored row, idempotency and the foreign service are covered by the
// DB integration test in httpserver.
func TestMobileMeasurementRouteGate(t *testing.T) {
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterMobileRoutes(mux, handler.New(nil), tokens, noLoader{}, nil, 1, 1)

	oid := uuid.New()
	panel, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), OrganizationID: &oid, Audience: jwt.AudiencePanel})
	if err != nil {
		t.Fatal(err)
	}
	mobile, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), OrganizationID: &oid, Audience: jwt.AudienceMobile})
	if err != nil {
		t.Fatal(err)
	}
	call := func(bearer, version string) (int, string) {
		req := httptest.NewRequest("POST", "/v1/mobile/measurements", strings.NewReader(`{"raw":{}}`))
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
		if env.Error == nil {
			return rec.Code, ""
		}
		return rec.Code, env.Error.Code
	}
	for _, v := range []string{"", "0", "2"} {
		if st, code := call(mobile, v); st != http.StatusUpgradeRequired || code != "MOBILE_API_VERSION_UNSUPPORTED" {
			t.Fatalf("version %q = %d %s, want 426", v, st, code)
		}
	}
	if st, code := call(panel, "1"); st != http.StatusForbidden || code != "REALM_FORBIDDEN" {
		t.Fatalf("panel token = %d %s, want 403 REALM_FORBIDDEN", st, code)
	}
	if st, _ := call("", "1"); st != http.StatusUnauthorized {
		t.Fatalf("without bearer = %d, want 401", st)
	}
	if st, code := call(mobile, "1"); st != http.StatusUnauthorized || code == "REALM_FORBIDDEN" {
		t.Fatalf("mobile token = %d %s, want loader 401", st, code)
	}
}
