package handler

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// totpPrincipal returns the caller when it may manage authenticator 2FA:
// every panel account, and on the portal only fleet users (TEC-637), who
// sign in with e-mail + password. Phone-OTP customers have no password
// login a second factor could guard, so they get 403 REALM_FORBIDDEN.
func totpPrincipal(w http.ResponseWriter, r *http.Request) (authctx.Principal, bool) {
	p := authctx.MustPrincipal(r.Context())
	if jwt.NormalizeAudience(p.Realm) == jwt.AudiencePortal && !p.HasPermission(rbac.PermFleetPortalRead) {
		response.Error(w, r, http.StatusForbidden, response.CodeRealmForbidden,
			"Two-factor authentication is available to fleet accounts only")
		return p, false
	}
	return p, true
}

func (h *Handler) TOTPStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := totpPrincipal(w, r)
	if !ok {
		return
	}
	status, err := h.uc.TOTPStatus(r.Context(), p.UserID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, status)
}

func (h *Handler) TOTPSetup(w http.ResponseWriter, r *http.Request) {
	p, ok := totpPrincipal(w, r)
	if !ok {
		return
	}
	out, err := h.uc.BeginTOTPSetup(r.Context(), p.UserID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) TOTPConfirm(w http.ResponseWriter, r *http.Request) {
	p, ok := totpPrincipal(w, r)
	if !ok {
		return
	}
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
	p, ok := totpPrincipal(w, r)
	if !ok {
		return
	}
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
