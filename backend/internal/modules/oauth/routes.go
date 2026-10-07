// Package oauth is the MCP OAuth 2.1 authorization server (TEC-400,
// F4-03a). The endpoints sit outside /v1 at the paths MCP clients expect;
// the frontend proxies /oauth/*, /.well-known/* and /mcp/* to Go because only
// the frontend is public in production.
package oauth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// New builds the authorization server. issuer is the public frontend origin.
func New(conn usecase.DB, featureSvc *features.Service, limiter usecase.Limiter, issuer string, log *slog.Logger) *usecase.Service {
	var fc usecase.FeatureChecker
	if featureSvc != nil {
		fc = featureSvc
	}
	return usecase.New(conn, fc, limiter, issuer, log)
}

// RegisterRoutes mounts the public endpoints (no session; per-IP limits in
// the usecase).
func RegisterRoutes(mux *http.ServeMux, svc *usecase.Service, log *slog.Logger) {
	h := handler.New(svc, log)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.AuthorizationServer)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp/{endpoint}", h.ProtectedResource)
	mux.HandleFunc("POST /oauth/register", h.Register)
	mux.HandleFunc("POST /oauth/token", h.Token)
	mux.HandleFunc("POST /oauth/revoke", h.Revoke)
	for _, p := range []string{
		"/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource/mcp/{endpoint}",
		"/oauth/register", "/oauth/token", "/oauth/revoke",
	} {
		mux.HandleFunc("OPTIONS "+p, h.Preflight)
	}
}

// AuthAccess resolves a user's permissions inside one organization with the
// auth usecase (global roles + member roles there), as a session switched
// to that organization would see them (TEC-401).
type AuthAccess struct{ UC *authusecase.AuthUseCase }

// OrgPermission reports whether the user holds perm in the organization.
func (a AuthAccess) OrgPermission(ctx context.Context, userID int64, globalRoles []string, orgUUID uuid.UUID, perm string) (bool, error) {
	acc, err := a.UC.ResolveAccess(ctx, userID, globalRoles, &orgUUID)
	if err != nil {
		return false, err
	}
	_, ok := acc.Grants[perm]
	return ok || acc.IsSuperAdmin, nil
}

// RegisterSessionRoutes mounts GET /oauth/authorize and the /v1 session
// endpoints (TEC-401): consent data and decision for panel sessions
// (/v1/oauth/*) and customer portal sessions (/v1/portal/oauth/*), the
// user's connected apps, and the platform client list.
func RegisterSessionRoutes(mux *http.ServeMux, svc *usecase.Service, tokens *jwt.Manager, loader middleware.IdentityLoader, log *slog.Logger) {
	c := handler.NewConsent(svc, handler.New(svc, log))
	mux.HandleFunc("GET /oauth/authorize", c.Authorize)

	authn := middleware.Authenticate(tokens, loader)
	session := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, authn) }
	for _, prefix := range []string{"/v1/oauth", "/v1/portal/oauth"} {
		mux.Handle("GET "+prefix+"/requests/{uuid}", session(c.ConsentInfo))
		mux.Handle("POST "+prefix+"/requests/{uuid}/decide", session(c.Decide))
		mux.Handle("GET "+prefix+"/grants", session(c.ListGrants))
		mux.Handle("DELETE "+prefix+"/grants/{uuid}", session(c.RevokeGrant))
	}
	platform := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermMCPClientsManage))
	}
	mux.Handle("GET /v1/platform/oauth/clients", platform(c.ListClients))
	mux.Handle("DELETE /v1/platform/oauth/clients/{uuid}", platform(c.RevokeClient))
}
