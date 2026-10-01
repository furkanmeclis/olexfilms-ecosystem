package i18n

import (
	"strings"
	"testing"
)

// Every non-empty catalog, translated or not, only uses en keys.
func TestCatalogsHaveNoExtraKeys(t *testing.T) {
	for l, cat := range catalogs {
		for k := range cat {
			if _, ok := enCatalog[k]; !ok {
				t.Errorf("%s: extra key %s", l, k)
			}
		}
	}
}

// Linear TEC-138 acceptance: Arabic CSV/XLSX headers are translated.
func TestArabicExportHeadersTranslated(t *testing.T) {
	for _, k := range []string{"users.email", "jobs.plate", "cari.balance", "export.title.tenant.jobs"} {
		got := Translate(LocaleAR, k)
		if got == enCatalog[k] {
			t.Errorf("ar %s still en: %q", k, got)
		}
		if !strings.ContainsFunc(got, func(r rune) bool { return r >= 0x0600 && r <= 0x06FF }) {
			t.Errorf("ar %s has no Arabic script: %q", k, got)
		}
	}
}

func TestBatchBLocalesResolve(t *testing.T) {
	want := map[Locale]string{
		LocaleBG: "Клиент", LocaleEL: "Πελάτης", LocaleZhCN: "客户", LocaleAZ: "Müştəri", LocaleAR: "العميل",
	}
	for l, v := range want {
		if got := Translate(l, "cari.customer_name"); got != v {
			t.Errorf("%s cari.customer_name = %q, want %q", l, got, v)
		}
	}
}
