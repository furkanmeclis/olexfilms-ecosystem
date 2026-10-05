// Package handler exposes warranty claim endpoints.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct{ svc *usecase.Service }

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body model.CreateInput
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.Create(r.Context(), caller(r), body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	filter, ok := listFilter(w, r)
	if !ok {
		return
	}
	out, err := h.svc.List(r.Context(), caller(r), filter)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) AddPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(usecase.MaxPhotoBytes + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		file, header, err = r.FormFile("photo")
	}
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	item, err := h.svc.AddPhoto(r.Context(), caller(r), id, usecase.PhotoInput{
		Body: file, Size: header.Size, Filename: header.Filename,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body model.TransitionInput
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.Transition(r.Context(), caller(r), id, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) ReapplyService(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, err := h.svc.ReapplyService(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) PortalList(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	items, err := h.svc.PortalList(r.Context(), portalBrandID(r), p.UserInternal)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{
		UserID: p.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID, OrgType: org.OrgType,
		Filter: f, Permissions: p.PermissionScopes,
	}
}

func portalBrandID(r *http.Request) int64 {
	if b, ok := brandctx.From(r.Context()); ok {
		return b.ID
	}
	return 0
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
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

func listFilter(w http.ResponseWriter, r *http.Request) (model.ListFilter, bool) {
	q := r.URL.Query()
	var f model.ListFilter
	f.Status = q.Get("status")
	f.Limit = int32Param(q.Get("limit"), 50)
	f.Offset = int32Param(q.Get("offset"), 0)
	if raw := q.Get("warranty_uuid"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "warranty_uuid is invalid")
			return f, false
		}
		f.WarrantyID = id
	}
	if raw := q.Get("vehicle_uuid"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "vehicle_uuid is invalid")
			return f, false
		}
		f.VehicleID = id
	}
	if raw := q.Get("created_from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "created_from is invalid")
			return f, false
		}
		f.CreatedFrom = &t
	}
	if raw := q.Get("created_to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "created_to is invalid")
			return f, false
		}
		f.CreatedTo = &t
	}
	return f, true
}

func int32Param(raw string, def int32) int32 {
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return def
	}
	return int32(n)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "warranty claim not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "")
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, "WARRANTY_CLAIM_EXISTS", "warranty already has a live claim")
	case errors.Is(err, usecase.ErrReapplyOpen):
		response.Conflict(w, r, "WARRANTY_CLAIM_REAPPLY_OPEN", "re-application service is still open")
	case errors.Is(err, usecase.ErrPhotoRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodePhotoRequired, "at least one photo is required")
	case errors.Is(err, usecase.ErrUnsupportedFlow):
		response.Error(w, r, http.StatusUnprocessableEntity, "CLAIM_STATUS_FLOW", "status transition is not allowed")
	default:
		response.InternalErr(w, r, err, "warranty claim request failed")
	}
}
