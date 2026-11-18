package usecase

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// TEC-1233: the connection alarm exists in every supported locale and the
// recipient's locale picks it.
func TestAlarmTextsEveryLocale(t *testing.T) {
	for _, l := range i18n.Supported {
		texts, ok := alarmTexts[string(l)]
		if !ok {
			t.Fatalf("%s: no alarm texts", l)
		}
		for _, state := range []string{whatsapp.StateLoggedOut, whatsapp.StateBanned} {
			pair := texts[state]
			if strings.TrimSpace(pair[0]) == "" || strings.Count(pair[1], "%s") != 1 {
				t.Errorf("%s/%s: bad text %q", l, state, pair)
			}
			if l != i18n.LocaleEN && pair == alarmTexts["en"][state] {
				t.Errorf("%s/%s: same as en", l, state)
			}
		}
		if got := alarmLocale(string(l)); got != string(l) {
			t.Errorf("alarmLocale(%s) = %s", l, got)
		}
	}
	if alarmLocale("") != "tr" || alarmLocale("xx") != "en" || alarmLocale("de-DE") != "de" {
		t.Error("alarmLocale fallbacks")
	}
}
