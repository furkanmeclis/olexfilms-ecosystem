// Package ai mounts the AI assistant chat (TEC-388, F4-01f): the panel
// routes under /v1/ai (panel tokens, active organization) and the portal
// routes under /v1/portal/ai (portal tokens, the request brand's center).
// Module, permission, consent, quota and rate limit checks live in the
// chat use case, so both channels apply them in the same order.
package ai

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// RegisterRoutes mounts the panel and portal routes.
func RegisterRoutes(mux *http.ServeMux, chat *usecase.Chat, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries) {
	RegisterPanelRoutes(mux, handler.New(chat, model.ChannelPanel), tokens, loader, q)
	RegisterPortalRoutes(mux, handler.New(chat, model.ChannelPortal), tokens, loader)
}

// RegisterPanelRoutes mounts /v1/ai/*.
func RegisterPanelRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	route := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, authn, org) }

	mux.Handle("GET /v1/ai/status", route(h.Status))
	mux.Handle("GET /v1/ai/conversations", route(h.List))
	mux.Handle("POST /v1/ai/conversations", route(h.Create))
	mux.Handle("GET /v1/ai/conversations/{uuid}", route(h.Get))
	mux.Handle("PATCH /v1/ai/conversations/{uuid}", route(h.Rename))
	mux.Handle("DELETE /v1/ai/conversations/{uuid}", route(h.Delete))
	mux.Handle("POST /v1/ai/conversations/{uuid}/messages", route(h.SendMessage))
	mux.Handle("POST /v1/ai/actions/{uuid}/confirm", route(h.ConfirmAction))
	mux.Handle("POST /v1/ai/actions/{uuid}/cancel", route(h.CancelAction))
}

// RegisterPortalRoutes mounts /v1/portal/ai/*.
func RegisterPortalRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	route := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, authn) }

	mux.Handle("GET /v1/portal/ai/status", route(h.Status))
	mux.Handle("GET /v1/portal/ai/conversations", route(h.List))
	mux.Handle("POST /v1/portal/ai/conversations", route(h.Create))
	mux.Handle("GET /v1/portal/ai/conversations/{uuid}", route(h.Get))
	mux.Handle("PATCH /v1/portal/ai/conversations/{uuid}", route(h.Rename))
	mux.Handle("DELETE /v1/portal/ai/conversations/{uuid}", route(h.Delete))
	mux.Handle("POST /v1/portal/ai/conversations/{uuid}/messages", route(h.SendMessage))
	mux.Handle("POST /v1/portal/ai/actions/{uuid}/confirm", route(h.ConfirmAction))
	mux.Handle("POST /v1/portal/ai/actions/{uuid}/cancel", route(h.CancelAction))
}
