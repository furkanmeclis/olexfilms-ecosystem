package providers

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
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

// InappProvider marks in-app rows as delivered (row itself is the inbox).
type InappProvider struct{}

func (InappProvider) Channel() string { return model.ChannelInapp }

func (InappProvider) Deliver(_ context.Context, _ db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	return DeliveryResult{Status: model.StatusDelivered, Provider: "inapp"}, nil
}

// EmailProvider sends via SMTP.
type EmailProvider struct {
	Mail mail.Sender
}

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
	if err := p.Mail.Send(ctx, mail.Message{
		To: []string{to}, Subject: n.Title, Body: n.Body,
	}); err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "smtp"}, nil
}

// RealtimeProvider publishes to Centrifugo user channel.
type RealtimeProvider struct {
	Pub realtime.Publisher
}

func (RealtimeProvider) Channel() string { return model.ChannelRealtime }

func (p RealtimeProvider) Deliver(ctx context.Context, n db.Notification, userUUID *uuid.UUID) (DeliveryResult, error) {
	if p.Pub == nil {
		p.Pub = realtime.NoopPublisher{}
	}
	channel := ""
	if userUUID != nil {
		channel = "user:" + userUUID.String()
	}
	if channel == "" {
		return DeliveryResult{Status: model.StatusSent, Provider: "centrifugo"}, nil
	}
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
	payload := map[string]any{
		"type":         "notification.created",
		"notification": notification,
	}
	if err := p.Pub.Publish(ctx, channel, payload); err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "centrifugo"}, nil
}

// NoopProvider logs and marks sms/push as sent.
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

// ErrNoRecipient is returned when email has no recipient.
var ErrNoRecipient = errString("notification recipient required")

type errString string

func (e errString) Error() string { return string(e) }
