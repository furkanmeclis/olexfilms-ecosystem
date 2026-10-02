package handler

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/accounting. Routes live in the handler package
// because the module root package (accounting) holds the category catalog
// the use cases import.
//
// Reads need accounting.read, writes accounting.write (dealer roles hold no
// accounting.write, TEC-99 decision 7, so their writes answer 403); the
// scope is resolved per request (RequireScope) and the use case limits the
// book to the active organization and the organizations below it. Voiding
// an entry also needs a recent step-up.
func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	stepUp middleware.StepUpChecker,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleAccounting)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermAccountingRead))
	}
	write := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermAccountingWrite))
	}
	sensitive := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermAccountingWrite),
			middleware.RequireStepUp(stepUp))
	}

	mux.Handle("GET /v1/accounting/categories", read(h.ListCategories))

	mux.Handle("GET /v1/accounting/accounts", read(h.ListAccounts))
	mux.Handle("POST /v1/accounting/accounts", write(h.CreateAccount))
	mux.Handle("GET /v1/accounting/accounts/{uuid}", read(h.GetAccount))
	mux.Handle("PATCH /v1/accounting/accounts/{uuid}", write(h.UpdateAccount))

	mux.Handle("GET /v1/accounting/cari", read(h.ListCari))
	mux.Handle("GET /v1/accounting/cari/{uuid}", read(h.GetCari))
	// TEC-175: statement, balance report and their export jobs (read scope).
	mux.Handle("GET /v1/accounting/cari/{uuid}/statement", read(h.GetStatement))
	mux.Handle("POST /v1/accounting/cari/{uuid}/statement/export", read(h.ExportStatement))
	mux.Handle("GET /v1/accounting/reports/balances", read(h.GetBalances))
	mux.Handle("POST /v1/accounting/reports/balances/export", read(h.ExportBalances))
	mux.Handle("GET /v1/accounting/exports/{uuid}", read(h.GetExport))
	mux.Handle("GET /v1/accounting/exports/{uuid}/download", read(h.DownloadExport))

	mux.Handle("GET /v1/accounting/entries", read(h.ListEntries))
	mux.Handle("POST /v1/accounting/entries", write(h.CreateEntry))
	mux.Handle("GET /v1/accounting/entries/{uuid}", read(h.GetEntry))
	mux.Handle("POST /v1/accounting/entries/{uuid}/void", sensitive(h.VoidEntry))

	// TEC-174: disputes (K24). The child opens (accounting.dispute), both
	// sides read (accounting.read), the parent resolves (accounting.resolve).
	dispute := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermAccountingDispute))
	}
	resolve := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermAccountingResolve))
	}
	mux.Handle("GET /v1/accounting/disputes", read(h.ListDisputes))
	mux.Handle("POST /v1/accounting/disputes", dispute(h.OpenDispute))
	mux.Handle("GET /v1/accounting/disputes/{uuid}", read(h.GetDispute))
	mux.Handle("POST /v1/accounting/disputes/{uuid}/resolve", resolve(h.ResolveDispute))

	mux.Handle("POST /v1/accounting/collections", write(h.CreateCollection))
	mux.Handle("POST /v1/accounting/payments", write(h.CreatePayment))
}
