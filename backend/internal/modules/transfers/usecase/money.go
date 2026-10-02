package usecase

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

func parseRat(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return nil, fmt.Errorf("transfers: invalid decimal %q", s)
	}
	return r, nil
}

func numeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("transfers: numeric %q: %w", s, err)
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

// numericTextPtr formats a NUMERIC with a fixed number of decimals (nil when NULL).
func numericTextPtr(n pgtype.Numeric, decimals int) *string {
	r := numericRat(n)
	if r == nil {
		return nil
	}
	s := r.FloatString(decimals)
	return &s
}
