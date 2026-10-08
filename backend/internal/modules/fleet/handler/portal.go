package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-474 (F5-02c): the fleet portal reads (/v1/portal/fleet/*).

// portalCaller is the portal session: the signed-in user and the domain
// brand.
func portalCaller(r *http.Request) usecase.PortalCaller {
	c := usecase.PortalCaller{UserID: authctx.MustPrincipal(r.Context()).UserInternal}
	if b, ok := brandctx.From(r.Context()); ok {
		c.BrandID = b.ID
	}
	return c
}

// PortalOverview is GET /v1/portal/fleet/overview (date_from / date_to:
// the period of service_count, default the current month).
func (h *Handler) PortalOverview(w http.ResponseWriter, r *http.Request) {
	period, err := apiquery.DateRange(r.URL.Query(), "date")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out, err := h.svc.PortalOverview(r.Context(), portalCaller(r), period)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PortalVehicles is GET /v1/portal/fleet/vehicles (docs/list-contract.md):
// q (plate / VIN), brand (car brand uuids, CSV), has_active_warranty, sort
// plate | last_service_at | warranty_until (default plate).
func (h *Handler) PortalVehicles(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	brands, err := uuidList(values, "brand")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	warranty, err := apiquery.Bool(values, "has_active_warranty")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.PortalVehicles(r.Context(), portalCaller(r), usecase.PortalVehicleFilter{
		Q: q.Q, CarBrandUUIDs: brands, HasActiveWarranty: warranty, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// PortalVehicle is GET /v1/portal/fleet/vehicles/{uuid}.
func (h *Handler) PortalVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.PortalVehicle(r.Context(), portalCaller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PortalServices is GET /v1/portal/fleet/services: q (service no / plate),
// status (CSV), dealer (dealer uuids, CSV), vehicle, date_from / date_to
// (created_at), sort created_at | completed_at | service_no | status
// (default -created_at).
func (h *Handler) PortalServices(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	statuses, err := apiquery.EnumList(values, "status", usecase.PortalServiceStatuses...)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	dealers, err := uuidList(values, "dealer")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	vehicle, err := uuidParam(values, "vehicle")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	created, err := apiquery.DateRange(values, "date")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.PortalServices(r.Context(), portalCaller(r), usecase.PortalServiceFilter{
		Q: q.Q, Statuses: statuses, DealerUUIDs: dealers, VehicleUUID: vehicle, Created: created,
		Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// PortalWarranties is GET /v1/portal/fleet/warranties: state (active |
// expired | void, CSV), dealer, vehicle, q (plate / code / product), sort
// end_at | start_at (default end_at).
func (h *Handler) PortalWarranties(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	states, err := apiquery.EnumList(values, "state", usecase.PortalWarrantyStates...)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	dealers, err := uuidList(values, "dealer")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	vehicle, err := uuidParam(values, "vehicle")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.PortalWarranties(r.Context(), portalCaller(r), usecase.PortalWarrantyFilter{
		Q: q.Q, States: states, DealerUUIDs: dealers, VehicleUUID: vehicle,
		Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// PortalAccounting is GET /v1/portal/fleet/accounting (date_from /
// date_to, both or neither; default the current month).
func (h *Handler) PortalAccounting(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	out, err := h.svc.PortalAccounting(r.Context(), portalCaller(r), values.Get("date_from"), values.Get("date_to"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PortalReports is GET /v1/portal/fleet/reports: period_kind (CSV), sort
// period_start (default -period_start).
func (h *Handler) PortalReports(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	kinds, err := apiquery.EnumList(values, "period_kind", model.ReportMonthly, model.ReportQuarterly)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.PortalReports(r.Context(), portalCaller(r), usecase.PortalReportFilter{
		PeriodKinds: kinds, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// PortalReportFile is GET /v1/portal/fleet/reports/{uuid}/file: the PDF.
func (h *Handler) PortalReportFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rc, name, err := h.svc.PortalReportFile(r.Context(), portalCaller(r), id)
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

// uuidList parses a multi-value uuid filter (CSV or repeated keys).
func uuidList(values url.Values, key string) ([]uuid.UUID, error) {
	raw := apiquery.CSVValues(values, key)
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, v := range raw {
		id, err := uuid.Parse(strings.TrimSpace(v))
		if err != nil {
			return nil, &usecase.ValidationError{Field: key, Message: "must be a list of uuids"}
		}
		out = append(out, id)
	}
	return out, nil
}

// uuidParam parses an optional single uuid filter.
func uuidParam(values url.Values, key string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, &usecase.ValidationError{Field: key, Message: "must be a uuid"}
	}
	return &id, nil
}
