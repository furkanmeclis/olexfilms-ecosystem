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
)

// TEC-210: the orders index filter pins the domain brand (K20) and mirrors
// the side / scope of the SQL list.
func TestOrdersIndexFilter(t *testing.T) {
	cases := []struct {
		name string
		sc   orderScope
		want string
		ok   bool
	}{
		{"brand scope", orderScope{brand: 2}, "brand_ids = 2", true},
		{"subtree", orderScope{brand: 2, orgIDs: []int64{5, 7}, statuses: []string{"shipped"}}, `brand_ids = 2 AND organization_ids IN [5, 7] AND status IN ["shipped"]`, true},
		{"statuses", orderScope{brand: 2, statuses: []string{"draft", "shipped"}}, `brand_ids = 2 AND status IN ["draft", "shipped"]`, true},
		{"seller", orderScope{side: SideSeller, brand: 2, orgID: 5, orgIDs: []int64{5}}, "brand_ids = 2 AND seller_org_id = 5", true},
		{"buyer", orderScope{side: SideBuyer, brand: 2, orgID: 5}, "brand_ids = 2 AND buyer_org_id = 5", true},
		{"empty reach", orderScope{brand: 2, orgIDs: []int64{}}, "", false},
		{"no brand", orderScope{}, "", false},
	}
	for _, tc := range cases {
		got, ok := indexFilter(tc.sc)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeOrderIndex struct {
	row db.GetOrderForIndexRow
	err error
}

func (f fakeOrderIndex) ListOrdersForIndex(context.Context) ([]db.ListOrdersForIndexRow, error) {
	return []db.ListOrdersForIndexRow{db.ListOrdersForIndexRow(f.row)}, f.err
}

func (f fakeOrderIndex) GetOrderForIndex(context.Context, uuid.UUID) (db.GetOrderForIndexRow, error) {
	return f.row, f.err
}

func TestOrdersSearchAdapter(t *testing.T) {
	id := uuid.New()
	a := NewSearchAdapter(fakeOrderIndex{row: db.GetOrderForIndexRow{
		Uuid: id, OrderNo: "ORD-1", OrganizationID: 3, BuyerOrgID: 9, BrandID: 2, Status: "approved",
		SellerName: "Merkez", SellerSlug: "olex", BuyerName: "Bayi", BuyerSlug: "bayi-9",
	}})
	d, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "ORD-1" || !slices.Contains(d.Keywords, "bayi-9") || !slices.Equal(d.OrganizationIDs, []int64{3, 9}) ||
		d.SellerOrgID != 3 || d.BuyerOrgID != 9 || d.Status != "approved" {
		t.Fatalf("doc = %+v", d)
	}
	gone := NewSearchAdapter(fakeOrderIndex{err: pgx.ErrNoRows})
	if _, err := gone.Document(context.Background(), id.String()); !errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("missing order err = %v", err)
	}
}
