package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
)

// TEC-236 acceptance (F2-05d) against the real server: with
// MOBILE_APP_MIN_VERSION=2.0.0 an older X-App-Version gets 426
// UPDATE_REQUIRED with the store links on the new /v1/mobile/* routes and
// on the legacy aliases, before authentication and the
// X-Mobile-Api-Version check; an equal version passes to the route; a
// legacy alias request without a version passes (the old hub app sends
// none), and the stored system setting overrides the environment minimum.
func TestIntegrationMobileAppVersionGate(t *testing.T) {
	it := newIntegrationWith(t, func(c *config.Config) {
		c.Mobile.LegacyAliases = true
		c.Mobile.MinAPIVersion, c.Mobile.MaxAPIVersion = 1, 1
		c.Mobile.AppMinVersion = "2.0.0"
		c.Mobile.AppStoreURLIOS = "https://apps.apple.com/app/id-t236"
		c.Mobile.AppStoreURLAndroid = "https://play.google.com/store/apps/details?id=t236"
		c.Mobile.AppUserAgentProducts = []string{"OlexFilms"}
	})
	ctx := context.Background()
	settings := []string{sysconfig.KeyMobileAppMinVersion, sysconfig.KeyMobileAppStoreURLIOS}
	t.Cleanup(func() {
		for _, k := range settings {
			_, _ = it.srv.sysconfig.Reset(context.Background(), k)
		}
	})

	send := func(method, path string, headers map[string]string) (int, envelope, middleware.UpdateRequiredData) {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Host = "backend:8080"
		req.Header.Set("X-Forwarded-Host", hostOlex)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		it.handler.ServeHTTP(rec, req)
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var data middleware.UpdateRequiredData
		if rec.Code == http.StatusUpgradeRequired {
			_ = json.Unmarshal(env.Data, &data)
		}
		return rec.Code, env, data
	}
	old := map[string]string{middleware.AppVersionHeader: "1.9.9", middleware.MobileAPIVersionHeader: "1"}

	// 1. Below the minimum: 426 UPDATE_REQUIRED + store links, on a new
	// route (no token needed: the gate runs first) and on an alias.
	for _, path := range []string{"/v1/mobile/auth/me", "/v1/mobile/legacy/auth/me", "/v1/mobile/legacy/auth/login"} {
		code, env, data := send(http.MethodGet, path, old)
		if path == "/v1/mobile/legacy/auth/login" {
			code, env, data = send(http.MethodPost, path, old)
		}
		if code != http.StatusUpgradeRequired || errCode(env) != "UPDATE_REQUIRED" {
			t.Fatalf("%s = %d %s, want 426 UPDATE_REQUIRED", path, code, errCode(env))
		}
		if data.MinVersion != "2.0.0" || data.CurrentVersion == nil || *data.CurrentVersion != "1.9.9" ||
			data.StoreURLs.IOS == nil || *data.StoreURLs.IOS != "https://apps.apple.com/app/id-t236" ||
			data.StoreURLs.Android == nil || *data.StoreURLs.Android != "https://play.google.com/store/apps/details?id=t236" {
			t.Fatalf("%s data = %s", path, env.Data)
		}
	}
	// The User-Agent token counts when the header is missing.
	if code, env, _ := send(http.MethodGet, "/v1/mobile/auth/me", map[string]string{
		"User-Agent": "OlexFilms/1.0.0 (Android 14) okhttp/4.12.0", middleware.MobileAPIVersionHeader: "1",
	}); code != http.StatusUpgradeRequired {
		t.Fatalf("UA 1.0.0 = %d %s, want 426", code, errCode(env))
	}

	// 2. Equal version passes the gate: the route's own checks answer.
	if code, env, _ := send(http.MethodGet, "/v1/mobile/auth/me", map[string]string{
		middleware.AppVersionHeader: "2.0.0", middleware.MobileAPIVersionHeader: "1",
	}); code != http.StatusUnauthorized {
		t.Fatalf("2.0.0 /v1/mobile/auth/me = %d %s, want 401 (gate passed, no token)", code, errCode(env))
	}
	// A newer app with an unsupported contract still gets the TEC-91 426.
	if code, env, _ := send(http.MethodGet, "/v1/mobile/auth/me", map[string]string{
		middleware.AppVersionHeader: "2.1.0", middleware.MobileAPIVersionHeader: "9",
	}); code != http.StatusUpgradeRequired || errCode(env) != "MOBILE_API_VERSION_UNSUPPORTED" {
		t.Fatalf("contract 9 = %d %s, want 426 MOBILE_API_VERSION_UNSUPPORTED", code, errCode(env))
	}

	// 3. A legacy alias request without any version passes the gate
	// (MOBILE_APP_VERSION_REQUIRED off): me without a token is 401.
	if code, env, _ := send(http.MethodGet, "/v1/mobile/legacy/auth/me", map[string]string{"User-Agent": "OlexLegacy/1.9"}); code != http.StatusUnauthorized {
		t.Fatalf("headerless alias = %d %s, want 401 (gate passed)", code, errCode(env))
	}

	// 4. The system setting overrides the environment minimum and store
	// link; an unset link keeps the environment value.
	if _, err := it.srv.sysconfig.Set(ctx, sysconfig.KeyMobileAppMinVersion, json.RawMessage(`"3.0.0"`), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := it.srv.sysconfig.Set(ctx, sysconfig.KeyMobileAppStoreURLIOS, json.RawMessage(`"https://apps.apple.com/app/id-db"`), 0); err != nil {
		t.Fatal(err)
	}
	code, env, data := send(http.MethodGet, "/v1/mobile/auth/me", map[string]string{
		middleware.AppVersionHeader: "2.5.0", middleware.MobileAPIVersionHeader: "1",
	})
	if code != http.StatusUpgradeRequired || data.MinVersion != "3.0.0" ||
		data.StoreURLs.IOS == nil || *data.StoreURLs.IOS != "https://apps.apple.com/app/id-db" ||
		data.StoreURLs.Android == nil || *data.StoreURLs.Android != "https://play.google.com/store/apps/details?id=t236" {
		t.Fatalf("setting override = %d %s %s", code, errCode(env), env.Data)
	}
	if code, env, _ := send(http.MethodGet, "/v1/mobile/auth/me", map[string]string{
		middleware.AppVersionHeader: "3.0.0", middleware.MobileAPIVersionHeader: "1",
	}); code != http.StatusUnauthorized {
		t.Fatalf("3.0.0 after override = %d %s, want 401", code, errCode(env))
	}
}
