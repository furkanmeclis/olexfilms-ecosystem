package tools

import (
	"encoding/json"
	"strings"
	"testing"

	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
)

// TEC-1243 (K14): the stock tool shows the meters and the box / open roll
// / cut piece / remnant split of a piece product with metered units, and
// nothing extra for a box-only product.
func TestStockRowMeasures(t *testing.T) {
	piece := stockmodel.ProductStock{
		Product: stockmodel.StockProduct{UnitType: "piece"}, Quantity: 18, Meters: "28.60",
		Balance: &stockmodel.StockBalance{Measures: stockmodel.StockMeasures{
			Boxes: 12, OpenRolls: 2, OpenMeters: "18.40", CutPieces: 3, CutMeters: "9.00", Remnants: 1, RemnantMeters: "1.20",
		}},
	}
	var row stockRow
	row.withMeasures(piece)
	raw, _ := json.Marshal(row)
	for _, want := range []string{`"meters":"28.60"`, `"closed_boxes":12`, `"open_rolls":2`, `"open_meters":"18.40"`, `"cut_pieces":3`, `"remnant_meters":"1.20"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("row %s misses %s", raw, want)
		}
	}
	box := stockmodel.ProductStock{
		Product: stockmodel.StockProduct{UnitType: "piece"}, Quantity: 4, Meters: "0.00",
		Balance: &stockmodel.StockBalance{Measures: stockmodel.StockMeasures{Boxes: 4}},
	}
	row = stockRow{}
	row.withMeasures(box)
	if raw, _ := json.Marshal(row); strings.Contains(string(raw), "meters") || strings.Contains(string(raw), "boxes") {
		t.Fatalf("box only row = %s", raw)
	}
}
