package performance

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/handler"
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
	module := middleware.RequireFeature(checker, features.ModulePerformance)
	route := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, perm))
	}
	read := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermPerformanceRead) }
	targets := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermPerformanceTargetsManage) }
	staff := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermPerformanceStaffTargetsManage) }
	bonus := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireFeature(checker, features.ModuleDealerAccounting), middleware.RequireScope(q, rbac.PermPerformanceBonusManage))
	}
	rules := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermPerformanceRulesManage) }

	mux.Handle("GET /v1/performance/dashboard", read(h.Dashboard))
	mux.Handle("GET /v1/performance/ranking", read(h.Ranking))
	mux.Handle("GET /v1/performance/benchmark", read(h.Benchmark))

	mux.Handle("GET /v1/performance/targets", read(h.ListTargets))
	mux.Handle("POST /v1/performance/targets", targets(h.CreateTarget))
	mux.Handle("PUT /v1/performance/targets/{uuid}", targets(h.UpdateTarget))
	mux.Handle("DELETE /v1/performance/targets/{uuid}", targets(h.DeleteTarget))

	mux.Handle("GET /v1/performance/staff-targets", staff(h.ListStaffTargets))
	mux.Handle("POST /v1/performance/staff-targets", staff(h.UpsertStaffTarget))
	mux.Handle("PUT /v1/performance/staff-targets", staff(h.UpsertStaffTarget))
	mux.Handle("DELETE /v1/performance/staff-targets/{uuid}", staff(h.DeleteStaffTarget))

	mux.Handle("GET /v1/performance/bonuses", bonus(h.ListBonuses))
	mux.Handle("POST /v1/performance/bonuses/{uuid}/approve", bonus(h.ApproveBonus))
	mux.Handle("POST /v1/performance/bonuses/{uuid}/cancel", bonus(h.CancelBonus))

	mux.Handle("GET /v1/performance/rules", rules(h.ListRules))
	mux.Handle("POST /v1/performance/rules", rules(h.CreateRule))
	mux.Handle("PUT /v1/performance/rules/{uuid}", rules(h.UpdateRule))
	mux.Handle("DELETE /v1/performance/rules/{uuid}", rules(h.DeleteRule))
}
