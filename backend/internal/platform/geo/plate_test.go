package geo

import (
	"errors"
	"testing"
)

// Same expressions as the migration 000035 seed (TR, DE, NL).
var seedRegex = map[string]string{
	"TR": `^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$`,
	"DE": `^[A-ZÄÖÜ]{1,3}[A-Z]{1,2}[1-9][0-9]{0,3}[EH]?$`,
	"NL": `^([A-Z]{2}[0-9]{4}|[0-9]{4}[A-Z]{2}|[0-9]{2}[A-Z]{2}[0-9]{2}|[A-Z]{2}[0-9]{2}[A-Z]{2}|[A-Z]{4}[0-9]{2}|[0-9]{2}[A-Z]{4}|[0-9]{2}[A-Z]{3}[0-9]|[0-9][A-Z]{3}[0-9]{2}|[A-Z]{2}[0-9]{3}[A-Z]|[A-Z][0-9]{3}[A-Z]{2}|[A-Z]{3}[0-9]{2}[A-Z]|[A-Z][0-9]{2}[A-Z]{3}|[0-9][A-Z]{2}[0-9]{3}|[0-9]{3}[A-Z]{2}[0-9])$`,
}

func TestSeedPlateRegexes(t *testing.T) {
	cases := []struct {
		country, plate string
		valid          bool
	}{
		{"TR", "34 ABC 123", true},
		{"TR", "06 a 1234", true},
		{"TR", "81-AB-12", true},
		{"TR", "00 ABC 123", false},
		{"TR", "82 ABC 123", false},
		{"TR", "34 ABCD 12", false},
		{"DE", "B AB 1234", true},
		{"DE", "M-XY 99", true},
		{"DE", "MÜ AB 12", true},
		{"DE", "HH AB 123E", true},
		{"DE", "B AB 0123", false},
		{"DE", "1234 AB", false},
		{"DE", "BERL AB 1", false},
		{"NL", "AB-123-C", true},
		{"NL", "12-ABC-3", true},
		{"NL", "XX-99-99", true},
		{"NL", "ABC-123", false},
		{"NL", "1234567", false},
	}
	for _, tc := range cases {
		n, ok, err := MatchPlate(seedRegex[tc.country], tc.plate)
		if err != nil {
			t.Fatalf("%s %q: %v", tc.country, tc.plate, err)
		}
		if ok != tc.valid {
			t.Errorf("%s %q (normalized %q): valid=%v, want %v", tc.country, tc.plate, n, ok, tc.valid)
		}
	}
}

func TestNormalizePlate(t *testing.T) {
	if got := NormalizePlate(" b-ab 12.34 "); got != "BAB1234" {
		t.Fatalf("NormalizePlate = %q", got)
	}
}

func TestCompilePlateRegexRejects(t *testing.T) {
	for _, expr := range []string{"", "[A-Z]+", "^(?=A)[A-Z]$", "^[A-Z$"} {
		if _, err := CompilePlateRegex(expr); !errors.Is(err, ErrInvalid) {
			t.Errorf("CompilePlateRegex(%q) = %v, want ErrInvalid", expr, err)
		}
	}
}

func TestPlateFormatInputValidate(t *testing.T) {
	re, label, ex := `^[A-Z]{2}[0-9]{2}$`, "X", "AB 12"
	if err := (PlateFormatInput{Regex: &re, CountryLabel: &label, Example: &ex}).validate(true); err != nil {
		t.Fatalf("valid input: %v", err)
	}
	bad := "AB 1"
	if err := (PlateFormatInput{Regex: &re, CountryLabel: &label, Example: &bad}).validate(true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("example mismatch: %v", err)
	}
	color := "red"
	if err := (PlateFormatInput{StripColor: &color}).validate(false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad color: %v", err)
	}
	if err := (PlateFormatInput{Regex: &re}).validate(true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing label: %v", err)
	}
}
