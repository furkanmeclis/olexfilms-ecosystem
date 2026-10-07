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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
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

// RegisterApprovalRoutes mounts the TEC-403 (F4-03d) pending actions
// screen: the caller's MCP / WhatsApp write-tool proposals in the active
// organization, confirmed or cancelled with ai.actions.confirm. The tool's
// own permission, module and scope are checked again on confirmation.
func RegisterApprovalRoutes(mux *http.ServeMux, h *handler.Approvals, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries) {
	route := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, middleware.Authenticate(tokens, loader), middleware.RequireOrganization(tokens, q),
			middleware.RequirePermission(rbac.PermAIActionsConfirm))
	}
	mux.Handle("GET /v1/ai/pending-actions", route(h.List))
	mux.Handle("POST /v1/ai/pending-actions/{uuid}/confirm", route(h.Confirm))
	mux.Handle("POST /v1/ai/pending-actions/{uuid}/cancel", route(h.Cancel))
}

// RegisterAdminRoutes mounts the TEC-389 (F4-01g) routes: the platform
// settings, the organization quota table and the platform usage report
// (ai.settings.manage, super_admin only), and the panel usage report of one
// organization inside the caller's ai.usage.read scope.
func RegisterAdminRoutes(mux *http.ServeMux, h *handler.Admin, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries) {
	authn := middleware.Authenticate(tokens, loader)
	platform := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermAISettingsManage))
	}
	mux.Handle("GET /v1/platform/ai/settings", platform(h.GetSettings))
	mux.Handle("PUT /v1/platform/ai/settings", platform(h.PutSettings))
	mux.Handle("GET /v1/platform/ai/orgs", platform(h.ListOrgs))
	mux.Handle("PUT /v1/platform/ai/orgs/{uuid}", platform(h.PutOrg))
	mux.Handle("GET /v1/platform/ai/usage", platform(h.PlatformListUsage))
	mux.Handle("POST /v1/platform/ai/usage/export", platform(h.PlatformExportUsage))

	usage := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequireOrganization(tokens, q),
			middleware.RequireScope(q, rbac.PermAIUsageRead))
	}
	mux.Handle("GET /v1/ai/usage", usage(h.ListUsage))
	mux.Handle("GET /v1/ai/usage/summary", usage(h.UsageSummary))
	mux.Handle("POST /v1/ai/usage/export", usage(h.ExportUsage))
}
