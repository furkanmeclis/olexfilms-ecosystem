package model

import "github.com/google/uuid"

// WarehouseStockSummary is one row of GET /v1/stock/warehouse-summary
// (TEC-1066): the in-stock totals of one warehouse of the active
// organization. A nil Warehouse is stock held without a location. Counts
// only; the warehouse home shows no prices.
type WarehouseStockSummary struct {
	Warehouse *WarehouseSummaryRef `json:"warehouse"`
	Products  int64                `json:"products"`
	// Pieces are the closed boxes (units without meters and fixed barcode
	// pieces); Rolls / Meters all metered units, split below (TEC-1243).
	Pieces        int64  `json:"pieces"`
	Rolls         int64  `json:"rolls"`
	Meters        string `json:"meters"`
	OpenRolls     int64  `json:"open_rolls"`
	OpenMeters    string `json:"open_meters"`
	CutPieces     int64  `json:"cut_pieces"`
	CutMeters     string `json:"cut_meters"`
	Remnants      int64  `json:"remnants"`
	RemnantMeters string `json:"remnant_meters"`
}

// WarehouseSummaryRef names the warehouse of a summary row.
type WarehouseSummaryRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	IsPrimary bool      `json:"is_primary"`
}
