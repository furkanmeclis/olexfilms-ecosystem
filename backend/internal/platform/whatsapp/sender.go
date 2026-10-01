package whatsapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
)

// Channels a Delivery can report.
const (
	ChannelWhatsApp = "whatsapp"
	ChannelSMS      = "sms"
)

// Delivery is the outcome of Sender.SendText.
type Delivery struct {
	Channel     string
	Provider    string
	ProviderRef string
	// WhatsAppErr is set when WhatsApp failed and SMS took over.
	WhatsAppErr error
}

// Sender sends critical texts (OTP) synchronously: WhatsApp first, then SMS
// when the admin enabled the fallback (K21).
type Sender struct {
	WhatsApp Provider
	SMS      sms.Provider
	// SMSFallback reports whether the SMS fallback is enabled (admin setting).
	SMSFallback func(ctx context.Context) bool
}

// ErrNoChannel is returned when no channel could deliver the message.
var ErrNoChannel = errors.New("whatsapp: no channel delivered the message")

// SendText delivers body to an E.164 number.
func (s *Sender) SendText(ctx context.Context, to, body string, opts SendOptions) (Delivery, error) {
	var waErr error
	if s.WhatsApp != nil {
		ref, err := s.WhatsApp.SendText(ctx, to, body, opts)
		if err == nil {
			return Delivery{Channel: ChannelWhatsApp, Provider: s.WhatsApp.Name(), ProviderRef: ref.ID}, nil
		}
		waErr = err
	} else {
		waErr = ErrNotConfigured
	}
	if s.SMS == nil || s.SMSFallback == nil || !s.SMSFallback(ctx) {
		return Delivery{}, fmt.Errorf("%w: %v", ErrNoChannel, waErr)
	}
	ref, err := s.SMS.Send(ctx, to, body)
	if err != nil {
		return Delivery{}, fmt.Errorf("%w: whatsapp: %v; sms: %v", ErrNoChannel, waErr, err)
	}
	return Delivery{Channel: ChannelSMS, Provider: s.SMS.Name(), ProviderRef: ref, WhatsAppErr: waErr}, nil
}
