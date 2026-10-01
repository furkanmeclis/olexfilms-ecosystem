package middleware

import (
	"net/http"
	"strconv"
	"strings"

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
