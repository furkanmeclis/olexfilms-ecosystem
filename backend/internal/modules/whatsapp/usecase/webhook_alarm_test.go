package usecase

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// TEC-1233 / TEC-1235: the connection alarm exists in every supported locale
// (i18n catalog email.whatsapp_alarm.*) and the recipient's locale picks it.
func TestAlarmTextsEveryLocale(t *testing.T) {
	const reason = "REASON-X"
	for _, l := range i18n.Supported {
		for _, state := range []string{whatsapp.StateLoggedOut, whatsapp.StateBanned} {
			a := AlarmText(string(l), state, reason)
			if a.Lang != string(l) {
				t.Errorf("%s/%s: lang = %s", l, state, a.Lang)
			}
			if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Preheader) == "" ||
				!strings.Contains(a.Body, reason) || strings.Contains(a.Title, "email.") {
				t.Errorf("%s/%s: bad text %+v", l, state, a)
			}
			if l != i18n.LocaleEN {
				en := AlarmText("en", state, reason)
				if a.Title == en.Title || a.Body == en.Body {
					t.Errorf("%s/%s: same as en", l, state)
				}
			}
		}
	}
	if AlarmText("", whatsapp.StateBanned, "r").Lang != "tr" ||
		AlarmText("xx", whatsapp.StateBanned, "r").Lang != "en" ||
		AlarmText("de-DE", whatsapp.StateBanned, "r").Lang != "de" {
		t.Error("alarm locale fallbacks")
	}
}
