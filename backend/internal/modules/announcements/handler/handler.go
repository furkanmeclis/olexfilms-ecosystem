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
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
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

// AdminList handles GET /v1/announcements/manage (TEC-367): the author
// list of the active organization in every status. Params: status (CSV of
// draft|published|archived), pinned, publish_from/publish_to, q (title),
// sort, limit, offset.
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	q := apiquery.Parse(qv)
	srt, err := apiquery.ResolveSort(q.Sort, usecase.AdminSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	f := usecase.AdminFilter{Q: q.Q, SortKey: srt.Key, SortDesc: srt.Desc, Limit: q.Limit, Offset: q.Offset}
	if f.Statuses, err = apiquery.EnumList(qv, "status", usecase.Statuses...); err != nil {
		writeError(w, r, err)
		return
	}
	if f.Pinned, err = apiquery.Bool(qv, "pinned"); err != nil {
		writeError(w, r, err)
		return
	}
	if f.Publish, err = apiquery.DateRange(qv, "publish"); err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.AdminList(r.Context(), caller(r), f)
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
	// The read report pages up to usecase.MaxReadsLimit (above the
	// apiquery list cap of 100).
	q := apiquery.Parse(r.URL.Query())
	limit := q.Limit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			response.ValidationError(w, r, []response.Detail{{Field: "limit", Message: "must be a positive integer", Code: "invalid"}})
			return
		}
		limit = int32(min(n, usecase.MaxReadsLimit))
	}
	out, err := h.svc.Reads(r.Context(), caller(r), id, limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
