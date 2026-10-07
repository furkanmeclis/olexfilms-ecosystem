package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/google/uuid"
)

// pushData is the data object of web and Expo push messages.
func pushData(n db.Notification) map[string]any {
	data := map[string]any{"notification_uuid": n.Uuid.String()}
	if n.ActionUrl.Valid && n.ActionUrl.String != "" {
		data["action_url"] = n.ActionUrl.String
	}
	if n.TemplateCode.Valid && n.TemplateCode.String != "" {
		data["event_code"] = n.TemplateCode.String
	}
	if img := pushImage(n); img != "" {
		data["image_url"] = img
	}
	return data
}

// pushImage is the https image URL of a push (payload image_url, set by
// campaign pushes, TEC-407), or "".
func pushImage(n db.Notification) string {
	if len(n.Payload) == 0 {
		return ""
	}
	var p struct {
		ImageURL string `json:"image_url"`
	}
	if json.Unmarshal(n.Payload, &p) != nil || !strings.HasPrefix(p.ImageURL, "https://") {
		return ""
	}
	return p.ImageURL
}

// WebPushStore is the persistence used by WebPushProvider.
type WebPushStore interface {
	ListPushSubscriptionsByUser(ctx context.Context, userID int64) ([]db.PushSubscription, error)
	DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error
}

// VAPID holds Web Push keys.
type VAPID struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// WebPushSendFunc sends one web push message (webpush.SendNotification).
type WebPushSendFunc func(message []byte, s *webpush.Subscription, o *webpush.Options) (*http.Response, error)

// WebPushProvider sends a VAPID web push to every browser subscription of
// the user. Gone subscriptions (404/410) are deleted.
type WebPushProvider struct {
	Store WebPushStore
	VAPID VAPID
	Send  WebPushSendFunc // nil = webpush.SendNotification
}

func (WebPushProvider) Channel() string { return model.ChannelWebPush }

func (p WebPushProvider) Deliver(ctx context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	if p.VAPID.PublicKey == "" || p.VAPID.PrivateKey == "" {
		return DeliveryResult{}, errors.New("webpush: VAPID keys are not configured")
	}
	if p.Store == nil || !n.UserID.Valid {
		return DeliveryResult{}, ErrNoRecipient
	}
	subs, err := p.Store.ListPushSubscriptionsByUser(ctx, n.UserID.Int64)
	if err != nil {
		return DeliveryResult{}, err
	}
	if len(subs) == 0 {
		return DeliveryResult{}, ErrNoRecipient
	}
	send := p.Send
	if send == nil {
		send = webpush.SendNotification
	}
	msg := map[string]any{
		"title": n.Title, "body": msgtemplate.MarkdownToText(n.Body), "data": pushData(n),
	}
	if img := pushImage(n); img != "" {
		msg["image"] = img
	}
	payload, _ := json.Marshal(msg)
	sent := 0
	var lastErr error
	for _, sub := range subs {
		resp, err := send(payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.KeyP256dh, Auth: sub.KeyAuth},
		}, &webpush.Options{
			VAPIDPublicKey: p.VAPID.PublicKey, VAPIDPrivateKey: p.VAPID.PrivateKey,
			Subscriber: p.VAPID.Subject, TTL: 60,
		})
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound:
			_ = p.Store.DeletePushSubscriptionByEndpoint(ctx, sub.Endpoint)
		case resp.StatusCode >= 300:
			lastErr = fmt.Errorf("webpush: status %d", resp.StatusCode)
		default:
			sent++
		}
	}
	if sent == 0 {
		if lastErr != nil {
			return DeliveryResult{}, lastErr
		}
		return DeliveryResult{}, ErrNoRecipient
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "webpush", ProviderReference: fmt.Sprintf("%d/%d", sent, len(subs))}, nil
}

// ExpoStore is the persistence used by ExpoProvider.
type ExpoStore interface {
	ListActiveDevicePushTokens(ctx context.Context, userID int64) ([]db.DevicePushToken, error)
	RevokeDevicePushToken(ctx context.Context, arg db.RevokeDevicePushTokenParams) (int64, error)
}

// DefaultExpoURL is the Expo push send endpoint.
const DefaultExpoURL = "https://exp.host/--/api/v2/push/send"

// ExpoProvider sends through the Expo push HTTP API to every active device
// token of the user. Tokens Expo reports as DeviceNotRegistered are revoked.
type ExpoProvider struct {
	Store       ExpoStore
	URL         string // DefaultExpoURL when empty
	AccessToken string // optional (Expo enhanced security)
	HTTP        *http.Client
}

func (ExpoProvider) Channel() string { return model.ChannelExpoPush }

type expoMessage struct {
	To       string         `json:"to"`
	Title    string         `json:"title,omitempty"`
	Body     string         `json:"body"`
	Data     map[string]any `json:"data,omitempty"`
	Sound    string         `json:"sound,omitempty"`
	Priority string         `json:"priority,omitempty"`
	// RichContent carries the notification image (TEC-407).
	RichContent *expoRichContent `json:"richContent,omitempty"`
}

type expoRichContent struct {
	Image string `json:"image"`
}

type expoTicket struct {
	Status  string `json:"status"`
	ID      string `json:"id"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

func (p ExpoProvider) Deliver(ctx context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	if p.Store == nil || !n.UserID.Valid {
		return DeliveryResult{}, ErrNoRecipient
	}
	tokens, err := p.Store.ListActiveDevicePushTokens(ctx, n.UserID.Int64)
	if err != nil {
		return DeliveryResult{}, err
	}
	if len(tokens) == 0 {
		return DeliveryResult{}, ErrNoRecipient
	}
	priority := "default"
	if n.Priority == model.PriorityHigh || n.Priority == model.PriorityCritical {
		priority = "high"
	}
	var rich *expoRichContent
	if img := pushImage(n); img != "" {
		rich = &expoRichContent{Image: img}
	}
	msgs := make([]expoMessage, 0, len(tokens))
	for _, t := range tokens {
		msgs = append(msgs, expoMessage{
			To: t.ExpoToken, Title: n.Title, Body: msgtemplate.MarkdownToText(n.Body),
			Data: pushData(n), Sound: "default", Priority: priority, RichContent: rich,
		})
	}
	body, _ := json.Marshal(msgs)
	url := p.URL
	if url == "" {
		url = DefaultExpoURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return DeliveryResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if p.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.AccessToken)
	}
	client := p.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return DeliveryResult{}, fmt.Errorf("expo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return DeliveryResult{}, fmt.Errorf("expo: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Data []expoTicket `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return DeliveryResult{}, fmt.Errorf("expo: decode: %w", err)
	}
	var ids []string
	var lastErr string
	for i, t := range out.Data {
		if t.Status == "ok" {
			ids = append(ids, t.ID)
			continue
		}
		lastErr = t.Message
		if t.Details.Error == "DeviceNotRegistered" && i < len(tokens) {
			_, _ = p.Store.RevokeDevicePushToken(ctx, db.RevokeDevicePushTokenParams{ExpoToken: tokens[i].ExpoToken})
		}
	}
	if len(ids) == 0 {
		if lastErr == "" {
			lastErr = "no ticket"
		}
		return DeliveryResult{}, fmt.Errorf("expo: %s", lastErr)
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "expo", ProviderReference: strings.Join(ids, ",")}, nil
}
