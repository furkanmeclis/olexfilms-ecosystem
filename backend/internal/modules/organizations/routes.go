package organizations

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	orghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/handler"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// RegisterRoutes mounts organization routes.
func RegisterRoutes(
	mux *http.ServeMux,
	svc *orgusecase.Service,
	auth *authusecase.AuthUseCase,
	store storage.Driver,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	limiter *ratelimit.Limiter,
) {
	h := orghandler.New(svc, auth, store)
	h.SetRateLimiter(limiter)
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.HandleFunc("POST /v1/public/organizations/register", h.PublicRegister)
	mux.HandleFunc("GET /v1/public/organizations/by-slug/{slug}", h.PublicBySlug)
	mux.HandleFunc("GET /v1/public/organizations/logo/{uuid}", h.PublicStreamLogo)
	mux.HandleFunc("GET /v1/public/brand", h.PublicBrand)
	mux.Handle("GET /v1/me/organizations", middleware.Chain(http.HandlerFunc(h.MeOrganizations), authn))

	mux.Handle("GET /v1/platform/organizations/meta", middleware.Chain(
		http.HandlerFunc(h.PlatformMeta), authn, require(rbac.PermPlatformOrganizationsRead),
	))
	mux.Handle("GET /v1/platform/organizations", middleware.Chain(
		http.HandlerFunc(h.PlatformList), authn, require(rbac.PermPlatformOrganizationsRead),
	))
	mux.Handle("POST /v1/platform/organizations", middleware.Chain(
		http.HandlerFunc(h.PlatformCreate), authn, require(rbac.PermPlatformOrganizationsWrite),
	))
	mux.Handle("GET /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.PlatformGet), authn, require(rbac.PermPlatformOrganizationsRead),
	))
	mux.Handle("GET /v1/platform/organizations/{uuid}/children", middleware.Chain(
		http.HandlerFunc(h.PlatformChildren), authn, require(rbac.PermPlatformOrganizationsRead),
	))
	mux.Handle("PATCH /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.PlatformPatch), authn, require(rbac.PermPlatformOrganizationsWrite),
	))
	mux.Handle("PUT /v1/platform/organizations/{uuid}/logo", middleware.Chain(
		http.HandlerFunc(h.PlatformUploadLogo), authn, require(rbac.PermPlatformOrganizationsWrite),
	))
	mux.Handle("DELETE /v1/platform/organizations/{uuid}/logo", middleware.Chain(
		http.HandlerFunc(h.PlatformDeleteLogo), authn, require(rbac.PermPlatformOrganizationsWrite),
	))
	mux.Handle("POST /v1/platform/organizations/{uuid}/members", middleware.Chain(
		http.HandlerFunc(h.PlatformAddMember), authn, require(rbac.PermPlatformOrganizationsWrite),
	))

	requireOrg := middleware.RequireOrganization(tokens, q)
	requireOwner := middleware.RequireOrgRole("owner")
	tenantSettingsRead := func(handler http.HandlerFunc) http.Handler {
		return middleware.Chain(http.HandlerFunc(handler), authn, requireOrg, requireOwner, require(rbac.PermTenantSettingsRead))
	}
	tenantSettingsWrite := func(handler http.HandlerFunc) http.Handler {
		return middleware.Chain(http.HandlerFunc(handler), authn, requireOrg, requireOwner, require(rbac.PermTenantSettingsWrite))
	}
	mux.Handle("GET /v1/tenant/settings", tenantSettingsRead(h.TenantGetSettings))
	mux.Handle("PATCH /v1/tenant/settings", tenantSettingsWrite(h.TenantPatchSettings))
	mux.Handle("PUT /v1/tenant/settings/logo", tenantSettingsWrite(h.TenantUploadLogo))
	mux.Handle("DELETE /v1/tenant/settings/logo", tenantSettingsWrite(h.TenantDeleteLogo))
}
