package rbac

import "strings"

// PriceViewPermissions are the grants that let a caller see prices and money
// at all (TEC-1050, design §4: price visibility follows permissions, not
// roles): unit prices, line totals, order and transfer totals, tax, exchange
// rates and their snapshots. Holding any one of them at any scope is enough;
// the finer purchase / sale rules stay with each module. Roles without them
// (center and distributor warehouse, dealer staff, the center service
// point) receive those fields as null and never get a price column in an
// export.
var PriceViewPermissions = []string{
	PermPricingPurchaseRead,
	PermPricingSaleRead,
	PermPricingSaleWrite,
	PermPricingRecommendedRead,
	PermPricingRecommendedWrite,
}

// PriceViewColumnPermission is the any-of permission of an export column
// that carries money (ioengine.Column.Permission, "|" separated).
var PriceViewColumnPermission = strings.Join(PriceViewPermissions, "|")

// SeesPrices reports whether any of the granted slugs unlocks prices.
func SeesPrices(has func(slug string) bool) bool {
	for _, slug := range PriceViewPermissions {
		if has(slug) {
			return true
		}
	}
	return false
}
