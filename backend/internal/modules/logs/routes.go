package logs

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	logshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *logshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformLogsRead)
	write := middleware.RequirePermission(rbac.PermPlatformLogsWrite)

	mux.Handle("GET /v1/platform/logs/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, read,
	))
	mux.Handle("GET /v1/platform/logs/stats", middleware.Chain(
		http.HandlerFunc(h.Stats), authn, read,
	))
	mux.Handle("GET /v1/platform/logs/sources", middleware.Chain(
		http.HandlerFunc(h.Sources), authn, read,
	))
	mux.Handle("GET /v1/platform/logs", middleware.Chain(
		http.HandlerFunc(h.List), authn, read,
	))
	mux.Handle("POST /v1/platform/logs/purge", middleware.Chain(
		http.HandlerFunc(h.Purge), authn, write,
	))
	mux.Handle("GET /v1/platform/logs/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, read,
	))
	mux.Handle("DELETE /v1/platform/logs/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Delete), authn, write,
	))

	mux.Handle("GET /v1/platform/log-rules/meta", middleware.Chain(
		http.HandlerFunc(h.RulesMeta), authn, read,
	))
	mux.Handle("GET /v1/platform/log-rules", middleware.Chain(
		http.HandlerFunc(h.ListRules), authn, read,
	))
	mux.Handle("POST /v1/platform/log-rules", middleware.Chain(
		http.HandlerFunc(h.CreateRule), authn, write,
	))
	mux.Handle("GET /v1/platform/log-rules/{uuid}", middleware.Chain(
		http.HandlerFunc(h.GetRule), authn, read,
	))
	mux.Handle("PATCH /v1/platform/log-rules/{uuid}", middleware.Chain(
		http.HandlerFunc(h.PatchRule), authn, write,
	))
	mux.Handle("DELETE /v1/platform/log-rules/{uuid}", middleware.Chain(
		http.HandlerFunc(h.DeleteRule), authn, write,
	))
	mux.Handle("POST /v1/platform/log-rules/{uuid}/run", middleware.Chain(
		http.HandlerFunc(h.RunRule), authn, write,
	))
}
