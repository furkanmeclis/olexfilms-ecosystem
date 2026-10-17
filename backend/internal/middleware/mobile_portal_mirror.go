package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// MobilePortalMirrorPrefix is the mobile path of the customer portal:
// /v1/mobile/portal/<rest> serves /v1/portal/<rest>.
const MobilePortalMirrorPrefix = "/v1/mobile/portal/"

// MobilePortalMirrorDenied are the portal path segments the mirror does not
// expose (the OAuth consent flow and the AI chat stay web only). Each entry
// matches itself and its descendants at a slash boundary.
var MobilePortalMirrorDenied = []string{"oauth", "ai"}

// mobileCustomerRoutes are the only non-mirrored routes a customer mobile
// session (mobile_kind=customer) may call: its own account and push token.
var mobileCustomerRoutes = []string{
	"/v1/mobile/auth/me",
	"/v1/mobile/auth/logout",
	"/v1/mobile/auth/logout-all",
	"/v1/mobile/auth/refresh",
	"/v1/mobile/push-token",
}

type mobilePortalMirrorKey struct{}

// IsMobilePortalMirror reports whether the request was rewritten by
// MobilePortalMirror.
func IsMobilePortalMirror(ctx context.Context) bool {
	marked, _ := ctx.Value(mobilePortalMirrorKey{}).(bool)
	return marked
}

// MobileCustomerRealm applies the customer mobile session rules before the
// path realm rule (RealmAllows) and reports whether the request may go on:
//   - a request rewritten by MobilePortalMirror accepts a customer mobile
//     token only; claims then become a portal session (aud=portal), so the
//     portal routes and the principal (Realm) see exactly a portal session;
//   - a customer mobile token is accepted on mobileCustomerRoutes only (every
//     other /v1/mobile/* route, the panel mirror included, is refused);
//   - every other token is left to RealmAllows.
func MobileCustomerRealm(claims *jwt.Claims, r *http.Request) bool {
	customer := claims.SessionKind() == jwt.MobileKindCustomer
	if IsMobilePortalMirror(r.Context()) {
		// The mirror only ever targets /v1/portal/*; never let the portal
		// realm reach the portal account routes (/v1/auth/*) this way.
		if !customer || !strings.HasPrefix(r.URL.Path, "/v1/portal/") {
			return false
		}
		claims.Audience = jwtlib.ClaimStrings{jwt.AudiencePortal}
		claims.MobileKind = ""
		return true
	}
	if !customer {
		return true
	}
	for _, p := range mobileCustomerRoutes {
		if r.URL.Path == p {
			return true
		}
	}
	return false
}

// MobilePortalMirror serves /v1/mobile/portal/<rest> as /v1/portal/<rest>
// through next (the server mux, so the portal routes keep their whole
// chain: Authenticate, feature, read-only and permission gates). It is
// registered on the mux under MobilePortalMirrorPrefix, inside the app
// version gate, which therefore sees the original mobile path. It requires
// X-Mobile-Api-Version and a customer mobile Bearer token (401 without a
// valid token, 403 REALM_FORBIDDEN for any other session); Authenticate
// checks the token again on the rewritten request.
func MobilePortalMirror(tokens *jwt.Manager, minVersion, maxVersion int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		rewrite := MobileAPIVersion(minVersion, maxVersion)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				response.Unauthorized(w, r, "Bearer access token is required")
				return
			}
			claims, err := tokens.ParseAccess(raw)
			if err != nil {
				response.Unauthorized(w, r, "Invalid or expired access token")
				return
			}
			if claims.SessionKind() != jwt.MobileKindCustomer {
				response.Error(w, r, http.StatusForbidden, response.CodeRealmForbidden,
					"This session cannot access this resource")
				return
			}
			mirrored := r.Clone(context.WithValue(r.Context(), mobilePortalMirrorKey{}, true))
			mirrored.URL.Path = "/v1/portal/" + strings.TrimPrefix(r.URL.Path, MobilePortalMirrorPrefix)
			if r.URL.RawPath != "" {
				// Keep escaped identifiers when the prefix is literal; otherwise
				// let net/url re-encode the new path.
				mirrored.URL.RawPath = ""
				if rest, ok := strings.CutPrefix(r.URL.EscapedPath(), MobilePortalMirrorPrefix); ok {
					mirrored.URL.RawPath = "/v1/portal/" + rest
				}
			}
			next.ServeHTTP(w, mirrored)
		}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rest, ok := strings.CutPrefix(r.URL.Path, MobilePortalMirrorPrefix)
			if !ok || rest == "" || hasDotSegment(rest) {
				response.NotFound(w, r, "Resource not found")
				return
			}
			for _, denied := range MobilePortalMirrorDenied {
				if rest == denied || strings.HasPrefix(rest, denied+"/") {
					response.NotFound(w, r, "Resource not found")
					return
				}
			}
			rewrite.ServeHTTP(w, r)
		})
	}
}

// hasDotSegment reports a "." or ".." path segment (the rewritten path must
// stay under /v1/portal/).
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}
