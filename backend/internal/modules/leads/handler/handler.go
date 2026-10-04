// Package handler serves /v1/leads endpoints.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves lead endpoints.
type Handler struct{ svc *usecase.Service }

// New creates the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	var conflict *usecase.UserConflictError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message, Code: ve.Code}})
	case errors.As(err, &conflict):
		response.ErrorWithData(w, r, http.StatusConflict, usecase.CodeLeadUserConflict, "Candidate contact already belongs to an existing user", nil, map[string]any{"existing_user": conflict.User})
	case errors.Is(err, usecase.ErrAlreadyConverted):
		response.Conflict(w, r, usecase.CodeLeadAlreadyConverted, "Lead is already converted")
	case errors.Is(err, usecase.ErrInvalidTransition):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeInvalidTransition, "Lead status transition is not allowed")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "This lead action is center-only")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Lead not found")
	default:
		response.InternalErr(w, r, err, "lead request failed")
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
		response.NotFound(w, r, "Lead not found")
		return uuid.Nil, false
	}
	return id, true
}

type field[T any] struct {
	Set   bool
	Value *T
}

func (f *field[T]) UnmarshalJSON(b []byte) error {
	f.Set = true
	if string(b) == "null" {
		f.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	f.Value = &v
	return nil
}

func toField[T any](f field[T]) usecase.Field[T] {
	return usecase.Field[T]{Set: f.Set, Value: f.Value}
}

type leadBody struct {
	TargetType           string     `json:"target_type"`
	CustomerUserID       *int64     `json:"customer_user_id"`
	VehicleID            *int64     `json:"vehicle_id"`
	CandidateCompanyName *string    `json:"candidate_company_name"`
	CandidateContactName *string    `json:"candidate_contact_name"`
	CandidatePhoneE164   *string    `json:"candidate_phone_e164"`
	CandidateEmail       *string    `json:"candidate_email"`
	CountryID            *int64     `json:"country_id"`
	ProvinceID           *int64     `json:"province_id"`
	DistrictID           *int64     `json:"district_id"`
	Source               string     `json:"source"`
	Temperature          string     `json:"temperature"`
	FollowUpDate         *time.Time `json:"follow_up_date"`
	AssigneeUserID       *int64     `json:"assignee_user_id"`
	Notes                *string    `json:"notes"`
}

func (b leadBody) input() usecase.CreateInput {
	return usecase.CreateInput{
		TargetType: b.TargetType, CustomerUserID: b.CustomerUserID, VehicleID: b.VehicleID,
		CandidateCompanyName: b.CandidateCompanyName, CandidateContactName: b.CandidateContactName,
		CandidatePhoneE164: b.CandidatePhoneE164, CandidateEmail: b.CandidateEmail,
		CountryID: b.CountryID, ProvinceID: b.ProvinceID, DistrictID: b.DistrictID,
		Source: b.Source, Temperature: b.Temperature, FollowUpDate: b.FollowUpDate,
		AssigneeUserID: b.AssigneeUserID, Notes: b.Notes,
	}
}

// List (GET /v1/leads).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), caller(r), usecase.ListFilter{
		Status: strings.TrimSpace(r.URL.Query().Get("status")), TargetType: strings.TrimSpace(r.URL.Query().Get("target_type")),
		FollowUp: strings.TrimSpace(r.URL.Query().Get("follow_up")), Q: q.Q, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// FollowUpCount (GET /v1/leads/follow-up-count).
func (h *Handler) FollowUpCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.svc.FollowUpCount(r.Context(), caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, count)
}

// Create (POST /v1/leads).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body leadBody
	if !decode(w, r, &body) {
		return
	}
	lead, err := h.svc.Create(r.Context(), caller(r), body.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, lead)
}

// Get (GET /v1/leads/{uuid}).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	lead, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, lead)
}

type patchBody struct {
	TargetType           *string          `json:"target_type"`
	CustomerUserID       field[int64]     `json:"customer_user_id"`
	VehicleID            field[int64]     `json:"vehicle_id"`
	CandidateCompanyName field[string]    `json:"candidate_company_name"`
	CandidateContactName field[string]    `json:"candidate_contact_name"`
	CandidatePhoneE164   field[string]    `json:"candidate_phone_e164"`
	CandidateEmail       field[string]    `json:"candidate_email"`
	CountryID            field[int64]     `json:"country_id"`
	ProvinceID           field[int64]     `json:"province_id"`
	DistrictID           field[int64]     `json:"district_id"`
	Source               *string          `json:"source"`
	Temperature          *string          `json:"temperature"`
	FollowUpDate         field[time.Time] `json:"follow_up_date"`
	AssigneeUserID       field[int64]     `json:"assignee_user_id"`
	Notes                *string          `json:"notes"`
}

// Patch (PATCH /v1/leads/{uuid}).
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body patchBody
	if !decode(w, r, &body) {
		return
	}
	lead, err := h.svc.Patch(r.Context(), caller(r), id, usecase.PatchInput{
		TargetType: body.TargetType, CustomerUserID: toField(body.CustomerUserID), VehicleID: toField(body.VehicleID),
		CandidateCompanyName: toField(body.CandidateCompanyName), CandidateContactName: toField(body.CandidateContactName),
		CandidatePhoneE164: toField(body.CandidatePhoneE164), CandidateEmail: toField(body.CandidateEmail),
		CountryID: toField(body.CountryID), ProvinceID: toField(body.ProvinceID), DistrictID: toField(body.DistrictID),
		Source: body.Source, Temperature: body.Temperature, FollowUpDate: toField(body.FollowUpDate),
		AssigneeUserID: toField(body.AssigneeUserID), Notes: body.Notes,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, lead)
}

type statusBody struct {
	Status     string  `json:"status"`
	LostReason *string `json:"lost_reason"`
	WonRefType *string `json:"won_ref_type"`
	WonRefID   *int64  `json:"won_ref_id"`
}

type convertBody struct {
	Kind                string     `json:"kind"`
	DistributorUUID     *uuid.UUID `json:"distributor_uuid"`
	Currency            string     `json:"currency"`
	RegisterAsWarehouse bool       `json:"register_as_warehouse"`
}

// Convert (POST /v1/leads/{uuid}/convert).
func (h *Handler) Convert(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body convertBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Convert(r.Context(), caller(r), id, usecase.ConvertInput{
		Kind: body.Kind, DistributorUUID: body.DistributorUUID, Currency: body.Currency,
		RegisterAsWarehouse: body.RegisterAsWarehouse,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SetStatus (POST /v1/leads/{uuid}/status).
func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body statusBody
	if !decode(w, r, &body) {
		return
	}
	lead, err := h.svc.SetStatus(r.Context(), caller(r), id, usecase.StatusInput(body))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, lead)
}

type noteBody struct {
	Body string `json:"body"`
}

// AddNote (POST /v1/leads/{uuid}/notes).
func (h *Handler) AddNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body noteBody
	if !decode(w, r, &body) {
		return
	}
	ev, err := h.svc.AddNote(r.Context(), caller(r), id, body.Body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, ev)
}

type assignBody struct {
	AssigneeUserID *int64 `json:"assignee_user_id"`
}

// Assign (POST /v1/leads/{uuid}/assign).
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body assignBody
	if !decode(w, r, &body) {
		return
	}
	lead, err := h.svc.Assign(r.Context(), caller(r), id, body.AssigneeUserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, lead)
}

// Events (GET /v1/leads/{uuid}/events).
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Events(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "total": int64(len(items))})
}

type taskBody struct {
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	AssigneeUserUUID *uuid.UUID `json:"assignee_user_uuid"`
	Priority         string     `json:"priority"`
	DueAt            *time.Time `json:"due_at"`
}

// CreateTask (POST /v1/leads/{uuid}/task).
func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body taskBody
	if !decode(w, r, &body) {
		return
	}
	task, err := h.svc.CreateTask(r.Context(), caller(r), id, usecase.TaskInput{
		Title: body.Title, Description: body.Description, AssigneeUUID: body.AssigneeUserUUID,
		Priority: body.Priority, DueAt: body.DueAt,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, task)
}
