package usecase

import (
	"encoding/json"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// asRole gives the caller the grants of a system role (TEC-1050).
func asRole(c Caller, role string) Caller {
	def, ok := rbac.RoleBySlug(role)
	if !ok {
		panic("unknown role " + role)
	}
	g := rbac.RoleGrants(def)
	c.Principal = authctx.Principal{
		UserID: c.Principal.UserID, UserInternal: c.Principal.UserInternal,
		Roles: []string{role}, PermissionScopes: g,
	}
	for slug := range g {
		c.Principal.Permissions = append(c.Principal.Permissions, slug)
	}
	return c
}

// TEC-1050: the warehouse roles get the order list and detail without a
// single money field; accounting and the center owner see them.
func TestOrderPricesHiddenFromWarehouse(t *testing.T) {
	f := newInvoicingFixture(t)

	for _, role := range []string{rbac.RoleCenterWarehouse, rbac.RoleCenterService} {
		c := asRole(f.center, role)
		if c.SeesPrices() {
			t.Fatalf("%s: SeesPrices = true", role)
		}
		rows, _, err := f.svc.List(f.ctx, c, ListFilter{Side: SideSeller, Limit: 50})
		if err != nil {
			t.Fatalf("%s list: %v", role, err)
		}
		found := false
		for _, r := range rows {
			if r.UUID != f.order {
				continue
			}
			found = true
			if r.Subtotal != nil || r.TaxTotal != nil || r.Total != nil || r.TryRate != nil || string(r.RateSnapshot) != "null" {
				t.Fatalf("%s list row carries money: %+v", role, r)
			}
		}
		if !found {
			t.Fatalf("%s: order missing from the list", role)
		}
		v, err := f.svc.Get(f.ctx, c, f.order)
		if err != nil {
			t.Fatalf("%s get: %v", role, err)
		}
		if v.Total != nil || v.Subtotal != nil || len(v.Items) != 2 {
			t.Fatalf("%s detail: total=%v items=%d", role, v.Total, len(v.Items))
		}
		for _, it := range v.Items {
			if it.UnitPrice != nil || it.LineTotal != nil || it.PriceSource != nil || it.PriceMissing {
				t.Fatalf("%s line carries money: %+v", role, it)
			}
		}
		// The JSON has the keys with null values.
		raw, _ := json.Marshal(v)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if val, ok := m["total"]; !ok || val != nil {
			t.Fatalf("%s json total = %v (present %v)", role, val, ok)
		}
		// The total filter cannot probe the hidden totals.
		probe, _, err := f.svc.List(f.ctx, c, ListFilter{Side: SideSeller, TotalMin: ptrFloat(1e12), Limit: 50})
		if err != nil {
			t.Fatalf("%s total probe: %v", role, err)
		}
		if len(probe) == 0 {
			t.Fatalf("%s: total filter applied for a caller without prices", role)
		}
	}

	for _, role := range []string{rbac.RoleCenterAccounting, rbac.RoleCenterOwner, rbac.RoleCenterStaff} {
		c := asRole(f.center, role)
		v, err := f.svc.Get(f.ctx, c, f.order)
		if err != nil {
			t.Fatalf("%s get: %v", role, err)
		}
		if v.Total == nil || *v.Total != "300.00" || v.Items[0].UnitPrice == nil || v.Items[0].LineTotal == nil {
			t.Fatalf("%s: prices missing: total=%v items=%+v", role, v.Total, v.Items)
		}
		rows, _, err := f.svc.List(f.ctx, c, ListFilter{Side: SideSeller, Limit: 50})
		if err != nil {
			t.Fatalf("%s list: %v", role, err)
		}
		for _, r := range rows {
			if r.UUID == f.order && r.Total == nil {
				t.Fatalf("%s: list total hidden", role)
			}
		}
	}
}

// TEC-1050: an export job without a price grant has no money columns; a
// granted job keeps them filled.
func TestOrderListExportPriceColumns(t *testing.T) {
	f := newInvoicingFixture(t)
	a := NewListExportAdapter(f.svc)
	cols := a.ExportColumns()
	for _, granted := range []bool{false, true} {
		var slugs []string
		if granted {
			slugs = []string{rbac.PermPricingSaleRead}
		}
		keep := ioengine.VisibleColumns(cols, map[string]bool{})
		if granted {
			keep = ioengine.VisibleColumns(cols, map[string]bool{rbac.PermPricingSaleRead: true})
		}
		has := false
		for _, c := range keep {
			has = has || c.Key == "total"
		}
		if has != granted {
			t.Fatalf("granted=%v: total column visible=%v", granted, has)
		}
		q := ioengine.ExportQuery{}
		ioengine.SetGrantedPermissions(q, slugs)
		if got := ioengine.GrantedAny(q, rbac.PriceViewColumnPermission); got != granted {
			t.Fatalf("granted=%v: GrantedAny=%v", granted, got)
		}
	}
	// Without a principal the caller sees no prices unless the job says so.
	if (Caller{}).SeesPrices() || !(Caller{PriceGrant: true}).SeesPrices() {
		t.Fatal("PriceGrant does not drive SeesPrices")
	}
}

func ptrFloat(v float64) *float64 { return &v }
