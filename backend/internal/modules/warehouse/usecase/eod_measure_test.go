package usecase

import (
	"testing"

	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
)

// TEC-1243: the end-of-day PDF names the K14 split of a product with
// metered stock and nothing for a box-only product.
func TestEODMeasureLine(t *testing.T) {
	tr := func(k string) string {
		return map[string]string{
			"stock.measure.boxes": "Kapalı kutu", "stock.measure.open": "Açık", "stock.measure.cut": "Kesik",
			"stock.measure.remnant": "Artık", "stock.measure.rolls": "rulo", "stock.measure.pieces": "parça",
		}[k]
	}
	twelve := int64(12)
	x := EODBalance{ClosingBoxes: &twelve, Measures: &stockmodel.StockMeasures{
		OpenRolls: 2, OpenMeters: "18.40", CutPieces: 3, CutMeters: "9.00", Remnants: 1, RemnantMeters: "1.20",
	}}
	want := "Kapalı kutu 12 · Açık 2 rulo / 18.40 m · Kesik 3 parça / 9.00 m · Artık 1.20 m"
	if got := eodMeasureLine(x, tr); got != want {
		t.Fatalf("line = %q", got)
	}
	if got := eodMeasureLine(EODBalance{ClosingBoxes: &twelve}, tr); got != "" {
		t.Fatalf("box only = %q", got)
	}
}
