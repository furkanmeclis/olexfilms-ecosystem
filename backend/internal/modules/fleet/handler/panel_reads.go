package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// ListServicePlans is GET /v1/fleets/{uuid}/service-plans (TEC-477): the
// caller organization's plans of the fleet; status (CSV), sort created_at
// (default -created_at) or start_date.
func (h *Handler) ListServicePlans(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	values := r.URL.Query()
	q := apiquery.Parse(values)
	statuses, err := apiquery.EnumList(values, "status", "scheduled", "cancelled")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.ListServicePlans(r.Context(), caller(r), id, usecase.PlanFilter{
		Statuses: statuses, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetServicePlan is GET /v1/fleets/{uuid}/service-plans/{plan} (TEC-477).
func (h *Handler) GetServicePlan(w http.ResponseWriter, r *http.Request) {
	fleetID, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	planID, ok := pathUUID(w, r, "plan")
	if !ok {
		return
	}
	out, err := h.svc.GetServicePlan(r.Context(), caller(r), fleetID, planID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListReports is GET /v1/fleets/{uuid}/reports (TEC-477): every report of
// the fleet; status and period_kind (CSV), sort period_start (default
// -period_start).
func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	values := r.URL.Query()
	q := apiquery.Parse(values)
	statuses, err := apiquery.EnumList(values, "status", model.ReportPending, model.ReportReady, model.ReportFailed)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	kinds, err := apiquery.EnumList(values, "period_kind", model.ReportMonthly, model.ReportQuarterly)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.ListReports(r.Context(), caller(r), id, usecase.ReportFilter{
		Statuses: statuses, PeriodKinds: kinds, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ReportFile is GET /v1/fleets/{uuid}/reports/{report}/file: the PDF of a
// ready report.
func (h *Handler) ReportFile(w http.ResponseWriter, r *http.Request) {
	fleetID, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	reportID, ok := pathUUID(w, r, "report")
	if !ok {
		return
	}
	rc, name, err := h.svc.ReportFile(r.Context(), caller(r), fleetID, reportID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, name, url.PathEscape(name)))
	w.Header().Set("Cache-Control", "private, max-age=0")
	_, _ = io.Copy(w, rc)
}
