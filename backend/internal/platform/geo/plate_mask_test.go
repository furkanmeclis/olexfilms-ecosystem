package geo

import "testing"

func TestMaskPlate(t *testing.T) {
	cases := []struct{ country, plate, want string }{
		{"TR", "34 ABC 123", "34 *** 23"},
		{"TR", "34 A 1212", "34 *** 12"},
		{"tr", "06-ab-12", "06 *** 12"},
		{"TR", "34ABC12345", "34 *** 45"},
		{"TR", "XX 999 AB", "XX *** AB"}, // not a TR format: generic rule
		{"DE", "B AB 1234", "BA *** 34"},
		{"NL", "AB-123-C", "AB *** 3C"},
		{"", "1234", "***"},
		{"TR", "  ", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := MaskPlate(tc.country, tc.plate); got != tc.want {
			t.Errorf("MaskPlate(%q, %q) = %q, want %q", tc.country, tc.plate, got, tc.want)
		}
	}
}

func TestMaskPlateHidesLength(t *testing.T) {
	a, b := MaskPlate("TR", "34 A 12"), MaskPlate("TR", "34 ABC 99912")
	if len(a) != len(b) {
		t.Fatalf("masked lengths differ: %q vs %q", a, b)
	}
}

func TestVINLast4(t *testing.T) {
	if got := VINLast4("wvwzzz1jz3w386752"); got != "6752" {
		t.Fatalf("VINLast4 = %q", got)
	}
	if got := VINLast4("abc"); got != "" {
		t.Fatalf("short VIN = %q", got)
	}
}
