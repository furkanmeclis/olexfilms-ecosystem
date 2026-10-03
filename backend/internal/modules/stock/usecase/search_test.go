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

// TEC-210: the unit index filter pins the holding organization and mirrors
// the SQL list (brand of a brand scope, product, stock-on-hand statuses).
func TestStockUnitsIndexFilter(t *testing.T) {
	cases := []struct {
		name string
		p    db.ListOrganizationStockUnitRowsParams
		want string
	}{
		{"on hand", db.ListOrganizationStockUnitRowsParams{OrganizationID: 4},
			`organization_ids IN [4] AND status IN ["available", "placed"]`},
		{"brand + product + status", db.ListOrganizationStockUnitRowsParams{
			OrganizationID: 4, BrandID: pgtype.Int8{Int64: 2, Valid: true}, ProductID: pgtype.Int8{Int64: 8, Valid: true},
			Status: pgtype.Text{String: "reserved", Valid: true},
		}, `organization_ids IN [4] AND brand_ids = 2 AND product_id = 8 AND status = "reserved"`},
	}
	for _, tc := range cases {
		if got := unitIndexFilter(tc.p); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

type fakeUnitIndex struct {
	row db.GetStockUnitForIndexRow
	err error
}

func (f fakeUnitIndex) ListStockUnitsForIndex(context.Context) ([]db.ListStockUnitsForIndexRow, error) {
	return []db.ListStockUnitsForIndexRow{db.ListStockUnitsForIndexRow(f.row)}, f.err
}

func (f fakeUnitIndex) GetStockUnitForIndex(context.Context, uuid.UUID) (db.GetStockUnitForIndexRow, error) {
	return f.row, f.err
}

func TestStockUnitsSearchAdapter(t *testing.T) {
	id := uuid.New()
	a := NewSearchAdapter(fakeUnitIndex{row: db.GetStockUnitForIndexRow{
		Uuid: id, Barcode: "OLX000123", BrandID: 2, Status: "placed", ProductID: 8, Sku: "PPF-1", ProductName: "PPF Gloss",
		HolderOrgIds: []int64{4}, LocationCodes: []string{"A-01-R2"},
	}})
	d, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "OLX000123" || !slices.Contains(d.Keywords, "A-01-R2") || !slices.Contains(d.Keywords, "PPF-1") ||
		!slices.Equal(d.OrganizationIDs, []int64{4}) || d.ProductID != 8 || d.Status != "placed" {
		t.Fatalf("doc = %+v", d)
	}
	gone := NewSearchAdapter(fakeUnitIndex{err: pgx.ErrNoRows})
	if _, err := gone.Document(context.Background(), id.String()); !errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("missing unit err = %v", err)
	}
}
