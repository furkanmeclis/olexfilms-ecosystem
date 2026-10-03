package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

type updateRequiredBody struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Data UpdateRequiredData `json:"data"`
}

func gateHandler(p AppVersionPolicy) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return ResolveLocale(MobileAppVersion(func(*http.Request) AppVersionPolicy { return p }, []string{"OlexFilms"})(ok))
}

func gateDo(h http.Handler, method, path string, headers map[string]string) (*httptest.ResponseRecorder, updateRequiredBody) {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body updateRequiredBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

var gatePolicy = AppVersionPolicy{
	MinVersion:      "2.4",
	StoreURLIOS:     "https://apps.apple.com/app/id1",
	StoreURLAndroid: "https://play.google.com/store/apps/details?id=com.olexfilms",
}

// TEC-236 acceptance: a release below the minimum gets 426 UPDATE_REQUIRED
// with the store links, on /v1/mobile/* and on the legacy aliases.
func TestMobileAppVersionBelowMinimum(t *testing.T) {
	h := gateHandler(gatePolicy)
	for _, path := range []string{"/v1/mobile/auth/me", "/v1/mobile/legacy/auth/login", "/v1/mobile/no-such-route"} {
		for _, hdr := range []map[string]string{
			{AppVersionHeader: "2.3.9"},
			{AppVersionHeader: "v1"},
			{"User-Agent": "OlexFilms/2.3.0 (iPhone; iOS 17.4) CFNetwork/1494"},
		} {
			rec, body := gateDo(h, http.MethodPost, path, hdr)
			if rec.Code != http.StatusUpgradeRequired || body.Success || body.Error.Code != "UPDATE_REQUIRED" {
				t.Fatalf("%s %v = %d %+v", path, hdr, rec.Code, body)
			}
			d := body.Data
			if d.MinVersion != "2.4.0" || d.CurrentVersion == nil ||
				d.StoreURLs.IOS == nil || *d.StoreURLs.IOS != gatePolicy.StoreURLIOS ||
				d.StoreURLs.Android == nil || *d.StoreURLs.Android != gatePolicy.StoreURLAndroid {
				t.Fatalf("%s %v data = %+v", path, hdr, d)
			}
			if body.Error.Message != i18n.Translate(i18n.LocaleTR, "mobile.update_required") {
				t.Fatalf("default message = %q", body.Error.Message)
			}
		}
	}
}

func TestMobileAppVersionLocalizedMessage(t *testing.T) {
	h := gateHandler(gatePolicy)
	_, body := gateDo(h, http.MethodGet, "/v1/mobile/auth/me", map[string]string{AppVersionHeader: "1.0.0", "Accept-Language": "de-DE,de;q=0.9"})
	if want := i18n.Translate(i18n.LocaleDE, "mobile.update_required"); body.Error.Message != want || want == "mobile.update_required" {
		t.Fatalf("de message = %q, want %q", body.Error.Message, want)
	}
}

// TEC-236 acceptance: an equal or newer release passes.
func TestMobileAppVersionAtOrAboveMinimumPasses(t *testing.T) {
	h := gateHandler(gatePolicy)
	for _, hdr := range []map[string]string{
		{AppVersionHeader: "2.4"},
		{AppVersionHeader: "2.4.0"},
		{AppVersionHeader: "2.4.0+77"},
		{AppVersionHeader: "2.10.1"},
		{AppVersionHeader: "3"},
		{"User-Agent": "OlexFilms/2.4.1 okhttp/4.12.0"},
	} {
		for _, path := range []string{"/v1/mobile/auth/me", "/v1/mobile/legacy/services"} {
			if rec, _ := gateDo(h, http.MethodGet, path, hdr); rec.Code != http.StatusNoContent {
				t.Errorf("%s %v = %d, want pass", path, hdr, rec.Code)
			}
		}
	}
}

// TEC-236 acceptance: the behavior of an alias request without a version
// (the old hub app sends neither X-App-Version nor an app User-Agent token)
// is pinned: it passes by default and gets 426 (current_version null) when
// VersionRequired is on. A malformed header counts as unknown.
func TestMobileAppVersionUnknownVersion(t *testing.T) {
	unknown := []map[string]string{
		{},
		{"User-Agent": "okhttp/4.12.0"},
		{"User-Agent": "OlexLegacy/1.9"},
		{AppVersionHeader: "latest"},
	}
	open := gateHandler(gatePolicy)
	for _, hdr := range unknown {
		if rec, _ := gateDo(open, http.MethodPost, "/v1/mobile/legacy/auth/login", hdr); rec.Code != http.StatusNoContent {
			t.Errorf("default %v = %d, want pass", hdr, rec.Code)
		}
	}
	strict := gatePolicy
	strict.VersionRequired = true
	sh := gateHandler(strict)
	for _, hdr := range unknown {
		rec, body := gateDo(sh, http.MethodPost, "/v1/mobile/legacy/auth/login", hdr)
		if rec.Code != http.StatusUpgradeRequired || body.Error.Code != "UPDATE_REQUIRED" || body.Data.CurrentVersion != nil {
			t.Errorf("required %v = %d %+v", hdr, rec.Code, body)
		}
	}
}

func TestMobileAppVersionScopeAndDisabled(t *testing.T) {
	h := gateHandler(gatePolicy)
	// Outside /v1/mobile/* the gate never applies.
	for _, path := range []string{"/v1/auth/me", "/v1/services", "/v1/mobile", "/healthz"} {
		if rec, _ := gateDo(h, http.MethodGet, path, map[string]string{AppVersionHeader: "1.0.0"}); rec.Code != http.StatusNoContent {
			t.Errorf("%s = %d, want pass", path, rec.Code)
		}
	}
	// No (or an invalid) minimum: no gate, even with VersionRequired.
	for _, minV := range []string{"", "not-a-version"} {
		off := gateHandler(AppVersionPolicy{MinVersion: minV, VersionRequired: true})
		if rec, _ := gateDo(off, http.MethodGet, "/v1/mobile/auth/me", map[string]string{AppVersionHeader: "0.1"}); rec.Code != http.StatusNoContent {
			t.Errorf("min %q = %d, want pass", minV, rec.Code)
		}
	}
	// Store links not configured are null.
	bare := gateHandler(AppVersionPolicy{MinVersion: "2.0.0"})
	rec, body := gateDo(bare, http.MethodGet, "/v1/mobile/auth/me", map[string]string{AppVersionHeader: "1.0.0"})
	if rec.Code != http.StatusUpgradeRequired || body.Data.StoreURLs.IOS != nil || body.Data.StoreURLs.Android != nil {
		t.Fatalf("bare = %d %+v", rec.Code, body.Data)
	}
}
