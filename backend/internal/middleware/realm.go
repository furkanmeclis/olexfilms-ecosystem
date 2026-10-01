package middleware

import (
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// portalAuthRoutes are the account routes a portal (aud=portal) session may
// call outside /v1/portal/*. Everything else (platform, tenant, organization
// context, impersonation, step-up, realtime, ...) is panel only.
var portalAuthRoutes = []string{
	"/v1/auth/me",
	"/v1/auth/logout",
	"/v1/auth/profile",
	"/v1/auth/password/change",
	"/v1/auth/sessions",
}

// RealmAllows reports whether a token of realm may call path (TEC-90):
//   - /v1/portal/* accepts portal tokens only;
//   - a portal token is accepted on /v1/portal/* and the account routes in
//     portalAuthRoutes only (default deny for every panel route).
func RealmAllows(realm, path string) bool {
	portalRoute := strings.HasPrefix(path, "/v1/portal/")
	if jwt.NormalizeAudience(realm) != jwt.AudiencePortal {
		return !portalRoute
	}
	if portalRoute {
		return true
	}
	for _, p := range portalAuthRoutes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}
