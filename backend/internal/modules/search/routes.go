package search

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	searchhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts command-palette search endpoints.
func RegisterRoutes(
	mux *http.ServeMux,
	h *searchhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	session := middleware.RequirePermission(rbac.PermAuthSession)

	mux.Handle("GET /v1/search/specs", middleware.Chain(
		http.HandlerFunc(h.Specs), authn, session,
	))
	mux.Handle("GET /v1/search", middleware.Chain(
		http.HandlerFunc(h.Search), authn, session,
	))
}
