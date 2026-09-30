package oauthprovider

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	oauthproviderhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts Google / Facebook / Apple integration routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *oauthproviderhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}
	superAdmin := middleware.RequireSuperAdmin

	mux.Handle("GET /v1/platform/integrations/google", middleware.Chain(
		providerHandler(h.GetSettings, "google"), authn, require(rbac.PermPlatformIntegrationsGoogleRead), superAdmin,
	))
	mux.Handle("PATCH /v1/platform/integrations/google", middleware.Chain(
		providerHandler(h.PatchSettings, "google"), authn, require(rbac.PermPlatformIntegrationsGoogleWrite), superAdmin,
	))
	mux.Handle("GET /v1/platform/integrations/facebook", middleware.Chain(
		providerHandler(h.GetSettings, "facebook"), authn, require(rbac.PermPlatformIntegrationsFacebookRead), superAdmin,
	))
	mux.Handle("PATCH /v1/platform/integrations/facebook", middleware.Chain(
		providerHandler(h.PatchSettings, "facebook"), authn, require(rbac.PermPlatformIntegrationsFacebookWrite), superAdmin,
	))
	mux.Handle("GET /v1/platform/integrations/apple", middleware.Chain(
		providerHandler(h.GetSettings, "apple"), authn, require(rbac.PermPlatformIntegrationsAppleRead), superAdmin,
	))
	mux.Handle("PATCH /v1/platform/integrations/apple", middleware.Chain(
		providerHandler(h.PatchSettings, "apple"), authn, require(rbac.PermPlatformIntegrationsAppleWrite), superAdmin,
	))
}

func providerHandler(next http.HandlerFunc, provider string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("provider", provider)
		next(w, r)
	})
}
