package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-210: the index filter pins the request brand and mirrors the SQL
// scope (organization id set of a managed / subtree reach, type filter).
// TEC-473: without a type filter, fleets are excluded.
func TestOrganizationsIndexFilter(t *testing.T) {
	cases := []struct {
		name  string
		brand int64
		p     db.ListOrganizationsInScopeParams
		want  string
		ok    bool
	}{
		// TEC-473: fleets share the index; the tree list leaves them out.
		{"brand scope", 2, db.ListOrganizationsInScopeParams{},
			`brand_ids = 2 AND org_type IN ["center", "distributor", "dealer"]`, true},
		{"subtree + type", 2, db.ListOrganizationsInScopeParams{OrgIds: []int64{5, 7}, Type: pgtype.Text{String: "dealer", Valid: true}},
			`brand_ids = 2 AND organization_ids IN [5, 7] AND org_type = "dealer"`, true},
		{"empty reach", 2, db.ListOrganizationsInScopeParams{OrgIds: []int64{}}, "", false},
		{"no brand", 0, db.ListOrganizationsInScopeParams{}, "", false},
	}
	for _, tc := range cases {
		got, ok := indexFilter(tc.brand, tc.p)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeOrgIndex struct {
	row db.GetOrganizationForIndexRow
	err error
}

func (f fakeOrgIndex) ListOrganizationsForIndex(context.Context) ([]db.ListOrganizationsForIndexRow, error) {
	return []db.ListOrganizationsForIndexRow{db.ListOrganizationsForIndexRow(f.row)}, f.err
}

func (f fakeOrgIndex) GetOrganizationForIndex(context.Context, uuid.UUID) (db.GetOrganizationForIndexRow, error) {
	return f.row, f.err
}

func TestOrganizationsSearchAdapter(t *testing.T) {
	id := uuid.New()
	a := NewSearchAdapter(fakeOrgIndex{row: db.GetOrganizationForIndexRow{
		Uuid: id, ID: 11, Slug: "kadikoy-oto", Name: "Kadıköy Oto", Type: "dealer", Status: "active", BrandID: 2,
		City: "İstanbul", Phone: "+905551112233", ParentName: pgtype.Text{String: "Marmara Dist", Valid: true},
	}})
	d, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Kadıköy Oto" || !slices.Contains(d.Keywords, "kadikoy-oto") || !slices.Contains(d.Keywords, "905551112233") ||
		!slices.Equal(d.OrganizationIDs, []int64{11}) || !slices.Equal(d.BrandIDs, []int64{2}) || d.OrgType != "dealer" {
		t.Fatalf("doc = %+v", d)
	}
	if !a.Spec().ListScoped {
		t.Fatal("organizations spec must be list scoped")
	}
	gone := NewSearchAdapter(fakeOrgIndex{err: pgx.ErrNoRows})
	if _, err := gone.Document(context.Background(), id.String()); !errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("missing org err = %v", err)
	}
}

// TEC-473: a fleet document is typed fleet and reachable only through the
// dealers with an active link (a dealer's search finds only its fleets).
func TestOrganizationsSearchAdapterFleet(t *testing.T) {
	id := uuid.New()
	a := NewSearchAdapter(fakeOrgIndex{row: db.GetOrganizationForIndexRow{
		Uuid: id, ID: 40, Slug: "fleet-2-1234567890", Name: "Filo A", Type: "fleet", Status: "active", BrandID: 2,
		LinkedOrgIds: []int64{11, 12},
	}})
	d, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if d.OrgType != "fleet" || !slices.Equal(d.OrganizationIDs, []int64{11, 12}) || d.Href != "/fleets/"+id.String() {
		t.Fatalf("fleet doc = %+v", d)
	}
	lone := NewSearchAdapter(fakeOrgIndex{row: db.GetOrganizationForIndexRow{Uuid: id, ID: 41, Type: "fleet", BrandID: 2}})
	if d, _ := lone.Document(context.Background(), id.String()); d.OrganizationIDs == nil || len(d.OrganizationIDs) != 0 {
		t.Fatalf("unlinked fleet doc ids = %v", d.OrganizationIDs)
	}
}
