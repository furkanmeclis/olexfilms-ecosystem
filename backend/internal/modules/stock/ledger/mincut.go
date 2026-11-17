package ledger

// Minimum cut (TEC-1230): products.min_cut_meters is the shortest piece a
// roll of the product may be cut to. The rule is enforced where a roll is
// cut by hand (POST /v1/stock/splits) and where a service consumes part of
// a roll; taking the whole remainder of a roll is always allowed, so a
// short leftover (a remnant, "ragle") can still be used up.

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrBelowMinCut: the cut is shorter than the product's minimum cut and is
// not the whole remainder of the roll.
var ErrBelowMinCut = errors.New("ledger: cut below the minimum cut length")

// CheckMinCut refuses a cut of cutCm centimeters from a roll with
// remainingCm left when the product's minimum cut is longer, unless the
// cut takes the whole remainder. A null minimum cut accepts every cut.
func CheckMinCut(cutCm, remainingCm int64, minCut pgtype.Numeric) error {
	if !minCut.Valid {
		return nil
	}
	minCm, err := NumericToCentimeters(minCut)
	if err != nil || minCm <= 0 {
		return nil //nolint:nilerr // an unreadable minimum does not block a cut
	}
	if cutCm < minCm && cutCm != remainingCm {
		return ErrBelowMinCut
	}
	return nil
}

// IsRemnant reports whether a roll with remaining meters left is a remnant
// ("ragle"/artık): something is left but less than the product's minimum
// cut. A unit without meters or a product without a minimum is never one.
func IsRemnant(remaining, minCut pgtype.Numeric) bool {
	if !remaining.Valid || !minCut.Valid {
		return false
	}
	rem, err := NumericToCentimeters(remaining)
	if err != nil || rem <= 0 {
		return false
	}
	minCm, err := NumericToCentimeters(minCut)
	if err != nil {
		return false
	}
	return rem < minCm
}
