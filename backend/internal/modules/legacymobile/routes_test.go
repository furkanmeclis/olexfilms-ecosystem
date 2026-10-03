package legacymobile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

type noLoader struct{}

func (noLoader) LoadPrincipal(*http.Request, jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{}, http.ErrNoCookie
}

// stubHandlers answers 299 with the alias name, so a test can tell the
// request reached the adapted handler.
func stubHandlers() Handlers {
	stub := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Stub", name)
			w.WriteHeader(299)
		}
	}
	return Handlers{
		Login: stub("login"), Me: stub("me"),
		ListServices: stub("services_list"), GetService: stub("service_detail"),
		CreateMeasurement: stub("measurement"),
		PutPushToken:      stub("push_token"), DeletePushToken: stub("push_token_delete"),
	}
}

func fixtures(t *testing.T) []Fixture {
	t.Helper()
	vars := map[string]string{
		"email": "legacy@example.com", "password": "x", "organization_slug": "org", "device_id": "dev",
		"access_token": "t", "service_uuid": uuid.NewString(), "idempotency_key": "k", "expo_push_token": "ExponentPushToken[x]",
	}
	var out []Fixture
	for _, r := range Routes {
		f, err := LoadFixture(FixtureDir, r.Name, vars)
		if err != nil {
			t.Fatalf("fixture %s: %v", r.Name, err)
		}
		out = append(out, f)
	}
	return out
}

func newMux(t *testing.T, enabled bool) (*http.ServeMux, *jwt.Manager) {
	t.Helper()
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, enabled, stubHandlers(), tokens, noLoader{}, nil, nil)
	return mux, tokens
}

func call(mux *http.ServeMux, f Fixture, bearer string) (*httptest.ResponseRecorder, string) {
	var body *strings.Reader
	if len(f.Request.Body) > 0 && string(f.Request.Body) != "null" {
		body = strings.NewReader(string(f.Request.Body))
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(f.Method, f.Path, body)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
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
		return rec, ""
	}
	return rec, env.Error.Code
}

// TEC-234: every alias has a contract fixture, every fixture names a mounted
// alias under the separate prefix and the route it adapts, and no fixture is
// left without a route.
func TestFixturesMatchRoutes(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(FixtureDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(Routes) {
		t.Fatalf("%d fixtures for %d routes", len(files), len(Routes))
	}
	for i, f := range fixtures(t) {
		r := Routes[i]
		if f.Name != r.Name || f.Method != r.Method || f.Adapts != r.Target {
			t.Fatalf("fixture %s = %s %s (%s), route %s %s (%s)", r.Name, f.Method, f.Path, f.Adapts, r.Method, r.Path, r.Target)
		}
		if !strings.HasPrefix(f.Path, Prefix+"/") || !strings.HasPrefix(r.Path, Prefix+"/") {
			t.Fatalf("%s: path %s outside %s", r.Name, f.Path, Prefix)
		}
		if f.Response.Status == 0 || len(f.Response.DataKeys) == 0 || f.Source == "" {
			t.Fatalf("%s: response status, data_keys and source are required", r.Name)
		}
	}
	raw, err := os.ReadFile(filepath.Join(FixtureDir, "login.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixture(FixtureDir, "login", map[string]string{"email": "x"}); err == nil || !strings.Contains(string(raw), "{{password}}") {
		t.Fatalf("an unfilled placeholder must fail: %v", err)
	}
}

// Flag off (MOBILE_LEGACY_ALIASES=false, the default): every alias is 404.
func TestFlagOffIsNotFound(t *testing.T) {
	mux, _ := newMux(t, false)
	for _, f := range fixtures(t) {
		if rec, _ := call(mux, f, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s with the flag off = %d, want 404", f.Method, f.Path, rec.Code)
		}
	}
}

// Flag on: login is public and reaches the adapted handler without the
// X-Mobile-Api-Version header the old app does not send; every other alias
// needs a Bearer (401) of the mobile realm (a panel token is 403
// REALM_FORBIDDEN) before it reaches its handler.
func TestFlagOnGates(t *testing.T) {
	mux, tokens := newMux(t, true)
	oid := uuid.New()
	panel, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), OrganizationID: &oid, Audience: jwt.AudiencePanel})
	if err != nil {
		t.Fatal(err)
	}
	mobile, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), OrganizationID: &oid, Audience: jwt.AudienceMobile})
	if err != nil {
		t.Fatal(err)
	}
	gates := map[string]string{}
	for _, r := range Routes {
		gates[r.Name] = r.Gate
	}
	for _, f := range fixtures(t) {
		if gates[f.Name] == GatePublic {
			rec, _ := call(mux, f, "")
			if rec.Code != 299 || rec.Header().Get("X-Stub") != f.Name {
				t.Fatalf("%s = %d (%s), want the adapted handler", f.Name, rec.Code, rec.Header().Get("X-Stub"))
			}
			continue
		}
		if rec, _ := call(mux, f, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without bearer = %d, want 401", f.Name, rec.Code)
		}
		if rec, code := call(mux, f, panel); rec.Code != http.StatusForbidden || code != "REALM_FORBIDDEN" {
			t.Fatalf("%s with a panel token = %d %s, want 403 REALM_FORBIDDEN", f.Name, rec.Code, code)
		}
		// The mobile realm passes the realm check; the stub loader then
		// refuses the identity (401), not the realm.
		if rec, code := call(mux, f, mobile); rec.Code != http.StatusUnauthorized || code == "REALM_FORBIDDEN" {
			t.Fatalf("%s with a mobile token = %d %s, want loader 401", f.Name, rec.Code, code)
		}
	}
}
