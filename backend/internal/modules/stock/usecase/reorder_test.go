package usecase

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
)

func TestComposeBalance(t *testing.T) {
	b := ComposeBalance(5, 1250, Amount{Quantity: 2, Centimeters: 300}, Amount{Quantity: 3}, Amount{Quantity: 1})
	if b.AvailableQuantity != 3 || b.QuarantineQuantity != 1 || b.AvailableMeters != "9.50" || b.ReservedMeters != "3.00" || b.IncomingQuantity != 3 || b.OnHandMeters != "12.50" {
		t.Fatalf("balance = %+v", b)
	}
	if b := ComposeBalance(1, 0, Amount{Quantity: 3}, Amount{}, Amount{}); b.AvailableQuantity != 0 {
		t.Fatalf("available below zero = %+v", b)
	}
}

// boxes is a balance of n available closed boxes and incoming pieces.
func boxes(n, incoming int64) model.StockBalance {
	return model.StockBalance{AvailableQuantity: n, IncomingQuantity: incoming, Measures: model.StockMeasures{Boxes: n, AvailableBoxes: n}}
}

func TestEvaluateReorder(t *testing.T) {
	p := func(v int32) *int32 { return &v }
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name      string
		rule      ReorderRule
		avg       *float64
		level     string
		suggested int64
	}{
		{"ok", ReorderRule{MinQuantity: 2, ReorderPoint: p(3), Balance: boxes(5, 0)}, nil, ReorderLevelOK, 0},
		{"reorder to max", ReorderRule{MinQuantity: 2, ReorderPoint: p(3), MaxQuantity: p(10), Balance: boxes(3, 2)}, nil, ReorderLevelReorder, 5},
		{"below min", ReorderRule{MinQuantity: 2, Balance: boxes(1, 0)}, nil, ReorderLevelBelowMin, 1},
		{"days low with cover", ReorderRule{WarningDays: p(10), CoverDays: p(30), Balance: boxes(4, 0)}, f(1), ReorderLevelDaysLow, 26},
		{"days ok", ReorderRule{WarningDays: p(3), Balance: boxes(4, 0)}, f(1), ReorderLevelOK, 0},
	}
	for _, c := range cases {
		r := c.rule
		EvaluateReorder(&r, c.avg)
		if r.Level != c.level || r.SuggestedQuantity != c.suggested {
			t.Errorf("%s: level %s suggested %d", c.name, r.Level, r.SuggestedQuantity)
		}
	}
}

func TestReorderRuleInputValidate(t *testing.T) {
	p := func(v int32) *int32 { return &v }
	for name, in := range map[string]ReorderRuleInput{
		"empty":            {},
		"point below min":  {MinQuantity: 5, ReorderPoint: p(2)},
		"max below point":  {ReorderPoint: p(5), MaxQuantity: p(4)},
		"negative min":     {MinQuantity: -1},
		"warning days 0":   {WarningDays: p(0)},
		"cover days large": {MinQuantity: 1, CoverDays: p(5000)},
	} {
		if err := in.validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := (ReorderRuleInput{MinQuantity: 1, ReorderPoint: p(2), MaxQuantity: p(5), WarningDays: p(7)}).validate(); err != nil {
		t.Fatal(err)
	}
}

// TEC-1243 (K14): closed boxes, open rolls, cut pieces and remnants split
// the on-hand stock; available boxes and meters subtract reservations.
func TestComposeMeasures(t *testing.T) {
	m := ComposeMeasures(17, 3, Breakdown{
		Open:    Amount{Quantity: 2, Centimeters: 1840},
		Cut:     Amount{Quantity: 3, Centimeters: 900},
		Remnant: Amount{Quantity: 1, Centimeters: 120},
		// one piece reservation sits on a metered unit, 3 m are reserved
		ReservedMetered: 1, ReservedMetersCm: 300,
	})
	want := model.StockMeasures{
		Boxes: 11, AvailableBoxes: 9, OpenRolls: 2, OpenMeters: "18.40", CutPieces: 3, CutMeters: "9.00",
		Remnants: 1, RemnantMeters: "1.20", AvailableMeters: "24.40",
	}
	if m != want {
		t.Fatalf("measures = %+v, want %+v", m, want)
	}
	if m := ComposeMeasures(1, 5, Breakdown{Open: Amount{Quantity: 2}, ReservedMetersCm: 50}); m.Boxes != 0 || m.AvailableBoxes != 0 || m.AvailableMeters != "0.00" {
		t.Fatalf("below zero = %+v", m)
	}
}

// TEC-1243 (K14): the box and meter thresholds trigger apart; remnants do
// not count for meters (AvailableMeters excludes them).
func TestEvaluateReorderTwoThresholds(t *testing.T) {
	p := func(v int32) *int32 { return &v }
	s := func(v string) *string { return &v }
	bal := func(boxes int64, meters string) model.StockBalance {
		return model.StockBalance{
			AvailableQuantity: boxes + 4, // metered units count as pieces here
			Measures:          model.StockMeasures{Boxes: boxes, AvailableBoxes: boxes, AvailableMeters: meters, RemnantMeters: "50.00"},
		}
	}
	cases := []struct {
		name         string
		rule         ReorderRule
		level, meter string
	}{
		{"both ok", ReorderRule{MinQuantity: 2, MinMeters: s("20.00"), Balance: bal(5, "25.00")}, ReorderLevelOK, ReorderLevelOK},
		{"boxes only", ReorderRule{MinQuantity: 6, MinMeters: s("20.00"), Balance: bal(5, "25.00")}, ReorderLevelBelowMin, ReorderLevelOK},
		{"meters only", ReorderRule{MinQuantity: 2, MinMeters: s("30.00"), Balance: bal(5, "25.00")}, ReorderLevelOK, ReorderLevelBelowMin},
		{"both", ReorderRule{MinQuantity: 6, MinMeters: s("30.00"), Balance: bal(5, "25.00")}, ReorderLevelBelowMin, ReorderLevelBelowMin},
		{"meter rule only", ReorderRule{MinMeters: s("10.00"), Balance: bal(0, "9.99")}, ReorderLevelOK, ReorderLevelBelowMin},
		{"no meter rule", ReorderRule{MinQuantity: 1, ReorderPoint: p(1), Balance: bal(3, "0.00")}, ReorderLevelOK, ReorderLevelOK},
		// a roll_meter product has no boxes: its rolls meet min_quantity
		{"roll product", ReorderRule{MinQuantity: 3, Product: model.StockProduct{UnitType: "roll_meter"}, Balance: bal(0, "40.00")}, ReorderLevelOK, ReorderLevelOK},
	}
	for _, c := range cases {
		r := c.rule
		EvaluateReorder(&r, nil)
		if r.Level != c.level || r.MeterLevel != c.meter {
			t.Errorf("%s: level %s meter %s", c.name, r.Level, r.MeterLevel)
		}
		if r.Critical() != (c.level != ReorderLevelOK || c.meter != ReorderLevelOK) {
			t.Errorf("%s: critical %v", c.name, r.Critical())
		}
	}
}

// TEC-1243: a product sold by the meter counts its meters as boxes for the
// forecast days (5 boxes + 30 m / 15 m = 7 boxes, 1 box a day).
func TestEvaluateReorderBoxEquivalentDays(t *testing.T) {
	w := int32(10)
	avg := 1.0
	r := ReorderRule{WarningDays: &w, boxMeters: 1500, Balance: model.StockBalance{
		Measures: model.StockMeasures{AvailableBoxes: 5, AvailableMeters: "30.00"},
	}}
	EvaluateReorder(&r, &avg)
	if r.DaysLeft == nil || *r.DaysLeft != "7.0" || r.Level != ReorderLevelDaysLow {
		t.Fatalf("days = %v level %s", r.DaysLeft, r.Level)
	}
}

func TestReorderRuleInputMinMeters(t *testing.T) {
	s := func(v string) *string { return &v }
	for _, bad := range []string{"0", "-1", "abc", "100000.01"} {
		if err := (ReorderRuleInput{MinMeters: s(bad)}).validate(); err == nil {
			t.Errorf("min_meters %q accepted", bad)
		}
	}
	if err := (ReorderRuleInput{MinMeters: s("25.5")}).validate(); err != nil {
		t.Fatalf("meter only rule: %v", err)
	}
}
