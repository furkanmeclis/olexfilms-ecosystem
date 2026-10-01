// Package legal mounts portal consents and the admin legal text editor
// (TEC-90, K19/K22).
package legal

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the routes. /v1/portal/* accepts portal (aud=portal)
// tokens only; that rule lives in middleware.Authenticate.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	write := middleware.RequirePermission(rbac.PermPlatformLegalTextsWrite)

	mux.Handle("GET /v1/portal/consents/pending", middleware.Chain(http.HandlerFunc(h.PendingConsents), authn))
	mux.Handle("POST /v1/portal/consents", middleware.Chain(http.HandlerFunc(h.DecideConsent), authn))

	mux.Handle("GET /v1/platform/legal-texts/{kind}", middleware.Chain(http.HandlerFunc(h.AdminGet), authn, write))
	mux.Handle("PUT /v1/platform/legal-texts/{kind}", middleware.Chain(http.HandlerFunc(h.AdminPut), authn, write))
}
