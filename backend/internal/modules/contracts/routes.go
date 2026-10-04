// Package contracts mounts contract template routes.
package contracts

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	contracthandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the center template editor. Platform path naming is
// kept, but it still uses the active organization and intake_contracts module
// gate because templates are brand scoped.
func RegisterRoutes(
	mux *http.ServeMux,
	h *contracthandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleIntakeContracts)
	manage := middleware.RequireScope(q, rbac.PermContractsTemplatesManage)
	route := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, manage)
	}

	mux.Handle("GET /v1/contract-templates/variables", route(h.Variables))
	mux.Handle("GET /v1/platform/contract-templates", route(h.List))
	mux.Handle("POST /v1/platform/contract-templates", route(h.Create))
	mux.Handle("GET /v1/platform/contract-templates/{uuid}", route(h.Get))
	mux.Handle("PATCH /v1/platform/contract-templates/{uuid}", route(h.Update))
	mux.Handle("DELETE /v1/platform/contract-templates/{uuid}", route(h.Delete))
	mux.Handle("PUT /v1/platform/contract-templates/{uuid}/locales/{locale}", route(h.PutLocale))
	mux.Handle("POST /v1/platform/contract-templates/{uuid}/default", route(h.SetDefault))
}
