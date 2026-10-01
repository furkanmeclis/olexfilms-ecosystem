package i18n

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

var catalogParamRE = regexp.MustCompile(`{{\s*([a-zA-Z0-9_]+)\s*}}`)

func catalogParams(s string) string {
	var out []string
	for _, m := range catalogParamRE.FindAllStringSubmatch(s, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}

// translatedLocales are the catalogs TEC-138 filled; each must match en key
// for key with the same {{params}}. A locale joins the list once translated.
var translatedLocales = []Locale{LocaleBG, LocaleEL, LocaleZhCN, LocaleAZ, LocaleAR}

func TestTranslatedCatalogParity(t *testing.T) {
	for _, l := range translatedLocales {
		cat := catalogs[l]
		if cat == nil {
			t.Fatalf("%s: catalog not registered", l)
		}
		for k, en := range enCatalog {
			v, ok := cat[k]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("%s: missing %s", l, k)
				continue
			}
			if catalogParams(v) != catalogParams(en) {
				t.Errorf("%s: %s params %q, en %q", l, k, catalogParams(v), catalogParams(en))
			}
		}
		for k := range cat {
			if _, ok := enCatalog[k]; !ok {
				t.Errorf("%s: extra key %s (not in en)", l, k)
			}
		}
	}
}

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
