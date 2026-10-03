// Package services mounts the service routes (TEC-179, F1-05b): draft
// services of the active organization, items from its stock, the stock-free
// state machine (completion consumes the stock, TEC-180), the stock picker
// and images.
package services

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	serviceshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/services.
//
// RequireOrganization enforces the organization gate: a suspended or
// expired organization gets 403, a read-only one (K23: a dealer without an
// uploaded contract) keeps GET only, so every write answers 403
// ORGANIZATION_READ_ONLY. Reads resolve the services.read scope (dealer:
// own organization, distributor: subtree, center: brand; out of scope =
// 404). Create resolves services.write (the customer must be in that
// scope); the other writes check the action's permission against the
// service in the use case (services.write, services.cancel for cancel,
// services.complete for completion).
func RegisterRoutes(
	mux *http.ServeMux,
	h *serviceshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleServices)
	read := middleware.RequireScope(q, rbac.PermServicesRead)
	route := func(fn http.HandlerFunc, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, module, read}, extra...)
		return middleware.Chain(fn, mws...)
	}
	write := middleware.RequirePermission(rbac.PermServicesWrite)
	act := middleware.RequireAnyPermission(rbac.PermServicesWrite, rbac.PermServicesCancel, rbac.PermServicesComplete)
	create := middleware.Chain(http.HandlerFunc(h.Create), authn, org, module,
		middleware.RequireScope(q, rbac.PermServicesWrite))

	mux.Handle("GET /v1/services", route(h.List))
	mux.Handle("POST /v1/services", create)
	mux.Handle("GET /v1/services/{uuid}", route(h.Get))
	mux.Handle("PATCH /v1/services/{uuid}", route(h.Update, write))
	mux.Handle("POST /v1/services/{uuid}/items", route(h.AddItem, write))
	mux.Handle("DELETE /v1/services/{uuid}/items/{item}", route(h.RemoveItem, write))
	mux.Handle("GET /v1/services/{uuid}/stock-units", route(h.StockUnits, write))
	mux.Handle("POST /v1/services/{uuid}/transitions", route(h.Transition, act))
	mux.Handle("POST /v1/services/{uuid}/images", route(h.UploadImage, write))
	mux.Handle("GET /v1/services/{uuid}/images/{image}", route(h.DownloadImage))
	mux.Handle("DELETE /v1/services/{uuid}/images/{image}", route(h.DeleteImage, write))
	// TEC-230: consumption correction of a completed service (center only,
	// services.cancel; the use case checks the scope and the time window).
	mux.Handle("POST /v1/services/{uuid}/items/{item}/consumption-correction",
		route(h.CorrectConsumption, middleware.RequirePermission(rbac.PermServicesCancel)))

	// TEC-151: top-10 car brands / models of completed services (center
	// dashboard). Same gates as the reads; the use case additionally needs
	// a brand-wide grant (center) or scope all (super_admin), else 403.
	mux.Handle("GET /v1/stats/top-vehicle-models", route(h.TopVehicleModels))
}

// RegisterPDFRoutes mounts the service PDF (TEC-196): POST
// /v1/services/{uuid}/pdf queues the export job for a service inside the
// caller's services.read scope (else 404); GET /v1/service-pdfs/{uuid}
// [/download] polls and downloads the job of the active organization. Same
// gates as the reads (organization, services module, services.read).
func RegisterPDFRoutes(
	mux *http.ServeMux,
	h *serviceshandler.PDF,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	route := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, middleware.Authenticate(tokens, loader), middleware.RequireOrganization(tokens, q),
			middleware.RequireFeature(checker, features.ModuleServices), middleware.RequireScope(q, rbac.PermServicesRead))
	}
	mux.Handle("POST /v1/services/{uuid}/pdf", route(h.Request))
	mux.Handle("GET /v1/service-pdfs/{uuid}", route(h.Get))
	mux.Handle("GET /v1/service-pdfs/{uuid}/download", route(h.Download))
}
