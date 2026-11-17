package usecase

import "testing"

// TEC-1214: a money column gets the shared row currency; rows in different
// currencies keep their row currency and lose the sum footer.
func TestMoneyColumns(t *testing.T) {
	cols := func() []Column {
		return []Column{
			{Key: "order_count", Type: "number", Aggregate: "sum"},
			{Key: "total", Type: "money", Aggregate: "sum"},
		}
	}
	single := Envelope{Columns: cols(), Rows: []map[string]any{
		{"total": "10.00", "currency": "USD"}, {"total": "5.00", "currency": "USD"}, {"total": nil, "currency": nil},
	}}
	moneyColumns(&single)
	if c := single.Columns[1]; c.Currency == nil || *c.Currency != "USD" || c.Aggregate != "sum" {
		t.Fatalf("single currency column = %+v", c)
	}
	mixed := Envelope{Columns: cols(), Rows: []map[string]any{
		{"total": "10.00", "currency": "USD"}, {"total": "500.00", "currency": "TRY"},
	}}
	moneyColumns(&mixed)
	if c := mixed.Columns[1]; c.Currency != nil || c.Aggregate != "none" {
		t.Fatalf("mixed currency column = %+v", c)
	}
	if c := mixed.Columns[0]; c.Aggregate != "sum" {
		t.Fatalf("count column aggregate = %q", c.Aggregate)
	}
	none := Envelope{Columns: cols(), Rows: []map[string]any{{"total": nil}}}
	moneyColumns(&none)
	if c := none.Columns[1]; c.Currency != nil || c.Aggregate != "sum" {
		t.Fatalf("no currency column = %+v", c)
	}
}
