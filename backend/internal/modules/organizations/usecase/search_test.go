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
func TestOrganizationsIndexFilter(t *testing.T) {
	cases := []struct {
		name  string
		brand int64
		p     db.ListOrganizationsInScopeParams
		want  string
		ok    bool
	}{
		{"brand scope", 2, db.ListOrganizationsInScopeParams{}, "brand_ids = 2", true},
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
