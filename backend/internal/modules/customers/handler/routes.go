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
//
// TEC-190: the vehicle transfer endpoints need vehicles.transfer; the
// transfer's current owner must be a customer in scope.
//
// TEC-243: the portal transfer endpoints (/v1/portal/vehicles/{uuid}/transfers,
// /v1/portal/vehicle-transfers/{uuid}/verify|cancel) run the same flow for
// the signed-in customer (vehicles.read, customer scope; the customer role
// has no vehicles.transfer). The use case limits them to the user's own
// vehicles and the transfers the user started; anything else is 404.
//
// TEC-164: GET /v1/customers?q= searches the Meilisearch customers index
// (filtered on the scope) when it is up; the list export needs
// customers.read with the list's scope.
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
	transferV := with(rbac.PermVehiclesTransfer)
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
	// TEC-164: list export (I/O engine), same permission and scope as the list.
	mux.Handle("POST /v1/customers/export", readC(h.RequestListExport))
	mux.Handle("GET /v1/customer-list-exports/{uuid}", readC(h.GetListExport))
	mux.Handle("GET /v1/customer-list-exports/{uuid}/download", readC(h.DownloadListExport))
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

	// TEC-190: vehicle transfer with two codes (vehicles.transfer).
	mux.Handle("GET /v1/vehicles/{uuid}/transfers", transferV(h.ListVehicleTransfers))
	mux.Handle("POST /v1/vehicles/{uuid}/transfers", transferV(h.StartVehicleTransfer))
	mux.Handle("POST /v1/vehicle-transfers/{uuid}/verify", transferV(h.VerifyVehicleTransfer))
	mux.Handle("POST /v1/vehicle-transfers/{uuid}/cancel", transferV(h.CancelVehicleTransfer))

	// TEC-243: the same flow from the portal (own vehicles only).
	portalV := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermVehiclesRead))
	}
	mux.Handle("GET /v1/portal/vehicles/{uuid}/transfers", portalV(h.PortalListVehicleTransfers))
	mux.Handle("POST /v1/portal/vehicles/{uuid}/transfers", portalV(h.PortalStartVehicleTransfer))
	mux.Handle("POST /v1/portal/vehicle-transfers/{uuid}/verify", portalV(h.PortalVerifyVehicleTransfer))
	mux.Handle("POST /v1/portal/vehicle-transfers/{uuid}/cancel", portalV(h.PortalCancelVehicleTransfer))
}
