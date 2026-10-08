package stockforecast

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockforecast/handler"
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
	module := middleware.RequireFeature(checker, features.ModuleStockForecast)
	route := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, perm))
	}
	read := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermStockForecastRead) }
	manage := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermStockForecastManage) }
	network := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermStockForecastNetworkRead) }

	mux.Handle("GET /v1/stock-forecasts", read(h.List))
	mux.Handle("GET /v1/stock-forecasts/meta", read(h.Meta))
	mux.Handle("GET /v1/stock-forecasts/{product_uuid}", read(h.Detail))
	mux.Handle("PUT /v1/stock-forecasts/thresholds", manage(h.PutThresholds))
	mux.Handle("POST /v1/stock-forecasts/order-draft", manage(h.OrderDraft))
	mux.Handle("GET /v1/stock-forecasts/network", network(h.Network))
	mux.Handle("POST /v1/stock-forecasts/network/export", network(h.RequestNetworkExport))
	mux.Handle("GET /v1/stock-forecasts/subtree", read(h.Subtree))
}
