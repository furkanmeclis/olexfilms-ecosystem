// Package providers holds the channel drivers of the notification center:
// inapp (+ Centrifugo), email (HTML), webpush (VAPID), expo_push (Expo HTTP
// API), sms (driver behind an admin switch) and whatsapp (TEC-92 provider).
package providers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/google/uuid"
)

// DeliveryResult is the outcome of a provider send.
type DeliveryResult struct {
	Status            string
	Provider          string
	ProviderReference string
}

// Provider delivers a notification on a channel.
type Provider interface {
	Channel() string
	Deliver(ctx context.Context, n db.Notification, userUUID *uuid.UUID) (DeliveryResult, error)
}

// ErrNoRecipient is returned when the channel has no address for the user
// (no e-mail, phone, push subscription or Expo token). The delivery is
// recorded as skipped_no_recipient, not failed.
var ErrNoRecipient = errors.New("notification recipient required")

// InappProvider marks in-app rows delivered (the row is the inbox) and
// publishes them to the user's Centrifugo channel user:{uuid}.
type InappProvider struct {
	Pub realtime.Publisher
	Log *slog.Logger
}

func (InappProvider) Channel() string { return model.ChannelInapp }

func (p InappProvider) Deliver(ctx context.Context, n db.Notification, userUUID *uuid.UUID) (DeliveryResult, error) {
	if p.Pub != nil && userUUID != nil {
		if err := p.Pub.Publish(ctx, realtime.UserChannel(*userUUID), RealtimePayload(n)); err != nil && p.Log != nil {
			// The inbox row is the source of truth; the client refetches.
			p.Log.Warn("notification_realtime_publish_failed", "uuid", n.Uuid, "error", err)
		}
	}
	return DeliveryResult{Status: model.StatusDelivered, Provider: "inapp"}, nil
}

// RealtimePayload is the Centrifugo message for an in-app notification.
func RealtimePayload(n db.Notification) map[string]any {
	notification := map[string]any{
		"uuid": n.Uuid, "channel": n.Channel, "title": n.Title, "body": n.Body,
		"status": n.Status, "priority": n.Priority,
	}
	if n.ActionUrl.Valid && n.ActionUrl.String != "" {
		notification["action_url"] = n.ActionUrl.String
	}
	if n.TemplateCode.Valid && n.TemplateCode.String != "" {
		notification["template_code"] = n.TemplateCode.String
	}
	if len(n.Payload) > 0 {
		var extra map[string]any
		if err := json.Unmarshal(n.Payload, &extra); err == nil && len(extra) > 0 {
			notification["payload"] = extra
		}
	}
	return map[string]any{"type": "notification.created", "notification": notification}
}

// EmailBrand is the e-mail frame of a brand.
type EmailBrand struct {
	Name    string
	LogoURL string
	Color   string
}

// EmailProvider sends a multipart (text + HTML) e-mail. The body is the
// rendered template markdown; the HTML frame takes dir from the language.
// A notification with an action URL gets a primary button (relative paths
// are resolved against BaseURL; without one the button is left out).
type EmailProvider struct {
	Mail mail.Sender
	// Brand returns the frame for a brand id (0 = no brand). Optional.
	Brand func(ctx context.Context, brandID int64) EmailBrand
	// BaseURL is the public web origin (PUBLIC_FRONTEND_URL) that
	// relative action URLs are resolved against. Optional.
	BaseURL string
}

// PayloadPreheader is the notification payload key of an explicit inbox
// preview text; without it the preheader is derived from the body.
const PayloadPreheader = model.PayloadEmailPreheader

func (EmailProvider) Channel() string { return model.ChannelEmail }

func (p EmailProvider) Deliver(ctx context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	to := ""
	if n.Recipient.Valid {
		to = n.Recipient.String
	}
	if to == "" {
		return DeliveryResult{}, ErrNoRecipient
	}
	if p.Mail == nil {
		p.Mail = mail.NoopSender{}
	}
	msg, err := p.Message(ctx, n)
	if err != nil {
		return DeliveryResult{}, err
	}
	msg.To = []string{to}
	if err := p.Mail.Send(ctx, msg); err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "smtp"}, nil
}

// HTML renders the branded HTML part of a notification.
func (p EmailProvider) HTML(ctx context.Context, n db.Notification) (string, error) {
	return p.Layout(ctx, n).RenderHTML()
}

// Message renders the subject, the plain-text and the HTML part (no
// recipient).
func (p EmailProvider) Message(ctx context.Context, n db.Notification) (mail.Message, error) {
	l := p.Layout(ctx, n)
	html, err := l.RenderHTML()
	if err != nil {
		return mail.Message{}, err
	}
	return mail.Message{Subject: n.Title, Body: l.RenderText(), HTMLBody: html}, nil
}

// Layout is the e-mail frame of a notification in its language.
func (p EmailProvider) Layout(ctx context.Context, n db.Notification) mail.Layout {
	lang := i18n.DefaultLocale
	if n.Language.Valid {
		lang = i18n.Normalize(n.Language.String)
	}
	brand := EmailBrand{Name: mail.DefaultBrandName}
	if p.Brand != nil {
		var id int64
		if n.BrandID.Valid {
			id = n.BrandID.Int64
		}
		if b := p.Brand(ctx, id); b.Name != "" {
			brand = b
		}
	}
	labels := mail.LabelsFor(string(lang))
	l := mail.Layout{
		Lang: string(lang), Dir: i18n.Dir(lang), Title: n.Title,
		BrandName: brand.Name, LogoURL: brand.LogoURL, Color: brand.Color,
		BodyHTML: msgtemplate.MarkdownToHTML(n.Body), BodyText: msgtemplate.MarkdownToText(n.Body),
		Preheader: payloadString(n.Payload, PayloadPreheader),
		Reason:    labels.ReasonFor(brand.Name),
	}
	if n.ActionUrl.Valid {
		if u := AbsoluteURL(p.BaseURL, n.ActionUrl.String); u != "" {
			l.ButtonLabel, l.ButtonURL = labels.Button, u
			l.Secondary, l.SecondaryURL = labels.LinkFallback, u
		}
	}
	return l
}

// AbsoluteURL returns an absolute http(s) action URL: absolute URLs as
// they are, paths ("/x") prefixed with base. Empty when neither works.
func AbsoluteURL(base, u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if mail.SafeURL(u) {
		return u
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") {
		return ""
	}
	if abs := base + u; mail.SafeURL(abs) {
		return abs
	}
	return ""
}

func payloadString(raw []byte, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

// SMSProvider sends the plain-text body as SMS to notifications.recipient
// (E.164). The admin switch (notification_channel_settings.sms, off by
// default, K21) is checked by the dispatcher before a row is created.
type SMSProvider struct {
	SMS sms.Provider
}

func (SMSProvider) Channel() string { return model.ChannelSMS }

func (p SMSProvider) Deliver(ctx context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	if !n.Recipient.Valid || n.Recipient.String == "" {
		return DeliveryResult{}, ErrNoRecipient
	}
	if p.SMS == nil {
		p.SMS = sms.Noop{}
	}
	body := msgtemplate.MarkdownToText(n.Body)
	if n.Title != "" {
		body = n.Title + "\n" + body
	}
	ref, err := p.SMS.Send(ctx, n.Recipient.String, body)
	if err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: p.SMS.Name(), ProviderReference: ref}, nil
}

// NoopProvider logs and marks a channel sent (no driver configured).
type NoopProvider struct {
	Name string
	Log  *slog.Logger
}

func (p NoopProvider) Channel() string { return p.Name }

func (p NoopProvider) Deliver(_ context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	if p.Log != nil {
		p.Log.Info("notification_noop_provider", "channel", p.Name, "uuid", n.Uuid)
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "noop"}, nil
}
