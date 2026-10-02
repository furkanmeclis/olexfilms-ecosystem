// Package services mounts the service routes (TEC-179, F1-05b): draft
// services of the active organization, items from its stock, the stock-free
// part of the state machine (completion arrives with TEC-180) and images.
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
// service in the use case (services.write, services.cancel for cancel).
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
	act := middleware.RequireAnyPermission(rbac.PermServicesWrite, rbac.PermServicesCancel)
	create := middleware.Chain(http.HandlerFunc(h.Create), authn, org, module,
		middleware.RequireScope(q, rbac.PermServicesWrite))

	mux.Handle("GET /v1/services", route(h.List))
	mux.Handle("POST /v1/services", create)
	mux.Handle("GET /v1/services/{uuid}", route(h.Get))
	mux.Handle("PATCH /v1/services/{uuid}", route(h.Update, write))
	mux.Handle("POST /v1/services/{uuid}/items", route(h.AddItem, write))
	mux.Handle("DELETE /v1/services/{uuid}/items/{item}", route(h.RemoveItem, write))
	mux.Handle("POST /v1/services/{uuid}/transitions", route(h.Transition, act))
	mux.Handle("POST /v1/services/{uuid}/images", route(h.UploadImage, write))
	mux.Handle("GET /v1/services/{uuid}/images/{image}", route(h.DownloadImage))
	mux.Handle("DELETE /v1/services/{uuid}/images/{image}", route(h.DeleteImage, write))
}
