package authctx

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-1266: the ceiling caps record scopes of a super admin; permissions
// without a scope under the ceiling keep their broadest catalog scope.
func TestScopeForSuperAdminCeiling(t *testing.T) {
	free := Principal{IsSuperAdmin: true}
	if s, ok := free.ScopeFor(rbac.PermServicesRead); !ok || s != rbac.ScopeAll {
		t.Fatalf("uncapped services.read = %q,%v; want all", s, ok)
	}
	dist := Principal{IsSuperAdmin: true, ScopeCeiling: rbac.ScopeSubtree}
	if s, _ := dist.ScopeFor(rbac.PermServicesRead); s != rbac.ScopeSubtree {
		t.Fatalf("distributor services.read = %q; want subtree", s)
	}
	dealer := Principal{IsSuperAdmin: true, ScopeCeiling: rbac.ScopeManaged}
	if s, _ := dealer.ScopeFor(rbac.PermServicesRead); s != rbac.ScopeManaged {
		t.Fatalf("dealer services.read = %q; want managed", s)
	}
	if !dist.Can(rbac.PermServicesRead, rbac.ScopeManaged) {
		t.Fatal("subtree must still cover a managed check")
	}
	if dist.Can(rbac.PermServicesRead, rbac.ScopeBrand) {
		t.Fatal("distributor-capped super admin must not pass a brand check")
	}
	// Every catalog permission stays granted; none comes back as customer.
	for _, def := range rbac.Permissions {
		s, ok := dealer.ScopeFor(def.Slug)
		if !ok {
			t.Fatalf("%s: not granted to capped super admin", def.Slug)
		}
		if s == rbac.ScopeCustomer && rbac.Broadest(def.Scopes) != rbac.ScopeCustomer {
			t.Fatalf("%s: capped to customer scope", def.Slug)
		}
	}
}

func TestCapScope(t *testing.T) {
	if s, ok := rbac.CapScope([]rbac.Scope{rbac.ScopeAll}, rbac.ScopeSubtree); ok {
		t.Fatalf("platform-only permission capped to %q", s)
	}
	if s, ok := rbac.CapScope([]rbac.Scope{rbac.ScopeOwn, rbac.ScopeManaged, rbac.ScopeAll, rbac.ScopeCustomer}, rbac.ScopeSubtree); !ok || s != rbac.ScopeManaged {
		t.Fatalf("got %q,%v; want managed", s, ok)
	}
}
