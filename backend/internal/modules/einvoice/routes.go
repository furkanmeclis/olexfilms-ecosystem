// Package einvoice mounts the e-invoice API (TEC-503, F5-08c). Every route
// is the brand center's, behind the e_invoice add-on of the center and the
// einvoice.* grants: read for lists, previews and downloads, manage for
// drafts and the buyer invoice profile (archive and void also need a recent
// step-up), settings (super_admin) for the seller profile and stylesheet.
package einvoice

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// requireCenter limits a route to the brand center, before the step-up
// check so a distributor or dealer gets a plain 403.
func requireCenter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if org, ok := orgctx.ScopeFrom(r.Context()); ok && org.OrgType == "center" {
			next.ServeHTTP(w, r)
			return
		}
		response.Forbidden(w, r, "Only the brand center issues e-invoices")
	})
}

// RegisterRoutes mounts /v1/einvoices and the buyer invoice profile.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	stepUp middleware.StepUpChecker,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleEInvoice)
	route := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequirePermission(perm), requireCenter)
	}
	stepped := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequirePermission(perm), requireCenter,
			middleware.RequireStepUp(stepUp))
	}
	read, manage, settings := rbac.PermEinvoiceRead, rbac.PermEinvoiceManage, rbac.PermEinvoiceSettings

	mux.Handle("GET /v1/einvoices/billable", route(h.Billable, read))
	mux.Handle("GET /v1/einvoices", route(h.List, read))
	mux.Handle("POST /v1/einvoices", route(h.Create, manage))
	mux.Handle("GET /v1/einvoices/settings", route(h.GetSettings, read))
	mux.Handle("PUT /v1/einvoices/settings", route(h.PutSettings, settings))
	mux.Handle("POST /v1/einvoices/settings/xslt", route(h.UploadXSLT, settings))
	mux.Handle("DELETE /v1/einvoices/settings/xslt", route(h.ResetXSLT, settings))
	mux.Handle("GET /v1/einvoices/{uuid}", route(h.Get, read))
	mux.Handle("GET /v1/einvoices/{uuid}/preview", route(h.Preview, read))
	mux.Handle("POST /v1/einvoices/{uuid}/archive", stepped(h.Archive, manage))
	mux.Handle("POST /v1/einvoices/{uuid}/void", stepped(h.Void, manage))
	mux.Handle("POST /v1/einvoices/{uuid}/pdf/retry", route(h.RetryPDF, manage))
	mux.Handle("GET /v1/einvoices/{uuid}/xml", route(h.XML, read))
	mux.Handle("GET /v1/einvoices/{uuid}/pdf", route(h.PDF, read))
	mux.Handle("GET /v1/einvoices/{uuid}/html", route(h.HTML, read))
	mux.Handle("PUT /v1/platform/organizations/{uuid}/invoice-profile", route(h.PutBuyerProfile, manage))
}
