// Package handler serves /v1/platform/stock-bulk (TEC-1008).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockbulk/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Handler serves the bulk job endpoints.
type Handler struct{ svc *usecase.Service }

// New creates the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) usecase.Caller {
	p, _ := authctx.PrincipalFrom(r.Context())
	return usecase.Caller{UserID: p.UserInternal, OrgID: orgctx.MustScope(r.Context()).InternalID}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message})
		}
		response.ValidationError(w, r, details)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "bulk job not found")
	case errors.Is(err, usecase.ErrBusy):
		response.Error(w, r, http.StatusConflict, "STOCK_BULK_BUSY", "another bulk job of the organization is running")
	case errors.Is(err, usecase.ErrInvalidStatus):
		response.Error(w, r, http.StatusConflict, "STOCK_BULK_STATUS", "the job is not in a status for this")
	case errors.Is(err, usecase.ErrTooMany):
		response.Error(w, r, http.StatusUnprocessableEntity, "STOCK_BULK_TOO_MANY", err.Error())
	case errors.Is(err, usecase.ErrNothingToRun):
		response.Error(w, r, http.StatusUnprocessableEntity, "STOCK_BULK_EMPTY", "no eligible unit")
	case errors.Is(err, usecase.ErrNotReversible):
		response.Error(w, r, http.StatusUnprocessableEntity, "STOCK_BULK_NOT_REVERSIBLE", "the action cannot be undone")
	default:
		response.InternalErr(w, r, err, "bulk job failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

type previewBody struct {
	Action    string            `json:"action"`
	Params    usecase.Params    `json:"params"`
	Selection usecase.Selection `json:"selection"`
}

// Preview (POST /v1/platform/stock-bulk): resolves the selection and
// stores a preview job; nothing changes in stock.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var b previewBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
		return
	}
	v, err := h.svc.Preview(r.Context(), caller(r), usecase.PreviewInput{Action: b.Action, Params: b.Params, Selection: b.Selection})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// List (GET /v1/platform/stock-bulk): the job history.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseListFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Get (GET /v1/platform/stock-bulk/{uuid}).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Items (GET /v1/platform/stock-bulk/{uuid}/items?status=&limit&offset).
func (h *Handler) Items(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	statuses, err := apiquery.EnumList(r.URL.Query(), "status", usecase.ItemStatuses...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.Items(r.Context(), caller(r), id, statuses, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Run (POST /v1/platform/stock-bulk/{uuid}/run, step-up).
func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Run(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, v)
}

// Undo (POST /v1/platform/stock-bulk/{uuid}/undo, step-up).
func (h *Handler) Undo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Undo(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, v)
}

// Discard (DELETE /v1/platform/stock-bulk/{uuid}): drops a preview.
func (h *Handler) Discard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Discard(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
