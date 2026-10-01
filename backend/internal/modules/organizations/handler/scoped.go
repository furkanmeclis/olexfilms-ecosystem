package handler

import (
	"context"
	"net/http"
	"strconv"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// StepUpChecker reports whether a user has a recent step-up grant.
type StepUpChecker interface {
	HasValidGrant(ctx context.Context, userID uuid.UUID) (bool, error)
}

// SetStepUp enables the step-up check on sensitive organization changes.
func (h *Handler) SetStepUp(s StepUpChecker) {
	h.stepUp = s
}

// requireSupplierChange guards a parent (distributor) change: the
// organizations.supplier.write permission plus a recent step-up (K25).
func (h *Handler) requireSupplierChange(w http.ResponseWriter, r *http.Request) bool {
	p := authctx.MustPrincipal(r.Context())
	if !p.HasPermission(rbac.PermOrganizationsSupplierWrite) {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermOrganizationsSupplierWrite)
		return false
	}
	if h.stepUp == nil {
		response.StepUpRequired(w, r)
		return false
	}
	ok, err := h.stepUp.HasValidGrant(r.Context(), p.UserID)
	if err != nil {
		response.Internal(w, r, "Failed to verify step-up grant")
		return false
	}
	if !ok {
		response.StepUpRequired(w, r)
		return false
	}
	return true
}

// TenantListOrganizations lists organizations reachable through the caller's
// organizations.read scope (GET /v1/tenant/organizations).
func (h *Handler) TenantListOrganizations(w http.ResponseWriter, r *http.Request) {
	f, ok := scopefilter.From(r.Context())
	if !ok {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermOrganizationsRead)
		return
	}
	in := orgusecase.ScopedListInput{Type: r.URL.Query().Get("type")}
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		in.Limit = int32(v)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil {
		in.Offset = int32(v)
	}
	items, err := h.svc.ListInScope(r.Context(), f, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "scope": f.Scope})
}

// TenantGetOrganization returns one organization inside the caller's scope
// (GET /v1/tenant/organizations/{uuid}); others read as 404.
func (h *Handler) TenantGetOrganization(w http.ResponseWriter, r *http.Request) {
	f, ok := scopefilter.From(r.Context())
	if !ok {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermOrganizationsRead)
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	org, err := h.svc.GetInScope(r.Context(), f, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, org)
}
