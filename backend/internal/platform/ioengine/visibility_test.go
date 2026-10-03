package ioengine

import (
	"reflect"
	"testing"
)

func TestApplyColumnVisibilityDropsUngrantedColumns(t *testing.T) {
	cols := []Column{
		{Key: "sku", LabelKey: "catalog.products.sku"},
		{Key: "purchase_price", LabelKey: "catalog.products.purchase_price", Permission: "pricing.purchase.read"},
		{Key: "sale_price", LabelKey: "catalog.products.sale_price", Permission: "pricing.sale.read"},
	}
	ds := Dataset{
		Columns: cols,
		Rows:    []map[string]any{{"sku": "A", "purchase_price": "10", "sale_price": "12"}},
		Totals:  map[string]any{"purchase_price": "10", "sale_price": "12"},
	}
	q := ExportQuery{}
	SetGrantedPermissions(q, []string{"pricing.sale.read", " ", "pricing.sale.read"})
	if q[QueryGrantedPermissions] != "pricing.sale.read,pricing.sale.read" && q[QueryGrantedPermissions] != "pricing.sale.read" {
		t.Fatalf("stored = %q", q[QueryGrantedPermissions])
	}
	got := ApplyColumnVisibility(ds, GrantedPermissions(q))
	keys := []string{}
	for _, c := range got.Columns {
		keys = append(keys, c.Key)
	}
	if !reflect.DeepEqual(keys, []string{"sku", "sale_price"}) {
		t.Fatalf("columns = %v", keys)
	}
	if _, ok := got.Rows[0]["purchase_price"]; ok {
		t.Fatal("purchase_price still in the row")
	}
	if _, ok := got.Totals["purchase_price"]; ok {
		t.Fatal("purchase_price still in totals")
	}
	if got.Rows[0]["sale_price"] != "12" {
		t.Fatalf("sale_price = %v", got.Rows[0]["sale_price"])
	}
	if p := ColumnPermissions(cols); !reflect.DeepEqual(p, []string{"pricing.purchase.read", "pricing.sale.read"}) {
		t.Fatalf("permissions = %v", p)
	}
	// No grants at all: both price columns go, the plain one stays.
	none := ApplyColumnVisibility(ds, GrantedPermissions(ExportQuery{}))
	if len(none.Columns) != 1 || none.Columns[0].Key != "sku" {
		t.Fatalf("columns without grants = %v", none.Columns)
	}
	SetGrantedPermissions(q, nil)
	if _, ok := q[QueryGrantedPermissions]; ok {
		t.Fatal("empty grant list must remove the key")
	}
}
