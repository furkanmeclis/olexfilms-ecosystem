// Package portalvehicles mounts the customer portal "my vehicles" reads
// (TEC-238, F2-03a).
package portalvehicles

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the portal vehicle and service reads. A portal
// session (aud=portal; the realm middleware refuses panel tokens on
// /v1/portal/*) with vehicles.read / services.read; the use case returns
// only the signed-in user's records of the domain brand (Glorian closed),
// anything else answers 404.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	with := func(perm string) func(http.HandlerFunc) http.Handler {
		return func(fn http.HandlerFunc) http.Handler {
			return middleware.Chain(fn, authn, middleware.RequirePermission(perm))
		}
	}
	readV, readS := with(rbac.PermVehiclesRead), with(rbac.PermServicesRead)

	mux.Handle("GET /v1/portal/vehicles", readV(h.ListVehicles))
	mux.Handle("GET /v1/portal/vehicles/{uuid}", readV(h.GetVehicle))
	mux.Handle("GET /v1/portal/services", readS(h.ListServices))
	// TEC-245: signed vehicle intake contracts (services.read; a contract
	// belongs to a service). Fleet sessions read it like customers.
	mux.Handle("GET /v1/portal/contracts", readS(h.ListContracts))
}
