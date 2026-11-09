package ioengine

import (
	"reflect"
	"testing"
)

// TEC-1050: a column permission listing alternatives ("a|b") is unlocked by
// any of them, and ColumnPermissions lists each alternative.
func TestAnyOfColumnPermission(t *testing.T) {
	cols := []Column{
		{Key: "order_no"},
		{Key: "total", Permission: "pricing.purchase.read| pricing.sale.read"},
		{Key: "cost", Permission: "pricing.purchase.read"},
	}
	if got := ColumnPermissions(cols); !reflect.DeepEqual(got, []string{"pricing.purchase.read", "pricing.sale.read"}) {
		t.Fatalf("ColumnPermissions = %v", got)
	}
	ds := Dataset{Columns: cols, Rows: []map[string]any{{"order_no": "O1", "total": "10", "cost": "8"}}}
	got := ApplyColumnVisibility(ds, map[string]bool{"pricing.sale.read": true})
	if len(got.Columns) != 2 || got.Columns[1].Key != "total" {
		t.Fatalf("columns = %+v", got.Columns)
	}
	if _, ok := got.Rows[0]["cost"]; ok {
		t.Fatal("cost kept without pricing.purchase.read")
	}
	none := ApplyColumnVisibility(Dataset{Columns: cols, Rows: []map[string]any{{"order_no": "O1", "total": "10"}}}, nil)
	if len(none.Columns) != 1 {
		t.Fatalf("no grants: columns = %+v", none.Columns)
	}
	q := ExportQuery{}
	SetGrantedPermissions(q, []string{"pricing.sale.read"})
	if !GrantedAny(q, "pricing.purchase.read|pricing.sale.read") || GrantedAny(q, "pricing.purchase.read") || !GrantedAny(q, "") {
		t.Fatal("GrantedAny")
	}
}
