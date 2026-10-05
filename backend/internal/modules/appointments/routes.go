package appointments

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

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
	module := middleware.RequireFeature(checker, features.ModuleAppointments)
	read := middleware.RequireScope(q, rbac.PermAppointmentsRead)
	route := func(fn http.HandlerFunc, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, module, read}, extra...)
		return middleware.Chain(fn, mws...)
	}
	write := middleware.RequirePermission(rbac.PermAppointmentsWrite)
	settings := middleware.RequirePermission(rbac.PermAppointmentSettingsManage)

	mux.Handle("GET /v1/appointment-settings", route(h.GetSettings, settings))
	mux.Handle("PUT /v1/appointment-settings", route(h.PutSettings, settings))
	mux.Handle("GET /v1/appointment-closures", route(h.ListClosures, settings))
	mux.Handle("POST /v1/appointment-closures", route(h.CreateClosure, settings))
	mux.Handle("DELETE /v1/appointment-closures/{uuid}", route(h.DeleteClosure, settings))
	mux.Handle("GET /v1/appointments/availability", route(h.Availability))
	mux.Handle("GET /v1/appointments/occupancy", route(h.Occupancy))
	mux.Handle("GET /v1/appointments", route(h.List))
	mux.Handle("POST /v1/appointments", route(h.Create, write))
	mux.Handle("PATCH /v1/appointments/{uuid}", route(h.Patch, write))
	mux.Handle("POST /v1/appointments/{uuid}/status", route(h.SetStatus, write))
	mux.Handle("POST /v1/appointments/{uuid}/start-intake", route(h.StartIntake, write))
}

func RegisterPortalRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn)
	}
	write := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.DenyPortalReadOnly)
	}

	mux.Handle("GET /v1/portal/dealers/{uuid}/availability", read(h.PortalAvailability))
	mux.Handle("GET /v1/portal/appointments", read(h.PortalList))
	mux.Handle("POST /v1/portal/appointments", write(h.PortalCreate))
	mux.Handle("POST /v1/portal/appointments/{uuid}/cancel", write(h.PortalCancel))
}
