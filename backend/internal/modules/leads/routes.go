// Package leads mounts the lead API (TEC-313).
package leads

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/leads.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleLeads)
	read := middleware.RequireScope(q, rbac.PermLeadsRead)
	route := func(fn http.HandlerFunc, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, module, read}, extra...)
		return middleware.Chain(fn, mws...)
	}
	quoteRead := middleware.RequireScope(q, rbac.PermQuotesRead)
	quoteRoute := func(fn http.HandlerFunc, extra ...func(http.Handler) http.Handler) http.Handler {
		mws := append([]func(http.Handler) http.Handler{authn, org, module, quoteRead}, extra...)
		return middleware.Chain(fn, mws...)
	}
	write := middleware.RequirePermission(rbac.PermLeadsWrite)
	quoteWrite := middleware.RequirePermission(rbac.PermQuotesWrite)

	mux.Handle("GET /v1/leads", route(h.List))
	mux.Handle("POST /v1/leads", route(h.Create, write))
	mux.Handle("GET /v1/leads/follow-up-count", route(h.FollowUpCount))
	mux.Handle("GET /v1/leads/{uuid}", route(h.Get))
	mux.Handle("PATCH /v1/leads/{uuid}", route(h.Patch, write))
	mux.Handle("POST /v1/leads/{uuid}/status", route(h.SetStatus, write))
	mux.Handle("POST /v1/leads/{uuid}/notes", route(h.AddNote, write))
	mux.Handle("POST /v1/leads/{uuid}/assign", route(h.Assign, write))
	mux.Handle("GET /v1/leads/{uuid}/events", route(h.Events))
	mux.Handle("POST /v1/leads/{uuid}/task", route(h.CreateTask, write))
	mux.Handle("POST /v1/leads/{uuid}/quotes", route(h.CreateQuote, quoteWrite))
	mux.Handle("GET /v1/quotes/{uuid}", quoteRoute(h.GetQuote))
	mux.Handle("PATCH /v1/quotes/{uuid}", quoteRoute(h.PatchQuote, quoteWrite))
	mux.Handle("PUT /v1/quotes/{uuid}/lines", quoteRoute(h.ReplaceQuoteLines, quoteWrite))
	mux.Handle("POST /v1/quotes/{uuid}/accept", quoteRoute(h.AcceptQuote, quoteWrite))
	mux.Handle("POST /v1/quotes/{uuid}/reject", quoteRoute(h.RejectQuote, quoteWrite))
	mux.Handle("GET /v1/quotes/{uuid}/pdf", quoteRoute(h.QuotePDF))
}

// RegisterPublicRoutes mounts the public dealer application form (TEC-317):
// GET /v1/public/dealer-applications/config and POST
// /v1/public/dealer-applications, no authentication, brand from the
// request domain.
func RegisterPublicRoutes(mux *http.ServeMux, h *handler.Public) {
	mux.HandleFunc("GET /v1/public/dealer-applications/config", h.Config)
	mux.HandleFunc("POST /v1/public/dealer-applications", h.Submit)
}
