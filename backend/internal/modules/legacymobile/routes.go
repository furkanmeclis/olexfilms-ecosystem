// Package legacymobile is the temporary compatibility layer for the mobile
// app of the old hub (TEC-234, F2-05b). F5'te kaldırılır.
//
// Every alias is an adapter over a handler that already serves the new
// contract (mobile auth, services, push token, measurements); no business
// rule lives here. The aliases are mounted only when MOBILE_LEGACY_ALIASES
// is on (default off), otherwise every path below answers 404.
//
// Path decision (conservative, see the TEC-234 Linear comment): the old hub
// contract (olexfilms docs/mobile-api.md) was not reachable when this was
// built, so the aliases live under their own prefix, /v1/mobile/legacy/*,
// instead of guessing the old paths at the root. That keeps them clear of
// the web /v1/auth/* and the new /v1/mobile/* routes, and still inside
// /v1/mobile/* so the BFF passthrough (/api/v1/mobile/*) forwards them and
// a mobile token (aud=mobile) is accepted (middleware.RealmAllows). The old
// app is pointed at https://<host>/api/v1/mobile/legacy as its base URL.
//
// Differences from the adapted routes:
//   - no X-Mobile-Api-Version gate: the old app does not send the header
//     (version forcing is TEC-236: middleware.MobileAppVersion wraps the
//     router, so the X-App-Version minimum applies here too; a request
//     without a version passes unless mobile.app_version_required is on);
//   - the request and response bodies are those of the new contract until
//     the old shapes are confirmed; the mapping goes in this package, one
//     adapter per route (testdata/legacy_mobile/*.json pins the contract).
package legacymobile

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// Prefix is the root of every legacy alias.
const Prefix = "/v1/mobile/legacy"

// Handlers are the existing handlers the aliases adapt. A nil handler leaves
// its alias unmounted (404).
type Handlers struct {
	Login             http.HandlerFunc // auth MobileHandler.Login
	Me                http.HandlerFunc // auth MobileHandler.Me
	ListServices      http.HandlerFunc // services Handler.List
	GetService        http.HandlerFunc // services Handler.Get
	CreateMeasurement http.HandlerFunc // measurements Handler.Create
	PutPushToken      http.HandlerFunc // auth MobileHandler.PutPushToken
	DeletePushToken   http.HandlerFunc // auth MobileHandler.DeletePushToken
}

// Gates of the aliases: the adapted route's gate minus the version header.
const (
	GatePublic       = "public"       // no token (login)
	GateMobile       = "mobile"       // mobile Bearer
	GateServices     = "services"     // + organization, services module, services.read scope
	GateMeasurements = "measurements" // + organization, measurements.write scope
)

// Route is one alias: the method and path, the new route it adapts and its
// gate. Name matches the contract fixture testdata/legacy_mobile/<name>.json.
type Route struct {
	Name   string
	Method string
	Path   string
	Target string
	Gate   string
}

// Routes lists the aliases in mount order.
var Routes = []Route{
	{"login", "POST", Prefix + "/auth/login", "POST /v1/mobile/auth/login", GatePublic},
	{"me", "GET", Prefix + "/auth/me", "GET /v1/mobile/auth/me", GateMobile},
	{"services_list", "GET", Prefix + "/services", "GET /v1/services", GateServices},
	{"service_detail", "GET", Prefix + "/services/{uuid}", "GET /v1/services/{uuid}", GateServices},
	{"measurement", "POST", Prefix + "/measurements", "POST /v1/mobile/measurements", GateMeasurements},
	{"push_token", "PUT", Prefix + "/push-token", "PUT /v1/mobile/push-token", GateMobile},
	{"push_token_delete", "DELETE", Prefix + "/push-token", "DELETE /v1/mobile/push-token", GateMobile},
}

func (h Handlers) byName(name string) http.HandlerFunc {
	switch name {
	case "login":
		return h.Login
	case "me":
		return h.Me
	case "services_list":
		return h.ListServices
	case "service_detail":
		return h.GetService
	case "measurement":
		return h.CreateMeasurement
	case "push_token":
		return h.PutPushToken
	case "push_token_delete":
		return h.DeletePushToken
	}
	return nil
}

// RegisterRoutes mounts the aliases when enabled (MOBILE_LEGACY_ALIASES).
// A service out of the caller's services.read scope answers 404, as on
// /v1/services.
func RegisterRoutes(
	mux *http.ServeMux,
	enabled bool,
	h Handlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	if !enabled {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	gates := map[string][]func(http.Handler) http.Handler{
		GatePublic: nil,
		GateMobile: {authn},
		GateServices: {authn, org,
			middleware.RequireFeature(checker, features.ModuleServices),
			middleware.RequireScope(q, rbac.PermServicesRead)},
		GateMeasurements: {authn, org, middleware.RequireScope(q, rbac.PermMeasurementsWrite)},
	}
	for _, r := range Routes {
		fn := h.byName(r.Name)
		if fn == nil {
			continue
		}
		mux.Handle(r.Method+" "+r.Path, middleware.Chain(fn, gates[r.Gate]...))
	}
}
