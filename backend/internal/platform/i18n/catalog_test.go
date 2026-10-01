package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// translatedLocales are the locales whose label catalog must match enCatalog
// key for key (TEC-138 batch A).
var translatedLocales = []Locale{LocaleDE, LocaleFR, LocaleES, LocaleIT, LocaleRU, LocaleUK}

// universalLabels may stay equal to en in every language.
var universalLabels = map[string]bool{
	"UUID": true, "PDF": true, "CSV": true, "JSON": true, "Excel": true, "IBAN": true,
}

// sameAsEN lists real cognates that are correct translations in a locale.
var sameAsEN = map[Locale]map[string]bool{
	LocaleFR: {"Active": true},
	LocaleES: {"No": true},
	LocaleIT: {"No": true},
}

var paramRE = regexp.MustCompile(`{{\s*([a-zA-Z0-9_]+)\s*}}`)

func params(s string) string {
	var out []string
	for _, m := range paramRE.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestTranslatedCatalogParity(t *testing.T) {
	for _, l := range translatedLocales {
		cat := catalogs[l]
		if len(cat) == 0 {
			t.Fatalf("%s catalog is empty", l)
		}
		for k, en := range enCatalog {
			v, ok := cat[k]
			switch {
			case !ok || strings.TrimSpace(v) == "":
				t.Errorf("%s catalog missing %q", l, k)
			case params(v) != params(en):
				t.Errorf("%s %q: params %q differ from en %q", l, k, params(v), params(en))
			case v == en && !universalLabels[en] && !sameAsEN[l][en]:
				t.Errorf("%s %q still equals en %q", l, k, en)
			}
		}
		for k := range cat {
			if _, ok := enCatalog[k]; !ok {
				t.Errorf("%s catalog has %q, en does not", l, k)
			}
		}
	}
}

func TestTranslatedLocalesResolve(t *testing.T) {
	want := map[Locale]string{
		LocaleDE: "Kunde", LocaleFR: "Client", LocaleES: "Cliente",
		LocaleIT: "Cliente", LocaleRU: "Клиент", LocaleUK: "Клієнт",
	}
	for l, v := range want {
		if got := Translate(l, "cari.customer_name"); got != v {
			t.Errorf("%s cari.customer_name = %q, want %q", l, got, v)
		}
	}
	if got := ResourceLabel(LocaleDE, "platform.users"); got != "Benutzer" {
		t.Errorf("de resource label = %q", got)
	}
}
