package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) requireAdapterKey(w http.ResponseWriter, r *http.Request) bool {
	if h.adapterSecret == "" {
		response.Internal(w, r, "Adapter integration is not configured")
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Auth-Adapter-Key"))
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(h.adapterSecret)) != 1 {
		response.Unauthorized(w, r, "Invalid adapter credentials")
		return false
	}
	return true
}

func (h *Handler) AdapterGetUserByEmail(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	user, err := h.uc.GetAdapterUserByEmail(r.Context(), email)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, user)
}

func (h *Handler) AdapterGetUserByUUID(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid user uuid")
		return
	}
	user, err := h.uc.GetAdapterUserByUUID(r.Context(), id)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, user)
}

func (h *Handler) AdapterGetAuthenticator(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	credentialID := strings.TrimSpace(r.PathValue("credential_id"))
	authn, err := h.uc.GetAdapterAuthenticator(r.Context(), credentialID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, authn)
}

func (h *Handler) AdapterListAuthenticators(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid user uuid")
		return
	}
	items, err := h.uc.ListAdapterAuthenticatorsByUser(r.Context(), id)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	if items == nil {
		items = []model.AdapterAuthenticator{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) AdapterCreateAuthenticator(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	var in model.CreateAdapterAuthenticatorInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	authn, err := h.uc.CreateAdapterAuthenticator(r.Context(), in)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, authn)
}

func (h *Handler) AdapterUpdateAuthenticatorCounter(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	credentialID := strings.TrimSpace(r.PathValue("credential_id"))
	var in struct {
		Counter int64 `json:"counter"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.uc.UpdateAdapterAuthenticatorCounter(r.Context(), credentialID, in.Counter); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) AdapterDeleteAuthenticator(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	credentialID := strings.TrimSpace(r.PathValue("credential_id"))
	if err := h.uc.DeleteAdapterAuthenticator(r.Context(), credentialID); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) AdapterIssueSession(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	var in struct {
		UserUUID   string `json:"user_uuid"`
		AuthMethod string `json:"auth_method"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	userUUID, err := uuid.Parse(strings.TrimSpace(in.UserUUID))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid user uuid")
		return
	}
	tokens, err := h.uc.IssueSessionForUser(r.Context(), userUUID, sessionMeta(r), in.AuthMethod)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}
