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

func TestMobilePanelMirrorDenied(t *testing.T) {
	paths := []string{"", "auth", "platform", "portal", "public", "mobile", "oauth", "realtime", "ai",
		"accounting", "impersonation", "unknown", "ordersx", "tenant", "tenant/settings", "tenant/organizationsx",
		"tenant/pricing-x", "tenant/other", "certificate-warningsx",
		// TEC-958 siblings: the dealer sales prefixes match whole segments only.
		"dealer-pricesx", "dealer-price", "dealer-sales", "product-salesx", "product", "purchasesx",
		"purchase", "suppliersx", "supplier", "accounting/entries", "dealer-accounting"}
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
