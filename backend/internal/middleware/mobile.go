package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/appversion"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// MobileAPIVersionHeader carries the mobile API contract version (TEC-91).
const MobileAPIVersionHeader = "X-Mobile-Api-Version"

// ParseMobileAPIVersion reads the integer major version of the header
// ("1", "1.4" and "v1" are major 1). ok is false for a missing or malformed
// value.
func ParseMobileAPIVersion(raw string) (int, bool) {
	raw = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "v")
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		raw = raw[:i]
	}
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// MobileAPIVersion guards /v1/mobile/*: a missing or unsupported
// X-Mobile-Api-Version answers 426 MOBILE_API_VERSION_UNSUPPORTED, so an old
// app asks the user to update instead of failing on a changed contract. The
// response always names the newest supported version in the same header.
func MobileAPIVersion(minVersion, maxVersion int) func(http.Handler) http.Handler {
	if minVersion <= 0 {
		minVersion = 1
	}
	if maxVersion < minVersion {
		maxVersion = minVersion
	}
	current := strconv.Itoa(maxVersion)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(MobileAPIVersionHeader, current)
			v, ok := ParseMobileAPIVersion(r.Header.Get(MobileAPIVersionHeader))
			if !ok || v < minVersion || v > maxVersion {
				response.ErrorWithDetails(w, r, http.StatusUpgradeRequired, response.CodeMobileAPIVersionUnsupported,
					"This app version is no longer supported. Update the app.",
					[]response.Detail{{
						Field:   MobileAPIVersionHeader,
						Message: "supported versions: " + strconv.Itoa(minVersion) + "-" + current,
						Code:    "unsupported_version",
					}})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AppVersionHeader carries the mobile app release ("2.4.1"), unlike
// X-Mobile-Api-Version, which is the API contract major (TEC-236).
const AppVersionHeader = "X-App-Version"

// MobilePathPrefix is the root the app version gate applies to; the legacy
// aliases (/v1/mobile/legacy/*) are under it too.
const MobilePathPrefix = "/v1/mobile/"

// AppVersionPolicy is the effective gate of one request. An empty or
// unparsable MinVersion disables the gate.
type AppVersionPolicy struct {
	MinVersion      string
	StoreURLIOS     string
	StoreURLAndroid string
	// VersionRequired refuses a request whose version cannot be read.
	VersionRequired bool
}

// UpdateRequiredData is the data of a 426 UPDATE_REQUIRED.
type UpdateRequiredData struct {
	MinVersion     string         `json:"min_version"`
	CurrentVersion *string        `json:"current_version"`
	StoreURLs      AppStoreURLSet `json:"store_urls"`
}

// AppStoreURLSet holds the store links (null when not configured).
type AppStoreURLSet struct {
	IOS     *string `json:"ios"`
	Android *string `json:"android"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// RequestAppVersion reads the app release from X-App-Version, else from a
// User-Agent token of one of uaProducts ("OlexFilms/2.4.1 ..."). A present
// but malformed header counts as unknown.
func RequestAppVersion(r *http.Request, uaProducts []string) (appversion.Version, bool) {
	if raw := strings.TrimSpace(r.Header.Get(AppVersionHeader)); raw != "" {
		return appversion.Parse(raw)
	}
	return appversion.FromUserAgent(r.UserAgent(), uaProducts)
}

// MobileAppVersion is the app version gate (TEC-236, F2-05d). It wraps the
// whole router and acts on /v1/mobile/* only, legacy aliases included, so
// it runs before routing, authentication and the X-Mobile-Api-Version
// check: an app below the minimum gets 426 UPDATE_REQUIRED, the localized
// message (i18n "mobile.update_required", Accept-Language) and the store
// links, whatever route it calls. An app at or above the minimum passes.
// An app whose version cannot be read (no header, no known User-Agent
// token: the old hub app) passes unless VersionRequired is on.
//
// policy is read per request (system settings with a short cache, env
// fallback), so changing the minimum needs no restart.
func MobileAppVersion(policy func(*http.Request) AppVersionPolicy, uaProducts []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, MobilePathPrefix) || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			p := policy(r)
			minV, gated := appversion.Parse(p.MinVersion)
			if !gated {
				next.ServeHTTP(w, r)
				return
			}
			v, known := RequestAppVersion(r, uaProducts)
			if (known && appversion.Compare(v, minV) >= 0) || (!known && !p.VersionRequired) {
				next.ServeHTTP(w, r)
				return
			}
			data := UpdateRequiredData{
				MinVersion: minV.String(),
				StoreURLs:  AppStoreURLSet{IOS: optional(p.StoreURLIOS), Android: optional(p.StoreURLAndroid)},
			}
			if known {
				data.CurrentVersion = optional(v.String())
			}
			loc := i18n.FromContext(r.Context()).Locale
			response.ErrorWithData(w, r, http.StatusUpgradeRequired, response.CodeUpdateRequired,
				i18n.Translate(loc, "mobile.update_required"),
				[]response.Detail{{
					Field:   AppVersionHeader,
					Message: "minimum version: " + data.MinVersion,
					Code:    "update_required",
				}}, data)
		})
	}
}
