package usecase

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1214: an order bought in another currency (a USD brand order for a
// TRY dealer) is converted with the rate frozen at approval; without a
// frozen pair the cost is unknown instead of the raw foreign amount.
func TestFrozenCost(t *testing.T) {
	var price pgtype.Numeric
	if err := price.Scan("12.50"); err != nil {
		t.Fatal(err)
	}
	snap := []byte(`{"base":"USD","quote":"TRY","rate":"40.5","rate_date":"2026-11-16","source":"tcmb",
		"pairs":[{"base":"USD","quote":"EUR","rate":"0.9","rate_date":"2026-11-16","source":"ecb"}]}`)
	cases := []struct {
		from, to string
		raw      []byte
		want     string
		ok       bool
	}{
		{"TRY", "TRY", nil, "12.50", true},
		{"USD", "TRY", snap, "506.25", true},
		{"USD", "EUR", snap, "11.25", true},
		{"USD", "GBP", snap, "", false},
		{"USD", "TRY", nil, "", false},
		{"USD", "TRY", []byte(`not json`), "", false},
	}
	for _, c := range cases {
		got, ok := frozenCost(price, c.from, c.to, c.raw)
		if got != c.want || ok != c.ok {
			t.Fatalf("frozenCost(%s->%s) = %q, %v; want %q, %v", c.from, c.to, got, ok, c.want, c.ok)
		}
	}
	if _, ok := convertAt("10", "0"); ok {
		t.Fatal("convertAt with zero rate must fail")
	}
}
