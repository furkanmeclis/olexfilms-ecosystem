package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	oauthproviderusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Handler exposes OAuth provider integration HTTP endpoints.
type Handler struct {
	svc      *oauthproviderusecase.Service
	activity *activity.Recorder
}

// New creates an OAuth provider handler.
func New(svc *oauthproviderusecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	settings, err := h.svc.Get(r.Context(), provider)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, settings)
}

func (h *Handler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	var in oauthproviderusecase.PatchInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	settings, err := h.svc.Patch(r.Context(), provider, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		uid := actorInternalID(r)
		h.activity.Record(r.Context(), uid, "integrations.oauth.updated", "platform.integrations."+settings.Provider, nil, map[string]any{
			"provider":         settings.Provider,
			"login_enabled":    settings.LoginEnabled,
			"register_enabled": settings.RegisterEnabled,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, settings)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, oauthproviderusecase.ErrUnknownProvider) {
		response.NotFound(w, r, "Unknown OAuth provider")
		return
	}
	if errors.Is(err, oauthproviderusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	response.InternalErr(w, r, err, "failed to update OAuth provider settings")
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
