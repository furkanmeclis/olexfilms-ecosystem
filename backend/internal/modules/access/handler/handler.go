package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Handler exposes access policy HTTP endpoints.
type Handler struct {
	svc      *stepup.Service
	activity *activity.Recorder
}

// New creates an access settings handler.
func New(svc *stepup.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	policy, err := h.svc.GetPolicy(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to load access settings")
		return
	}
	response.JSON(w, r, http.StatusOK, policy)
}

func (h *Handler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	var in stepup.PatchPolicyInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	policy, err := h.svc.PatchPolicy(r.Context(), in)
	if err != nil {
		writePolicyError(w, r, err)
		return
	}
	if h.activity != nil {
		uid := actorInternalID(r)
		h.activity.Record(r.Context(), uid, "access.settings.updated", "platform.access", nil, map[string]any{
			"ttl_hours":                    policy.TTLHours,
			"password_enabled":             policy.PasswordEnabled,
			"passkey_enabled":              policy.PasskeyEnabled,
			"totp_enabled":                 policy.TOTPEnabled,
			"password_login_totp_required": policy.PasswordLoginTOTPRequired,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, policy)
}

func actorInternalID(r *http.Request) *int64 {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return nil
	}
	id := p.UserInternal
	return &id
}

func writePolicyError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, stepup.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	response.InternalErr(w, r, err, "failed to update access settings")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return err
	}
	return nil
}
