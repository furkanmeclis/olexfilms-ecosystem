package handler

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TenantOrganizationSummary (GET /v1/tenant/organizations/{uuid}/summary,
// TEC-1216): the organization, its parent ref and counters inside the
// caller's organizations.read scope.
func (h *Handler) TenantOrganizationSummary(w http.ResponseWriter, r *http.Request) {
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
	out, err := h.svc.OrganizationSummaryInScope(r.Context(), f, id, authctx.MustPrincipal(r.Context()).HasPermission)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// TenantMemberSummary (GET /v1/tenant/users/{uuid}/summary, TEC-1216): a
// staff user, the memberships, roles and warehouses inside the caller's
// members.read scope and what the user did there.
func (h *Handler) TenantMemberSummary(w http.ResponseWriter, r *http.Request) {
	f, ok := scopefilter.From(r.Context())
	if !ok {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermMembersRead)
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user uuid is invalid")
		return
	}
	out, err := h.svc.MemberSummaryInScope(r.Context(), f, id, authctx.MustPrincipal(r.Context()).HasPermission)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
