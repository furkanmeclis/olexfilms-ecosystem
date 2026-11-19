package usecase

import (
	"math/big"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// TEC-1250 (K7): the shipping note writes the free extra meters of a meter
// line whose whole roll went with it; nothing when the pieces fit.
func TestExcessNote(t *testing.T) {
	line, _ := numeric("3")
	cases := []struct {
		loc    i18n.Locale
		pieces *big.Rat
		want   string
	}{
		{i18n.LocaleTR, big.NewRat(7, 2), "Fazla 0,5 m ücretsiz"},
		{i18n.LocaleEN, big.NewRat(7, 2), "Extra 0.5 m free of charge"},
		{i18n.LocaleTR, big.NewRat(3, 1), ""},
		{i18n.LocaleTR, big.NewRat(2, 1), ""},
		{i18n.LocaleTR, nil, ""},
	}
	for i, c := range cases {
		if got := excessNote(c.loc, line, c.pieces); got != c.want {
			t.Errorf("case %d: %q, want %q", i, got, c.want)
		}
	}
	if got := joinNotes("Kullanım kredisi: parça başına 5", "", "Fazla 0,5 m ücretsiz"); got != "Kullanım kredisi: parça başına 5 · Fazla 0,5 m ücretsiz" {
		t.Errorf("joinNotes = %q", got)
	}
}

// TEC-1250 (K7): the line's excess is what its pieces carry over it.
func TestLineExcess(t *testing.T) {
	m, _ := numeric("3")
	it := db.OrderItem{Meters: m}
	if got := lineExcess(it, "3.50"); got == nil || *got != "0.50" {
		t.Fatalf("3.50 on 3 = %v", got)
	}
	if got := lineExcess(it, "3.00"); got != nil {
		t.Fatalf("3.00 on 3 = %v", *got)
	}
	if got := lineExcess(db.OrderItem{}, "1"); got != nil {
		t.Fatalf("box line = %v", *got)
	}
}
