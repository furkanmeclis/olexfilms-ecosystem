package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/popularity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

// TEC-1043: the frequent list ranks by the mixed score and keeps the first
// FrequentLimit used rows; service_count is the caller organization's own
// usage only (0 for a brand only other organizations use).
func TestFrequentBrandsMixedRanking(t *testing.T) {
	rows := []db.ActiveCarBrandsByIDsRow{
		{ID: 1, Uuid: uuid.New(), Name: "Audi"},
		{ID: 2, Uuid: uuid.New(), Name: "BMW", LogoObjectKey: pgtype.Text{String: "vehicle-brands/b/logo-1.svg", Valid: true}},
		{ID: 3, Uuid: uuid.New(), Name: "Citroen"},
	}
	res := popularity.Result{
		Scores:    popularity.Scores(popularity.Counts{1: 2}, popularity.Counts{1: 2, 2: 50, 3: 10}, true),
		OrgCounts: popularity.Counts{1: 2},
	}
	got := frequentBrands(rows, res, FrequentLimit)
	if len(got) != 3 || got[0].Name != "Audi" || got[1].Name != "BMW" || got[2].Name != "Citroen" {
		t.Fatalf("order = %+v", got)
	}
	if got[0].ServiceCount != 2 || got[1].ServiceCount != 0 || got[2].ServiceCount != 0 {
		t.Fatalf("service counts leak other organizations: %+v", got)
	}
	if !got[1].HasLogo || got[0].HasLogo {
		t.Fatalf("logos = %+v", got)
	}
	if two := frequentBrands(rows, res, 2); len(two) != 2 {
		t.Fatalf("limit not applied: %d", len(two))
	}
}

// A new organization without own usage still gets a list from the brand's
// general usage; equal scores sort by name (case-insensitive), then id.
func TestFrequentModelsNewOrganization(t *testing.T) {
	rows := []db.ActiveCarModelsByIDsRow{
		{ID: 9, Uuid: uuid.New(), Name: "X5"},
		{ID: 8, Uuid: uuid.New(), Name: "x3"},
		{ID: 7, Uuid: uuid.New(), Name: "X3"},
	}
	res := popularity.Result{
		Scores:    popularity.Scores(popularity.Counts{}, popularity.Counts{9: 1, 8: 1, 7: 1}, true),
		OrgCounts: popularity.Counts{},
	}
	got := frequentModels(rows, res, FrequentLimit)
	if len(got) != 3 || got[0].UUID != rows[2].Uuid || got[1].UUID != rows[1].Uuid || got[2].Name != "X5" {
		t.Fatalf("order = %+v", got)
	}
	for _, m := range got {
		if m.ServiceCount != 0 {
			t.Fatalf("service count without own usage: %+v", m)
		}
	}
	if empty := frequentModels(nil, popularity.Result{}, FrequentLimit); empty == nil || len(empty) != 0 {
		t.Fatalf("empty = %#v, want []", empty)
	}
}

func TestUsedIDs(t *testing.T) {
	if ids := usedIDs(map[int64]float64{3: 0, 2: 0.5, 1: 0.1}); len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("used ids = %v", ids)
	}
	if ids := usedIDs(nil); len(ids) != 0 {
		t.Fatalf("used ids = %v", ids)
	}
}
