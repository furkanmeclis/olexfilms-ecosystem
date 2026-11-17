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
		{"ok", ReorderRule{MinQuantity: 2, ReorderPoint: p(3), Balance: model.StockBalance{AvailableQuantity: 5}}, nil, ReorderLevelOK, 0},
		{"reorder to max", ReorderRule{MinQuantity: 2, ReorderPoint: p(3), MaxQuantity: p(10), Balance: model.StockBalance{AvailableQuantity: 3, IncomingQuantity: 2}}, nil, ReorderLevelReorder, 5},
		{"below min", ReorderRule{MinQuantity: 2, Balance: model.StockBalance{AvailableQuantity: 1}}, nil, ReorderLevelBelowMin, 1},
		{"days low with cover", ReorderRule{WarningDays: p(10), CoverDays: p(30), Balance: model.StockBalance{AvailableQuantity: 4}}, f(1), ReorderLevelDaysLow, 26},
		{"days ok", ReorderRule{WarningDays: p(3), Balance: model.StockBalance{AvailableQuantity: 4}}, f(1), ReorderLevelOK, 0},
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
