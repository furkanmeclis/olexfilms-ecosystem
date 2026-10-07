// Package handler exposes the campaign authoring endpoints (TEC-405) and
// the approval chain and scheduling (TEC-406).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/campaigns/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/campaigns.
type Handler struct{ svc *usecase.Service }

// New creates the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseListFilter(r.URL.Query())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out, err := h.svc.List(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body usecase.Input
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Create(r.Context(), caller(r), body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body usecase.PatchInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Update(r.Context(), caller(r), id, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), caller(r), id); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PutContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body usecase.ContentInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.PutContent(r.Context(), caller(r), id, r.PathValue("locale"), body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) AddMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is too large"}})
			return
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "is required"}})
		return
	}
	defer func() { _ = file.Close() }()
	out, err := h.svc.AddMedia(r.Context(), caller(r), id, r.PathValue("locale"), usecase.MediaInput{
		Body: file, Filename: header.Filename,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	mediaID, ok := pathUUID(w, r, "media")
	if !ok {
		return
	}
	if err := h.svc.DeleteMedia(r.Context(), caller(r), id, r.PathValue("locale"), mediaID); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Preview(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Approvals lists the campaigns waiting for the active organization
// (TEC-406).
func (h *Handler) Approvals(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseApprovalFilter(r.URL.Query())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out, err := h.svc.Approvals(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, func(c usecase.Caller, id uuid.UUID) (usecase.Campaign, error) {
		return h.svc.Submit(r.Context(), c, id)
	})
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decision(w, r, h.svc.Approve, true)
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decision(w, r, h.svc.Reject, false)
}

func (h *Handler) RequestChanges(w http.ResponseWriter, r *http.Request) {
	h.decision(w, r, h.svc.RequestChanges, false)
}

func (h *Handler) Schedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body usecase.ScheduleInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Schedule(r.Context(), caller(r), id, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) SendNow(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, func(c usecase.Caller, id uuid.UUID) (usecase.Campaign, error) {
		return h.svc.SendNow(r.Context(), c, id)
	})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, func(c usecase.Caller, id uuid.UUID) (usecase.Campaign, error) {
		return h.svc.Cancel(r.Context(), c, id)
	})
}

type decideFunc func(context.Context, usecase.Caller, uuid.UUID, usecase.DecisionInput) (usecase.Campaign, error)

// decision reads the optional (approve) or required reason body.
func (h *Handler) decision(w http.ResponseWriter, r *http.Request, fn decideFunc, emptyBody bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body usecase.DecisionInput
	if r.ContentLength != 0 || !emptyBody {
		if !decode(w, r, &body) {
			return
		}
	}
	out, err := fn(r.Context(), caller(r), id, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) byID(w http.ResponseWriter, r *http.Request, fn func(usecase.Caller, uuid.UUID) (usecase.Campaign, error)) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := fn(caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{
		UserID: p.UserInternal, OrganizationID: org.InternalID, OrgType: org.OrgType, BrandID: org.BrandID, Filter: f,
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a uuid"}})
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			response.BadRequest(w, r, response.CodeValidationError, "body is required")
			return false
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return false
	}
	return true
}

// WriteError maps use case errors (exported for F4-04c handlers).
func WriteError(w http.ResponseWriter, r *http.Request, err error) { writeErr(w, r, err) }

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	var qe *apiquery.ValidationError
	var lm *usecase.LocaleMissingError
	switch {
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.As(err, &lm):
		details := make([]response.Detail, 0, len(lm.Locales))
		for _, l := range lm.Locales {
			details = append(details, response.Detail{Field: "contents." + l, Message: "content is missing or incomplete"})
		}
		response.ErrorWithData(w, r, http.StatusUnprocessableEntity, usecase.CodeLocaleMissing,
			"every audience locale needs content for all selected channels", details,
			map[string]any{"missing_locales": lm.Locales})
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "campaign not found")
	case errors.Is(err, usecase.ErrNotDraft):
		response.Conflict(w, r, usecase.CodeNotDraft, "only draft campaigns can be changed")
	case errors.Is(err, usecase.ErrInvalidStatus):
		response.Conflict(w, r, usecase.CodeInvalidStatus, "the action is not allowed in the campaign status")
	case errors.Is(err, usecase.ErrOrgReadOnly):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeOrganizationReadOnly,
			"the organization cannot send campaigns")
	case errors.Is(err, usecase.ErrScheduleSoon):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeScheduleTooSoon,
			"scheduled_at must be at least 5 minutes from now")
	default:
		response.InternalErr(w, r, err, "campaign request failed")
	}
}
