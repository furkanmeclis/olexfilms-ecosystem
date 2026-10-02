// Package tasks mounts the TEC-214 center task routes. Reads need
// tasks.read, writes and comments tasks.write; only center roles hold them,
// so distributor and dealer members get 403. On top of the permission the
// use case requires the active organization to be the brand center and
// bounds every read by its brand (K1/K20).
package tasks

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/tasks.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	route := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, org, middleware.RequireScope(q, slug))
	}
	read := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermTasksRead) }
	write := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermTasksWrite) }

	mux.Handle("GET /v1/tasks", read(h.List))
	mux.Handle("POST /v1/tasks", write(h.Create))
	mux.Handle("GET /v1/tasks/{uuid}", read(h.Get))
	mux.Handle("PATCH /v1/tasks/{uuid}", write(h.Update))
	mux.Handle("GET /v1/tasks/{uuid}/comments", read(h.ListComments))
	mux.Handle("POST /v1/tasks/{uuid}/comments", write(h.AddComment))
}
