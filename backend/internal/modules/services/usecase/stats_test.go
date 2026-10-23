package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

func TestStatsSince(t *testing.T) {
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	for period, want := range map[string]time.Time{
		StatsPeriod30d: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		StatsPeriod90d: time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC),
		StatsPeriod12m: time.Date(2025, 3, 31, 12, 0, 0, 0, time.UTC),
	} {
		got, err := StatsSince(period, now)
		if err != nil || got == nil || !got.Equal(want) {
			t.Fatalf("%s = %v, %v; want %v", period, got, err, want)
		}
	}
	if got, err := StatsSince(StatsPeriodAll, now); err != nil || got != nil {
		t.Fatalf("all = %v, %v; want nil", got, err)
	}
	var ve *ValidationError
	if _, err := StatsSince("7d", now); !errors.As(err, &ve) || ve.Field != "period" {
		t.Fatalf("7d err = %v", err)
	}
}

func TestStatsBrand(t *testing.T) {
	if b, err := statsBrand(centerCaller()); err != nil || b != 1 {
		t.Fatalf("center = %d, %v; want brand 1", b, err)
	}
	// super_admin: brand of the selected organization.
	admin := Caller{
		Principal: authctx.Principal{UserInternal: 9, IsSuperAdmin: true},
		Org:       orgctx.Scope{InternalID: 30, BrandID: 2, OrgType: OrgCenter},
	}
	if b, err := statsBrand(admin); err != nil || b != 2 {
		t.Fatalf("super_admin = %d, %v; want brand 2", b, err)
	}
	if _, err := statsBrand(dealerCaller(rbac.ScopeManaged, false)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer err = %v, want ErrForbidden", err)
	}
	dist := Caller{
		Principal: authctx.Principal{UserInternal: 3, PermissionScopes: map[string]rbac.Scope{rbac.PermServicesRead: rbac.ScopeSubtree}},
		Org:       orgctx.Scope{InternalID: 20, BrandID: 1, OrgType: OrgDistributor},
	}
	if _, err := statsBrand(dist); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor err = %v, want ErrForbidden", err)
	}
	// A brand-wide grant outside the center organization is not enough.
	odd := centerCaller()
	odd.Org.OrgType = OrgDealer
	if _, err := statsBrand(odd); !errors.Is(err, ErrForbidden) {
		t.Fatalf("brand grant in a dealer err = %v, want ErrForbidden", err)
	}
}

func TestTopVehicleModelsValidation(t *testing.T) {
	s := &Service{}
	var ve *ValidationError
	if _, err := s.TopVehicleModels(context.Background(), centerCaller(), "30d", "trim", time.Now()); !errors.As(err, &ve) || ve.Field != "group" {
		t.Fatalf("group err = %v", err)
	}
	if _, err := s.TopVehicleModels(context.Background(), centerCaller(), "1y", "model", time.Now()); !errors.As(err, &ve) || ve.Field != "period" {
		t.Fatalf("period err = %v", err)
	}
	if _, err := s.TopVehicleModels(context.Background(), dealerCaller(rbac.ScopeManaged, false), "all", "brand", time.Now()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer err = %v", err)
	}
}

func TestOverviewOrg(t *testing.T) {
	// A dealer reads its own organization.
	if id, err := overviewOrg(dealerCaller(rbac.ScopeManaged, false)); err != nil || id != 10 {
		t.Fatalf("dealer = %d, %v; want org 10", id, err)
	}
	// A center with a brand-wide grant still gets its own organization only.
	if id, err := overviewOrg(centerCaller()); err != nil || id != 1 {
		t.Fatalf("center = %d, %v; want org 1", id, err)
	}
	// A filter that does not reach the active organization is refused.
	odd := dealerCaller(rbac.ScopeManaged, false)
	odd.Filter.OrgIDs = []int64{11}
	if _, err := overviewOrg(odd); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign org err = %v, want ErrForbidden", err)
	}
	// No services.read grant.
	none := dealerCaller(rbac.ScopeManaged, false)
	none.Principal.PermissionScopes = map[string]rbac.Scope{}
	if _, err := overviewOrg(none); !errors.Is(err, ErrForbidden) {
		t.Fatalf("no grant err = %v, want ErrForbidden", err)
	}
	// Customer scope never reaches organization records.
	cust := dealerCaller(rbac.ScopeManaged, false)
	cust.Filter = scopefilter.Filter{Scope: rbac.ScopeCustomer, UserID: 7}
	if _, err := overviewOrg(cust); !errors.Is(err, ErrForbidden) {
		t.Fatalf("customer err = %v, want ErrForbidden", err)
	}
	// Without an active organization.
	noOrg := dealerCaller(rbac.ScopeManaged, false)
	noOrg.Org = orgctx.Scope{}
	if _, err := overviewOrg(noOrg); !errors.Is(err, ErrForbidden) {
		t.Fatalf("no org err = %v, want ErrForbidden", err)
	}
}

func TestHomeOverviewValidation(t *testing.T) {
	s := &Service{}
	var ve *ValidationError
	if _, err := s.HomeOverview(context.Background(), dealerCaller(rbac.ScopeManaged, false), "7d", time.Now()); !errors.As(err, &ve) || ve.Field != "period" {
		t.Fatalf("period err = %v", err)
	}
	none := dealerCaller(rbac.ScopeManaged, false)
	none.Principal.PermissionScopes = map[string]rbac.Scope{}
	if _, err := s.HomeOverview(context.Background(), none, "", time.Now()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("no grant err = %v", err)
	}
}
