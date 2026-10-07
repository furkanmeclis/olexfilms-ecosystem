package handler

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/accounting. Routes live in the handler package
// because the module root package (accounting) holds the category catalog
// the use cases import.
//
// Reads need accounting.read, writes accounting.write; the scope is
// resolved per request (RequireScope) and the use case limits the book to
// the active organization and the organizations below it. Voiding an entry
// and booking an opening balance also need a recent step-up.
//
// TEC-342 (F3-07b): in a dealer organization every write also needs the
// dealer_accounting module (403 FEATURE_DISABLED when it is off; reads and
// disputes stay as in F1). dealer_owner / dealer_accounting hold
// accounting.write since TEC-341; dealer_staff holds no accounting
// permission. The center and distributors are not gated.
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
	dealerModule := middleware.RequireFeatureForOrgType(checker, acc.OrgDealer, features.ModuleDealerAccounting)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermAccountingRead))
	}
	write := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, dealerModule, middleware.RequireScope(q, rbac.PermAccountingWrite))
	}
	staffManage := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, dealerModule, middleware.RequireScope(q, rbac.PermStaffManage))
	}
	staffPayments := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, dealerModule, middleware.RequireScope(q, rbac.PermStaffPaymentsWrite))
	}
	sensitive := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, dealerModule, middleware.RequireScope(q, rbac.PermAccountingWrite),
			middleware.RequireStepUp(stepUp))
	}

	mux.Handle("GET /v1/accounting/categories", read(h.ListCategories))

	mux.Handle("GET /v1/accounting/accounts", read(h.ListAccounts))
	mux.Handle("POST /v1/accounting/accounts", write(h.CreateAccount))
	mux.Handle("GET /v1/accounting/accounts/{uuid}", read(h.GetAccount))
	mux.Handle("PATCH /v1/accounting/accounts/{uuid}", write(h.UpdateAccount))
	// TEC-198: one-off cash/bank opening balance (step-up, no cari, no P&L);
	// reversed through POST /v1/accounting/entries/{uuid}/void.
	mux.Handle("POST /v1/accounting/accounts/{uuid}/opening-balance", sensitive(h.CreateAccountOpening))

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
	// TEC-379: entry list export (list filters + q + sort, read scope).
	mux.Handle("POST /v1/accounting/entries/export", read(h.ExportEntries))

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

	// TEC-177: one-off opening balance of a cari (step-up); reversed through
	// POST /v1/accounting/entries/{uuid}/void.
	mux.Handle("POST /v1/accounting/opening-balances", sensitive(h.CreateOpeningBalance))

	// TEC-342: open a customer cari (a customer the organization serves).
	mux.Handle("POST /v1/accounting/cari", write(h.OpenCari))

	// TEC-345: staff cards and salary/advance/bonus payments. These write
	// sourced expense rows under source_type=staff_payment.
	mux.Handle("GET /v1/staff-profiles", staffManage(h.ListStaffProfiles))
	mux.Handle("POST /v1/staff-profiles", staffManage(h.CreateStaffProfile))
	mux.Handle("PATCH /v1/staff-profiles/{uuid}", staffManage(h.UpdateStaffProfile))
	mux.Handle("POST /v1/staff-profiles/{uuid}/payments", staffPayments(h.CreateStaffPayment))
	// TEC-349: payment history of a staff card.
	mux.Handle("GET /v1/staff-profiles/{uuid}/payments", staffManage(h.ListStaffPayments))
	mux.Handle("POST /v1/staff-payments/payroll", staffPayments(h.RunPayroll))
	// TEC-381: planned payments (paid_on ahead): book-wide list, edit, cancel.
	mux.Handle("GET /v1/staff-payments", staffManage(h.ListBookStaffPayments))
	mux.Handle("PATCH /v1/staff-payments/{uuid}", staffPayments(h.UpdateStaffPayment))
	mux.Handle("POST /v1/staff-payments/{uuid}/cancel", staffPayments(h.CancelStaffPayment))

	// TEC-346: reports of the active organization's own book (no subtree)
	// and their export jobs (read scope, like the balance report).
	mux.Handle("GET /v1/accounting/reports/pnl", read(h.GetPnlReport))
	mux.Handle("POST /v1/accounting/reports/pnl/export", read(h.ExportPnlReport))
	mux.Handle("GET /v1/accounting/reports/margin", read(h.GetMarginReport))
	mux.Handle("POST /v1/accounting/reports/margin/export", read(h.ExportMarginReport))
	mux.Handle("GET /v1/accounting/reports/cari-aging", read(h.GetCariAgingReport))
	mux.Handle("POST /v1/accounting/reports/cari-aging/export", read(h.ExportCariAgingReport))
	mux.Handle("GET /v1/accounting/reports/staff-cost", read(h.GetStaffCostReport))
	mux.Handle("POST /v1/accounting/reports/staff-cost/export", read(h.ExportStaffCostReport))
}
