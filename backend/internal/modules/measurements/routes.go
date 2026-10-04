// Package measurements is the minimal measurement storage of F2 (TEC-233,
// K28); F3-02 (TEC-113) grows it into the measurement module.
package measurements

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterMobileRoutes mounts POST /v1/mobile/measurements: the version
// header first, then a mobile Bearer (a panel token is 403 REALM_FORBIDDEN,
// see middleware.RealmAllows), the active organization and
// measurements.write. No feature gate: the F2 cut-over needs the upload to
// keep working whatever the measurements module flag says (K28).
func RegisterMobileRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	minVersion, maxVersion int,
) {
	version := middleware.MobileAPIVersion(minVersion, maxVersion)
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	mux.Handle("POST /v1/mobile/measurements", middleware.Chain(http.HandlerFunc(h.Create),
		version, authn, org, middleware.RequireScope(q, rbac.PermMeasurementsWrite)))
}

func RegisterPanelRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleMeasurements)
	read := middleware.RequireScope(q, rbac.PermMeasurementsRead)
	devices := middleware.RequireScope(q, rbac.PermMeasurementDevicesManage)

	mux.Handle("GET /v1/measurement-devices", middleware.Chain(http.HandlerFunc(h.ListDevices), authn, org, module, devices))
	mux.Handle("POST /v1/measurement-devices", middleware.Chain(http.HandlerFunc(h.CreateDevice), authn, org, module, devices))
	mux.Handle("PATCH /v1/measurement-devices/{uuid}", middleware.Chain(http.HandlerFunc(h.UpdateDevice), authn, org, module, devices))

	mux.Handle("GET /v1/measurements", middleware.Chain(http.HandlerFunc(h.ListMeasurements), authn, org, module, read))
	mux.Handle("GET /v1/measurements/{uuid}", middleware.Chain(http.HandlerFunc(h.GetMeasurement), authn, org, module, read))
}
