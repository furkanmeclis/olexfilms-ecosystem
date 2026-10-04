// Package handler exposes announcements over HTTP.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/announcements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/announcements.
type Handler struct{ svc *usecase.Service }

// New creates a handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	return usecase.Caller{UserID: p.UserInternal, Roles: p.Roles, Org: orgctx.MustScope(r.Context())}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Announcement access denied")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Announcement not found")
	case errors.Is(err, usecase.ErrConflict):
		response.Error(w, r, http.StatusConflict, response.CodeConflict, "Announcement state conflict")
	default:
		response.InternalErr(w, r, err, "announcement request failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Announcement not found")
		return uuid.Nil, false
	}
	return id, true
}

func locale(r *http.Request) string {
	if v := strings.TrimSpace(r.URL.Query().Get("locale")); v != "" {
		return v
	}
	return strings.TrimSpace(r.Header.Get("Accept-Language"))
}

func boolQuery(w http.ResponseWriter, r *http.Request, name string) (bool, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return false, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be true or false"}})
		return false, false
	}
	return v, true
}

// List handles GET /v1/announcements.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	unread, ok := boolQuery(w, r, "unread_only")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), caller(r), locale(r), unread, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create handles POST /v1/announcements.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in usecase.Input
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Create(r.Context(), caller(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Get handles GET /v1/announcements/{uuid}; visible reads are marked read.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetVisible(r.Context(), caller(r), id, locale(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Update handles PATCH /v1/announcements/{uuid}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.Input
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Update(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpsertLocale handles PUT /v1/announcements/{uuid}/locales/{locale}.
func (h *Handler) UpsertLocale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.LocaleInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpsertLocale(r.Context(), caller(r), id, r.PathValue("locale"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Publish handles POST /v1/announcements/{uuid}/publish.
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Publish(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Archive handles POST /v1/announcements/{uuid}/archive.
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Archive(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type pinBody struct {
	Pinned bool `json:"pinned"`
}

// Pin handles POST /v1/announcements/{uuid}/pin.
func (h *Handler) Pin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in pinBody
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Pin(r.Context(), caller(r), id, in.Pinned)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UnreadCount handles GET /v1/announcements/unread-count.
func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.UnreadCount(r.Context(), caller(r), locale(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]int64{"unread_count": n})
}

// Reads handles GET /v1/announcements/{uuid}/reads.
func (h *Handler) Reads(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	out, err := h.svc.Reads(r.Context(), caller(r), id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
