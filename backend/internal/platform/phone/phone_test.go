package phone

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in, region, want, country string
	}{
		{"0555 123 45 67", "", "+905551234567", "TR"},
		{"5551234567", "TR", "+905551234567", "TR"},
		{"+90 (555) 123-4567", "DE", "+905551234567", "TR"},
		{"00905551234567", "", "+905551234567", "TR"},
		{"030 1234567", "de", "+49301234567", "DE"},
		{"+1 202-555-0187", "", "+12025550187", "US"},
	}
	for _, c := range cases {
		n, err := Parse(c.in, c.region)
		if err != nil {
			t.Fatalf("Parse(%q, %q): %v", c.in, c.region, err)
		}
		if n.E164 != c.want || n.Country != c.country {
			t.Fatalf("Parse(%q) = %+v, want %s %s", c.in, n, c.want, c.country)
		}
	}
	for _, bad := range []string{"", "abc", "123", "+90 555"} {
		if _, err := Parse(bad, ""); err == nil {
			t.Fatalf("Parse(%q) must fail", bad)
		}
	}
}

func TestDigitsAndFromDigits(t *testing.T) {
	n, err := Parse("05551234567", "")
	if err != nil {
		t.Fatal(err)
	}
	if n.Digits() != "905551234567" || n.National != "5551234567" {
		t.Fatalf("digits = %s national = %s", n.Digits(), n.National)
	}
	for _, id := range []string{"905551234567", "905551234567@s.whatsapp.net", "905551234567:12@s.whatsapp.net", "905551234567.0:3@s.whatsapp.net"} {
		got, err := FromDigits(id)
		if err != nil || got.E164 != "+905551234567" {
			t.Fatalf("FromDigits(%q) = %+v %v", id, got, err)
		}
	}
	if _, err := FromDigits("12345@lid"); err == nil {
		t.Fatal("short id must fail")
	}
	if Region("", " de ") != "DE" || Region() != "TR" {
		t.Fatal("Region fallback")
	}
	if Mask("+905551234567") != "*********4567" {
		t.Fatalf("mask = %s", Mask("+905551234567"))
	}
}
