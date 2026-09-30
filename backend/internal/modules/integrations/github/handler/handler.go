package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	githubusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/github/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Handler exposes GitHub integration HTTP endpoints.
type Handler struct {
	svc      *githubusecase.Service
	activity *activity.Recorder
}

// New creates a GitHub integration handler.
func New(svc *githubusecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc.Get(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to load GitHub settings")
		return
	}
	response.JSON(w, r, http.StatusOK, settings)
}

func (h *Handler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	var in githubusecase.PatchInput
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
		h.activity.Record(r.Context(), uid, "integrations.github.updated", "platform.integrations.github", nil, map[string]any{
			"enabled":          settings.Enabled,
			"register_enabled": settings.RegisterEnabled,
			"app_id":           settings.AppID,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, settings)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, githubusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	response.InternalErr(w, r, err, "failed to update GitHub settings")
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
