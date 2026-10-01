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
