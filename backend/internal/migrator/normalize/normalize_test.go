package normalize

import "testing"

func TestNormalizePhone(t *testing.T) {
	const want = "+905551234567"
	cases := []struct {
		name     string
		in       string
		region   string
		e164     string
		verified bool
	}{
		{"national with trunk 0", "05551234567", "", want, true},
		{"national 0 with spaces", "0555 123 45 67", "", want, true},
		{"national 0 with parens", "0(555) 123-45-67", "", want, true},
		{"ten digits", "5551234567", "", want, true},
		{"plus country code", "+905551234567", "", want, true},
		{"plus country code spaced", "+90 555 123 45 67", "", want, true},
		{"country code no plus", "905551234567", "", want, true},
		{"country code no plus spaced", "90 555 123 45 67", "", want, true},
		{"double zero prefix", "00905551234567", "", want, true},
		{"surrounding whitespace", "  0555 123 4567\t", "", want, true},
		{"foreign international", "+49 30 901820", "", "+4930901820", true},
		{"foreign default region", "030 901820", "DE", "+4930901820", true},
		{"empty", "", "", "", false},
		{"blank", "   ", "", "", false},
		{"letters", "telefon yok", "", "", false},
		{"too short", "12345", "", "", false},
		{"too long", "055512345678901", "", "", false},
		{"dashes only", "---", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePhone(tc.in, tc.region)
			if got.Verified != tc.verified || got.E164 != tc.e164 {
				t.Fatalf("NormalizePhone(%q) = %+v, want e164=%q verified=%v", tc.in, got, tc.e164, tc.verified)
			}
			if !got.Verified && got.Country != "" {
				t.Errorf("unverified phone has country %q", got.Country)
			}
		})
	}
	if p := NormalizePhone("0555 123 45 67", ""); p.Country != "TR" || p.National != "5551234567" {
		t.Errorf("derived fields = %+v", p)
	}
	if p := NormalizePhone(" telefon yok ", ""); p.Raw != "telefon yok" {
		t.Errorf("raw = %q", p.Raw)
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"  Ali.Veli@Example.TEST ": "ali.veli@example.test",
		"a@b.test":                 "a@b.test",
		"":                         "",
		"   ":                      "",
	}
	for in, want := range cases {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
