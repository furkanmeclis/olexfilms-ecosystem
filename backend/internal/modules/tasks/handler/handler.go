// Package handler serves the /v1/tasks endpoints (TEC-214, F1-11a).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves task endpoints.
type Handler struct{ svc *usecase.Service }

// New creates the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	return usecase.Caller{UserID: p.UserInternal, Org: orgctx.MustScope(r.Context())}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrCenterOnly):
		response.Forbidden(w, r, "Only the center organization manages tasks")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Task not found")
	default:
		response.InternalErr(w, r, err, "task request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Task not found")
		return uuid.Nil, false
	}
	return id, true
}

func queryUUID(w http.ResponseWriter, r *http.Request, name string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a UUID"}})
		return nil, false
	}
	return &id, true
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

// nullable tells an absent PATCH field from an explicit null.
type nullable[T any] struct {
	Set   bool
	Value *T
}

func (n *nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// List (GET /v1/tasks?status&priority&subject_organization_uuid&assignee_user_uuid&mine&limit&offset).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	f := usecase.Filter{
		Status:   strings.TrimSpace(qv.Get("status")),
		Priority: strings.TrimSpace(qv.Get("priority")),
	}
	var ok bool
	if f.SubjectOrgUUID, ok = queryUUID(w, r, "subject_organization_uuid"); !ok {
		return
	}
	if f.AssigneeUUID, ok = queryUUID(w, r, "assignee_user_uuid"); !ok {
		return
	}
	switch strings.TrimSpace(qv.Get("mine")) {
	case "", "false":
	case "true":
		id := authctx.MustPrincipal(r.Context()).UserID
		f.AssigneeUUID = &id
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "mine", Message: "must be true or false"}})
		return
	}
	q := apiquery.Parse(qv)
	f.Limit, f.Offset = q.Limit, q.Offset
	items, total, err := h.svc.List(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get (GET /v1/tasks/{uuid}).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	t, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

type createBody struct {
	SubjectOrganizationUUID uuid.UUID  `json:"subject_organization_uuid"`
	Title                   string     `json:"title"`
	Description             string     `json:"description"`
	AssigneeUserUUID        *uuid.UUID `json:"assignee_user_uuid"`
	Priority                string     `json:"priority"`
	DueAt                   *time.Time `json:"due_at"`
}

// Create (POST /v1/tasks).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var b createBody
	if !decode(w, r, &b) {
		return
	}
	if b.SubjectOrganizationUUID == uuid.Nil {
		response.ValidationError(w, r, []response.Detail{{Field: "subject_organization_uuid", Message: "is required"}})
		return
	}
	t, err := h.svc.Create(r.Context(), caller(r), usecase.CreateInput{
		SubjectOrgUUID: b.SubjectOrganizationUUID, Title: b.Title, Description: b.Description,
		AssigneeUUID: b.AssigneeUserUUID, Priority: b.Priority, DueAt: b.DueAt,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, t)
}

type updateBody struct {
	Title                   *string             `json:"title"`
	Description             *string             `json:"description"`
	SubjectOrganizationUUID *uuid.UUID          `json:"subject_organization_uuid"`
	Priority                *string             `json:"priority"`
	Status                  *string             `json:"status"`
	AssigneeUserUUID        nullable[uuid.UUID] `json:"assignee_user_uuid"`
	DueAt                   nullable[time.Time] `json:"due_at"`
}

// Update (PATCH /v1/tasks/{uuid}): status done or cancelled closes the task.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b updateBody
	if !decode(w, r, &b) {
		return
	}
	t, err := h.svc.Update(r.Context(), caller(r), id, usecase.UpdateInput{
		Title: b.Title, Description: b.Description, SubjectOrgUUID: b.SubjectOrganizationUUID,
		Priority: b.Priority, Status: b.Status,
		AssigneeSet: b.AssigneeUserUUID.Set, AssigneeUUID: b.AssigneeUserUUID.Value,
		DueAtSet: b.DueAt.Set, DueAt: b.DueAt.Value,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// ListComments (GET /v1/tasks/{uuid}/comments?limit&offset).
func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListComments(r.Context(), caller(r), id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

type commentBody struct {
	Body string `json:"body"`
}

// AddComment (POST /v1/tasks/{uuid}/comments).
func (h *Handler) AddComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b commentBody
	if !decode(w, r, &b) {
		return
	}
	c, err := h.svc.AddComment(r.Context(), caller(r), id, b.Body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, c)
}
