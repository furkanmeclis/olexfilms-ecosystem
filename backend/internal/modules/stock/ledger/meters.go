package ledger

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Meters are NUMERIC(10,2) in the database and int64 centimeters (hundredths
// of a meter) in Go, so the ledger arithmetic is exact.

// ParseMeters parses "15", "5.5" or "4.25" into centimeters.
func ParseMeters(s string) (int64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(frac) > 2 || strings.ContainsAny(whole+frac, "+-") {
		return 0, fmt.Errorf("%w: meters %q", ErrInvalidMovement, s)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	v, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: meters %q", ErrInvalidMovement, s)
	}
	if neg {
		v = -v
	}
	return v, nil
}

// FormatMeters renders centimeters as a decimal meter string ("6.00").
func FormatMeters(cm int64) string {
	sign := ""
	if cm < 0 {
		sign, cm = "-", -cm
	}
	return fmt.Sprintf("%s%d.%02d", sign, cm/100, cm%100)
}

// numericToCm converts a NUMERIC to centimeters; NULL is 0. A value with
// more than two decimals is rejected (the column scale is 2).
func numericToCm(n pgtype.Numeric) (int64, error) {
	if !n.Valid {
		return 0, nil
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return 0, fmt.Errorf("ledger: meters not finite")
	}
	v := new(big.Int).Set(n.Int)
	exp := int64(n.Exp) + 2
	ten := big.NewInt(10)
	if exp >= 0 {
		v.Mul(v, new(big.Int).Exp(ten, big.NewInt(exp), nil))
	} else {
		var rem big.Int
		v.QuoRem(v, new(big.Int).Exp(ten, big.NewInt(-exp), nil), &rem)
		if rem.Sign() != 0 {
			return 0, fmt.Errorf("ledger: meters %s has more than two decimals", n.Int.String())
		}
	}
	if !v.IsInt64() {
		return 0, fmt.Errorf("ledger: meters out of range")
	}
	return v.Int64(), nil
}

// cmToNumeric converts centimeters to a NUMERIC(…,2).
func cmToNumeric(cm int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(cm), Exp: -2, Valid: true}
}
