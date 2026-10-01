// Package handler exposes the WhatsApp admin API and the wuzapi webhook.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/wuzapi"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Error codes.
const (
	CodeNotConfigured = "WHATSAPP_NOT_CONFIGURED"
	CodeNotConnected  = "WHATSAPP_NOT_CONNECTED"
	CodeGatewayError  = "WHATSAPP_GATEWAY_ERROR"
	CodeBadSignature  = "INVALID_SIGNATURE"
	maxWebhookBytes   = 8 << 20
)

// Handler serves /v1/platform/whatsapp/* and /hooks/wuzapi.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	log      *slog.Logger
}

// New creates the handler.
func New(svc *usecase.Service, rec *activity.Recorder, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, activity: rec, log: log}
}

// Webhook receives wuzapi events. The signature is checked over the raw
// body before anything is parsed; unsigned or forged requests get 401.
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "body too large")
		return
	}
	res, err := h.svc.HandleWebhook(r.Context(), r.Header, body)
	if err != nil {
		if errors.Is(err, whatsapp.ErrInvalidSignature) {
			response.Error(w, r, http.StatusUnauthorized, CodeBadSignature, "invalid signature")
			return
		}
		if errors.Is(err, usecase.ErrNotConfigured) {
			response.ServiceUnavailable(w, r, CodeNotConfigured, "WhatsApp gateway is not configured")
			return
		}
		// 5xx makes wuzapi retry the delivery; inserts are idempotent.
		h.log.Error("whatsapp_webhook_failed", "error", err)
		response.Internal(w, r, "webhook processing failed")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]int{
		"messages": res.Messages, "duplicates": res.Duplicates, "statuses": res.Statuses,
		"connections": res.Connections, "alarms": res.Alarms,
	})
}

// GetOverview returns the gateway status and settings.
func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Overview(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Connect starts the session.
func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Connect(r.Context()); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "whatsapp.connect", nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "connecting"})
}

// QR returns the QR code while not logged in.
func (h *Handler) QR(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.QR(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PairPhone returns a pairing code for a phone.
func (h *Handler) PairPhone(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	code, err := h.svc.PairPhone(r.Context(), in.Phone)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "whatsapp.pair_phone", nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"linking_code": code})
}

// Logout ends the session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context()); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "whatsapp.logout", nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "logged_out"})
}

// TestMessage sends a test text.
func (h *Handler) TestMessage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
		Body  string `json:"body"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	out, err := h.svc.SendTestMessage(r.Context(), in.Phone, in.Body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "whatsapp.test_message", map[string]any{"message_id": out.MessageID})
	response.JSON(w, r, http.StatusOK, out)
}

// PutSettings updates the SMS fallback switch and KVKK texts.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in usecase.SettingsInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.svc.UpdateSettings(r.Context(), in, actorID(r)); err != nil {
		writeError(w, r, err)
		return
	}
	locales := make([]string, 0, len(in.KVKK))
	for l := range in.KVKK {
		locales = append(locales, l)
	}
	h.record(r, "whatsapp.settings.updated", map[string]any{"sms_fallback_enabled": in.SMSFallbackEnabled, "kvkk_locales": locales})
	out, err := h.svc.Overview(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) record(r *http.Request, action string, meta map[string]any) {
	if h.activity == nil {
		return
	}
	h.activity.Record(r.Context(), actorID(r), action, "platform.integrations.whatsapp", nil, meta, r)
}

func actorID(r *http.Request) *int64 {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return nil
	}
	id := p.UserInternal
	return &id
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *wuzapi.APIError
	switch {
	case errors.Is(err, usecase.ErrInvalidRequest), errors.Is(err, whatsapp.ErrInvalidRecipient):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrNotConfigured), errors.Is(err, whatsapp.ErrNotConfigured):
		response.ServiceUnavailable(w, r, CodeNotConfigured, "WhatsApp gateway is not configured")
	case errors.Is(err, whatsapp.ErrNotConnected):
		response.Conflict(w, r, CodeNotConnected, "WhatsApp session is not connected")
	case errors.As(err, &apiErr):
		response.Error(w, r, http.StatusBadGateway, CodeGatewayError, apiErr.Message)
	default:
		response.InternalErr(w, r, err, "whatsapp request failed")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return err
	}
	return nil
}
