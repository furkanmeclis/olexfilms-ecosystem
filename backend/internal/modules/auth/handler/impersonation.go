package handler

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) ImpersonatePlatformUser(w http.ResponseWriter, r *http.Request) {
	targetUUID, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user uuid is invalid")
		return
	}

	p := authctx.MustPrincipal(r.Context())
	actor, err := h.uc.LoadUserByUUID(r.Context(), p.UserID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}

	result, err := h.uc.ImpersonatePlatformUser(
		r.Context(),
		actor,
		p.IsSuperAdmin,
		p.ImpersonatorUserID != nil,
		targetUUID,
		sessionMeta(r),
	)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}

	if h.activity != nil {
		h.activity.Record(r.Context(), &p.UserInternal, "users.impersonated", "platform.users", &targetUUID, map[string]any{
			"target_uuid": targetUUID.String(),
		}, r)
	}

	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) StopImpersonation(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if p.ImpersonatorUserID == nil {
		response.BadRequest(w, r, response.CodeValidationError, "Not in an impersonation session")
		return
	}

	result, err := h.uc.StopImpersonation(r.Context(), *p.ImpersonatorUserID, sessionMeta(r))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}

	if h.activity != nil {
		impersonator, loadErr := h.uc.LoadUserByUUID(r.Context(), *p.ImpersonatorUserID)
		if loadErr == nil {
			h.activity.Record(r.Context(), &impersonator.ID, "users.impersonation_stopped", "platform.users", &p.UserID, map[string]any{
				"target_uuid": p.UserID.String(),
			}, r)
		}
	}

	response.JSON(w, r, http.StatusOK, result)
}
