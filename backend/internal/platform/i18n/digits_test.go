package i18n

import "testing"

func TestDigits(t *testing.T) {
	if got := Digits(LocaleAR, "26.10.2026 · 1.250,50 ₺"); got != "٢٦.١٠.٢٠٢٦ · ١.٢٥٠,٥٠ ₺" {
		t.Fatalf("ar = %q", got)
	}
	if got := Digits(LocaleTR, "26.10.2026"); got != "26.10.2026" {
		t.Fatalf("tr = %q", got)
	}
	if got := LatinDigits("٣٤ ABC ١٢ / ۱۲۳"); got != "34 ABC 12 / 123" {
		t.Fatalf("latin = %q", got)
	}
}

func TestLatinOnlyKey(t *testing.T) {
	for _, k := range []string{"plate", "vin", "barcode", "service_no", "phone", "public_code", "quote_link",
		"app_link", "job_id", "id", "quote_number", "device_serial", "url"} {
		if !LatinOnlyKey(k) {
			t.Errorf("%s should keep Latin digits", k)
		}
	}
	for _, k := range []string{"amount", "total_amount", "due_at", "days", "deviation_count", "valid_until", "due_in"} {
		if LatinOnlyKey(k) {
			t.Errorf("%s should be localized", k)
		}
	}
}
