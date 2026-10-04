package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// SystemHandler serves the global system settings store (TEC-215).
type SystemHandler struct {
	svc *sysconfig.Service
}

// NewSystem creates the system settings handler.
func NewSystem(svc *sysconfig.Service) *SystemHandler {
	return &SystemHandler{svc: svc}
}

type systemSettingList struct {
	Items []sysconfig.Entry `json:"items"`
}

// List returns every catalog entry with its effective value.
func (h *SystemHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to load system settings")
		return
	}
	response.JSON(w, r, http.StatusOK, systemSettingList{Items: items})
}

// Get returns one entry.
func (h *SystemHandler) Get(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.Get(r.Context(), r.PathValue("key"))
	if h.fail(w, r, err, "failed to load system setting") {
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

type systemSettingInput struct {
	Value json.RawMessage `json:"value"`
}

// Put validates and stores a value (400 VALIDATION_ERROR on schema
// mismatch, 404 for a key outside the catalog).
func (h *SystemHandler) Put(w http.ResponseWriter, r *http.Request) {
	var in systemSettingInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var userID int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		userID = p.UserInternal
	}
	e, err := h.svc.Set(r.Context(), r.PathValue("key"), in.Value, userID)
	if h.fail(w, r, err, "failed to save system setting") {
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

// Reset removes the override so the key returns to its default.
func (h *SystemHandler) Reset(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.Reset(r.Context(), r.PathValue("key"))
	if h.fail(w, r, err, "failed to reset system setting") {
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

func (h *SystemHandler) fail(w http.ResponseWriter, r *http.Request, err error, msg string) bool {
	if err == nil {
		return false
	}
	var ve *sysconfig.ValidationError
	var re *sysconfig.RuleError
	switch {
	case errors.Is(err, sysconfig.ErrUnknownKey):
		response.NotFound(w, r, "unknown setting key")
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: "value", Message: ve.Message}})
	case errors.As(err, &re):
		response.Error(w, r, http.StatusUnprocessableEntity, re.Code, re.Message)
	default:
		response.InternalErr(w, r, err, msg)
	}
	return true
}
