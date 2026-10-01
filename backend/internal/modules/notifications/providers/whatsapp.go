package providers

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/google/uuid"
)

// ChannelWhatsApp is the notification channel name of WhatsAppProvider.
// The notifications channel CHECK and registration in the provider list
// arrive with the notification center (TEC-87); this adapter is ready for it.
const ChannelWhatsApp = "whatsapp"

// WhatsAppProvider delivers a notification body over the WhatsApp provider
// (K21). The recipient is the E.164 phone in notifications.recipient.
type WhatsAppProvider struct {
	WA whatsapp.Provider
}

// Channel implements Provider.
func (WhatsAppProvider) Channel() string { return ChannelWhatsApp }

// Deliver implements Provider. The notification uuid is the idempotent
// message id, so a retried delivery is not sent twice by the gateway.
func (p WhatsAppProvider) Deliver(ctx context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	if !n.Recipient.Valid || n.Recipient.String == "" {
		return DeliveryResult{}, ErrNoRecipient
	}
	to, err := phone.NormalizeE164(n.Recipient.String, "")
	if err != nil {
		return DeliveryResult{}, whatsapp.ErrInvalidRecipient
	}
	if p.WA == nil {
		return DeliveryResult{}, whatsapp.ErrNotConfigured
	}
	body := n.Body
	if n.Title != "" {
		body = "*" + n.Title + "*\n" + n.Body
	}
	ref, err := p.WA.SendText(ctx, to, body, whatsapp.SendOptions{ID: notificationMessageID(n.Uuid)})
	if err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: p.WA.Name(), ProviderReference: ref.ID}, nil
}

func notificationMessageID(id uuid.UUID) string {
	b := [16]byte(id)
	const hexdigits = "0123456789ABCDEF"
	out := make([]byte, 0, 32)
	for _, c := range b {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}
