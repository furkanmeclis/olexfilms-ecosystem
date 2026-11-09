package handler

import (
	"net/http"
	"time"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// FrequentVehicles (GET /v1/stats/frequent-vehicles[?car_brand_uuid=]): the
// car brands the active organization services most, or the models of one
// car brand, for the "frequently used" section of the mobile vehicle picker.
func (h *Handler) FrequentVehicles(w http.ResponseWriter, r *http.Request) {
	carBrand, err := svcuc.ParseCarBrandUUID(r.URL.Query().Get("car_brand_uuid"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.svc.FrequentVehicles(r.Context(), caller(r), carBrand)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// TopVehicleModels (GET /v1/stats/top-vehicle-models?period=30d|90d|12m|all
// &group=brand|model, TEC-151): the top-10 car brands or models by
// completed services of the caller's brand. Center / super_admin only.
func (h *Handler) TopVehicleModels(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	out, err := h.svc.TopVehicleModels(r.Context(), caller(r), v.Get("period"), v.Get("group"), time.Now().UTC())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// HomeOverview (GET /v1/stats/overview?period=30d|90d|12m|all, TEC-868): the
// active organization's most serviced car models, most used products and
// latest completed services (dealer home "Analiz").
func (h *Handler) HomeOverview(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.HomeOverview(r.Context(), caller(r), r.URL.Query().Get("period"), time.Now().UTC())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListStats (GET /v1/services/stats[?organization_uuid=], TEC-1075): the
// summary cards above the service list, in the list's scope; this_month in
// the request's time zone (K10 default Europe/Istanbul).
func (h *Handler) ListStats(w http.ResponseWriter, r *http.Request) {
	orgs, err := svcuc.UUIDList(r.URL.Query(), svcuc.QueryOrganizationUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	tz := i18n.FromContext(r.Context()).Timezone
	out, err := h.svc.ListStats(r.Context(), caller(r), orgs, svcuc.MonthStart(time.Now(), tz))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
