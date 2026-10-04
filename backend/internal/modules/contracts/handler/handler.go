// Package handler exposes contract template endpoints.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves contract template routes.
type Handler struct{ svc *usecase.Service }

// New creates a handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Variables lists the allowed placeholders.
func (h *Handler) Variables(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": h.svc.Variables()})
}

// List lists templates of the active brand.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), caller(r), usecase.ListFilter{
		Kind: strings.TrimSpace(r.URL.Query().Get("kind")), ActiveOnly: r.URL.Query().Get("active") == "true",
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get returns one template.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

type templateBody struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	IsDefault         *bool  `json:"is_default"`
	OTPRequired       *bool  `json:"otp_required"`
	SignatureRequired *bool  `json:"signature_required"`
	IsActive          *bool  `json:"is_active"`
}

func (b templateBody) input() usecase.Input {
	return usecase.Input{
		Name: b.Name, Kind: b.Kind, IsDefault: b.IsDefault, OTPRequired: b.OTPRequired,
		SignatureRequired: b.SignatureRequired, IsActive: b.IsActive,
	}
}

// Create creates a template.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in templateBody
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.Create(r.Context(), caller(r), in.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

// Update patches a template.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in templateBody
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.Update(r.Context(), caller(r), id, in.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// Delete deletes unused templates and deactivates used templates.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, deleted, err := h.svc.Delete(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if deleted {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// SetDefault makes one template default for its kind.
func (h *Handler) SetDefault(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.SetDefault(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

type localeBody struct {
	LexicalJSON json.RawMessage `json:"lexical_json"`
	HTML        string          `json:"html"`
}

// PutLocale upserts one language version and bumps its version.
func (h *Handler) PutLocale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in localeBody
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.PutLocale(r.Context(), caller(r), id, usecase.LocaleInput{
		Locale: r.PathValue("locale"), LexicalJSON: in.LexicalJSON, HTML: in.HTML,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	return usecase.Caller{UserID: p.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
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
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var unknown *usecase.UnknownVariablesError
	switch {
	case errors.As(err, &unknown):
		details := make([]response.Detail, 0, len(unknown.Keys))
		for _, k := range unknown.Keys {
			details = append(details, response.Detail{Field: "html", Message: "unknown variable: " + k})
		}
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, unknown.Error(), details)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "contract template not found")
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "contract template request failed")
	}
}
