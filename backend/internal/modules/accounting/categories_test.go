package accounting

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// Every catalog key fits chk_finance_entries_category, keys are unique, the
// direction is one of chk_finance_entries_direction and the label resolves
// in all 13 languages.
func TestCategoryCatalog(t *testing.T) {
	directions := map[string]bool{
		DirectionIncome: true, DirectionExpense: true, DirectionCharge: true,
		DirectionCollection: true, DirectionPayment: true,
	}
	seen := map[string]bool{}
	for _, c := range Categories() {
		if !ValidCategoryKey(c.Key) {
			t.Errorf("%q does not satisfy the category CHECK", c.Key)
		}
		if seen[c.Key] {
			t.Errorf("duplicate key %q", c.Key)
		}
		seen[c.Key] = true
		if !directions[c.Direction] {
			t.Errorf("%q: unknown direction %q", c.Key, c.Direction)
		}
		for _, l := range i18n.Supported {
			if got := i18n.Translate(l, c.LabelKey); got == c.LabelKey || got == "" {
				t.Errorf("%s: %s has no label", l, c.LabelKey)
			}
		}
	}
	for d := range directions {
		if got := i18n.Translate(i18n.LocaleTR, "accounting.direction."+d); got == "accounting.direction."+d {
			t.Errorf("direction %s has no label", d)
		}
	}
	for _, k := range []string{CategorySale, CategoryPurchase} {
		c, ok := LookupCategory(k)
		if !ok || c.Manual {
			t.Errorf("%s must be a system category", k)
		}
	}
	if _, ok := LookupCategory("nope"); ok {
		t.Error("unknown key found")
	}
}
