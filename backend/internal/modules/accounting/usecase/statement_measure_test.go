package usecase

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// TEC-1244: the statement export shows a box line in pieces and a meter
// line in meters at its per-meter price, side by side for one product.
func TestStatementOrderItemTextMeasures(t *testing.T) {
	one, meters := int32(1), "3.50"
	box := StatementOrderItem{ProductSKU: "BPPF", ProductName: "Black PPF", SaleMeasure: "box", Quantity: &one,
		UnitPrice: "400.0000", LineTotal: "400.00"}
	meter := StatementOrderItem{ProductSKU: "BPPF", ProductName: "Black PPF", SaleMeasure: "meter", Meters: &meters,
		UnitPrice: "26.6667", LineTotal: "93.33"}
	if got := StatementOrderItemText(box, "TRY", i18n.Locale("tr")); got != "BPPF · Black PPF — 1 adet × 400.0000 = 400.00 TRY" {
		t.Fatalf("box = %q", got)
	}
	if got := StatementOrderItemText(meter, "TRY", i18n.Locale("tr")); got != "BPPF · Black PPF — 3.50 m × 26.6667 = 93.33 TRY" {
		t.Fatalf("meter = %q", got)
	}
}
