package usecase

import (
	"context"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
)

type stubAdapter struct{ spec searchengine.Spec }

func (a stubAdapter) Spec() searchengine.Spec { return a.spec }
func (a stubAdapter) ListAll(context.Context) ([]searchengine.Document, error) {
	return nil, nil
}
func (a stubAdapter) Document(context.Context, string) (searchengine.Document, error) {
	return searchengine.Document{}, nil
}

// TEC-164: list scoped specs (customers) are never offered or searched by
// the command palette, even with the permission; only the module list
// applies their customer_organizations scope.
func TestListSpecsHidesListScoped(t *testing.T) {
	reg := searchengine.NewRegistry(
		stubAdapter{searchengine.Spec{ID: "roles"}},
		stubAdapter{searchengine.Spec{ID: "customers", Permission: rbac.PermCustomersRead, ListScoped: true}},
	)
	svc := New(nil, reg, nil, nil)
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{
		Permissions:      []string{rbac.PermCustomersRead},
		PermissionScopes: map[string]rbac.Scope{rbac.PermCustomersRead: rbac.ScopeBrand},
	})
	specs := svc.ListSpecs(ctx)
	if len(specs) != 1 || specs[0].ID != "roles" {
		t.Fatalf("specs = %+v", specs)
	}
	if ids, _ := svc.allowedSpecs(ctx, "customers"); len(ids) != 0 {
		t.Fatalf("customers searchable from the palette: %v", ids)
	}
}
