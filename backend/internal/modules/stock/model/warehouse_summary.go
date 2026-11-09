package model

import "github.com/google/uuid"

// WarehouseStockSummary is one row of GET /v1/stock/warehouse-summary
// (TEC-1066): the in-stock totals of one warehouse of the active
// organization. A nil Warehouse is stock held without a location. Counts
// only; the warehouse home shows no prices.
type WarehouseStockSummary struct {
	Warehouse *WarehouseSummaryRef `json:"warehouse"`
	Products  int64                `json:"products"`
	Pieces    int64                `json:"pieces"`
	Rolls     int64                `json:"rolls"`
	Meters    string               `json:"meters"`
}

// WarehouseSummaryRef names the warehouse of a summary row.
type WarehouseSummaryRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	IsPrimary bool      `json:"is_primary"`
}
