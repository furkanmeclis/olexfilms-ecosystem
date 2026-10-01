package usecase

import (
	"math/big"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNumericString(t *testing.T) {
	for _, c := range []struct {
		n    pgtype.Numeric
		want string
	}{
		{pgtype.Numeric{Int: big.NewInt(1500), Exp: -2, Valid: true}, "15.00"},
		{pgtype.Numeric{Int: big.NewInt(-45), Exp: -1, Valid: true}, "-4.50"},
		{pgtype.Numeric{Int: big.NewInt(6), Exp: 0, Valid: true}, "6.00"},
		{pgtype.Numeric{Int: big.NewInt(2), Exp: 1, Valid: true}, "20.00"},
		{pgtype.Numeric{}, "0.00"},
	} {
		if got := numericString(c.n); got != c.want {
			t.Fatalf("numericString(%v) = %s, want %s", c.n, got, c.want)
		}
	}
	if numericPtr(pgtype.Numeric{}) != nil {
		t.Fatal("NULL meters must stay null")
	}
}

// A dealer (managed) names itself and its supplier chain; any other
// organization is masked together with its locations. Service owners follow
// the row.
func TestOwnerMasking(t *testing.T) {
	center := db.Organization{ID: 1, Uuid: uuid.New(), Name: "Center", Type: "center", BrandID: 1}
	dist := db.Organization{ID: 2, Uuid: uuid.New(), Name: "Dist", Type: "distributor", BrandID: 1}
	dealer := db.Organization{ID: 3, Uuid: uuid.New(), Name: "Dealer", Type: "dealer", BrandID: 1}
	other := db.Organization{ID: 4, Uuid: uuid.New(), Name: "Other", Type: "distributor", BrandID: 1}
	r := refs{
		orgs: map[int64]db.Organization{1: center, 2: dist, 3: dealer, 4: other},
		locs: map[int64]db.WarehouseLocation{
			10: {ID: 10, OrganizationID: 1, Code: "C"},
			40: {ID: 40, OrganizationID: 4, Code: "O"},
		},
	}
	v := viewer{
		f:         scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{3}},
		ancestors: map[int64]bool{1: true, 2: true},
	}
	if !v.reaches(3, 1) || v.reaches(2, 1) {
		t.Fatal("a managed dealer reaches only its own stock")
	}
	if o := r.owner(v, ownerLocation, 10, true); o.Masked || o.Location == nil || o.Organization.UUID != center.Uuid {
		t.Fatalf("center location = %+v", o)
	}
	if o := r.owner(v, ownerLocation, 40, true); !o.Masked || o.Location != nil || o.Organization != nil {
		t.Fatalf("other location must be masked: %+v", o)
	}
	if o := r.owner(v, ownerOrganization, 4, true); !o.Masked || o.Organization != nil {
		t.Fatalf("other organization must be masked: %+v", o)
	}
	if o := r.owner(v, ownerOrganization, 3, false); o.Masked || o.Organization.UUID != dealer.Uuid {
		t.Fatalf("own organization = %+v", o)
	}
	if o := r.owner(v, ownerService, 99, false); !o.Masked {
		t.Fatal("service owner of a masked row is masked")
	}
	all := viewer{f: scopefilter.Filter{Scope: rbac.ScopeAll}}
	if o := r.owner(all, ownerOrganization, 4, true); o.Masked {
		t.Fatal("scope all sees every organization (K20)")
	}
}
