package handler

import (
	"errors"
	"net/http"

	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Error codes of the TEC-1070 endpoints.
const (
	CodeBillingNotEditable = "BILLING_NOT_EDITABLE"
)

func writeQueueError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, acc.ErrReturnNotFound):
		response.NotFound(w, r, "Return not found in the accounting queue")
	case errors.Is(err, acc.ErrBillingNotEditable):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeBillingNotEditable,
			"Billing details are edited only for an organization below the active book")
	default:
		writeError(w, r, err)
	}
}

// QueueSummary serves GET /v1/accounting/queue/summary (TEC-1070).
func (h *Handler) QueueSummary(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.QueueSummary(r.Context(), caller(r))
	if err != nil {
		writeQueueError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListQueueReturns serves GET /v1/accounting/queue/returns (TEC-1070).
func (h *Handler) ListQueueReturns(w http.ResponseWriter, r *http.Request) {
	f, err := acc.ParseQueueReturnFilter(r.URL.Query())
	if err != nil {
		writeQueueError(w, r, err)
		return
	}
	items, total, err := h.svc.ListQueueReturns(r.Context(), caller(r), f)
	if err != nil {
		writeQueueError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

type processReturnBody struct {
	Note string `json:"note"`
}

// MarkReturnProcessed serves POST /v1/accounting/queue/returns/{uuid}/processed.
func (h *Handler) MarkReturnProcessed(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body processReturnBody
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	if err := h.svc.MarkReturnProcessed(r.Context(), caller(r), id, true, body.Note); err != nil {
		writeQueueError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnmarkReturnProcessed serves DELETE /v1/accounting/queue/returns/{uuid}/processed.
func (h *Handler) UnmarkReturnProcessed(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.MarkReturnProcessed(r.Context(), caller(r), id, false, ""); err != nil {
		writeQueueError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetCariBilling serves GET /v1/accounting/cari/{uuid}/billing (TEC-1070).
func (h *Handler) GetCariBilling(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetCariBilling(r.Context(), caller(r), org, id)
	if err != nil {
		writeQueueError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type billingBody struct {
	Title     string `json:"title"`
	TaxOffice string `json:"tax_office"`
	TaxNo     string `json:"tax_no"`
	Address   string `json:"address"`
	Email     string `json:"email"`
	IBAN      string `json:"iban"`
}

// UpdateCariBilling serves PUT /v1/accounting/cari/{uuid}/billing (TEC-1070).
func (h *Handler) UpdateCariBilling(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body billingBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.UpdateCariBilling(r.Context(), caller(r), id, acc.BillingInput{
		Title: body.Title, TaxOffice: body.TaxOffice, TaxNo: body.TaxNo,
		Address: body.Address, Email: body.Email, IBAN: body.IBAN,
	})
	if err != nil {
		writeQueueError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
