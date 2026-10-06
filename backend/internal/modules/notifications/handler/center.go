package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Events lists the notification event catalog (GET /v1/notification-events).
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": h.svc.Events()})
}

// RegisterPushDevice stores the caller's Expo push token
// (POST /v1/notifications/push-devices).
func (h *Handler) RegisterPushDevice(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in usecase.PushDeviceInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	dev, err := h.svc.RegisterPushDevice(r.Context(), p.UserInternal, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, dev)
}

// RevokePushDevice revokes the caller's Expo push token
// (DELETE /v1/notifications/push-devices).
func (h *Handler) RevokePushDevice(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		ExpoToken string `json:"expo_token"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if strings.TrimSpace(in.ExpoToken) == "" {
		response.ValidationError(w, r, []response.Detail{{Field: "expo_token", Message: "required", Code: "required"}})
		return
	}
	if err := h.svc.RevokePushDevice(r.Context(), p.UserInternal, in.ExpoToken); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// ListTemplates (GET /v1/platform/notification-templates).
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	items, err := h.svc.ListTemplates(r.Context(), usecase.TemplateFilter{
		Code: qv.Get("code"), Channel: qv.Get("channel"), Language: qv.Get("language"), Role: qv.Get("role"),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// UpsertTemplate (PUT /v1/platform/notification-templates).
func (h *Handler) UpsertTemplate(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in model.TemplateInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	tpl, err := h.svc.UpsertTemplate(r.Context(), p.UserInternal, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tpl)
}

// PreviewTemplate (POST /v1/platform/notification-templates/preview).
func (h *Handler) PreviewTemplate(w http.ResponseWriter, r *http.Request) {
	var in model.TemplateInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	out, err := h.svc.PreviewTemplate(r.Context(), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListChannels (GET /v1/platform/notification-channels).
func (h *Handler) ListChannels(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ChannelSettings(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SetChannel (PUT /v1/platform/notification-channels/{channel}).
func (h *Handler) SetChannel(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.Enabled == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "enabled", Message: "required", Code: "required"}})
		return
	}
	out, err := h.svc.SetChannelEnabled(r.Context(), p.UserInternal, r.PathValue("channel"), *in.Enabled)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListDeliveries (GET /v1/platform/notification-deliveries).
func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	f := usecase.DeliveryFilter{EventCode: qv.Get("event_code")}
	var err error
	if f.Statuses, err = apiquery.EnumList(qv, "status", model.DeliveryStatuses...); err != nil {
		writeErr(w, r, err)
		return
	}
	if f.Channels, err = apiquery.EnumList(qv, "channel", model.NotificationChannels...); err != nil {
		writeErr(w, r, err)
		return
	}
	if f.Created, err = apiquery.DateRange(qv, "created"); err != nil {
		writeErr(w, r, err)
		return
	}
	for name, dst := range map[string]**uuid.UUID{"event_id": &f.EventID, "user_uuid": &f.UserUUID} {
		raw := strings.TrimSpace(qv.Get(name))
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
			return
		}
		*dst = &id
	}
	page, err := h.svc.ListDeliveries(r.Context(), apiquery.Parse(qv), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func writeCenterErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message, Code: "invalid"}})
		return true
	}
	if errors.Is(err, usecase.ErrUnknownEvent) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return true
	}
	return false
}
