package handler

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	list, err := h.uc.ListPasskeys(r.Context(), p.UserInternal)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, list)
}

func (h *Handler) PatchPasskey(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	passkeyUUID, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid passkey uuid")
		return
	}
	var in struct {
		Name *string `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	passkey, err := h.uc.RenamePasskey(r.Context(), p.UserID, passkeyUUID, in.Name)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, passkey)
}

func (h *Handler) DeletePasskey(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	passkeyUUID, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid passkey uuid")
		return
	}
	if err := h.uc.DeletePasskey(r.Context(), p.UserID, passkeyUUID); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}
