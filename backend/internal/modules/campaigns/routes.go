// Package campaigns mounts the F4 campaign API (TEC-405, F4-04b; TEC-406,
// F4-04c).
package campaigns

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/campaigns/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/campaigns.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries, checker middleware.FeatureChecker) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleCampaigns)
	route := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, slug))
	}
	read := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermCampaignsRead) }
	write := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermCampaignsWrite) }
	approve := func(fn http.HandlerFunc) http.Handler { return route(fn, rbac.PermCampaignsApprove) }

	mux.Handle("GET /v1/campaigns", read(h.List))
	mux.Handle("POST /v1/campaigns", write(h.Create))
	mux.Handle("GET /v1/campaigns/{uuid}", read(h.Get))
	mux.Handle("PATCH /v1/campaigns/{uuid}", write(h.Update))
	mux.Handle("DELETE /v1/campaigns/{uuid}", write(h.Delete))
	mux.Handle("PUT /v1/campaigns/{uuid}/contents/{locale}", write(h.PutContent))
	mux.Handle("POST /v1/campaigns/{uuid}/contents/{locale}/media", write(h.AddMedia))
	mux.Handle("DELETE /v1/campaigns/{uuid}/contents/{locale}/media/{media}", write(h.DeleteMedia))
	mux.Handle("POST /v1/campaigns/{uuid}/preview", read(h.Preview))
	// TEC-406: approval chain (approver_org_id members with
	// campaigns.approve) and scheduling (campaigns.write).
	mux.Handle("POST /v1/campaigns/{uuid}/submit", write(h.Submit))
	mux.Handle("POST /v1/campaigns/{uuid}/schedule", write(h.Schedule))
	mux.Handle("POST /v1/campaigns/{uuid}/send-now", write(h.SendNow))
	mux.Handle("POST /v1/campaigns/{uuid}/cancel", write(h.Cancel))
	mux.Handle("GET /v1/campaigns/approvals", approve(h.Approvals))
	mux.Handle("POST /v1/campaigns/{uuid}/approve", approve(h.Approve))
	mux.Handle("POST /v1/campaigns/{uuid}/reject", approve(h.Reject))
	mux.Handle("POST /v1/campaigns/{uuid}/request-changes", approve(h.RequestChanges))
}
