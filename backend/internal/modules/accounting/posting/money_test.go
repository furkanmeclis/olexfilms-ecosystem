package posting

import (
	"errors"
	"testing"
)

func TestParseAmount(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"1", "1.5", "1234.56", " 0.01 "} {
		if _, err := parseAmount(ok); err != nil {
			t.Fatalf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "0", "-1", "1.234", "1e3", "1/2", "abc"} {
		if _, err := parseAmount(bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%q: want ErrInvalid, got %v", bad, err)
		}
	}
}

func TestConvertRoundsHalfAwayFromZero(t *testing.T) {
	t.Parallel()
	cases := []struct{ amount, rate, want string }{
		{"1234.56", "45.1234", "55707.54"},
		{"1000", "45.12345678", "45123.46"},
		{"0.01", "0.5", "0.01"},        // 0.005 → 0.01
		{"10", "1.123456789", "11.23"}, // rate rounded to 8 decimals first
		{"100", "0.000000004", ""},     // rounds to zero → invalid
	}
	for _, c := range cases {
		a, err := parseAmount(c.amount)
		if err != nil {
			t.Fatal(err)
		}
		r, err := parseRate(c.rate)
		if c.want == "" {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("rate %s: want ErrInvalid, got %v", c.rate, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := convert(a, r); got != c.want {
			t.Fatalf("%s × %s = %s, want %s", c.amount, c.rate, got, c.want)
		}
	}
}

func TestFormatNumericRoundTrip(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"55707.54", "-12.50", "1.00"} {
		n, err := numeric(s)
		if err != nil {
			t.Fatal(err)
		}
		if got := FormatNumeric(n); got != s {
			t.Fatalf("%s → %s", s, got)
		}
	}
	n, _ := numeric("45.12340000")
	if got := FormatRate(n); got != "45.12340000" {
		t.Fatalf("rate → %s", got)
	}
}
