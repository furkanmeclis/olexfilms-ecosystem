package handler

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

func (h *Handler) TOTPStatus(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	status, err := h.uc.TOTPStatus(r.Context(), p.UserID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, status)
}

func (h *Handler) TOTPSetup(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	out, err := h.uc.BeginTOTPSetup(r.Context(), p.UserID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) TOTPConfirm(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	out, err := h.uc.ConfirmTOTPSetup(r.Context(), p.UserID, strings.TrimSpace(in.Code))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) TOTPDisable(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.uc.DisableTOTP(r.Context(), p.UserID, strings.TrimSpace(in.Code)); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "disabled"})
}
