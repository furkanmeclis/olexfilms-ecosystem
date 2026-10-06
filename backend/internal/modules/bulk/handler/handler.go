package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/orglist"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Undo error codes (TEC-212), both HTTP 409.
const (
	CodeBulkUndoUnavailable = "BULK_UNDO_UNAVAILABLE"
	CodeBulkUndoExpired     = "BULK_UNDO_EXPIRED"
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

// ExecuteOrganizations runs a platform organizations bulk action
// (TEC-365). The request brand is stamped on the target query so the job
// (sync or async) stays inside it.
func (h *Handler) ExecuteOrganizations(w http.ResponseWriter, r *http.Request) {
	brand, ok := brandctx.From(r.Context())
	if !ok {
		response.NotFound(w, r, "brand not resolved")
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
	body.Target.Query[orglist.QueryBrandID] = strconv.FormatInt(brand.ID, 10)
	h.executeBody(w, r, adapters.ResourceOrganizations, body)
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
		"sync":      true,
		"summary":   result.Summary,
		"operation": result.Operation,
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

// UndoTenant reverts an operation of the active organization
// (POST /v1/tenant/bulk-operations/{uuid}/undo).
func (h *Handler) UndoTenant(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	orgID := scope.InternalID
	h.undo(w, r, &orgID)
}

// UndoPlatform reverts a platform operation (no organization)
// (POST /v1/platform/bulk-operations/{uuid}/undo).
func (h *Handler) UndoPlatform(w http.ResponseWriter, r *http.Request) {
	h.undo(w, r, nil)
}

func (h *Handler) undo(w http.ResponseWriter, r *http.Request, orgID *int64) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	op, err := h.svc.Undo(r.Context(), bulkusecase.UndoInput{
		UUID: id, ActorID: p.UserInternal, OrganizationID: orgID, HasPermission: p.HasPermission,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, op)
}

// ListTenantOperations pages the operations of the active organization
// (GET /v1/tenant/bulk-operations).
func (h *Handler) ListTenantOperations(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListOperations(r.Context(), scope.InternalID, q.Limit, q.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, bulkusecase.ErrUndoUnavailable):
		response.Conflict(w, r, CodeBulkUndoUnavailable, "Bulk operation cannot be undone (already undone or nothing to undo)")
	case errors.Is(err, bulkusecase.ErrUndoExpired):
		response.Conflict(w, r, CodeBulkUndoExpired, "Bulk operation undo window has expired")
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

// ExecuteTenantScoped is ExecuteTenant for a resource whose reach is the
// caller's permission scope (TEC-371, leads): besides the active
// organization it stamps the scope resolved by RequireScope and the
// requesting user (bulkengine.QueryScope / QueryActorUserID) on the target
// query, overwriting anything the client sent under those keys.
func (h *Handler) ExecuteTenantScoped(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, ok := orgctx.ScopeFrom(r.Context())
		if !ok {
			response.Forbidden(w, r, "Organization context required")
			return
		}
		filter, ok := scopefilter.From(r.Context())
		if !ok {
			response.Forbidden(w, r, "Permission scope required")
			return
		}
		encoded, ok := bulkengine.EncodeScope(filter)
		if !ok {
			response.Forbidden(w, r, "The permission scope reaches no organization")
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
		body.Target.Query[bulkengine.QueryScope] = encoded
		body.Target.Query[bulkengine.QueryActorUserID] = strconv.FormatInt(authctx.MustPrincipal(r.Context()).UserInternal, 10)
		h.executeBody(w, r, resource, body)
	}
}
