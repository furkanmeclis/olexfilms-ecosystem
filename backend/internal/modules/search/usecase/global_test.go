package usecase

import (
	"context"
	"sync"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

type fakeFinder struct{ on bool }

func (f fakeFinder) Enabled() bool { return f.on }
func (fakeFinder) SearchIDs(context.Context, string, string, string, int, int) ([]string, int64, error) {
	return nil, 0, nil
}

type fakeFeatures map[string]bool

func (f fakeFeatures) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	on, ok := f[key]
	return !ok || on, nil
}

// docAdapter renders any id as a document of its spec.
type docAdapter struct{ spec searchengine.Spec }

func (a docAdapter) Spec() searchengine.Spec { return a.spec }
func (a docAdapter) ListAll(context.Context) ([]searchengine.Document, error) {
	return nil, nil
}
func (a docAdapter) Document(_ context.Context, id string) (searchengine.Document, error) {
	if id == skipID.String() {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	return searchengine.Document{ID: id, Title: "T " + id, Href: "/" + a.spec.ID + "/" + id}, nil
}

var skipID = uuid.MustParse("00000000-0000-0000-0000-00000000dead")

type recorder struct {
	mu      sync.Mutex
	called  map[string]int
	callers map[string]Caller
}

func (r *recorder) group(spec, perm, feature string, ids ...uuid.UUID) Group {
	return Group{Spec: spec, Permission: perm, Feature: feature,
		Search: func(_ context.Context, c Caller, _ string, limit int32) ([]uuid.UUID, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.called[spec]++
			r.callers[spec] = c
			if int(limit) < len(ids) {
				return ids[:limit], nil
			}
			return ids, nil
		}}
}

func globalFixture(finderOn bool) (*Service, *recorder, context.Context, orgctx.Scope) {
	reg := searchengine.NewRegistry(
		docAdapter{searchengine.Spec{ID: "customers", LabelKey: "search.specs_customers", Icon: "users", ListScoped: true}},
		docAdapter{searchengine.Spec{ID: "services", LabelKey: "search.specs_services", ListScoped: true}},
		docAdapter{searchengine.Spec{ID: "orders", LabelKey: "search.specs_orders", ListScoped: true}},
		docAdapter{searchengine.Spec{ID: "stock_units", LabelKey: "search.specs_stock_units", ListScoped: true}},
		// List scoped without a group: never searched.
		docAdapter{searchengine.Spec{ID: "ungrouped", ListScoped: true}},
		// Platform-wide, no scope filter: not in the global search.
		docAdapter{searchengine.Spec{ID: "roles"}},
	)
	svc := New(nil, reg, nil, nil)
	rec := &recorder{called: map[string]int{}, callers: map[string]Caller{}}
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	svc.SetGroups(fakeFinder{on: finderOn}, nil, fakeFeatures{"orders": false},
		rec.group("customers", rbac.PermCustomersRead, "customers", a, skipID, b),
		rec.group("services", rbac.PermServicesRead, "services", c),
		rec.group("orders", rbac.PermOrdersRead, "orders", c),
		rec.group("stock_units", rbac.PermStockRead, "stock", c),
	)
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{
		UserInternal: 7,
		Permissions:  []string{rbac.PermCustomersRead, rbac.PermOrdersRead, rbac.PermServicesRead},
		PermissionScopes: map[string]rbac.Scope{
			rbac.PermCustomersRead: rbac.ScopeManaged,
			rbac.PermServicesRead:  rbac.ScopeBrand,
			rbac.PermOrdersRead:    rbac.ScopeManaged,
		},
	})
	return svc, rec, ctx, orgctx.Scope{InternalID: 42, BrandID: 3}
}

// TEC-213: groups run through their list with the resolved scope; a group
// without the permission (stock) or with the module off (orders) is not
// queried and not returned; a skipped document is left out.
func TestGlobalGroupsPermissionAndScope(t *testing.T) {
	svc, rec, ctx, org := globalFixture(true)
	res, err := svc.Global(ctx, org, "  abc ", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Enabled || res.Info != nil {
		t.Fatalf("enabled=%v info=%v", res.Enabled, res.Info)
	}
	specs := map[string]GroupResult{}
	for _, g := range res.Groups {
		specs[g.Spec] = g
	}
	if len(specs) != 2 || specs["customers"].LabelKey != "search.specs_customers" || specs["customers"].Icon != "users" {
		t.Fatalf("groups = %+v", res.Groups)
	}
	if got := specs["customers"].Items; len(got) != 2 || got[0].Href == "" || got[0].Spec != "customers" {
		t.Fatalf("customers items = %+v", got)
	}
	if rec.called["orders"] != 0 || rec.called["stock_units"] != 0 {
		t.Fatalf("queried a forbidden group: %v", rec.called)
	}
	if f := rec.callers["customers"].Filter; f.Scope != rbac.ScopeManaged || len(f.OrgIDs) != 1 || f.OrgIDs[0] != 42 {
		t.Fatalf("customers filter = %+v", f)
	}
	if f := rec.callers["services"].Filter; f.Scope != rbac.ScopeBrand || f.BrandID != 3 {
		t.Fatalf("services filter = %+v", f)
	}
	if res.Groups[0].Spec != "customers" || res.Groups[1].Spec != "services" {
		t.Fatalf("group order = %s, %s", res.Groups[0].Spec, res.Groups[1].Spec)
	}
}

func TestGlobalSpecNarrowsAndLimit(t *testing.T) {
	svc, rec, ctx, org := globalFixture(true)
	res, err := svc.Global(ctx, org, "abc", "customers", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 1 || len(res.Groups[0].Items) != 1 || rec.called["services"] != 0 {
		t.Fatalf("groups = %+v called = %v", res.Groups, rec.called)
	}
	if _, err := svc.Global(ctx, org, string(make([]rune, 101)), "", 5); err != ErrQueryTooLong {
		t.Fatalf("long q err = %v", err)
	}
}

// Meilisearch off: the allowed groups stay, empty, with the info code;
// nothing is queried.
func TestGlobalSearchDisabled(t *testing.T) {
	svc, rec, ctx, org := globalFixture(false)
	res, err := svc.Global(ctx, org, "abc", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if res.Enabled || res.Info == nil || *res.Info != InfoSearchDisabled {
		t.Fatalf("enabled=%v info=%v", res.Enabled, res.Info)
	}
	if len(res.Groups) != 2 || len(rec.called) != 0 {
		t.Fatalf("groups = %+v called = %v", res.Groups, rec.called)
	}
	for _, g := range res.Groups {
		if g.Items == nil || len(g.Items) != 0 {
			t.Fatalf("%s items = %v", g.Spec, g.Items)
		}
	}
}
