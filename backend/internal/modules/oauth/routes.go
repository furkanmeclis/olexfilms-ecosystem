// Package oauth is the MCP OAuth 2.1 authorization server (TEC-400,
// F4-03a). The endpoints sit outside /v1 at the paths MCP clients expect;
// the frontend proxies /oauth/*, /.well-known/* and /mcp/* to Go because only
// the frontend is public in production.
package oauth

import (
	"log/slog"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
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
