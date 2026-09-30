package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	authsettingsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/authsettings/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Handler exposes auth settings HTTP endpoints.
type Handler struct {
	svc      *authsettingsusecase.Service
	activity *activity.Recorder
}

// New creates an auth settings handler.
func New(svc *authsettingsusecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc.Get(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to load auth settings")
		return
	}
	response.JSON(w, r, http.StatusOK, settings)
}

func (h *Handler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	var in authsettingsusecase.PatchInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	settings, err := h.svc.Patch(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		uid := actorInternalID(r)
		h.activity.Record(r.Context(), uid, "auth.settings.updated", "platform.auth.settings", nil, map[string]any{
			"registration_enabled": settings.RegistrationEnabled,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, settings)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, authsettingsusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if errors.Is(err, authsettingsusecase.ErrNotFound) {
		response.NotFound(w, r, "Resource not found")
		return
	}
	response.InternalErr(w, r, err, "failed to update auth settings")
}

func actorInternalID(r *http.Request) *int64 {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return nil
	}
	id := p.UserInternal
	return &id
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return err
	}
	return nil
}
