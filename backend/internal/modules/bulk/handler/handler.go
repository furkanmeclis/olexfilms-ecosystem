package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc *bulkusecase.Service
}

func New(svc *bulkusecase.Service) *Handler {
	return &Handler{svc: svc}
}

type bulkRequest struct {
	Action string                `json:"action"`
	Target bulkengine.BulkTarget `json:"target"`
	Locale string                `json:"locale"`
}

func (h *Handler) ExecuteUsers(w http.ResponseWriter, r *http.Request) {
	h.execute(w, r, adapters.ResourceUsers)
}

func (h *Handler) ExecuteRoles(w http.ResponseWriter, r *http.Request) {
	h.execute(w, r, adapters.ResourceRoles)
}

// ExecuteTenant returns a handler that runs a bulk action on an
// organization-scoped resource. Tenant modules mount it on their own routes.
func (h *Handler) ExecuteTenant(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { h.executeTenant(w, r, resource) }
}

func (h *Handler) executeTenant(w http.ResponseWriter, r *http.Request, resource string) {
	scope, ok := orgctx.ScopeFrom(r.Context())
	if !ok {
		response.Forbidden(w, r, "Organization context required")
		return
	}
	var body bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
		return
	}
	if body.Target.Query == nil {
		body.Target.Query = map[string]string{}
	}
	body.Target.Query["organization_uuid"] = scope.UUID.String()
	h.executeBody(w, r, resource, body)
}

func (h *Handler) execute(w http.ResponseWriter, r *http.Request, resource string) {
	var body bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
		return
	}
	h.executeBody(w, r, resource, body)
}

func (h *Handler) executeBody(w http.ResponseWriter, r *http.Request, resource string, body bulkRequest) {
	p := authctx.MustPrincipal(r.Context())
	action := strings.TrimSpace(body.Action)
	if action == "" {
		response.BadRequest(w, r, response.CodeValidationError, "action is required")
		return
	}
	def, _, err := h.svc.Registry().ActionDef(resource, action)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "unknown bulk action")
		return
	}
	if !p.HasPermission(def.Permission) {
		response.Forbidden(w, r, "Missing permission for bulk action")
		return
	}
	result, err := h.svc.Execute(r.Context(), p.UserInternal, bulkusecase.ExecuteInput{
		Resource: resource,
		Action:   action,
		Target:   body.Target,
		Locale:   body.Locale,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if result.Async {
		response.JSON(w, r, http.StatusAccepted, result.Job)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"sync":    true,
		"summary": result.Summary,
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	q := apiquery.Parse(r.URL.Query())
	admin := p.HasPermission(rbac.PermPlatformSettingsWrite)
	items, total, err := h.svc.ListJobs(r.Context(), p.UserInternal, admin, q.Limit, q.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	admin := p.HasPermission(rbac.PermPlatformSettingsWrite)
	job, err := h.svc.GetJob(r.Context(), id, p.UserInternal, admin)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, job)
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	admin := p.HasPermission(rbac.PermPlatformSettingsWrite)
	job, err := h.svc.GetJob(r.Context(), id, p.UserInternal, admin)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	def, _, err := h.svc.Registry().ActionDef(job.Resource, job.Action)
	if err != nil || !p.HasPermission(def.Permission) {
		response.Forbidden(w, r, "Missing permission for bulk rollback")
		return
	}
	updated, err := h.svc.Rollback(r.Context(), id, p.UserInternal)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, updated)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, bulkusecase.ErrNotFound):
		response.NotFound(w, r, "Bulk job not found")
	case errors.Is(err, bulkusecase.ErrForbidden):
		response.Forbidden(w, r, "Forbidden")
	case errors.Is(err, bulkusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "unexpected error")
	}
}
