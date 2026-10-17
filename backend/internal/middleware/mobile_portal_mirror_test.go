package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

type claimsLoader struct{}

func (claimsLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, err := c.UserUUID()
	return authctx.Principal{UserID: id, Realm: c.Realm(), MobileKind: c.SessionKind()}, err
}

type mirrorFixture struct {
	mux    *http.ServeMux
	tokens *jwt.Manager
}

func newMirrorFixture(t *testing.T) mirrorFixture {
	t.Helper()
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	authn := Authenticate(tokens, claimsLoader{})
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := authctx.MustPrincipal(r.Context())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": r.URL.Path, "id": r.PathValue("uuid"), "realm": p.Realm, "kind": p.MobileKind,
			"mirrored": IsMobilePortalMirror(r.Context()),
		})
	})
	for _, pattern := range []string{
		"GET /v1/portal/vehicles/{uuid}", "POST /v1/portal/appointments", "GET /v1/portal/ai/status",
		"GET /v1/portal/oauth/authorize", "GET /v1/auth/me", "GET /v1/services",
		"GET /v1/mobile/auth/me", "POST /v1/mobile/auth/logout", "POST /v1/mobile/auth/logout-all",
		"PUT /v1/mobile/push-token", "POST /v1/mobile/auth/organization-context",
		"POST /v1/mobile/auth/qr/{code}/approve", "GET /v1/mobile/warehouse/tasks",
		"GET /v1/mobile/legacy/me",
	} {
		mux.Handle(pattern, authn(echo))
	}
	mux.Handle(MobilePortalMirrorPrefix, MobilePortalMirror(tokens, 1, 1)(mux))
	return mirrorFixture{mux: mux, tokens: tokens}
}

func (f mirrorFixture) token(t *testing.T, aud, kind string) string {
	t.Helper()
	tok, _, err := f.tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), SessionID: uuid.New(), Audience: aud, MobileKind: kind})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (f mirrorFixture) do(method, path, token string, version bool) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if version {
		req.Header.Set(MobileAPIVersionHeader, "1")
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestMobilePortalMirrorCustomerSession(t *testing.T) {
	f := newMirrorFixture(t)
	customer := f.token(t, jwt.AudienceMobile, jwt.MobileKindCustomer)
	id := uuid.NewString()
	rec, body := f.do(http.MethodGet, "/v1/mobile/portal/vehicles/"+id, customer, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	// The portal handler sees exactly a portal session.
	if body["path"] != "/v1/portal/vehicles/"+id || body["id"] != id || body["realm"] != jwt.AudiencePortal || body["kind"] != "" || body["mirrored"] != true {
		t.Fatalf("body=%v", body)
	}
	if rec.Header().Get(MobileAPIVersionHeader) != "1" {
		t.Fatal("api version header missing")
	}
	if rec, _ := f.do(http.MethodPost, "/v1/mobile/portal/appointments", customer, true); rec.Code != http.StatusOK {
		t.Fatalf("POST status=%d", rec.Code)
	}
}

func TestMobilePortalMirrorRefusals(t *testing.T) {
	f := newMirrorFixture(t)
	customer := f.token(t, jwt.AudienceMobile, jwt.MobileKindCustomer)
	for _, tc := range []struct {
		name, method, path, token string
		version                   bool
		want                      int
	}{
		{"no version", http.MethodGet, "/v1/mobile/portal/vehicles/x", customer, false, http.StatusUpgradeRequired},
		{"no token", http.MethodGet, "/v1/mobile/portal/vehicles/x", "", true, http.StatusUnauthorized},
		{"bad token", http.MethodGet, "/v1/mobile/portal/vehicles/x", "garbage", true, http.StatusUnauthorized},
		{"staff mobile", http.MethodGet, "/v1/mobile/portal/vehicles/x", f.token(t, jwt.AudienceMobile, ""), true, http.StatusForbidden},
		{"portal token", http.MethodGet, "/v1/mobile/portal/vehicles/x", f.token(t, jwt.AudiencePortal, jwt.MobileKindCustomer), true, http.StatusForbidden},
		{"panel token", http.MethodGet, "/v1/mobile/portal/vehicles/x", f.token(t, jwt.AudiencePanel, ""), true, http.StatusForbidden},
		{"ai denied", http.MethodGet, "/v1/mobile/portal/ai/status", customer, true, http.StatusNotFound},
		{"oauth denied", http.MethodGet, "/v1/mobile/portal/oauth/authorize", customer, true, http.StatusNotFound},
		{"dot segment", http.MethodGet, "/v1/mobile/portal/vehicles/%2E%2E/x", customer, true, http.StatusNotFound},
		{"bare prefix", http.MethodGet, "/v1/mobile/portal/", customer, true, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := f.do(tc.method, tc.path, tc.token, tc.version)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestMobileCustomerSessionRoutes(t *testing.T) {
	f := newMirrorFixture(t)
	customer := f.token(t, jwt.AudienceMobile, jwt.MobileKindCustomer)
	staff := f.token(t, jwt.AudienceMobile, "")
	for _, tc := range []struct {
		method, path string
		customer     int
		staff        int
	}{
		{http.MethodGet, "/v1/mobile/auth/me", http.StatusOK, http.StatusOK},
		{http.MethodPost, "/v1/mobile/auth/logout", http.StatusOK, http.StatusOK},
		{http.MethodPost, "/v1/mobile/auth/logout-all", http.StatusOK, http.StatusOK},
		{http.MethodPut, "/v1/mobile/push-token", http.StatusOK, http.StatusOK},
		{http.MethodPost, "/v1/mobile/auth/organization-context", http.StatusForbidden, http.StatusOK},
		{http.MethodPost, "/v1/mobile/auth/qr/abc/approve", http.StatusForbidden, http.StatusOK},
		{http.MethodGet, "/v1/mobile/warehouse/tasks", http.StatusForbidden, http.StatusOK},
		{http.MethodGet, "/v1/mobile/legacy/me", http.StatusForbidden, http.StatusOK},
		// Outside the mirror a mobile token never reaches portal or panel routes.
		{http.MethodGet, "/v1/portal/vehicles/x", http.StatusForbidden, http.StatusForbidden},
		{http.MethodGet, "/v1/auth/me", http.StatusForbidden, http.StatusForbidden},
		{http.MethodGet, "/v1/services", http.StatusForbidden, http.StatusForbidden},
	} {
		if rec, body := f.do(tc.method, tc.path, customer, false); rec.Code != tc.customer {
			t.Errorf("customer %s %s: status=%d want=%d", tc.method, tc.path, rec.Code, tc.customer)
		} else if rec.Code == http.StatusOK && body["kind"] != jwt.MobileKindCustomer {
			t.Errorf("customer %s: kind=%v", tc.path, body["kind"])
		}
		if rec, body := f.do(tc.method, tc.path, staff, false); rec.Code != tc.staff {
			t.Errorf("staff %s %s: status=%d want=%d", tc.method, tc.path, rec.Code, tc.staff)
		} else if rec.Code == http.StatusOK && body["kind"] != jwt.MobileKindStaff {
			t.Errorf("staff %s: kind=%v", tc.path, body["kind"])
		}
	}
}

func TestMobileCustomerRealmMirroredPathGuard(t *testing.T) {
	f := newMirrorFixture(t)
	claims, err := f.tokens.ParseAccess(f.token(t, jwt.AudienceMobile, jwt.MobileKindCustomer))
	if err != nil {
		t.Fatal(err)
	}
	// A marked request outside /v1/portal/ never becomes a portal session.
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	req = req.WithContext(t.Context())
	marked := req.Clone(contextWithPortalMirror(req))
	if MobileCustomerRealm(&claims, marked) {
		t.Fatal("marked non-portal path allowed")
	}
}

func contextWithPortalMirror(r *http.Request) context.Context {
	return context.WithValue(r.Context(), mobilePortalMirrorKey{}, true)
}
