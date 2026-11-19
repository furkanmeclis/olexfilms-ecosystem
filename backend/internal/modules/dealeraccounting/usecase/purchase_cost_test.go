package usecase

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
)

// TEC-1214: an order bought in another currency (a USD brand order for a
// TRY dealer) is converted with the rate frozen at approval; without a
// frozen pair the cost is unknown instead of the raw foreign amount.
func TestFrozenCost(t *testing.T) {
	price := "12.50"
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

// TEC-1244: a cut piece / a roll costs meters × the meter cost, rounded
// to cents once after the rate; a closed box the box price.
func TestMeterCost(t *testing.T) {
	m := big.NewRat(7, 2) // 3.5 m
	if got := mulMeters("26.6667", m); got != "93.333450" {
		t.Fatalf("mulMeters = %s", got)
	}
	snap := []byte(`{"base":"USD","quote":"TRY","rate":"40.5","rate_date":"2026-11-16","source":"tcmb"}`)
	// 3.5 m × 26.6667 USD = 93.33345 USD × 40.5 = 3780.0047 -> 3780.00 TRY
	if got, ok := frozenCost(mulMeters("26.6667", m), "USD", "TRY", snap); !ok || got != "3780.00" {
		t.Fatalf("frozen meter cost = %s %v", got, ok)
	}
	if got, ok := frozenCost(mulMeters("26.6667", m), "TRY", "TRY", nil); !ok || got != "93.33" {
		t.Fatalf("same currency meter cost = %s %v", got, ok)
	}
	p := usecase.PurchasePrice{Price: "400.0000", MeterPrice: "30.0000"}
	if got, ok := listCost(p, nil); !ok || got != "400.0000" {
		t.Fatalf("closed box = %s", got)
	}
	if got, ok := listCost(p, m); !ok || got != "105.000000" {
		t.Fatalf("cut piece = %s", got)
	}
	if _, ok := listCost(usecase.PurchasePrice{Price: "400"}, m); ok {
		t.Fatal("a cut piece without a meter price has no cost")
	}
}

func TestExactNumeric(t *testing.T) {
	for in, want := range map[string]string{"26.6667": "26.6667", "400": "400", "12.50": "12.50"} {
		var n pgtype.Numeric
		if err := n.Scan(in); err != nil {
			t.Fatal(err)
		}
		if got := exactNumeric(n); got != want {
			t.Errorf("exactNumeric(%s) = %s, want %s", in, got, want)
		}
	}
	if exactNumeric(pgtype.Numeric{}) != "" {
		t.Fatal("NULL")
	}
}
