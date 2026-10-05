package handler

import (
	"net/http"
	"time"

	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-346 (F3-07f): P&L, margin, cari aging and staff cost reports of the
// active organization's own book, and their export jobs. organization_uuid
// may only name the active organization (anything else is 404): a parent
// never reads a child's internal P&L.

// purchaseVisible reports whether the principal may read purchase costs in
// the active organization (pricing.purchase.read: brand scope at the
// center, managed below it; same rule as the export column grants).
func purchaseVisible(r *http.Request) bool {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return false
	}
	need := rbac.ScopeManaged
	if orgctx.MustScope(r.Context()).OrgType == rbac.OrgTypeCenter {
		need = rbac.ScopeBrand
	}
	return p.Can(rbac.PermPricingPurchaseRead, need)
}

func queryPeriod(w http.ResponseWriter, r *http.Request) (acc.StatementPeriod, bool) {
	var p acc.StatementPeriod
	var ok bool
	if p.From, ok = queryDate(w, r, "from"); !ok {
		return p, false
	}
	if p.To, ok = queryDate(w, r, "to"); !ok {
		return p, false
	}
	return p, true
}

// GetPnlReport (GET /v1/accounting/reports/pnl?from&to&group&locale).
func (h *Handler) GetPnlReport(w http.ResponseWriter, r *http.Request) {
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	p, ok := queryPeriod(w, r)
	if !ok {
		return
	}
	c := caller(r)
	loc := h.svc.ResolveLocale(r.Context(), c, r.URL.Query().Get("locale"))
	rep, err := h.svc.GetPnlReport(r.Context(), c, org, p, r.URL.Query().Get("group"), loc)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// GetMarginReport (GET /v1/accounting/reports/margin?from&to). Cost,
// profit and margin are null without pricing.purchase.read.
func (h *Handler) GetMarginReport(w http.ResponseWriter, r *http.Request) {
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	p, ok := queryPeriod(w, r)
	if !ok {
		return
	}
	rep, err := h.svc.GetMarginReport(r.Context(), caller(r), org, p, purchaseVisible(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// GetCariAgingReport (GET /v1/accounting/reports/cari-aging?as_of).
func (h *Handler) GetCariAgingReport(w http.ResponseWriter, r *http.Request) {
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	asOf, ok := queryDate(w, r, "as_of")
	if !ok {
		return
	}
	rep, err := h.svc.GetCariAgingReport(r.Context(), caller(r), org, asOf)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// GetStaffCostReport (GET /v1/accounting/reports/staff-cost?from&to).
func (h *Handler) GetStaffCostReport(w http.ResponseWriter, r *http.Request) {
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	p, ok := queryPeriod(w, r)
	if !ok {
		return
	}
	rep, err := h.svc.GetStaffCostReport(r.Context(), caller(r), org, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// exportReport queues a TEC-346 report export (202). periodic reports take
// from/to, the aging report as_of; group applies to the P&L only.
func (h *Handler) exportReport(w http.ResponseWriter, r *http.Request, resource string) {
	var b exportBody
	if !decode(w, r, &b) {
		return
	}
	format, err := exportFormat(b.Format)
	if err != nil {
		writeError(w, r, err)
		return
	}
	query := ioengine.ExportQuery{}
	if resource == acc.ResourceCariAging {
		asOf, err := bodyDate("as_of", b.AsOf)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if asOf != nil {
			query[acc.QueryAsOf] = asOf.Format(time.DateOnly)
		}
	} else {
		from, err := bodyDate("from", b.From)
		if err != nil {
			writeError(w, r, err)
			return
		}
		to, err := bodyDate("to", b.To)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if from != nil && to != nil && to.Before(*from) {
			writeError(w, r, &acc.ValidationError{Field: "to", Message: "must not be before from"})
			return
		}
		if from != nil {
			query[acc.QueryFrom] = from.Format(time.DateOnly)
		}
		if to != nil {
			query[acc.QueryTo] = to.Format(time.DateOnly)
		}
	}
	if resource == acc.ResourcePnl {
		group, err := acc.PnlGroup(b.Group)
		if err != nil {
			writeError(w, r, err)
			return
		}
		query[acc.QueryGroup] = group
	}
	c := caller(r)
	if _, err := h.svc.ResolveOwnBook(r.Context(), c, b.OrganizationUUID); err != nil {
		writeError(w, r, err)
		return
	}
	h.queueExport(w, r, c, resource, format, query, b.Locale)
}

// ExportPnlReport (POST /v1/accounting/reports/pnl/export).
func (h *Handler) ExportPnlReport(w http.ResponseWriter, r *http.Request) {
	h.exportReport(w, r, acc.ResourcePnl)
}

// ExportMarginReport (POST /v1/accounting/reports/margin/export).
func (h *Handler) ExportMarginReport(w http.ResponseWriter, r *http.Request) {
	h.exportReport(w, r, acc.ResourceMargin)
}

// ExportCariAgingReport (POST /v1/accounting/reports/cari-aging/export).
func (h *Handler) ExportCariAgingReport(w http.ResponseWriter, r *http.Request) {
	h.exportReport(w, r, acc.ResourceCariAging)
}

// ExportStaffCostReport (POST /v1/accounting/reports/staff-cost/export).
func (h *Handler) ExportStaffCostReport(w http.ResponseWriter, r *http.Request) {
	h.exportReport(w, r, acc.ResourceStaffCost)
}
