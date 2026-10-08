package efficiency

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/efficiency/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries, checker middleware.FeatureChecker) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleEfficiency)
	read := middleware.RequireScope(q, rbac.PermEfficiencyRead)
	manage := middleware.RequireScope(q, rbac.PermEfficiencyExpectationsManage)
	tenantRead := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, read)
	}
	tenantManage := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, manage)
	}

	mux.Handle("GET /v1/efficiency/summary", tenantRead(h.Summary))
	mux.Handle("POST /v1/efficiency/summary/export", tenantRead(h.ExportSummary))
	mux.Handle("GET /v1/efficiency/trend", tenantRead(h.Trend))
	mux.Handle("GET /v1/efficiency/settings", tenantRead(h.Settings))
	mux.Handle("GET /v1/efficiency/compare", tenantRead(h.Compare))
	mux.Handle("GET /v1/efficiency/rolls", tenantRead(h.Rolls))
	mux.Handle("POST /v1/efficiency/rolls/export", tenantRead(h.ExportRolls))
	mux.Handle("GET /v1/efficiency/rolls/{unit_uuid}", tenantRead(h.Roll))

	mux.Handle("GET /v1/platform/part-consumption-expectations", tenantManage(h.Expectations))
	mux.Handle("POST /v1/platform/part-consumption-expectations", tenantManage(h.CreateExpectation))
	mux.Handle("POST /v1/platform/part-consumption-expectations/import", tenantManage(h.ImportExpectations))
	mux.Handle("GET /v1/platform/part-consumption-expectations/import/sample", tenantManage(h.ImportExpectationsSample))
	mux.Handle("PUT /v1/platform/part-consumption-expectations/{uuid}", tenantManage(h.UpdateExpectation))
	mux.Handle("DELETE /v1/platform/part-consumption-expectations/{uuid}", tenantManage(h.DeleteExpectation))
}
