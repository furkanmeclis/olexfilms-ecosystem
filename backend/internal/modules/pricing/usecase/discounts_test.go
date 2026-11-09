package usecase

import "testing"

func TestNormalizeDiscount(t *testing.T) {
	ok := map[string]string{"0": "0.00", "15": "15.00", "12,5": "12.50", " 20.25 ": "20.25", "100": "100.00"}
	for in, want := range ok {
		if got, valid := NormalizeDiscount(in); !valid || got != want {
			t.Errorf("NormalizeDiscount(%q) = %q, %v; want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"", "-1", "100.01", "101", "1.234", "abc", "1e2"} {
		if _, valid := NormalizeDiscount(in); valid {
			t.Errorf("NormalizeDiscount(%q) accepted", in)
		}
	}
}
