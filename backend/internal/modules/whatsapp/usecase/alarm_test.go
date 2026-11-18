package usecase_test

import (
	"strings"
	"testing"

	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// TEC-1235: the connection alarm is written in the recipient's language
// (tr when unset, en when unsupported) with an e-mail preview text.
func TestAlarmTextLocalized(t *testing.T) {
	ar := usecase.AlarmText("ar", whatsapp.StateBanned, "rate_limit")
	if ar.Lang != "ar" || !strings.Contains(ar.Body, "rate_limit") || ar.Preheader == "" ||
		!strings.ContainsFunc(ar.Title, func(r rune) bool { return r >= 0x0600 && r <= 0x06FF }) {
		t.Fatalf("ar alarm = %+v", ar)
	}
	if tr := usecase.AlarmText("", whatsapp.StateLoggedOut, "x"); tr.Lang != "tr" || !strings.Contains(tr.Title, "WhatsApp") {
		t.Fatalf("default alarm = %+v", tr)
	}
	if en := usecase.AlarmText("xx", whatsapp.StateLoggedOut, "x"); en.Lang != "en" || en.Title != "WhatsApp disconnected" {
		t.Fatalf("fallback alarm = %+v", en)
	}
	if de := usecase.AlarmText("de-DE", whatsapp.StateLoggedOut, "x"); de.Lang != "de" || strings.Contains(de.Body, "{{") {
		t.Fatalf("de alarm = %+v", de)
	}
	p := ar.Payload(whatsapp.StateBanned, "rate_limit")
	if p[notifmodel.PayloadEmailPreheader] != ar.Preheader || p["state"] != whatsapp.StateBanned {
		t.Fatalf("payload = %v", p)
	}
}
