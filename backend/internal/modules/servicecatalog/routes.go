// Package servicecatalog mounts the service catalog API (TEC-306).
package servicecatalog

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

func requireCenter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org, ok := orgctx.ScopeFrom(r.Context())
		if !ok || org.OrgType != rbac.OrgTypeCenter {
			response.Forbidden(w, r, "Service catalog management is center-only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

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
	module := middleware.RequireFeature(checker, features.ModuleServiceCatalog)
	manage := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequirePermission(rbac.PermServiceCatalogManage), requireCenter)
	}
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequirePermission(rbac.PermServiceCatalogRead))
	}

	mux.Handle("GET /v1/platform/service-catalog", manage(h.ListPlatform))
	mux.Handle("POST /v1/platform/service-catalog", manage(h.Create))
	mux.Handle("GET /v1/platform/service-catalog/{uuid}", manage(h.GetPlatform))
	mux.Handle("PATCH /v1/platform/service-catalog/{uuid}", manage(h.Patch))
	mux.Handle("DELETE /v1/platform/service-catalog/{uuid}", manage(h.Delete))
	mux.Handle("PUT /v1/platform/service-catalog/{uuid}/modules", manage(h.SetModules))
	mux.Handle("GET /v1/platform/service-catalog/{uuid}/overrides/{org_uuid}", manage(h.GetOverride))
	mux.Handle("PUT /v1/platform/service-catalog/{uuid}/overrides/{org_uuid}", manage(h.PutOverride))
	mux.Handle("DELETE /v1/platform/service-catalog/{uuid}/overrides/{org_uuid}", manage(h.DeleteOverride))

	mux.Handle("GET /v1/service-catalog", read(h.ListVisible))
}
