package wuzapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

const testKey = "hmac-secret-0123456789abcdef0123456789"

// SampleMessage is a wuzapi 1.0.9 JSON-mode Message webhook body.
const sampleMessage = `{"type":"Message","token":"user-token","userID":"u1","instanceName":"olexfilms","event":{"Info":{"ID":"3EB0A1B2C3","Chat":"905551234567@s.whatsapp.net","Sender":"905551234567@s.whatsapp.net","IsFromMe":false,"IsGroup":false,"PushName":"Ali","Timestamp":"2026-10-01T12:00:00+03:00","Type":"text"},"Message":{"conversation":"Merhaba, randevu almak istiyorum"}}}`

func signed(body string) http.Header {
	h := http.Header{}
	h.Set(SignatureHeader, Sign(testKey, []byte(body)))
	return h
}

func TestParseWebhookMessage(t *testing.T) {
	c := New(Config{HMACKey: testKey})
	c.SetUserToken("user-token")
	evs, err := c.ParseWebhook(signed(sampleMessage), []byte(sampleMessage))
	if err != nil || len(evs) != 1 {
		t.Fatalf("parse: %v %v", evs, err)
	}
	ev := evs[0]
	if ev.Kind != whatsapp.KindMessage || ev.ExternalID != "3EB0A1B2C3" || ev.From != "+905551234567" ||
		ev.Text != "Merhaba, randevu almak istiyorum" || ev.PushName != "Ali" || ev.Timestamp.IsZero() {
		t.Fatalf("event = %+v", ev)
	}
}

func TestParseWebhookSignature(t *testing.T) {
	c := New(Config{HMACKey: testKey})
	body := []byte(sampleMessage)
	if _, err := c.ParseWebhook(http.Header{}, body); !errors.Is(err, whatsapp.ErrInvalidSignature) {
		t.Fatalf("missing signature: %v", err)
	}
	bad := http.Header{}
	bad.Set(SignatureHeader, Sign("other-key-0123456789abcdef0123456789", body))
	if _, err := c.ParseWebhook(bad, body); !errors.Is(err, whatsapp.ErrInvalidSignature) {
		t.Fatalf("wrong key: %v", err)
	}
	tampered := []byte(sampleMessage + " ")
	if _, err := c.ParseWebhook(signed(sampleMessage), tampered); !errors.Is(err, whatsapp.ErrInvalidSignature) {
		t.Fatalf("tampered body: %v", err)
	}
	// No key configured: fail closed.
	if _, err := New(Config{}).ParseWebhook(signed(sampleMessage), body); !errors.Is(err, whatsapp.ErrInvalidSignature) {
		t.Fatalf("empty key: %v", err)
	}
	// Signed but for another instance token.
	c.SetUserToken("another-token")
	if _, err := c.ParseWebhook(signed(sampleMessage), body); !errors.Is(err, whatsapp.ErrInvalidSignature) {
		t.Fatalf("token mismatch: %v", err)
	}
}

func TestParseWebhookConnectionAndReceipts(t *testing.T) {
	c := New(Config{HMACKey: testKey})
	cases := []struct {
		body, kind, conn, status string
	}{
		{`{"type":"LoggedOut","event":{"OnConnect":false,"Reason":401}}`, whatsapp.KindConnection, whatsapp.StateLoggedOut, ""},
		{`{"type":"TemporaryBan","event":{"Code":101,"Expire":3600}}`, whatsapp.KindConnection, whatsapp.StateBanned, ""},
		{`{"type":"Connected","event":{}}`, whatsapp.KindConnection, whatsapp.StateConnected, ""},
		{`{"type":"Disconnected","event":{}}`, whatsapp.KindConnection, whatsapp.StateDisconnected, ""},
		{`{"type":"ReadReceipt","state":"Read","event":{"MessageIDs":["A1","A2"],"Chat":"905551234567@s.whatsapp.net","Type":"read"}}`, whatsapp.KindStatus, "", "read"},
	}
	for _, tc := range cases {
		evs, err := c.ParseWebhook(signed(tc.body), []byte(tc.body))
		if err != nil || len(evs) != 1 {
			t.Fatalf("%s: %v %v", tc.body, evs, err)
		}
		if evs[0].Kind != tc.kind || evs[0].Connection != tc.conn || evs[0].Status != tc.status {
			t.Fatalf("%s: %+v", tc.body, evs[0])
		}
	}
	evs, _ := c.ParseWebhook(signed(`{"type":"TemporaryBan","event":{"Code":101,"Expire":3600}}`), []byte(`{"type":"TemporaryBan","event":{"Code":101,"Expire":3600}}`))
	if evs[0].Reason != "temporary_ban:101 expire=3600" {
		t.Fatalf("ban reason = %q", evs[0].Reason)
	}
	// Group and unknown events are ignored.
	group := `{"type":"Message","event":{"Info":{"ID":"G1","Chat":"123@g.us","IsGroup":true},"Message":{"conversation":"x"}}}`
	if evs, err := c.ParseWebhook(signed(group), []byte(group)); err != nil || len(evs) != 0 {
		t.Fatalf("group: %v %v", evs, err)
	}
	other := `{"type":"ChatPresence","event":{}}`
	if evs, err := c.ParseWebhook(signed(other), []byte(other)); err != nil || len(evs) != 0 {
		t.Fatalf("other: %v %v", evs, err)
	}
}

func TestParseWebhookLIDSender(t *testing.T) {
	c := New(Config{HMACKey: testKey})
	body := `{"type":"Message","event":{"Info":{"ID":"L1","Chat":"1234567890123@lid","Sender":"1234567890123@lid","SenderAlt":"905551234567@s.whatsapp.net"},"Message":{"imageMessage":{"caption":"foto","mimetype":"image/jpeg"}}}}`
	evs, err := c.ParseWebhook(signed(body), []byte(body))
	if err != nil || len(evs) != 1 || evs[0].From != "+905551234567" || evs[0].Media == nil || evs[0].Media.Type != "image" || evs[0].Text != "foto" {
		t.Fatalf("lid: %+v %v", evs, err)
	}
}

// TEC-397: a shared location becomes location media with its coordinates.
func TestParseWebhookLocation(t *testing.T) {
	c := New(Config{HMACKey: testKey})
	body := `{"type":"Message","event":{"Info":{"ID":"LOC1","Chat":"905551234567@s.whatsapp.net","Sender":"905551234567@s.whatsapp.net"},"Message":{"locationMessage":{"degreesLatitude":41.0082,"degreesLongitude":28.9784,"name":"Sultanahmet","address":"Fatih, İstanbul"}}}}`
	evs, err := c.ParseWebhook(signed(body), []byte(body))
	if err != nil || len(evs) != 1 {
		t.Fatalf("location: %+v %v", evs, err)
	}
	m := evs[0].Media
	if m == nil || m.Type != whatsapp.MediaLocation || m.Latitude == nil || *m.Latitude != 41.0082 ||
		m.Longitude == nil || *m.Longitude != 28.9784 || m.Caption != "Sultanahmet Fatih, İstanbul" || len(m.Download) != 0 {
		t.Fatalf("media = %+v", m)
	}
}
