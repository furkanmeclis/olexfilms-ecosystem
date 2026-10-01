package handler

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/customers and /v1/vehicles.
//
// Customers need customers.read / customers.write, vehicles vehicles.read /
// vehicles.write; the scope is resolved per request (RequireScope) and the
// use case limits every record to customers linked (customer_organizations)
// to an organization in scope: dealer = its organization, distributor = its
// subtree, center = the brand. Out-of-scope records answer 404.
// upgrade-to-dealer needs customers.write in a center or distributor
// organization.
func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleCustomers)
	with := func(perm string) func(http.HandlerFunc) http.Handler {
		return func(fn http.HandlerFunc) http.Handler {
			return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, perm))
		}
	}
	readC, writeC := with(rbac.PermCustomersRead), with(rbac.PermCustomersWrite)
	readV, writeV := with(rbac.PermVehiclesRead), with(rbac.PermVehiclesWrite)

	mux.Handle("GET /v1/customers", readC(h.ListCustomers))
	mux.Handle("POST /v1/customers", writeC(h.CreateCustomer))
	mux.Handle("GET /v1/customers/{uuid}", readC(h.GetCustomer))
	mux.Handle("PATCH /v1/customers/{uuid}", writeC(h.UpdateCustomer))
	mux.Handle("POST /v1/customers/{uuid}/upgrade-to-dealer", writeC(h.UpgradeToDealer))

	mux.Handle("GET /v1/vehicles", readV(h.ListVehicles))
	mux.Handle("POST /v1/vehicles", writeV(h.CreateVehicle))
	mux.Handle("GET /v1/vehicles/{uuid}", readV(h.GetVehicle))
	mux.Handle("PATCH /v1/vehicles/{uuid}", writeV(h.UpdateVehicle))
	mux.Handle("DELETE /v1/vehicles/{uuid}", writeV(h.DeleteVehicle))
}
