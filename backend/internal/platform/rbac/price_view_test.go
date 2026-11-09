package rbac

import (
	"strings"
	"testing"
)

// TEC-1050: the warehouse roles hold no price permission, so every price
// field reaches them as null; the selling and accounting roles see prices.
func TestPriceViewByRole(t *testing.T) {
	want := map[string]bool{
		RoleCenterWarehouse:           false,
		RoleDistributorWarehouseStaff: false,
		RoleCenterOwner:               true,
		RoleCenterStaff:               true,
		RoleCenterAccounting:          true,
		RoleDistributorOwner:          true,
		RoleDistributorAccounting:     true,
		RoleDealerOwner:               true,
		RoleDealerAccounting:          true,
	}
	for slug, sees := range want {
		def, ok := RoleBySlug(slug)
		if !ok {
			t.Fatalf("role %s missing", slug)
		}
		g := RoleGrants(def)
		got := SeesPrices(func(s string) bool { _, ok := g[s]; return ok })
		if got != sees {
			t.Errorf("%s: SeesPrices = %v, want %v", slug, got, sees)
		}
		if !sees {
			for p := range g {
				if strings.HasPrefix(p, "pricing.") || strings.HasPrefix(p, "dealer_pricing.") {
					t.Errorf("%s holds price permission %s", slug, p)
				}
			}
		}
	}
	if got := strings.Split(PriceViewColumnPermission, "|"); len(got) != len(PriceViewPermissions) {
		t.Fatalf("column permission %q", PriceViewColumnPermission)
	}
}
