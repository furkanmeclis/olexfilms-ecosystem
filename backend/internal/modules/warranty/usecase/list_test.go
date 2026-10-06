package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

func intp(n int) *int { return &n }

func TestNormalizeVoidReason(t *testing.T) {
	if _, err := NormalizeVoidReason("  ab "); err == nil {
		t.Fatal("short reason accepted")
	}
	if _, err := NormalizeVoidReason(strings.Repeat("ğ", 501)); err == nil {
		t.Fatal("long reason accepted")
	}
	got, err := NormalizeVoidReason("  hatalı uygulama  ")
	if err != nil || got != "hatalı uygulama" {
		t.Fatalf("reason = %q, %v", got, err)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`50%_a\b`); got != `50\%\_a\\b` {
		t.Fatalf("escape = %q", got)
	}
}

func TestPanelScope(t *testing.T) {
	org := orgctx.Scope{InternalID: 7, BrandID: 2}
	cases := []struct {
		f         scopefilter.Filter
		nilOrgs   bool
		holder    bool
		createdBy bool
	}{
		{scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: 2}, true, false, false},
		{scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgIDs: []int64{7, 8}}, false, false, false},
		{scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{7}}, false, false, false},
		{scopefilter.Filter{Scope: rbac.ScopeOwn, OrgIDs: []int64{7}, UserID: 9}, false, false, true},
		{scopefilter.Filter{Scope: rbac.ScopeCustomer, UserID: 9}, true, true, false},
	}
	for _, tc := range cases {
		sp := panelScope(Caller{Org: org, Filter: tc.f})
		if sp.brandID != 2 || (sp.orgIDs == nil) != tc.nilOrgs || sp.holderUserID.Valid != tc.holder ||
			sp.serviceCreatedBy.Valid != tc.createdBy {
			t.Fatalf("%s: scope = %+v", tc.f.Scope, sp)
		}
	}
	sub := panelScope(Caller{Org: org, Filter: scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgIDs: []int64{7, 8}}})
	if len(sub.orgIDs) != 2 {
		t.Fatalf("subtree org ids = %v", sub.orgIDs)
	}
}

func TestCanVoid(t *testing.T) {
	org := orgctx.Scope{BrandID: 2}
	center := authctx.Principal{PermissionScopes: map[string]rbac.Scope{rbac.PermWarrantiesVoid: rbac.ScopeBrand}}
	admin := authctx.Principal{PermissionScopes: map[string]rbac.Scope{rbac.PermWarrantiesVoid: rbac.ScopeAll}}
	dealer := authctx.Principal{PermissionScopes: map[string]rbac.Scope{rbac.PermWarrantiesRead: rbac.ScopeManaged}}
	if !canVoid(center, org, 2) || canVoid(center, org, 3) {
		t.Fatal("center brand scope")
	}
	if !canVoid(admin, org, 3) {
		t.Fatal("super admin")
	}
	if canVoid(dealer, org, 2) {
		t.Fatal("dealer may not void")
	}
	if voidable(StatusVoid) || !voidable(StatusExpired) || !voidable(StatusActive) {
		t.Fatal("voidable")
	}
}

func TestListArgs(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	r := (&Reader{}).WithClock(func() time.Time { return now })
	ctx := context.Background()
	sp := scopeParams{brandID: 2}
	var ve *ValidationError

	for _, f := range []ListFilter{
		{Statuses: []string{"active", "open"}},
		{Q: strings.Repeat("a", 101)},
		{DaysLeftMin: intp(-1)},
		{DaysLeftMax: intp(4000)},
		{DaysLeftMin: intp(30), DaysLeftMax: intp(7)},
		{ProductUUID: "nope"},
		{VehicleUUID: "nope"},
	} {
		if _, _, err := r.listArgs(ctx, sp, f); !errors.As(err, &ve) {
			t.Fatalf("%+v: err = %v, want validation", f, err)
		}
	}

	p, ok, err := r.listArgs(ctx, sp, ListFilter{Statuses: []string{"active"}, Q: " 34 abc-12 ", DaysLeftMax: intp(30), Limit: 20})
	if err != nil || !ok {
		t.Fatalf("args: %v %v", ok, err)
	}
	if len(p.Statuses) != 1 || p.Statuses[0] != "active" || p.SortKey != "expiry" || p.SortDesc || p.Q.String != "34 abc-12" || p.QPlate.String != "34ABC12" {
		t.Fatalf("q / status = %+v", p)
	}
	if !p.EndsAfter.Time.Equal(now) || !p.EndsBefore.Time.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("window = %v .. %v", p.EndsAfter.Time, p.EndsBefore.Time)
	}

	p, _, _ = r.listArgs(ctx, sp, ListFilter{DaysLeftMin: intp(7), DaysLeftMax: intp(30)})
	if !p.EndsAfter.Time.Equal(now.AddDate(0, 0, 7)) || !p.EndsBefore.Time.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("min/max window = %v .. %v", p.EndsAfter.Time, p.EndsBefore.Time)
	}
	p, _, _ = r.listArgs(ctx, sp, ListFilter{})
	if p.EndsAfter.Valid || p.EndsBefore.Valid || p.Q.Valid || p.Statuses != nil {
		t.Fatalf("empty filter = %+v", p)
	}
}

func TestPortalWithoutBrandIsEmpty(t *testing.T) {
	r := NewReader(nil, nil, nil, "")
	items, total, err := r.PortalList(context.Background(), 0, 5, ListFilter{})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("portal list without brand = %v %d %v", items, total, err)
	}
	if _, err := r.PortalGet(context.Background(), 2, 0, [16]byte{1}); !errors.Is(err, ErrWarrantyNotFound) {
		t.Fatalf("portal get without user = %v", err)
	}
}
