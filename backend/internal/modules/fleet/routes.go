// Package fleet mounts the fleet management API (TEC-473, F5-02b).
package fleet

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/fleets and the portal link decision.
//
// The panel routes need the fleet module of the active organization
// (RequireFeature: 403 FEATURE_DISABLED when it is off) and fleets.read /
// fleets.manage with the resolved scope. A dealer reaches a fleet only
// through an active link (pending: 404); services, warranties and the cari
// it sees are its own. The portal routes need fleet.portal.read: a fleet
// user accepts or rejects a pending link of their own fleet and reads the
// fleet's vehicles, services, warranties, accounts and reports (TEC-474).
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleFleet)
	appointmentsModule := middleware.RequireFeature(checker, features.ModuleAppointments)
	with := func(perm string) func(http.HandlerFunc) http.Handler {
		return func(fn http.HandlerFunc) http.Handler {
			return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, perm))
		}
	}
	read, manage := with(rbac.PermFleetsRead), with(rbac.PermFleetsManage)
	plan := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, appointmentsModule, middleware.RequireScope(q, rbac.PermFleetsPlan))
	}

	mux.Handle("GET /v1/fleets", read(h.List))
	mux.Handle("POST /v1/fleets", manage(h.Open))
	mux.Handle("GET /v1/fleets/vehicle-import/sample", manage(h.ImportSample))
	mux.Handle("GET /v1/fleets/lookup", manage(h.Lookup))
	// TEC-477: list export (I/O engine) with the list's scope and filters.
	mux.Handle("POST /v1/fleets/export", read(h.RequestListExport))
	mux.Handle("GET /v1/fleets/{uuid}", read(h.Card))
	mux.Handle("POST /v1/fleets/{uuid}/links", manage(h.RequestLink))
	mux.Handle("GET /v1/fleets/{uuid}/users", read(h.ListUsers))
	mux.Handle("POST /v1/fleets/{uuid}/users", manage(h.InviteUser))
	mux.Handle("POST /v1/fleets/{uuid}/users/{user_uuid}/disable", manage(h.DisableUser))
	mux.Handle("GET /v1/fleets/{uuid}/vehicles", read(h.ListVehicles))
	mux.Handle("POST /v1/fleets/{uuid}/vehicles", manage(h.AddVehicle))
	mux.Handle("POST /v1/fleets/{uuid}/vehicles/import", manage(h.ImportVehicles))
	mux.Handle("PATCH /v1/fleets/{uuid}/vehicles/{vehicle_uuid}", manage(h.UpdateVehicle))
	mux.Handle("DELETE /v1/fleets/{uuid}/vehicles/{vehicle_uuid}", manage(h.RemoveVehicle))
	mux.Handle("GET /v1/fleets/{uuid}/statement", read(h.Statement))
	mux.Handle("POST /v1/fleets/{uuid}/statement/export", read(h.ExportStatement))
	// TEC-476: periodic report of a closed period, on demand.
	mux.Handle("POST /v1/fleets/{uuid}/reports", manage(h.RequestReport))
	// TEC-477: the panel lists the reports and downloads a ready one.
	mux.Handle("GET /v1/fleets/{uuid}/reports", read(h.ListReports))
	mux.Handle("GET /v1/fleets/{uuid}/reports/{report}/file", read(h.ReportFile))
	mux.Handle("POST /v1/fleets/{uuid}/service-plans/preview", plan(h.PreviewServicePlan))
	mux.Handle("POST /v1/fleets/{uuid}/service-plans", plan(h.CreateServicePlan))
	mux.Handle("GET /v1/fleets/{uuid}/service-plans", plan(h.ListServicePlans))
	mux.Handle("GET /v1/fleets/{uuid}/service-plans/{plan}", plan(h.GetServicePlan))
	mux.Handle("POST /v1/fleets/{uuid}/service-plans/{plan}/cancel", plan(h.CancelServicePlan))
	mux.Handle("POST /v1/fleets/{uuid}/service-plans/{plan}/start-intake", plan(h.StartServicePlanIntake))

	portal := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermFleetPortalRead))
	}
	mux.Handle("GET /v1/portal/fleet/links", portal(h.PortalLinks))
	mux.Handle("POST /v1/portal/fleet/links/{uuid}/accept", portal(h.AcceptLink))
	mux.Handle("POST /v1/portal/fleet/links/{uuid}/reject", portal(h.RejectLink))

	// TEC-474 (F5-02c): the fleet portal reads. Only GETs: a fleet session
	// is read only (middleware.PortalReadOnly); the link decision above and
	// the report download are its only allowed actions. The use case opens
	// them while an actively linked dealer has the fleet module (else 403
	// FEATURE_DISABLED) and hides the dealers that turned it off.
	mux.Handle("GET /v1/portal/fleet/overview", portal(h.PortalOverview))
	mux.Handle("GET /v1/portal/fleet/vehicles", portal(h.PortalVehicles))
	mux.Handle("GET /v1/portal/fleet/vehicles/{uuid}", portal(h.PortalVehicle))
	mux.Handle("GET /v1/portal/fleet/services", portal(h.PortalServices))
	mux.Handle("GET /v1/portal/fleet/warranties", portal(h.PortalWarranties))
	mux.Handle("GET /v1/portal/fleet/accounting", portal(h.PortalAccounting))
	mux.Handle("GET /v1/portal/fleet/reports", portal(h.PortalReports))
	mux.Handle("GET /v1/portal/fleet/reports/{uuid}/file", portal(h.PortalReportFile))
}
