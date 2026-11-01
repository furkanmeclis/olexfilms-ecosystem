package usecase

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

func catRow(serviceID, categoryID int64, name string) db.ListServiceItemCategoriesRow {
	return db.ListServiceItemCategoriesRow{ServiceID: serviceID, CategoryID: categoryID, CategoryName: name}
}

func TestGroupItemCategoriesDistinctPerServiceInRowOrder(t *testing.T) {
	rows := []db.ListServiceItemCategoriesRow{
		catRow(1, 10, "PPF"),
		catRow(1, 11, "Cam Filmi"),
		catRow(1, 12, " ppf "), // same name, other spelling: first spelling wins
		catRow(1, 13, "  "),    // blank: skipped
		catRow(2, 11, "Cam Filmi"),
		catRow(2, 10, "PPF"),
	}
	got := groupItemCategories(rows)
	want := map[int64][]string{
		1: {"PPF", "Cam Filmi"},
		2: {"Cam Filmi", "PPF"}, // per-service order follows the query order
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groupItemCategories = %#v, want %#v", got, want)
	}
}

func TestCategoriesOfIsNeverNull(t *testing.T) {
	byService := groupItemCategories(nil)
	got := categoriesOf(byService, 42)
	if got == nil || len(got) != 0 {
		t.Fatalf("categoriesOf(missing) = %#v, want empty non-nil slice", got)
	}
	b, err := json.Marshal(ServiceView{ItemCategories: got})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["item_categories"]) != "[]" {
		t.Fatalf("item_categories JSON = %s, want []", m["item_categories"])
	}
}

func TestCategoriesOfKeepsStoredNames(t *testing.T) {
	// Category names have no translations (product_categories.name is a
	// single-language column), so every locale gets the stored name, as in
	// the portal detail.
	byService := groupItemCategories([]db.ListServiceItemCategoriesRow{catRow(7, 1, "Seramik Kaplama")})
	if got := categoriesOf(byService, 7); !reflect.DeepEqual(got, []string{"Seramik Kaplama"}) {
		t.Fatalf("categoriesOf = %#v", got)
	}
}
