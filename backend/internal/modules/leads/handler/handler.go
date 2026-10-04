// Package handler serves /v1/leads endpoints.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves lead endpoints.
type Handler struct {
	svc  *usecase.Service
	docs DocumentRenderer
}

// DocumentRenderer renders business documents.
type DocumentRenderer interface {
	RequestRender(ctx context.Context, viewer docmodel.Viewer, in docusecase.RenderInput) (docmodel.RenderView, bool, error)
}

// New creates the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// WithDocuments enables quote PDF rendering.
func (h *Handler) WithDocuments(docs DocumentRenderer) *Handler {
	h.docs = docs
	return h
}

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
	case errors.Is(err, usecase.ErrQuoteConflict):
		response.Error(w, r, http.StatusConflict, "QUOTE_CONFLICT", "Quote state does not allow this action")
	case errors.Is(err, usecase.ErrQuoteForbidden):
		response.Forbidden(w, r, "This quote price override requires pricing.sale.write")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "This lead action is center-only")
	case errors.Is(err, usecase.ErrQuoteNotFound):
		response.NotFound(w, r, "Quote not found")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Lead not found")
	default:
		response.InternalErr(w, r, err, "lead request failed")
	}
}

type quoteLineBody struct {
	LineType               string     `json:"line_type"`
	ProductUUID            *uuid.UUID `json:"product_uuid"`
	ServiceCatalogItemUUID *uuid.UUID `json:"service_catalog_item_uuid"`
	Description            *string    `json:"description"`
	Quantity               string     `json:"quantity"`
	UnitPrice              *string    `json:"unit_price"`
	DiscountAmount         *string    `json:"discount_amount"`
}

func (b quoteLineBody) input() usecase.QuoteLineInput {
	return usecase.QuoteLineInput{
		LineType: b.LineType, ProductUUID: b.ProductUUID, ServiceCatalogItemUUID: b.ServiceCatalogItemUUID,
		Description: b.Description, Quantity: b.Quantity, UnitPrice: b.UnitPrice, DiscountAmount: b.DiscountAmount,
	}
}

func quoteLineInputs(in []quoteLineBody) []usecase.QuoteLineInput {
	out := make([]usecase.QuoteLineInput, 0, len(in))
	for _, l := range in {
		out = append(out, l.input())
	}
	return out
}

type quoteBody struct {
	ValidUntil *time.Time      `json:"valid_until"`
	Lines      []quoteLineBody `json:"lines"`
}

// CreateQuote (POST /v1/leads/{uuid}/quotes).
func (h *Handler) CreateQuote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body quoteBody
	if !decode(w, r, &body) {
		return
	}
	q, err := h.svc.CreateQuote(r.Context(), caller(r), id, usecase.QuoteInput{ValidUntil: body.ValidUntil, Lines: quoteLineInputs(body.Lines)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, q)
}

func quoteUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Quote not found")
		return uuid.Nil, false
	}
	return id, true
}

// GetQuote (GET /v1/quotes/{uuid}).
func (h *Handler) GetQuote(w http.ResponseWriter, r *http.Request) {
	id, ok := quoteUUID(w, r)
	if !ok {
		return
	}
	q, err := h.svc.GetQuote(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, q)
}

type quotePatchBody struct {
	ValidUntil field[time.Time] `json:"valid_until"`
}

// PatchQuote (PATCH /v1/quotes/{uuid}).
func (h *Handler) PatchQuote(w http.ResponseWriter, r *http.Request) {
	id, ok := quoteUUID(w, r)
	if !ok {
		return
	}
	var body quotePatchBody
	if !decode(w, r, &body) {
		return
	}
	q, err := h.svc.PatchQuote(r.Context(), caller(r), id, usecase.QuotePatchInput{ValidUntil: toField(body.ValidUntil)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, q)
}

type quoteLinesBody struct {
	Lines []quoteLineBody `json:"lines"`
}

// ReplaceQuoteLines (PUT /v1/quotes/{uuid}/lines).
func (h *Handler) ReplaceQuoteLines(w http.ResponseWriter, r *http.Request) {
	id, ok := quoteUUID(w, r)
	if !ok {
		return
	}
	var body quoteLinesBody
	if !decode(w, r, &body) {
		return
	}
	q, err := h.svc.ReplaceQuoteLines(r.Context(), caller(r), id, quoteLineInputs(body.Lines))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, q)
}

type quoteDecisionBody struct {
	Reason *string `json:"reason"`
}

func (h *Handler) decideQuote(w http.ResponseWriter, r *http.Request, status string) {
	id, ok := quoteUUID(w, r)
	if !ok {
		return
	}
	var body quoteDecisionBody
	if r.Body != nil && r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	q, err := h.svc.DecideQuote(r.Context(), caller(r), id, status, usecase.QuoteDecisionInput{Reason: body.Reason})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, q)
}

// AcceptQuote (POST /v1/quotes/{uuid}/accept).
func (h *Handler) AcceptQuote(w http.ResponseWriter, r *http.Request) {
	h.decideQuote(w, r, usecase.QuoteStatusAccepted)
}

// RejectQuote (POST /v1/quotes/{uuid}/reject).
func (h *Handler) RejectQuote(w http.ResponseWriter, r *http.Request) {
	h.decideQuote(w, r, usecase.QuoteStatusRejected)
}

// QuotePDF (GET /v1/quotes/{uuid}/pdf).
func (h *Handler) QuotePDF(w http.ResponseWriter, r *http.Request) {
	if h.docs == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "documents are not configured")
		return
	}
	id, ok := quoteUUID(w, r)
	if !ok {
		return
	}
	orgID, brandID, err := h.svc.QuoteOwner(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	p := authctx.MustPrincipal(r.Context())
	// Render in the quote owner's scope: a managed organization's quote the
	// caller may read must not 404 in the documents module.
	v, ready, err := h.docs.RequestRender(r.Context(), docmodel.Viewer{
		UserID: p.UserInternal, OrganizationID: orgID, BrandID: brandID,
	}, docusecase.RenderInput{Kind: docmodel.KindQuote, SourceID: id.String(), Locale: r.URL.Query().Get("locale")})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusAccepted
	if ready {
		status = http.StatusOK
	}
	response.JSON(w, r, status, v)
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
