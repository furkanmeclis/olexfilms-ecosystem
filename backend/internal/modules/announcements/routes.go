// Package announcements mounts the F3 announcement API.
package announcements

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/announcements/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/announcements.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries, checker middleware.FeatureChecker) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleAnnouncements)
	route := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, slug))
	}
	read := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermAnnouncementsRead) }
	write := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermAnnouncementsWrite) }

	mux.Handle("GET /v1/announcements", read(h.List))
	mux.Handle("POST /v1/announcements", write(h.Create))
	mux.Handle("GET /v1/announcements/unread-count", read(h.UnreadCount))
	mux.Handle("GET /v1/announcements/{uuid}", read(h.Get))
	mux.Handle("PATCH /v1/announcements/{uuid}", write(h.Update))
	mux.Handle("PUT /v1/announcements/{uuid}/locales/{locale}", write(h.UpsertLocale))
	mux.Handle("POST /v1/announcements/{uuid}/publish", write(h.Publish))
	mux.Handle("POST /v1/announcements/{uuid}/archive", write(h.Archive))
	mux.Handle("POST /v1/announcements/{uuid}/pin", write(h.Pin))
	mux.Handle("GET /v1/announcements/{uuid}/reads", write(h.Reads))
}
