package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

var errCoverageReached = errors.New("coverage reached")

// createScopeStore returns a warranty issued by organization 300 of brand 10
// and stops Create at the coverage lookup (reached = the scope check passed).
type createScopeStore struct {
	Store
	reached bool
}

func (s *createScopeStore) GetWarrantyByUUID(_ context.Context, arg db.GetWarrantyByUUIDParams) (db.Warranty, error) {
	return db.Warranty{ID: 1, Uuid: arg.Uuid, OrganizationID: 300, BrandID: arg.BrandID}, nil
}

func (s *createScopeStore) GetWarrantyClaimCoverageContext(context.Context, db.GetWarrantyClaimCoverageContextParams) (db.GetWarrantyClaimCoverageContextRow, error) {
	s.reached = true
	return db.GetWarrantyClaimCoverageContextRow{}, errCoverageReached
}

// TEC-1259: a dealer cannot open a claim on another organization's
// warranty (404); its own organization and a brand-wide center can.
func TestCreateRequiresWarrantyInScope(t *testing.T) {
	in := model.CreateInput{WarrantyUUID: uuid.New(), Description: "Kaputta kabarma", Parts: []model.PartInput{{PartKey: "body_kaput"}}}
	cases := []struct {
		name    string
		c       Caller
		allowed bool
	}{
		{"other dealer", Caller{OrganizationID: 400, BrandID: 10, OrgType: "dealer",
			Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: 400, BrandID: 10, OrgIDs: []int64{400}}}, false},
		{"distributor without the dealer", Caller{OrganizationID: 200, BrandID: 10, OrgType: "distributor",
			Filter: scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgID: 200, BrandID: 10, OrgIDs: []int64{200, 201}}}, false},
		{"issuing dealer", Caller{OrganizationID: 300, BrandID: 10, OrgType: "dealer",
			Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: 300, BrandID: 10, OrgIDs: []int64{300}}}, true},
		{"center brand scope", Caller{OrganizationID: 100, BrandID: 10, OrgType: "center",
			Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, OrgID: 100, BrandID: 10}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &createScopeStore{}
			_, err := NewWithStore(store, nil, nil).Create(context.Background(), tc.c, in)
			if tc.allowed {
				if !store.reached || !errors.Is(err, errCoverageReached) {
					t.Fatalf("in scope: reached=%v err=%v", store.reached, err)
				}
				return
			}
			if !errors.Is(err, ErrNotFound) || store.reached {
				t.Fatalf("out of scope: err=%v reached=%v, want ErrNotFound", err, store.reached)
			}
		})
	}
}
