package migrator

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

func TestMapLegacyShortTarget(t *testing.T) {
	cases := []struct {
		raw, want, reason string
	}{
		// customer.notify: encrypted customer id -> portal (OTP login).
		{"https://olexfilms.app/customer/eyJpdiI6IkFCQyJ9", "/portal", ""},
		{"https://olexfilms.app/customer/eyJpdiI6IkFCQyJ9/", "/portal", ""},
		{"https://hub.example.test/garanti/SYN-S-0001", "/garanti/SYN-S-0001", ""},
		{"https://hub.example.test/bayi/SYN00001?lang=tr", "/bayi/SYN00001?lang=tr", ""},
		{"https://hub.example.test/portal", "/portal", ""},
		// External and unknown targets are reported, never redirected to.
		{"https://g.page/r/SYNTHETIC-review", "", ShortTargetUnmapped},
		{"https://www.google.com/maps/place/x", "", ShortTargetUnmapped},
		{"https://olexfilms.app/warranty/SRV-1", "", ShortTargetUnmapped},
		{"https://olexfilms.app/customer/abc/update", "", ShortTargetUnmapped},
		{"https://olexfilms.app/garanti/../admin", "", ShortTargetUnmapped},
		{"https://olexfilms.app/", "", ShortTargetUnmapped},
		{"javascript:alert(1)", "", ShortTargetInvalid},
		{"/customer/x", "", ShortTargetInvalid},
		{"", "", ShortTargetInvalid},
	}
	for _, c := range cases {
		got, reason := MapLegacyShortTarget(c.raw)
		if got != c.want || reason != c.reason {
			t.Errorf("MapLegacyShortTarget(%q) = %q, %q; want %q, %q", c.raw, got, reason, c.want, c.reason)
		}
	}
}

func TestSMSChannel(t *testing.T) {
	for raw, want := range map[string]string{"sms": ChannelSMS, "": ChannelSMS, "WhatsApp": ChannelWhatsApp, "whatsapp": ChannelWhatsApp} {
		if got, ok := SMSChannel(raw); got != want || !ok {
			t.Errorf("SMSChannel(%q) = %q, %v", raw, got, ok)
		}
	}
	if got, ok := SMSChannel("fax"); got != ChannelSMS || ok {
		t.Errorf("unknown channel = %q, %v; want sms, false", got, ok)
	}
}

func TestNotificationBody(t *testing.T) {
	if got := NotificationBody(`{"title": "Başlık", "body": "Metin"}`); got != "Başlık\nMetin" {
		t.Errorf("title+body = %q", got)
	}
	if got := NotificationBody(`{"x": 1}`); got != `{"x": 1}` {
		t.Errorf("no title = %q", got)
	}
	if got := NotificationBody("plain"); got != "plain" {
		t.Errorf("not json = %q", got)
	}
}

// The step queries must pass the read-only source guard (HANDOFF §5).
func TestMessageQueriesPassGuard(t *testing.T) {
	for _, q := range []string{
		legacyShortURLsQuery, legacySMSLogsQuery, legacyNotificationsQuery,
		legacyCustomerDealersQuery, legacyUserDealersQuery,
		legacyShortURLsQuery + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id",
	} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}
}
