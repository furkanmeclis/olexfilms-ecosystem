// Package whatsapp mounts the WhatsApp gateway routes (TEC-92).
package whatsapp

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the admin API (whatsapp.manage) and the webhook. The
// webhook is outside the auth chain: its HMAC signature authenticates it.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	mux.HandleFunc("POST /hooks/wuzapi", h.Webhook)

	authn := middleware.Authenticate(tokens, loader)
	manage := middleware.RequirePermission(rbac.PermWhatsAppManage)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, manage))
	}
	route("GET /v1/platform/whatsapp", h.GetOverview)
	route("POST /v1/platform/whatsapp/connect", h.Connect)
	route("GET /v1/platform/whatsapp/qr", h.QR)
	route("POST /v1/platform/whatsapp/pair-phone", h.PairPhone)
	route("POST /v1/platform/whatsapp/logout", h.Logout)
	route("POST /v1/platform/whatsapp/test-message", h.TestMessage)
	route("PUT /v1/platform/whatsapp/settings", h.PutSettings)
}

// RegisterConversationRoutes mounts the panel conversation API (TEC-398).
// Per the F4 decision S2 only platform admins hold the conversations.*
// permissions. A single conversation (and its timeline) answers 404 to
// anyone who may not see it, so its existence does not leak; replies and
// changes first need conversations.reply / conversations.manage (403).
func RegisterConversationRoutes(mux *http.ServeMux, h *handler.Conversations, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	route := func(pattern string, fn http.HandlerFunc, perm string) {
		if perm == "" {
			mux.Handle(pattern, middleware.Chain(fn, authn))
			return
		}
		mux.Handle(pattern, middleware.Chain(fn, authn, middleware.RequirePermission(perm)))
	}
	route("GET /v1/conversations/meta", h.Meta, rbac.PermConversationsRead)
	route("GET /v1/conversations", h.List, rbac.PermConversationsRead)
	route("POST /v1/conversations", h.Start, rbac.PermConversationsReply)
	route("GET /v1/conversations/{uuid}", h.Get, "")
	route("PATCH /v1/conversations/{uuid}", h.Patch, rbac.PermConversationsManage)
	route("GET /v1/conversations/{uuid}/messages", h.Messages, "")
	route("GET /v1/platform/whatsapp/conversations/{uuid}/ai-runs", h.AIRuns, rbac.PermConversationsRead)
	route("POST /v1/conversations/{uuid}/messages", h.Reply, rbac.PermConversationsReply)
	route("GET /v1/conversations/{uuid}/messages/{message_uuid}/media", h.Media, "")
	route("POST /v1/conversations/{uuid}/read", h.Read, "")
}
