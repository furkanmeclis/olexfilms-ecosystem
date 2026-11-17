package ledger

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func cmNum(cm int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(cm), Exp: -2, Valid: true}
}

func TestCheckMinCut(t *testing.T) {
	cases := []struct {
		name           string
		cut, remaining int64
		min            pgtype.Numeric
		wantErr        bool
	}{
		{"no minimum", 10, 1000, pgtype.Numeric{}, false},
		{"above minimum", 150, 1000, cmNum(100), false},
		{"equal to minimum", 100, 1000, cmNum(100), false},
		{"below minimum", 50, 1000, cmNum(100), true},
		{"whole remnant", 50, 50, cmNum(100), false},
	}
	for _, tc := range cases {
		err := CheckMinCut(tc.cut, tc.remaining, tc.min)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
}

func TestIsRemnant(t *testing.T) {
	if !IsRemnant(cmNum(40), cmNum(100)) {
		t.Error("40 cm left of a 1 m minimum is a remnant")
	}
	for _, tc := range [][2]pgtype.Numeric{
		{cmNum(100), cmNum(100)}, {cmNum(0), cmNum(100)}, {cmNum(40), {}}, {{}, cmNum(100)},
	} {
		if IsRemnant(tc[0], tc[1]) {
			t.Errorf("IsRemnant(%v, %v) = true", tc[0], tc[1])
		}
	}
}
