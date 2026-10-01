package msgtemplate

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

func TestLocalizedSamplesCoverBatchA(t *testing.T) {
	locales := []i18n.Locale{i18n.LocaleDE, i18n.LocaleFR, i18n.LocaleES, i18n.LocaleIT, i18n.LocaleRU, i18n.LocaleUK}
	for key, byLocale := range localizedSamples {
		for _, l := range locales {
			if byLocale[l] == "" {
				t.Errorf("sample %q has no %s value", key, l)
			}
		}
	}
	p := Placeholder{Key: "contract_title", SampleTR: "Hizmet Sözleşmesi", SampleEN: "Service Agreement"}
	if got := p.Sample("fr"); got != "Contrat de prestation" {
		t.Fatalf("fr contract sample = %q", got)
	}
	if got := p.Sample("tr"); got != "Hizmet Sözleşmesi" {
		t.Fatalf("tr contract sample = %q", got)
	}
	if got := p.Sample("en"); got != "Service Agreement" {
		t.Fatalf("en contract sample = %q", got)
	}
}
