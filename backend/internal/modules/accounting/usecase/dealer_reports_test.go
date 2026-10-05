package usecase

import (
	"math/big"
	"testing"
	"time"
)

func TestAgingBucketBoundaries(t *testing.T) {
	for days, want := range map[int]int{0: 0, 30: 0, 31: 1, 45: 1, 60: 1, 61: 2, 90: 2, 91: 3, 400: 3} {
		if got := agingBucket(days); got != want {
			t.Errorf("agingBucket(%d) = %d, want %d", days, got, want)
		}
	}
}

func TestAgeBalanceFIFO(t *testing.T) {
	asOf := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	r := func(s string) *big.Rat { v, _ := new(big.Rat).SetString(s); return v }
	// Newest first: a collection, a fresh charge, a 45-day and a 100-day
	// receivable. The open 900 is explained by the newest debits.
	entries := []agingEntry{
		{at: asOf.Add(-2 * time.Hour), signed: r("-700")},
		{at: asOf.AddDate(0, 0, -3), signed: r("500")},
		{at: asOf.AddDate(0, 0, -45).Add(20 * time.Hour), signed: r("400")},
		{at: asOf.AddDate(0, 0, -100), signed: r("700")},
	}
	got := ageBalance(r("900"), entries, asOf).buckets()
	if got != (AgingBuckets{Days0To30: "500.00", Days31To60: "400.00", Days61To90: "0.00", Days90Plus: "0.00"}) {
		t.Fatalf("aging = %+v", got)
	}
	// More open than the rows explain: the rest is the oldest bucket.
	got = ageBalance(r("2000"), entries, asOf).buckets()
	if got.Days90Plus != "1100.00" || got.Days0To30 != "500.00" || got.Days31To60 != "400.00" {
		t.Fatalf("aging overflow = %+v", got)
	}
}

func TestMarginFigures(t *testing.T) {
	f := marginFigures(big.NewRat(3000, 1), big.NewRat(1800, 1), true)
	if f.Revenue != "3000.00" || *f.Cost != "1800.00" || *f.GrossProfit != "1200.00" || *f.MarginPct != "40.00" {
		t.Fatalf("figures = %+v", f)
	}
	if f := marginFigures(new(big.Rat), new(big.Rat), true); f.MarginPct != nil || *f.GrossProfit != "0.00" {
		t.Fatalf("zero revenue = %+v", f)
	}
	if f := marginFigures(big.NewRat(10, 1), big.NewRat(4, 1), false); f.Cost != nil || f.GrossProfit != nil || f.MarginPct != nil {
		t.Fatalf("hidden cost = %+v", f)
	}
}
