// Package model holds the recommended price constants (TEC-505, F5-09a).
package model

// Version sources (recommended_price_versions.source).
const (
	// SourcePublish: published by the center (single or bulk, batch_id).
	SourcePublish = "publish"
	// SourcePriceList: a direct write of product_prices.recommended_sale_price
	// through the price list, recorded by the sync trigger.
	SourcePriceList = "price_list"
	// SourceMigration: carried over from product_prices by migration 000124.
	SourceMigration = "migration"
)

// Sources lists every version source.
var Sources = []string{SourcePublish, SourcePriceList, SourceMigration}

// CountryAll is the list filter value of the currency-wide (country_id
// NULL) price.
const CountryAll int64 = 0
