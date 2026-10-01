// Package documents wires PDF document templates and rendering (TEC-88).
package documents

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	dochandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the template editor (center admin) and the tenant
// render/download endpoints. Download rights follow the source: the loader
// decides at request time, and documents are scoped to the organization.
func RegisterRoutes(mux *http.ServeMux, h *dochandler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries) {
	authn := middleware.Authenticate(tokens, loader)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermPlatformDocumentTemplatesRead))
	}
	write := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermPlatformDocumentTemplatesWrite))
	}
	mux.Handle("GET /v1/platform/document-templates", read(h.List))
	mux.Handle("GET /v1/platform/document-templates/kinds", read(h.Kinds))
	mux.Handle("POST /v1/platform/document-templates", write(h.SaveDraft))
	mux.Handle("POST /v1/platform/document-templates/preview", write(h.Preview))
	mux.Handle("GET /v1/platform/document-templates/{uuid}", read(h.Get))
	mux.Handle("PUT /v1/platform/document-templates/{uuid}", write(h.Update))
	mux.Handle("POST /v1/platform/document-templates/{uuid}/publish", write(h.Publish))
	mux.Handle("GET /v1/platform/document-templates/{uuid}/versions", read(h.Versions))

	requireOrg := middleware.RequireOrganization(tokens, q)
	tenant := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, requireOrg)
	}
	mux.Handle("POST /v1/tenant/documents/render", tenant(h.RequestRender))
	mux.Handle("GET /v1/tenant/documents/{uuid}", tenant(h.GetRender))
	mux.Handle("GET /v1/tenant/documents/{uuid}/download", tenant(h.Download))
}
