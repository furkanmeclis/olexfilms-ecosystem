// Package rates mounts the exchange rate routes (TEC-84).
package rates

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	rateshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/rates/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts exchange rate routes.
func RegisterRoutes(mux *http.ServeMux, h *rateshandler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	platform := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(slug))
	}
	mux.Handle("GET /v1/currencies", middleware.Chain(http.HandlerFunc(h.Currencies), authn))
	mux.Handle("GET /v1/platform/exchange-rates", platform(h.List, rbac.PermPlatformRatesRead))
	mux.Handle("GET /v1/platform/exchange-rates/resolve", platform(h.Resolve, rbac.PermPlatformRatesRead))
	mux.Handle("PUT /v1/platform/exchange-rates/override", platform(h.SetOverride, rbac.PermPlatformRatesWrite))
	mux.Handle("DELETE /v1/platform/exchange-rates/override", platform(h.ClearOverride, rbac.PermPlatformRatesWrite))
	mux.Handle("POST /v1/platform/exchange-rates/fetch", platform(h.Fetch, rbac.PermPlatformRatesWrite))
}
