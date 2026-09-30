package github

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	githubhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/github/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts GitHub integration routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *githubhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}
	superAdmin := middleware.RequireSuperAdmin

	mux.Handle("GET /v1/platform/integrations/github", middleware.Chain(
		http.HandlerFunc(h.GetSettings), authn, require(rbac.PermPlatformIntegrationsGitHubRead), superAdmin,
	))
	mux.Handle("PATCH /v1/platform/integrations/github", middleware.Chain(
		http.HandlerFunc(h.PatchSettings), authn, require(rbac.PermPlatformIntegrationsGitHubWrite), superAdmin,
	))
}
