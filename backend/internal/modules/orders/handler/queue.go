package handler

import (
	"net/http"

	ord "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type scheduleBody struct {
	Priority string  `json:"priority"`
	DueDate  *string `json:"due_date"`
}

// SetSchedule (PUT /v1/orders/{uuid}/schedule): the priority and the due
// date of an order (TEC-1228).
func (h *Handler) SetSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body scheduleBody
	if !decode(w, r, &body) {
		return
	}
	o, err := h.svc.SetSchedule(r.Context(), caller(r), id, ord.ScheduleInput{Priority: body.Priority, DueDate: body.DueDate})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

type claimBody struct {
	Takeover bool `json:"takeover"`
}

// Claim (POST /v1/orders/{uuid}/claim): the caller takes the warehouse job
// (TEC-1228). 409 ORDER_ALREADY_CLAIMED names the holder; a seller manager
// takes the job over with takeover=true. An empty body is accepted.
func (h *Handler) Claim(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body claimBody
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	o, err := h.svc.Claim(r.Context(), caller(r), id, body.Takeover)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

// ReleaseClaim (DELETE /v1/orders/{uuid}/claim): gives the job back.
func (h *Handler) ReleaseClaim(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	o, err := h.svc.ReleaseClaim(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}
