package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler exposes notification HTTP endpoints.
type Handler struct {
	svc *usecase.Service
}

// New creates a notification handler.
func New(svc *usecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.Notifications())
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(r.URL.Query())
	status := r.URL.Query().Get("status")
	channel := r.URL.Query().Get("channel")
	var unread *bool
	if raw := strings.TrimSpace(r.URL.Query().Get("unread")); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{
				Field: "unread", Message: "unread must be a boolean", Code: "invalid",
			}})
			return
		}
		unread = &b
	}
	page, err := h.svc.ListInbox(r.Context(), p.UserInternal, q, status, channel, unread)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	n, err := h.svc.UnreadCount(r.Context(), p.UserInternal)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]int64{"count": n})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "notification uuid is invalid")
		return
	}
	n, err := h.svc.GetOwned(r.Context(), p.UserInternal, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, n)
}

func (h *Handler) FollowAction(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "notification uuid is invalid")
		return
	}
	exp, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("exp")), 10, 64)
	if err != nil || exp <= 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: "exp", Message: "exp must be a unix timestamp", Code: "invalid",
		}})
		return
	}
	sig := strings.TrimSpace(r.URL.Query().Get("sig"))
	if sig == "" {
		response.ValidationError(w, r, []response.Detail{{
			Field: "sig", Message: "required", Code: "required",
		}})
		return
	}
	n, dest, err := h.svc.RedeemAction(r.Context(), p.UserInternal, id, exp, sig)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"destination":  dest,
		"notification": n,
	})
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "notification uuid is invalid")
		return
	}
	n, err := h.svc.MarkRead(r.Context(), p.UserInternal, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, n)
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if err := h.svc.MarkAllRead(r.Context(), p.UserInternal); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	prefs, err := h.svc.GetPreferences(r.Context(), p.UserInternal)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, prefs)
}

func (h *Handler) PutPreferences(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in model.Preferences
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	prefs, err := h.svc.UpsertPreferences(r.Context(), p.UserInternal, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, prefs)
}

func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Email bool `json:"email"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	channels := []string{model.ChannelInapp}
	if in.Email {
		channels = append(channels, model.ChannelEmail)
	}
	uid := p.UserInternal
	items, err := h.svc.Enqueue(r.Context(), model.EnqueueInput{
		UserID:       &uid,
		Channels:     channels,
		TemplateCode: "notifications.test",
		TemplateVars: map[string]string{"name": p.Email},
		SourceEvent:  "notifications.test",
		Title:        "Test bildirimi",
		Body:         "Bu bir test bildirimidir.",
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{"items": items})
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserUUID     string            `json:"user_uuid"`
		Channel      string            `json:"channel"`
		Title        string            `json:"title"`
		Body         string            `json:"body"`
		TemplateCode string            `json:"template_code"`
		Vars         map[string]string `json:"vars"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	uid, err := uuid.Parse(strings.TrimSpace(in.UserUUID))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user_uuid is invalid")
		return
	}
	userID, err := h.svc.ResolveUserID(r.Context(), uid)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	ch := strings.TrimSpace(in.Channel)
	if ch == "" {
		ch = model.ChannelInapp
	}
	items, err := h.svc.Enqueue(r.Context(), model.EnqueueInput{
		UserID:   &userID,
		Channels: []string{ch}, Title: in.Title, Body: in.Body,
		TemplateCode: in.TemplateCode, TemplateVars: in.Vars,
		SourceEvent: "notifications.send",
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{"items": items})
}

func (h *Handler) ListPlatform(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	q := apiquery.Parse(r.URL.Query())
	page, err := h.svc.ListPlatform(
		r.Context(),
		p,
		q,
		r.URL.Query().Get("status"),
		r.URL.Query().Get("channel"),
		r.URL.Query().Get("scope"),
		r.URL.Query().Get("user_uuid"),
	)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

func (h *Handler) PlatformMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformNotifications())
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return err
	}
	return nil
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Notification was not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "You do not have access to this resource")
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func (h *Handler) VAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]string{
		"vapid_public_key": h.svc.VAPIDPublicKey(),
	})
}

func (h *Handler) UpsertPushSubscription(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var body struct {
		Endpoint  string `json:"endpoint"`
		KeyP256dh string `json:"key_p256dh"`
		KeyAuth   string `json:"key_auth"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if strings.TrimSpace(body.Endpoint) == "" {
		response.ValidationError(w, r, []response.Detail{{
			Field: "endpoint", Message: "required", Code: "required",
		}})
		return
	}
	sub, err := h.svc.UpsertPushSubscription(r.Context(), p.UserInternal, body.Endpoint, body.KeyP256dh, body.KeyAuth)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"uuid":     sub.Uuid,
		"endpoint": sub.Endpoint,
	})
}

func (h *Handler) DeletePushSubscription(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if err := h.svc.DeletePushSubscription(r.Context(), p.UserInternal, body.Endpoint); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}
