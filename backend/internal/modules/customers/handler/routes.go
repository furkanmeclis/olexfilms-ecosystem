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
//
// TEC-161 (K19): anonymization and the personal data export need
// customers.anonymize (center roles only, sensitive) and a fresh step-up;
// the use case also requires a center organization. The portal export
// (/v1/portal/me/data-export) is the signed-in customer's own data.
//
// TEC-193: the customer merge needs customers.merge (center roles only,
// sensitive); the preview (dry run) writes nothing, the apply also needs a
// fresh step-up. The use case requires a center organization.
func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
	stepUp middleware.StepUpChecker,
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
	privacy := with(rbac.PermCustomersAnonymize)
	privacyStepUp := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermCustomersAnonymize),
			middleware.RequireStepUp(stepUp))
	}
	mergePreview := with(rbac.PermCustomersMerge)
	mergeStepUp := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermCustomersMerge),
			middleware.RequireStepUp(stepUp))
	}
	portal := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermCustomersRead))
	}

	mux.Handle("GET /v1/customers", readC(h.ListCustomers))
	mux.Handle("POST /v1/customers", writeC(h.CreateCustomer))
	mux.Handle("GET /v1/customers/{uuid}", readC(h.GetCustomer))
	mux.Handle("PATCH /v1/customers/{uuid}", writeC(h.UpdateCustomer))
	mux.Handle("POST /v1/customers/{uuid}/upgrade-to-dealer", writeC(h.UpgradeToDealer))
	mux.Handle("POST /v1/customers/{uuid}/anonymize", privacyStepUp(h.AnonymizeCustomer))
	mux.Handle("POST /v1/customers/{uuid}/data-export", privacyStepUp(h.RequestDataExport))
	mux.Handle("POST /v1/customers/{uuid}/merge/preview", mergePreview(h.PreviewMerge))
	mux.Handle("POST /v1/customers/{uuid}/merge", mergeStepUp(h.MergeCustomer))
	mux.Handle("GET /v1/customer-data-exports/{uuid}", privacy(h.GetDataExport))
	mux.Handle("GET /v1/customer-data-exports/{uuid}/download", privacy(h.DownloadDataExport))

	mux.Handle("POST /v1/portal/me/data-export", portal(h.RequestPortalDataExport))
	mux.Handle("GET /v1/portal/exports/{uuid}", portal(h.GetPortalExport))
	mux.Handle("GET /v1/portal/exports/{uuid}/download", portal(h.DownloadPortalExport))

	mux.Handle("GET /v1/vehicles", readV(h.ListVehicles))
	mux.Handle("POST /v1/vehicles", writeV(h.CreateVehicle))
	mux.Handle("GET /v1/vehicles/{uuid}", readV(h.GetVehicle))
	mux.Handle("PATCH /v1/vehicles/{uuid}", writeV(h.UpdateVehicle))
	mux.Handle("DELETE /v1/vehicles/{uuid}", writeV(h.DeleteVehicle))
}
