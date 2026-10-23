package handler

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

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
