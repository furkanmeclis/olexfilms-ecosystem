// Package legacymobile is the temporary compatibility layer for the mobile
// app of the old hub (TEC-234, F2-05b; contract mapped in TEC-284,
// F2-05b2). F5'te kaldırılır.
//
// Every alias is an adapter over a handler that already serves the new
// contract (mobile auth, services, push token, measurements); no business
// rule lives here. The aliases are mounted only when MOBILE_LEGACY_ALIASES
// is on (default off), otherwise every path below answers 404.
//
// Paths: the old hub served its mobile API at /api/v1/mobile (olexfilms
// routes/api.php, docs/mobile-api.md "Base URL"). The same paths at the
// root would collide with the new /v1/mobile/* routes (auth/login, auth/me,
// push-token), so the aliases keep the old paths one level down, under
// /v1/mobile/legacy: the old app only changes its base URL to
// https://<host>/api/v1/mobile/legacy (the BFF passthrough /api/v1/mobile/*
// forwards it and a mobile token, aud=mobile, is accepted).
//
// Bodies: requests and answers are the old shapes (auth.go, services.go,
// reports.go) inside the old envelope (envelope.go);
// testdata/legacy_mobile/*.json pins them, derived from the old
// controllers and resources.
//
// No X-Mobile-Api-Version gate: the old app does not send the header
// (version forcing is TEC-236: middleware.MobileAppVersion wraps the
// router, so the X-App-Version minimum applies here too; a request without
// a version passes unless mobile.app_version_required is on).
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
	Login http.HandlerFunc // auth MobileHandler.Login
	Me    http.HandlerFunc // auth MobileHandler.Me
	// SwitchOrganization (auth MobileHandler.SwitchOrganization) picks the
	// organization of a legacy login, whose body has no organization_slug.
	SwitchOrganization http.HandlerFunc
	ListServices       http.HandlerFunc // services Handler.List
	GetService         http.HandlerFunc // services Handler.Get
	CreateMeasurement  http.HandlerFunc // measurements Handler.Create
	PutPushToken       http.HandlerFunc // auth MobileHandler.PutPushToken
	DeletePushToken    http.HandlerFunc // auth MobileHandler.DeletePushToken
}

// Gates of the aliases: the adapted route's gate minus the version header.
const (
	GatePublic       = "public"       // no token (login)
	GateMobile       = "mobile"       // mobile Bearer
	GateServices     = "services"     // + organization, services module, services.read scope
	GateMeasurements = "measurements" // + organization, measurements.write scope
)

// Route is one alias: the method and the old path (under Prefix), the new
// route it adapts and its gate. Name matches the contract fixture
// testdata/legacy_mobile/<name>.json.
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
	{"service_detail", "GET", Prefix + "/services/{service}", "GET /v1/services/{uuid}", GateServices},
	{"measurement", "POST", Prefix + "/nexptg-reports", "POST /v1/mobile/measurements", GateMeasurements},
	{"push_token", "PUT", Prefix + "/push-token", "PUT /v1/mobile/push-token", GateMobile},
	{"push_token_delete", "DELETE", Prefix + "/push-token", "DELETE /v1/mobile/push-token", GateMobile},
}

// byName returns the adapted handler of an alias and the adapter that
// serves it in the old shape; a nil adapted handler leaves the alias
// unmounted.
func (a *adapters) byName(name string) (target, adapter http.HandlerFunc) {
	switch name {
	case "login":
		return a.h.Login, a.login
	case "me":
		return a.h.Me, a.me
	case "services_list":
		return a.h.ListServices, a.listServices
	case "service_detail":
		return a.h.GetService, a.getService
	case "measurement":
		return a.h.CreateMeasurement, a.storeReport
	case "push_token":
		return a.h.PutPushToken, a.putPushToken
	case "push_token_delete":
		return a.h.DeletePushToken, a.deletePushToken
	}
	return nil, nil
}

// RegisterRoutes mounts the aliases when enabled (MOBILE_LEGACY_ALIASES).
// A service out of the caller's services.read scope answers 404, as on
// /v1/services. Each alias is envelope(gates(adapter)): the adapter runs
// with the gates' identity and organization, and every error (gate or
// adapted handler) leaves in the old error envelope.
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
	a := &adapters{h: h, authn: authn}
	if q != nil {
		a.store = q
	}
	for _, r := range Routes {
		target, adapter := a.byName(r.Name)
		if target == nil {
			continue
		}
		mux.Handle(r.Method+" "+r.Path, envelope(r.Name, middleware.Chain(adapter, gates[r.Gate]...)))
	}
}
