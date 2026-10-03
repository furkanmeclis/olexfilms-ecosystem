package search

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
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
	q *db.Queries,
) {
	authn := middleware.Authenticate(tokens, loader)
	session := middleware.RequirePermission(rbac.PermAuthSession)

	mux.Handle("GET /v1/search/specs", middleware.Chain(
		http.HandlerFunc(h.Specs), authn, session,
	))
	mux.Handle("GET /v1/search", middleware.Chain(
		http.HandlerFunc(h.Search), authn, session,
	))
	// TEC-213: global search of the active organization (the same
	// organization gate as the module lists; each group then resolves its
	// list permission, scope and module feature).
	mux.Handle("GET /v1/search/global", middleware.Chain(
		http.HandlerFunc(h.Global), authn, middleware.RequireOrganization(tokens, q), session,
	))
}
