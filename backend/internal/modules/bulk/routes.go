package bulk

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	bulkhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *bulkhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/bulk", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformBulkRead),
	))
	mux.Handle("GET /v1/platform/bulk/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformBulkRead),
	))
	mux.Handle("POST /v1/platform/bulk/{uuid}/rollback", middleware.Chain(
		http.HandlerFunc(h.Rollback), authn, require(rbac.PermPlatformBulkRead),
	))

	mux.Handle("POST /v1/platform/users/bulk", middleware.Chain(
		http.HandlerFunc(h.ExecuteUsers), authn, require(rbac.PermPlatformUsersRead),
	))
	mux.Handle("POST /v1/platform/roles/bulk", middleware.Chain(
		http.HandlerFunc(h.ExecuteRoles), authn, require(rbac.PermPlatformRolesRead),
	))
}
