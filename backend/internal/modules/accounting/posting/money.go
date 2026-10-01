package posting

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// rateScale is the scale of finance_entries.rate (NUMERIC(18,8)).
const rateScale = 8

// parseAmount parses a positive decimal with at most two fraction digits.
func parseAmount(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, "eE/") {
		return nil, fmt.Errorf("%w: amount %q", ErrInvalid, s)
	}
	if i := strings.IndexByte(s, '.'); i >= 0 && len(s)-i-1 > 2 {
		return nil, fmt.Errorf("%w: amount %q has more than two decimals", ErrInvalid, s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return nil, fmt.Errorf("%w: amount %q must be a positive decimal", ErrInvalid, s)
	}
	return r, nil
}

// parseRate parses a published rate and rounds it to the stored scale; the
// converted amount is computed from the stored value so a row reproduces.
func parseRate(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok || r.Sign() <= 0 {
		return nil, fmt.Errorf("%w: rate %q", ErrInvalid, s)
	}
	r, _ = new(big.Rat).SetString(r.FloatString(rateScale))
	if r.Sign() <= 0 {
		return nil, fmt.Errorf("%w: rate %q rounds to zero", ErrInvalid, s)
	}
	return r, nil
}

// convert returns amount × rate rounded to cents (halves away from zero).
func convert(amount, rate *big.Rat) string {
	return new(big.Rat).Mul(amount, rate).FloatString(2)
}

func numeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return n, fmt.Errorf("%w: numeric %q: %v", ErrInvalid, s, err)
	}
	return n, nil
}

// ratOf converts a stored NUMERIC to a rational.
func ratOf(n pgtype.Numeric) *big.Rat {
	if !n.Valid || n.Int == nil {
		return new(big.Rat)
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp == 0 {
		return r
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(n.Exp))), nil)
	if n.Exp > 0 {
		return r.Mul(r, new(big.Rat).SetInt(scale))
	}
	return r.Quo(r, new(big.Rat).SetInt(scale))
}

// FormatNumeric renders a stored NUMERIC with two decimals ("-12.50").
func FormatNumeric(n pgtype.Numeric) string { return ratOf(n).FloatString(2) }

// FormatRate renders a stored rate with the stored scale.
func FormatRate(n pgtype.Numeric) string { return ratOf(n).FloatString(rateScale) }

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
