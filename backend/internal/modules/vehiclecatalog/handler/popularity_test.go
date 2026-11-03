package handler

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/popularity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

func TestWantsPopularity(t *testing.T) {
	for raw, want := range map[string]bool{
		"/v1/vehicle-catalog/brands":                         false,
		"/v1/vehicle-catalog/brands?sort=name":               false,
		"/v1/vehicle-catalog/brands?sort=popularity":         true,
		"/v1/vehicle-catalog/brands?sort=-popularity":        true,
		"/v1/vehicle-catalog/brands?sort=name,popularity":    false,
		"/v1/vehicle-catalog/models?sort=popularity,name&q=": true,
	} {
		if got := WantsPopularity(httptest.NewRequest("GET", raw, nil)); got != want {
			t.Fatalf("%s = %v, want %v", raw, got, want)
		}
	}
}

func TestPopularityScope(t *testing.T) {
	pop := apiquery.ResolvedSort{Key: model.SortPopularity}
	r := httptest.NewRequest("GET", "/v1/vehicle-catalog/brands?sort=popularity", nil)

	// Other sorts never need a scope.
	if s, err := popularityScope(r, apiquery.ResolvedSort{Key: "name"}); err != nil || s != (popularity.Scope{}) {
		t.Fatalf("name sort = %+v, %v", s, err)
	}
	// Active organization: its brand and itself.
	orgReq := r.WithContext(orgctx.WithScope(r.Context(), orgctx.Scope{InternalID: 10, BrandID: 1}))
	if s, err := popularityScope(orgReq, pop); err != nil || s != (popularity.Scope{BrandID: 1, OrgID: 10}) {
		t.Fatalf("org scope = %+v, %v", s, err)
	}
	// Super admin brand-center fallback: no organization, general only.
	saReq := r.WithContext(orgctx.WithScope(r.Context(), orgctx.Scope{InternalID: 1, BrandID: 2, SuperAdminFallback: true}))
	if s, err := popularityScope(saReq, pop); err != nil || s != (popularity.Scope{BrandID: 2}) {
		t.Fatalf("super admin scope = %+v, %v", s, err)
	}
	// Domain brand only.
	brReq := r.WithContext(brandctx.WithBrand(r.Context(), brandctx.Brand{ID: 3}))
	if s, err := popularityScope(brReq, pop); err != nil || s != (popularity.Scope{BrandID: 3}) {
		t.Fatalf("brand scope = %+v, %v", s, err)
	}
	if _, err := popularityScope(r, pop); !errors.Is(err, usecase.ErrPopularityUnavailable) {
		t.Fatalf("no scope err = %v", err)
	}
	// -popularity is refused (400 through response.QueryValidation).
	var verr *apiquery.ValidationError
	if _, err := popularityScope(orgReq, apiquery.ResolvedSort{Key: model.SortPopularity, Desc: true}); !errors.As(err, &verr) || verr.Details[0].Field != "sort" {
		t.Fatalf("-popularity err = %v", err)
	}
}
