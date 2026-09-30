package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

func (h *Handler) StepUpStatus(w http.ResponseWriter, r *http.Request) {
	if h.stepUp == nil {
		response.Internal(w, r, "Step-up verification is not configured")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	status, err := h.stepUp.Status(r.Context(), p.UserID)
	if err != nil {
		response.InternalErr(w, r, err, "failed to load step-up status")
		return
	}
	response.JSON(w, r, http.StatusOK, status)
}

func (h *Handler) StepUpPassword(w http.ResponseWriter, r *http.Request) {
	if h.stepUp == nil {
		response.Internal(w, r, "Step-up verification is not configured")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	grant, err := h.stepUp.VerifyPassword(r.Context(), p.UserID, strings.TrimSpace(in.Password))
	if err != nil {
		writeStepUpError(w, r, err)
		return
	}
	h.recordStepUpVerified(r, p.UserInternal, grant.Method)
	response.JSON(w, r, http.StatusOK, grant)
}

func (h *Handler) StepUpPasskeyOptions(w http.ResponseWriter, r *http.Request) {
	if h.stepUp == nil {
		response.Internal(w, r, "Step-up verification is not configured")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	options, err := h.stepUp.BeginPasskey(r.Context(), p.UserID)
	if err != nil {
		writeStepUpError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, options)
}

func (h *Handler) StepUpPasskeyVerify(w http.ResponseWriter, r *http.Request) {
	if h.stepUp == nil {
		response.Internal(w, r, "Step-up verification is not configured")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	grant, err := h.stepUp.VerifyPasskey(r.Context(), p.UserID, r)
	if err != nil {
		writeStepUpError(w, r, err)
		return
	}
	h.recordStepUpVerified(r, p.UserInternal, grant.Method)
	response.JSON(w, r, http.StatusOK, grant)
}

func (h *Handler) StepUpTOTP(w http.ResponseWriter, r *http.Request) {
	if h.stepUp == nil {
		response.Internal(w, r, "Step-up verification is not configured")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	grant, err := h.stepUp.VerifyTOTP(r.Context(), p.UserID, strings.TrimSpace(in.Code))
	if err != nil {
		writeStepUpError(w, r, err)
		return
	}
	h.recordStepUpVerified(r, p.UserInternal, grant.Method)
	response.JSON(w, r, http.StatusOK, grant)
}

func writeStepUpError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, stepup.ErrInvalidCredentials):
		response.Error(w, r, http.StatusUnauthorized, response.CodeInvalidCredentials, "Verification failed")
	case errors.Is(err, stepup.ErrRateLimited):
		response.Error(w, r, http.StatusTooManyRequests, response.CodeValidationError, "Too many failed attempts. Try again later.")
	case errors.Is(err, stepup.ErrMethodDisabled):
		response.BadRequest(w, r, response.CodeValidationError, "Verification method is disabled")
	case errors.Is(err, stepup.ErrNoPasskeys):
		response.BadRequest(w, r, response.CodeValidationError, "No passkeys registered for this account")
	case errors.Is(err, stepup.ErrNoTOTP):
		response.BadRequest(w, r, response.CodeValidationError, "Authenticator is not enabled for this account")
	case errors.Is(err, stepup.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "step-up verification failed")
	}
}

func (h *Handler) recordStepUpVerified(r *http.Request, actorID int64, method string) {
	if h.activity == nil {
		return
	}
	id := actorID
	h.activity.Record(r.Context(), &id, "stepup.verified", "auth.step_up", nil, map[string]any{
		"method": method,
	}, r)
}
