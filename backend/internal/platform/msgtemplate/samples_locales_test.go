package msgtemplate

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

func TestLocalizedSamplesCoverBatchB(t *testing.T) {
	locales := []i18n.Locale{i18n.LocaleBG, i18n.LocaleEL, i18n.LocaleZhCN, i18n.LocaleAZ, i18n.LocaleAR}
	for key, byLocale := range localizedSamples {
		for _, l := range locales {
			if byLocale[l] == "" {
				t.Errorf("sample %q has no %s value", key, l)
			}
		}
	}
	todo, _ := Lookup("todo.reminder")
	cases := map[string]string{"tr": "15 dakika sonra", "en": "in 15 minutes", "ar": "بعد 15 دقيقة", "zh-CN": "15分钟后"}
	for loc, want := range cases {
		if got := todo.SampleVars(loc)["due_in"]; got != want {
			t.Errorf("%s due_in = %q, want %q", loc, got, want)
		}
	}
	// Placeholders without a localized sample use the en one.
	if got := phPlate.Sample("bg"); got != "34 ABC 123" {
		t.Errorf("bg plate = %q", got)
	}
}
