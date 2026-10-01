package usecase

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Meters of a roll line: NUMERIC(10,2), positive.
var metersRe = regexp.MustCompile(`^[0-9]{1,8}(\.[0-9]{1,2})?$`)

// MaxQuantity bounds a piece line (INT column, sane upper limit).
const MaxQuantity = 1_000_000

func parseRat(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return nil, fmt.Errorf("orders: invalid decimal %q", s)
	}
	return r, nil
}

// normalizeMeters validates a meter amount and returns it with two decimals.
func normalizeMeters(raw string) (string, bool) {
	m := strings.TrimSpace(raw)
	if !metersRe.MatchString(m) {
		return "", false
	}
	r, err := parseRat(m)
	if err != nil || r.Sign() <= 0 {
		return "", false
	}
	return r.FloatString(2), true
}

// lineTotal is unit price × amount rounded to cents (half away from zero,
// like Postgres NUMERIC rounding).
func lineTotal(unitPrice string, amount *big.Rat) (string, error) {
	p, err := parseRat(unitPrice)
	if err != nil {
		return "", err
	}
	return new(big.Rat).Mul(p, amount).FloatString(2), nil
}

// sumTotals adds line totals (two decimals each).
func sumTotals(lines []string) (string, error) {
	sum := new(big.Rat)
	for _, l := range lines {
		r, err := parseRat(l)
		if err != nil {
			return "", err
		}
		sum.Add(sum, r)
	}
	return sum.FloatString(2), nil
}

func numeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("orders: numeric %q: %w", s, err)
	}
	return n, nil
}

func numericRat(n pgtype.Numeric) *big.Rat {
	if !n.Valid || n.Int == nil {
		return nil
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp != 0 {
		e := n.Exp
		if e < 0 {
			e = -e
		}
		p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(e)), nil))
		if n.Exp > 0 {
			r.Mul(r, p)
		} else {
			r.Quo(r, p)
		}
	}
	return r
}

// numericText formats a NUMERIC with a fixed number of decimals ("" when NULL).
func numericText(n pgtype.Numeric, decimals int) string {
	r := numericRat(n)
	if r == nil {
		return ""
	}
	return r.FloatString(decimals)
}

func numericTextPtr(n pgtype.Numeric, decimals int) *string {
	if !n.Valid {
		return nil
	}
	s := numericText(n, decimals)
	return &s
}

// rateText formats a rate without trailing zeros.
func rateText(n pgtype.Numeric) *string {
	r := numericRat(n)
	if r == nil {
		return nil
	}
	s := r.FloatString(10)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return &s
}
