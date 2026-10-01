package slug

import (
	"strings"
	"testing"
)

func TestFromName(t *testing.T) {
	cases := []struct{ name, in, want string }{
		// Turkish (TEC-142 gate findings).
		{"tr bayi", "Çankaya Bayi", "cankaya-bayi"},
		{"tr distributor", "Ankara Distribütör", "ankara-distributor"},
		{"tr all lower", "çğıöşü", "cgiosu"},
		{"tr all upper", "ÇĞIİÖŞÜ", "cgiiosu"},
		{"tr dotted capital I", "İSTANBUL İletişim", "istanbul-iletisim"},
		{"tr mixed", "Şişli Oto Kaplama Ltd. Şti.", "sisli-oto-kaplama-ltd-sti"},
		{"tr ğ", "Doğuş Ağaç", "dogus-agac"},
		// de / fr / es / it / az.
		{"de umlaut", "Müller Straße", "muller-strasse"},
		{"de ä", "Bär Ölwerk", "bar-olwerk"},
		{"fr", "Crème Brûlée à la Façade", "creme-brulee-a-la-facade"},
		{"fr oe", "Œuvre cœur", "oeuvre-coeur"},
		{"es", "Peña Niño Árbol", "pena-nino-arbol"},
		{"it", "Città Perché", "citta-perche"},
		{"az schwa", "Bakı Əhmədli", "baki-ehmedli"},
		{"nordic", "Øresund Æble", "oresund-aeble"},
		{"pl", "Łódź", "lodz"},
		// Cyrillic (ru / uk / bg).
		{"ru", "Москва", "moskva"},
		{"ru yo y", "Ёлка Йошкар", "yolka-yoshkar"},
		{"ru signs", "Объект Тюмень", "obekt-tyumen"},
		{"ru shch", "Щука Жук Цех Чай Шар", "shchuka-zhuk-tseh-chay-shar"},
		{"uk", "Київ Європа", "kiyiv-yevropa"},
		{"bg", "София Пловдив", "sofiya-plovdiv"},
		// Greek.
		{"el", "Αθήνα", "athina"},
		{"el tonos", "Θεσσαλονίκη", "thessaloniki"},
		{"el ps ch", "Ψυχή Χαρά", "psychi-chara"},
		{"el final sigma", "Ελλάς", "ellas"},
		// ASCII and edge cases (behaviour unchanged).
		{"ascii", "Olex Merkez", "olex-merkez"},
		{"punct", "  --A&B  Co.--  ", "a-b-co"},
		{"digits", "Bayi 34", "bayi-34"},
		{"empty", "   ", "item"},
		{"only symbols", "!!!", "item"},
		{"cjk dropped", "北京", "item"},
		{"arabic dropped", "دبي Dubai", "dubai"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromName(tc.in); got != tc.want {
				t.Fatalf("FromName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFromNameTruncates(t *testing.T) {
	got := FromName(strings.Repeat("ş", 100))
	if len(got) != 80 || strings.Trim(got, "s") != "" {
		t.Fatalf("got %q (%d)", got, len(got))
	}
}
