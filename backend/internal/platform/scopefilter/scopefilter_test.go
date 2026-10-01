package scopefilter

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type fakeTree map[int64][]int64

func (f fakeTree) Descendants(_ context.Context, id int64) ([]db.Organization, error) {
	out := []db.Organization{}
	for _, c := range f[id] {
		out = append(out, db.Organization{ID: c})
	}
	return out, nil
}

func TestResolveSubtree(t *testing.T) {
	// Distributor 10 has dealers 11, 12; distributor 20 has dealer 21.
	tree := fakeTree{10: {11, 12}, 20: {21}}
	p := authctx.Principal{UserInternal: 7, PermissionScopes: map[string]rbac.Scope{rbac.PermServicesRead: rbac.ScopeSubtree}}
	org := &orgctx.Scope{InternalID: 10, BrandID: 1}
	f, err := Resolve(context.Background(), tree, p, org, rbac.PermServicesRead)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.OrgIDsArg(), []int64{10, 11, 12}) {
		t.Fatalf("org ids = %v", f.OrgIDsArg())
	}
	if !f.AllowsOrg(11, 1) || f.AllowsOrg(21, 1) || f.AllowsOrg(20, 1) {
		t.Fatal("subtree must reach own dealers only")
	}
}

func TestResolveScopes(t *testing.T) {
	ctx := context.Background()
	org := &orgctx.Scope{InternalID: 10, BrandID: 1}
	mk := func(s rbac.Scope) authctx.Principal {
		return authctx.Principal{PermissionScopes: map[string]rbac.Scope{rbac.PermServicesRead: s}}
	}
	f, _ := Resolve(ctx, fakeTree{}, mk(rbac.ScopeBrand), org, rbac.PermServicesRead)
	if f.OrgIDsArg() != nil || !f.BrandIDArg().Valid || !f.AllowsOrg(99, 1) || f.AllowsOrg(99, 2) {
		t.Fatalf("brand filter = %+v", f)
	}
	f, _ = Resolve(ctx, fakeTree{}, mk(rbac.ScopeManaged), org, rbac.PermServicesRead)
	if !slices.Equal(f.OrgIDsArg(), []int64{10}) || f.UserOnly() {
		t.Fatalf("managed filter = %+v", f)
	}
	f, _ = Resolve(ctx, fakeTree{}, mk(rbac.ScopeOwn), org, rbac.PermServicesRead)
	if !f.UserOnly() {
		t.Fatal("own filter is user-only")
	}
	f, _ = Resolve(ctx, fakeTree{}, mk(rbac.ScopeCustomer), nil, rbac.PermServicesRead)
	if f.OrgIDsArg() == nil || len(f.OrgIDsArg()) != 0 || f.AllowsOrg(10, 1) {
		t.Fatal("customer filter reaches no organization records")
	}
	if _, err := Resolve(ctx, fakeTree{}, mk(rbac.ScopeManaged), nil, rbac.PermServicesRead); !errors.Is(err, ErrOrganizationRequired) {
		t.Fatalf("managed without org: %v", err)
	}
	if _, err := Resolve(ctx, fakeTree{}, authctx.Principal{}, org, rbac.PermPricingSaleRead); !errors.Is(err, ErrForbidden) {
		t.Fatalf("missing permission: %v", err)
	}
	sa := authctx.Principal{IsSuperAdmin: true}
	f, _ = Resolve(ctx, fakeTree{}, sa, nil, rbac.PermServicesRead)
	if f.Scope != rbac.ScopeAll || f.OrgIDsArg() != nil {
		t.Fatalf("super admin filter = %+v", f)
	}
}

func TestImpersonateOnlySuperAdmin(t *testing.T) {
	p := authctx.Principal{
		Permissions:      []string{rbac.PermPlatformUsersImpersonate},
		PermissionScopes: map[string]rbac.Scope{rbac.PermPlatformUsersImpersonate: rbac.ScopeAll},
	}
	if p.HasPermission(rbac.PermPlatformUsersImpersonate) {
		t.Fatal("impersonation must require super_admin")
	}
	if !(authctx.Principal{IsSuperAdmin: true}).HasPermission(rbac.PermPlatformUsersImpersonate) {
		t.Fatal("super_admin impersonates")
	}
}
