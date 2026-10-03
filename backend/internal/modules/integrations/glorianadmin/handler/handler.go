// Package handler serves the Glorian admin API (TEC-273) under
// /v1/platform/integrations/glorian.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the business rules (422) and the queue (503).
const (
	CodeCenterMissing     = "GLORIAN_CENTER_MISSING"
	CodeInactive          = "GLORIAN_CONNECTION_INACTIVE"
	CodeNotReplayable     = "GLORIAN_OUTBOUND_NOT_REPLAYABLE"
	CodeQueueUnavailable  = "QUEUE_UNAVAILABLE"
	activityResource      = "integrations.glorian"
	activityUpdated       = "integrations.glorian.updated"
	activitySyncQueued    = "integrations.glorian.sync_queued"
	activityReplayQueued  = "integrations.glorian.outbound_replay_queued"
	activityReconcileOpen = "integrations.glorian.reconcile_started"
)

// Handler exposes the endpoints.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New creates the handler; rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

type listResponse[T any] struct {
	Items []T `json:"items"`
}

// brand resolves the glorian brand and checks the caller's scope on it:
// RequirePermission let the slug through, a `brand` grant still has to be
// on the glorian domain.
func (h *Handler) brand(w http.ResponseWriter, r *http.Request, slug string) (db.Brand, bool) {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Authentication is required")
		return db.Brand{}, false
	}
	b, err := h.svc.Brand(r.Context())
	if err != nil {
		writeError(w, r, err)
		return db.Brand{}, false
	}
	var reqBrand int64
	if rb, ok := brandctx.From(r.Context()); ok {
		reqBrand = rb.ID
	}
	if !usecase.Allowed(p, slug, b.ID, reqBrand) {
		response.Forbidden(w, r, "Missing permission: "+slug)
		return db.Brand{}, false
	}
	return b, true
}

// Get (GET /v1/platform/integrations/glorian).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), b)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Put (PUT /v1/platform/integrations/glorian).
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	var in usecase.PutInput
	if !decode(w, r, &in) {
		return
	}
	v, created, err := h.svc.Put(r.Context(), b, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// The key itself never reaches the activity log, only that it changed.
	h.record(r, activityUpdated, v.UUID, map[string]any{
		"created": created, "base_url": v.BaseURL, "active": v.Active,
		"api_version": v.APIVersion, "api_key_changed": in.APIKey != nil && *in.APIKey != "",
	})
	response.JSON(w, r, http.StatusOK, v)
}

// Test (POST /v1/platform/integrations/glorian/test).
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	res, err := h.svc.Test(r.Context(), b)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// ListSyncRuns (GET .../sync-runs?kind=&status=&limit=).
func (h *Handler) ListSyncRuns(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	limit, ok := limitParam(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	items, err := h.svc.ListSyncRuns(r.Context(), b, usecase.SyncRunFilter{
		Kind: q.Get("kind"), Status: q.Get("status"), Limit: limit,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, listResponse[usecase.SyncRunView]{Items: items})
}

type syncBody struct {
	Kind string `json:"kind"`
}

// TriggerSync (POST .../sync-runs): queues a manual pull (default), a
// barcode push or a held outbound replay. 202.
func (h *Handler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	var body syncBody
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	out, err := h.svc.TriggerSync(r.Context(), b, body.Kind)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, activitySyncQueued, nil, map[string]any{"kind": out.Kind})
	response.JSON(w, r, http.StatusAccepted, out)
}

// GetSyncRun (GET .../sync-runs/{uuid}).
func (h *Handler) GetSyncRun(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	v, err := h.svc.GetSyncRun(r.Context(), b, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// ListOutbounds (GET .../outbounds?state=held&limit=).
func (h *Handler) ListOutbounds(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	limit, ok := limitParam(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListOutbounds(r.Context(), b, r.URL.Query().Get("state"), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, listResponse[usecase.OutboundView]{Items: items})
}

// ReplayOutbound (POST .../outbounds/{uuid}/replay). 202.
func (h *Handler) ReplayOutbound(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	v, err := h.svc.ReplayOutbound(r.Context(), b, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, activityReplayQueued, &v.UUID, map[string]any{"order_no": v.OrderNo, "state": v.State})
	response.JSON(w, r, http.StatusAccepted, v)
}

// Reconcile (POST .../reconcile): the running sync run. 202.
func (h *Handler) Reconcile(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	v, err := h.svc.Reconcile(r.Context(), b)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, activityReconcileOpen, &v.UUID, nil)
	response.JSON(w, r, http.StatusAccepted, v)
}

func (h *Handler) record(r *http.Request, action string, id *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		uid := p.UserInternal
		actor = &uid
	}
	h.activity.Record(r.Context(), actor, action, activityResource, id, payload, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *usecase.ValidationError
	switch {
	case errors.As(err, &verr):
		details := make([]response.Detail, 0, len(verr.Fields))
		for f, m := range verr.Fields {
			details = append(details, response.Detail{Field: f, Message: m})
		}
		sort.Slice(details, func(i, j int) bool { return details[i].Field < details[j].Field })
		response.ValidationError(w, r, details)
	case errors.Is(err, usecase.ErrBrandNotFound):
		response.NotFound(w, r, "Glorian brand not found")
	case errors.Is(err, usecase.ErrNotConfigured):
		response.NotFound(w, r, "Glorian connection is not configured")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrCenterMissing):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeCenterMissing, "The glorian brand has no center organization")
	case errors.Is(err, usecase.ErrInactive):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeInactive, "The Glorian connection is inactive")
	case errors.Is(err, usecase.ErrNotReplayable):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeNotReplayable, "Only held or failed outbounds can be replayed")
	case errors.Is(err, usecase.ErrQueueUnavailable):
		response.ServiceUnavailable(w, r, CodeQueueUnavailable, "Task queue is unavailable")
	default:
		if status, code, ok := glorian.HTTPStatus(err); ok {
			response.Error(w, r, status, code, err.Error())
			return
		}
		response.InternalErr(w, r, err, "glorian integration request failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Message: "invalid JSON body"}})
		return false
	}
	return true
}

func uuidParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "uuid", Message: "must be a UUID"}})
		return uuid.Nil, false
	}
	return id, true
}

func limitParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return usecase.ClampLimit(0), true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > usecase.MaxListLimit {
		response.ValidationError(w, r, []response.Detail{{Field: "limit", Message: "must be between 1 and " + strconv.Itoa(usecase.MaxListLimit)}})
		return 0, false
	}
	return n, true
}
