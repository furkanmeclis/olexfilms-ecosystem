package i18n

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
)

// TEC-508 acceptance: every module of the catalog has its own non-empty
// description in all 13 locales (no en fallback), and no description is
// written for a key outside the catalog.
func TestModuleDescriptionsParity(t *testing.T) {
	for _, l := range Supported {
		cat, ok := moduleDescriptions[l]
		if !ok {
			t.Errorf("%s: no module descriptions", l)
			continue
		}
		for _, m := range features.Modules {
			if cat[m.Key] == "" {
				t.Errorf("%s: module %s has no description", l, m.Key)
			}
			if l != LocaleEN && cat[m.Key] == moduleDescriptions[LocaleEN][m.Key] {
				t.Errorf("%s: module %s description is the en text", l, m.Key)
			}
		}
		for k := range cat {
			if _, ok := features.ModuleByKey(k); !ok {
				t.Errorf("%s: description for unknown module %s", l, k)
			}
		}
	}
	if got := ModuleDescription(LocaleDE, features.ModuleFleet); got == "" || got == ModuleDescription(LocaleEN, features.ModuleFleet) {
		t.Fatalf("de fleet description = %q", got)
	}
}
