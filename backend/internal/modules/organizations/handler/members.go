package handler

import (
	"errors"
	"net/http"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// memberPath parses {uuid} (organization) and {user_uuid} (member).
func memberPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	orgID, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return uuid.Nil, uuid.Nil, false
	}
	userID, err := uuid.Parse(r.PathValue("user_uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user uuid is invalid")
		return uuid.Nil, uuid.Nil, false
	}
	return orgID, userID, true
}

// PlatformUpdateMember replaces the membership kind and organization roles
// of a member: PUT /v1/platform/organizations/{uuid}/members/{user_uuid}.
func (h *Handler) PlatformUpdateMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := memberPath(w, r)
	if !ok {
		return
	}
	var in struct {
		Role      string   `json:"role"`
		RoleSlugs []string `json:"role_slugs"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	out, err := h.svc.UpdateMember(r.Context(), orgID, actor, activity.MetaFromRequest(r), orgusecase.UpdateMemberInput{
		UserUUID: userID, Role: in.Role, RoleSlugs: in.RoleSlugs,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PlatformRemoveMember removes a member from an organization:
// DELETE /v1/platform/organizations/{uuid}/members/{user_uuid}.
func (h *Handler) PlatformRemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := memberPath(w, r)
	if !ok {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	if err := h.svc.RemoveMember(r.Context(), orgID, actor, activity.MetaFromRequest(r), userID); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PlatformUserOverview is the user 360: GET /v1/platform/users/{uuid}/overview.
func (h *Handler) PlatformUserOverview(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user uuid is invalid")
		return
	}
	out, err := h.svc.UserOverview(r.Context(), id)
	if errors.Is(err, orgusecase.ErrNotFound) {
		response.NotFound(w, r, "User was not found")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
