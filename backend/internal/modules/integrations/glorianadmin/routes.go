// Package glorianadmin mounts the Glorian admin API (TEC-273, F2-02h):
// connection settings, sync runs, order outbounds and the manual pull,
// replay and reconcile triggers. Reads need integrations.glorian.view,
// writes and triggers integrations.glorian.manage; the handler also
// limits a `brand` grant to the glorian brand.
package glorianadmin

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// Prefix is the base path of the API.
const Prefix = "/v1/platform/integrations/glorian"

// RegisterRoutes mounts the routes behind authentication.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	Mount(mux, h, middleware.Authenticate(tokens, loader))
}

// Mount mounts the routes behind authn (tests pass a stub).
func Mount(mux *http.ServeMux, h *handler.Handler, authn func(http.Handler) http.Handler) {
	route := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(slug))
	}
	view := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermIntegrationsGlorianView) }
	manage := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermIntegrationsGlorianManage) }

	mux.Handle("GET "+Prefix, view(h.Get))
	mux.Handle("PUT "+Prefix, manage(h.Put))
	mux.Handle("POST "+Prefix+"/test", manage(h.Test))
	mux.Handle("GET "+Prefix+"/sync-runs", view(h.ListSyncRuns))
	mux.Handle("POST "+Prefix+"/sync-runs", manage(h.TriggerSync))
	mux.Handle("GET "+Prefix+"/sync-runs/{uuid}", view(h.GetSyncRun))
	mux.Handle("GET "+Prefix+"/outbounds", view(h.ListOutbounds))
	mux.Handle("POST "+Prefix+"/outbounds/{uuid}/replay", manage(h.ReplayOutbound))
	mux.Handle("POST "+Prefix+"/reconcile", manage(h.Reconcile))
}
