package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

func TestParseCarBrandUUID(t *testing.T) {
	if id, err := ParseCarBrandUUID("  "); err != nil || id != nil {
		t.Fatalf("empty = %v, %v; want nil, nil", id, err)
	}
	want := uuid.New()
	if id, err := ParseCarBrandUUID(" " + want.String() + " "); err != nil || id == nil || *id != want {
		t.Fatalf("valid = %v, %v; want %s", id, err, want)
	}
	var ve *ValidationError
	if _, err := ParseCarBrandUUID("bmw"); !errors.As(err, &ve) || ve.Field != "car_brand_uuid" {
		t.Fatalf("invalid err = %v", err)
	}
}

// The frequent list never leaves the active organization: every caller the
// overview refuses is refused before any query runs (the Service has no
// querier here, so reaching a query would panic).
func TestFrequentVehiclesScope(t *testing.T) {
	s := &Service{}
	ctx := context.Background()
	brand := uuid.New()

	none := dealerCaller(rbac.ScopeManaged, false)
	none.Principal.PermissionScopes = map[string]rbac.Scope{}
	odd := dealerCaller(rbac.ScopeManaged, false)
	odd.Filter.OrgIDs = []int64{11}
	cust := dealerCaller(rbac.ScopeManaged, false)
	cust.Filter = scopefilter.Filter{Scope: rbac.ScopeCustomer, UserID: 7}
	noOrg := dealerCaller(rbac.ScopeManaged, false)
	noOrg.Org = orgctx.Scope{}

	for name, c := range map[string]Caller{"no grant": none, "foreign org": odd, "customer": cust, "no org": noOrg} {
		if _, err := s.FrequentVehicles(ctx, c, nil); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s brands err = %v, want ErrForbidden", name, err)
		}
		if _, err := s.FrequentVehicles(ctx, c, &brand); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s models err = %v, want ErrForbidden", name, err)
		}
	}
}
