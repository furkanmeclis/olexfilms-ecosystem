package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	items, err := h.uc.ListSessions(r.Context(), p.UserInternal, p.SessionID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	if items == nil {
		items = []model.DeviceSession{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if err := h.uc.RevokeOtherSessions(r.Context(), p.UserInternal, p.SessionID); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid session id")
		return
	}
	if err := h.uc.RevokeSession(r.Context(), p.UserInternal, id); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handler) allowAuthRate(w http.ResponseWriter, r *http.Request, fn func() (bool, time.Duration)) bool {
	if h.limiter == nil {
		return true
	}
	ok, retry := fn()
	return !h.writeRateLimited(w, r, ok, retry)
}

func (h *Handler) writeRateLimited(w http.ResponseWriter, r *http.Request, allowed bool, retry time.Duration) bool {
	if allowed {
		return false
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "Too many attempts. Try again later.")
	return true
}
