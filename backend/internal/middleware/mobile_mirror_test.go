package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

func TestMobilePanelMirrorRewrite(t *testing.T) {
	for _, prefix := range MobilePanelMirrorPrefixes {
		t.Run(prefix, func(t *testing.T) {
			target := "/v1/mobile/panel/" + prefix + "/a%20b?q=one%20two&limit=3"
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Header.Set(MobileAPIVersionHeader, "2")
			originalPath, originalRawPath := req.URL.Path, req.URL.RawPath
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r == req || r.URL == req.URL || r.URL.Path != "/v1/"+prefix+"/a b" {
					t.Errorf("request was not cloned and rewritten: %s", r.URL.Path)
				}
				if !IsMobilePanelMirror(r.Context()) || r.URL.RawQuery != "q=one%20two&limit=3" {
					t.Errorf("mark/query = %v %q", IsMobilePanelMirror(r.Context()), r.URL.RawQuery)
				}
				w.WriteHeader(http.StatusNoContent)
			})
			rec := httptest.NewRecorder()
			MobilePanelMirror(2, 3)(next).ServeHTTP(rec, req)
			if !called || rec.Code != http.StatusNoContent || rec.Header().Get(MobileAPIVersionHeader) != "3" {
				t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
			}
			if req.URL.Path != originalPath || req.URL.RawPath != originalRawPath || IsMobilePanelMirror(req.Context()) {
				t.Fatal("original request mutated")
			}
		})
	}
	// An explicitly escaped identifier retains its RawPath on the clone.
	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/orders/a%2Fb", nil)
	req.Header.Set(MobileAPIVersionHeader, "1")
	MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.URL.RawPath != "/v1/orders/a%2Fb" || r.URL.Path != "/v1/orders/a/b" {
			t.Errorf("escaped rewrite = %q %q", r.URL.Path, r.URL.RawPath)
		}
	})).ServeHTTP(httptest.NewRecorder(), req)
}

func TestMobilePanelMirrorTEC933Allowed(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"certificate-warnings",
		"certificate-warnings/" + id + "/approve",
		"tenant/pricing/recommended/current",
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			if r.URL.Path != "/v1/"+path {
				t.Errorf("rewrite = %s", r.URL.Path)
			}
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s was not allowed", path)
		}
	}
}

// TEC-961: the mobile step-up verifications are mirrored; the status, the
// passkey routes and every other auth route are not.
func TestMobilePanelMirrorTEC961StepUp(t *testing.T) {
	for _, path := range []string{"auth/step-up/password", "auth/step-up/totp"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/mobile/panel/"+path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			if r.URL.Path != "/v1/"+path || !IsMobilePanelMirror(r.Context()) {
				t.Errorf("rewrite = %s", r.URL.Path)
			}
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s was not allowed", path)
		}
	}
	for _, path := range []string{"auth/step-up", "auth/step-up/passkey/options", "auth/step-up/passkey/verify",
		"auth/step-up/passwordx", "auth/step-up/other", "auth/me", "auth/sessions", "auth/login", "auth/impersonate"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/mobile/panel/"+path, nil)
		rec := httptest.NewRecorder()
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatalf("%s reached next", path)
		})).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", path, rec.Code)
		}
	}
}

func TestMobilePanelMirrorDenied(t *testing.T) {
	paths := []string{
		"", "auth", "platform", "portal", "public", "mobile", "oauth", "realtime", "ai", "accounting",
		"impersonation", "unknown", "ordersx", "tenant", "tenant/settings", "tenant/organizationsx",
		"tenant/pricing-x", "tenant/other", "certificate-warningsx", "accounting/accounts",
		"accounting/reports", "accounting/exports", "accounting/collections", "accounting/payments",
		"accounting/opening-balances", "accounting/carix", "accounting/disputesx", "accounting/entriesx",
		"accounting/categoriesx", "einvoicesx", "einvoice", "staff-payments", "dealer-pricesx",
		"dealer-price", "dealer-sales", "product-salesx", "product", "purchasesx", "purchase",
		"suppliersx", "supplier", "dealer-accounting", "ai/usage", "ai/usage/summary", "ai/statusx",
		"ai/conversationsx", "ai/actionsx", "ai/pending-actionsx", "ai/other", "platform/ai/settings",
		"campaignsx", "consentsx"}
	for _, path := range paths {
		for _, suffix := range []string{"", "/child"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path+suffix, nil)
			rec := httptest.NewRecorder()
			MobilePanelMirror(1, 1)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("denied path reached next")
			})).ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) {
				t.Errorf("%s = %d %s", req.URL.Path, rec.Code, rec.Body.String())
			}
		}
	}
}

func TestMobilePanelMirrorVersion(t *testing.T) {
	for _, version := range []string{"", "abc", "1", "4"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/services", nil)
		req.Header.Set(MobileAPIVersionHeader, version)
		rec := httptest.NewRecorder()
		MobilePanelMirror(2, 3)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("unsupported version reached next")
		})).ServeHTTP(rec, req)
		if rec.Code != http.StatusUpgradeRequired || !strings.Contains(rec.Body.String(), `"code":"MOBILE_API_VERSION_UNSUPPORTED"`) {
			t.Errorf("version %q = %d %s", version, rec.Code, rec.Body.String())
		}
	}
}

func TestMobilePanelMirrorPassThrough(t *testing.T) {
	for _, path := range []string{"/v1/services", "/v1/mobile/auth/me", "/v1/mobile/panelx/services", "/healthz"} {
		req := httptest.NewRequest(http.MethodGet, path+"?q=one", nil)
		rec := httptest.NewRecorder()
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			if r != req || IsMobilePanelMirror(r.Context()) || r.URL.String() != path+"?q=one" {
				t.Fatal("non-mirror request changed")
			}
		})).ServeHTTP(rec, req)
		if rec.Header().Get(MobileAPIVersionHeader) != "" {
			t.Fatal("non-mirror version checked")
		}
	}
}

type mirrorIdentityLoader struct{}

func (mirrorIdentityLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{Realm: c.Realm()}, nil
}

func TestMobilePanelMirrorAuthenticate(t *testing.T) {
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/services", Authenticate(tokens, mirrorIdentityLoader{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := authctx.PrincipalFrom(r.Context())
		if p.Realm != jwt.AudienceMobile && IsMobilePanelMirror(r.Context()) {
			t.Fatal("mirror principal realm changed")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	handler := MobilePanelMirror(1, 1)(mux)
	for _, tc := range []struct {
		realm, path string
		status      int
	}{
		{"mobile", "/v1/mobile/panel/services", http.StatusNoContent},
		{"panel", "/v1/mobile/panel/services", http.StatusForbidden},
		{"portal", "/v1/mobile/panel/services", http.StatusForbidden},
		{"mobile", "/v1/services", http.StatusForbidden},
		{"panel", "/v1/services", http.StatusNoContent},
	} {
		token, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), Audience: tc.realm})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(MobileAPIVersionHeader, "1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.status || (tc.status == http.StatusForbidden && !strings.Contains(rec.Body.String(), `"code":"REALM_FORBIDDEN"`)) {
			t.Errorf("%s %s = %d %s", tc.realm, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// TEC-939: the SmartPTG calibration routes ride on the existing
// "measurements" prefix (no new mirror entry); a sibling segment is denied.
func TestMobilePanelMirrorTEC939Calibrations(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "measurements/devices/smartptg/calibrations"},
		{http.MethodGet, "measurements/devices/smartptg/calibrations/32475"},
	} {
		req := httptest.NewRequest(c.method, "/v1/mobile/panel/"+c.path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = r.URL.Path == "/v1/"+c.path && r.Method == c.method
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s %s was not mirrored", c.method, c.path)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/panel/measurementsx/devices/smartptg/calibrations", nil)
	rec := httptest.NewRecorder()
	MobilePanelMirror(1, 1)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("sibling prefix reached next")
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sibling = %d", rec.Code)
	}
}

func TestMobilePanelMirrorTEC957Allowed(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"accounting/cari",
		"accounting/cari/" + id,
		"accounting/entries",
		"accounting/categories",
		"accounting/disputes",
		"accounting/disputes/" + id,
		"accounting/disputes/" + id + "/resolve",
		"einvoices",
		"einvoices/" + id,
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			if r.URL.Path != "/v1/"+path {
				t.Errorf("rewrite = %s", r.URL.Path)
			}
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s was not allowed", path)
		}
	}
}

func TestMobilePanelMirrorTEC958Allowed(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"dealer-prices",
		"dealer-prices/catalog",
		"product-sales",
		"product-sales/lookup",
		"product-sales/" + id + "/void",
		"purchases",
		"suppliers",
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			if r.URL.Path != "/v1/"+path {
				t.Errorf("rewrite = %s", r.URL.Path)
			}
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s was not allowed", path)
		}
	}
}

func TestMobilePanelMirrorTEC959Allowed(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"fleets",
		"fleets/" + id,
		"fleets/" + id + "/users",
		"fleets/" + id + "/users/" + id + "/disable",
		"fleets/" + id + "/vehicles/" + id,
		"fleets/" + id + "/statement",
		"fleets/" + id + "/reports",
		"service-subscriptions",
		"service-subscriptions/" + id,
		"service-subscriptions/" + id + "/cancel-request",
		"service-subscriptions/cancel-requests/" + id + "/approve",
		"staff-profiles",
		"staff-profiles/" + id,
		"staff-profiles/" + id + "/payments",
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			if r.URL.Path != "/v1/"+path {
				t.Errorf("rewrite = %s", r.URL.Path)
			}
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s was not allowed", path)
		}
	}
}

// TEC-959 siblings: only the exact segments open; the staff payment book,
// the fleet portal and look-alike segments stay closed.
func TestMobilePanelMirrorTEC959SiblingsDenied(t *testing.T) {
	for _, path := range []string{"fleet", "fleetsx", "fleets-export", "service-subscription", "service-subscriptionsx",
		"service-catalogx", "staff", "staff-profilesx", "staff-payments", "staff-payments/payroll"} {
		for _, suffix := range []string{"", "/child"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path+suffix, nil)
			req.Header.Set(MobileAPIVersionHeader, "1")
			rec := httptest.NewRecorder()
			MobilePanelMirror(1, 1)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatalf("%s reached next", path+suffix)
			})).ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s = %d", path+suffix, rec.Code)
			}
		}
	}
}

func TestMobilePanelMirrorTEC960Allowed(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"campaigns",
		"campaigns/approvals",
		"campaigns/" + id,
		"campaigns/" + id + "/approve",
		"campaigns/" + id + "/request-changes",
		"consents",
		"consents/pending",
		"ai/status",
		"ai/conversations",
		"ai/conversations/" + id + "/messages",
		"ai/actions/" + id + "/confirm",
		"ai/pending-actions",
		"ai/pending-actions/" + id + "/cancel",
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			if r.URL.Path != "/v1/"+path {
				t.Errorf("rewrite = %s", r.URL.Path)
			}
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s was not allowed", path)
		}
	}
}
