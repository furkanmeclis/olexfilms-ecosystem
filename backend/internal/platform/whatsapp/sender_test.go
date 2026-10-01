package whatsapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/fake"
)

func TestSenderFallback(t *testing.T) {
	ctx := context.Background()
	wa := &fake.Provider{}
	fallback := false
	s := &whatsapp.Sender{WhatsApp: wa, SMS: sms.Noop{}, SMSFallback: func(context.Context) bool { return fallback }}

	d, err := s.SendText(ctx, "+905551234567", "kod", whatsapp.SendOptions{})
	if err != nil || d.Channel != whatsapp.ChannelWhatsApp || d.ProviderRef == "" {
		t.Fatalf("whatsapp delivery: %+v %v", d, err)
	}

	wa.Err = errors.New("http 500")
	if _, err := s.SendText(ctx, "+905551234567", "kod", whatsapp.SendOptions{}); !errors.Is(err, whatsapp.ErrNoChannel) {
		t.Fatalf("fallback off must fail: %v", err)
	}
	fallback = true
	d, err = s.SendText(ctx, "+905551234567", "kod", whatsapp.SendOptions{})
	if err != nil || d.Channel != whatsapp.ChannelSMS || d.WhatsAppErr == nil {
		t.Fatalf("sms fallback: %+v %v", d, err)
	}
}
