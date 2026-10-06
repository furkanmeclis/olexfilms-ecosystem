// Package handler serves the service catalog API.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	pricing "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "You cannot manage the service catalog from this organization")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Service catalog item was not found")
	case errors.Is(err, usecase.ErrInUse):
		response.Conflict(w, r, "SERVICE_CATALOG_ITEM_IN_USE", "Service catalog item has subscriptions")
	case errors.Is(err, usecase.ErrModuleOnly):
		response.ValidationError(w, r, []response.Detail{{Field: "category", Message: "item must be a module_bundle"}})
	case errors.Is(err, usecase.ErrInvalidTarget):
		response.ValidationError(w, r, []response.Detail{{Field: "organization_uuid", Message: "invalid target organization"}})
	case errors.Is(err, usecase.ErrInvalidStatus):
		response.Conflict(w, r, response.CodeConflict, "Subscription cancellation request is already decided or not active")
	case errors.Is(err, usecase.ErrDuplicateCancel):
		response.Conflict(w, r, response.CodeConflict, "Subscription already has a pending cancellation request")
	case errors.Is(err, usecase.ErrModuleBlockedByParent):
		response.Error(w, r, http.StatusUnprocessableEntity, response.CodeModuleBlockedByParent,
			"Module is disabled at a higher level")
	case errors.Is(err, usecase.ErrRateNotFound):
		response.Error(w, r, http.StatusUnprocessableEntity, response.CodeRateNotFound,
			"No exchange rate for this subscription")
	default:
		response.InternalErr(w, r, err, "service catalog request failed")
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

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

func viewer(r *http.Request) pricing.Viewer {
	return pricing.ViewerFrom(authctx.MustPrincipal(r.Context()), orgctx.MustScope(r.Context()))
}

func caller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{
		Principal: authctx.MustPrincipal(r.Context()),
		Org:       orgctx.MustScope(r.Context()),
		Filter:    f,
	}
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal != 0 {
		v := p.UserInternal
		actor = &v
	}
	h.activity.Record(r.Context(), actor, action, "service_catalog", &id, payload, r)
}

type itemBody struct {
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	Category           *string `json:"category"`
	DefaultPrice       *string `json:"default_price"`
	Currency           *string `json:"currency"`
	Recurrence         *string `json:"recurrence"`
	CancellationFee    *string `json:"cancellation_fee"`
	ContractTemplateID *int64  `json:"contract_template_id"`
	IsActive           *bool   `json:"is_active"`
}

func (b itemBody) input() usecase.ItemInput {
	return usecase.ItemInput{
		Name: b.Name, Description: b.Description, Category: b.Category, DefaultPrice: b.DefaultPrice,
		Currency: b.Currency, Recurrence: b.Recurrence, CancellationFee: b.CancellationFee,
		ContractTemplateID: b.ContractTemplateID, IsActive: b.IsActive,
	}
}

// listFilter reads q, category / recurrence (CSV) and active (TEC-369).
func listFilter(w http.ResponseWriter, r *http.Request) (usecase.ListFilter, bool) {
	values := r.URL.Query()
	f := usecase.ListFilter{Q: strings.TrimSpace(values.Get("q"))}
	var err error
	if f.Active, err = apiquery.Bool(values, "active"); err != nil {
		response.QueryValidation(w, r, err)
		return f, false
	}
	if f.Categories, err = apiquery.EnumList(values, "category", usecase.Categories...); err != nil {
		response.QueryValidation(w, r, err)
		return f, false
	}
	if f.Recurrences, err = apiquery.EnumList(values, "recurrence", usecase.Recurrences...); err != nil {
		response.QueryValidation(w, r, err)
		return f, false
	}
	return f, true
}

func (h *Handler) ListPlatform(w http.ResponseWriter, r *http.Request) {
	f, ok := listFilter(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListPlatform(r.Context(), orgctx.MustScope(r.Context()), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body itemBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.Create(r.Context(), orgctx.MustScope(r.Context()), body.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_catalog.created", item.UUID, nil)
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) GetPlatform(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, err := h.svc.GetPlatform(r.Context(), orgctx.MustScope(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body itemBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.Update(r.Context(), orgctx.MustScope(r.Context()), id, body.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_catalog.updated", item.UUID, nil)
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, err := h.svc.Delete(r.Context(), orgctx.MustScope(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_catalog.deactivated", item.UUID, nil)
	response.JSON(w, r, http.StatusOK, item)
}

type modulesBody struct {
	Modules []string `json:"modules"`
}

func (h *Handler) SetModules(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body modulesBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.SetModules(r.Context(), orgctx.MustScope(r.Context()), id, body.Modules)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_catalog.modules_set", item.UUID, map[string]any{"modules": body.Modules})
	response.JSON(w, r, http.StatusOK, item)
}

type overrideBody struct {
	Price    string `json:"price"`
	Currency string `json:"currency"`
}

func (h *Handler) PutOverride(w http.ResponseWriter, r *http.Request) {
	itemID, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	orgID, ok := pathUUID(w, r, "org_uuid")
	if !ok {
		return
	}
	var body overrideBody
	if !decode(w, r, &body) {
		return
	}
	ov, err := h.svc.UpsertOverride(r.Context(), orgctx.MustScope(r.Context()), itemID, orgID, usecase.OverrideInput(body))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_catalog.override_set", itemID, map[string]any{"organization_uuid": orgID.String()})
	response.JSON(w, r, http.StatusOK, ov)
}

func (h *Handler) GetOverride(w http.ResponseWriter, r *http.Request) {
	itemID, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	orgID, ok := pathUUID(w, r, "org_uuid")
	if !ok {
		return
	}
	ov, err := h.svc.GetOverride(r.Context(), orgctx.MustScope(r.Context()), itemID, orgID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, ov)
}

type subscriptionBody struct {
	ItemUUID         string `json:"item_uuid"`
	OrganizationUUID string `json:"organization_uuid"`
	StartsOn         string `json:"starts_on"`
	EndsOn           string `json:"ends_on"`
}

func (h *Handler) AssignSubscription(w http.ResponseWriter, r *http.Request) {
	var body subscriptionBody
	if !decode(w, r, &body) {
		return
	}
	itemID, err := uuid.Parse(body.ItemUUID)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "item_uuid", Message: "is invalid"}})
		return
	}
	orgID, err := uuid.Parse(body.OrganizationUUID)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "organization_uuid", Message: "is invalid"}})
		return
	}
	starts, err := time.Parse(time.DateOnly, body.StartsOn)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "starts_on", Message: "must be YYYY-MM-DD"}})
		return
	}
	ends, err := time.Parse(time.DateOnly, body.EndsOn)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "ends_on", Message: "must be YYYY-MM-DD"}})
		return
	}
	sub, err := h.svc.Assign(r.Context(), caller(r), usecase.SubscriptionInput{
		ItemUUID: itemID, OrganizationUUID: orgID, StartsOn: starts, EndsOn: ends,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_subscription.assigned", sub.UUID, map[string]any{"organization_uuid": body.OrganizationUUID})
	response.JSON(w, r, http.StatusCreated, sub)
}

func (h *Handler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListSubscriptions(r.Context(), caller(r), r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	sub, err := h.svc.GetSubscription(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, sub)
}

type cancelBody struct {
	Reason string `json:"reason"`
}

func (h *Handler) RequestCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body cancelBody
	if !decode(w, r, &body) {
		return
	}
	req, err := h.svc.RequestCancel(r.Context(), caller(r), id, usecase.CancelRequestInput{Reason: body.Reason})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_subscription.cancel_requested", id, nil)
	response.JSON(w, r, http.StatusCreated, req)
}

type decisionBody struct {
	Note *string `json:"note"`
}

func (h *Handler) ApproveCancel(w http.ResponseWriter, r *http.Request) {
	h.decideCancel(w, r, true)
}

func (h *Handler) RejectCancel(w http.ResponseWriter, r *http.Request) {
	h.decideCancel(w, r, false)
}

func (h *Handler) decideCancel(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body decisionBody
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	var (
		req usecase.CancelRequestView
		err error
	)
	if approve {
		req, err = h.svc.ApproveCancel(r.Context(), caller(r), id, usecase.DecisionInput{Note: body.Note})
	} else {
		req, err = h.svc.RejectCancel(r.Context(), caller(r), id, usecase.DecisionInput{Note: body.Note})
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_subscription.cancel_decided", id, map[string]any{"approved": approve})
	response.JSON(w, r, http.StatusOK, req)
}

func (h *Handler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	itemID, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	orgID, ok := pathUUID(w, r, "org_uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteOverride(r.Context(), orgctx.MustScope(r.Context()), itemID, orgID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_catalog.override_deleted", itemID, map[string]any{"organization_uuid": orgID.String()})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) ListVisible(w http.ResponseWriter, r *http.Request) {
	f, ok := listFilter(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListVisible(r.Context(), orgctx.MustScope(r.Context()), viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
