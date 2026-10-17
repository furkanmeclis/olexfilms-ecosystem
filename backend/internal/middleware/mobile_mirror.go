package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// MobilePanelMirrorPrefixes is the allowlist of panel path segments exposed to
// mobile sessions. Each entry matches itself and descendants at a slash boundary;
// tenant/organizations is the only two-segment prefix. All other prefixes are denied.
var MobilePanelMirrorPrefixes = []string{
	"services", "service-pdfs", "warranty-certificates", "customers", "vehicles",
	"warranties", "warranty-claims", "orders", "quotes", "stock", "stock-transfers",
	"warehouse", "measurements", "reports", "search", "notifications", "appointments",
	"catalog", "vehicle-catalog", "plate-formats", "performance", "tasks", "leads",
	"pricing", "certificates", "photo-standard", "service-catalog", "geo",
	"announcements", "stats", "tenant/organizations",
}

type mobilePanelMirrorKey struct{}

// IsMobilePanelMirror reports whether the request was rewritten by MobilePanelMirror.
func IsMobilePanelMirror(ctx context.Context) bool {
	marked, _ := ctx.Value(mobilePanelMirrorKey{}).(bool)
	return marked
}

// MobilePanelMirror serves allowlisted panel routes through the mobile API while
// preserving their existing authentication, organization, permission and feature gates.
// Place it inside MobileAppVersion so that gate sees the original mobile path.
func MobilePanelMirror(minVersion, maxVersion int) func(http.Handler) http.Handler {
	const prefix = "/v1/mobile/panel/"
	return func(next http.Handler) http.Handler {
		rewrite := MobileAPIVersion(minVersion, maxVersion)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mirrored := r.Clone(context.WithValue(r.Context(), mobilePanelMirrorKey{}, true))
			mirrored.URL.Path = "/v1/" + strings.TrimPrefix(r.URL.Path, prefix)
			if r.URL.RawPath != "" {
				// Preserve escaped resource identifiers when the prefix is literal. If the
				// prefix itself was escaped, regenerate a valid encoding of the new path.
				mirrored.URL.RawPath = ""
				if rest, ok := strings.CutPrefix(r.URL.EscapedPath(), prefix); ok {
					mirrored.URL.RawPath = "/v1/" + rest
				}
			}
			next.ServeHTTP(w, mirrored)
		}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
			rest := strings.TrimPrefix(r.URL.Path, prefix)
			for _, allowed := range MobilePanelMirrorPrefixes {
				if rest == allowed || strings.HasPrefix(rest, allowed+"/") {
					rewrite.ServeHTTP(w, r)
					return
				}
			}
			response.NotFound(w, r, "Resource not found")
		})
	}
}
