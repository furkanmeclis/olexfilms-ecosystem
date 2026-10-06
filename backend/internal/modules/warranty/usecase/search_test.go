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

// TEC-209: the warranties index filter mirrors the list scope.
func TestWarrantiesIndexFilter(t *testing.T) {
	i8 := func(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
	cases := []struct {
		name string
		p    db.ListWarrantyRowsParams
		want string
		ok   bool
	}{
		{"brand", db.ListWarrantyRowsParams{BrandID: 2}, "brand_ids = 2", true},
		{"subtree own", db.ListWarrantyRowsParams{BrandID: 2, OrgIds: []int64{5, 6}, ServiceCreatedBy: i8(9)},
			"brand_ids = 2 AND organization_ids IN [5, 6] AND created_by_user_id = 9", true},
		{"portal holder", db.ListWarrantyRowsParams{BrandID: 2, HolderUserID: i8(3), Statuses: []string{"active", "expired"}},
			`brand_ids = 2 AND customer_user_id = 3 AND status IN ["active", "expired"]`, true},
		{"product + vehicle", db.ListWarrantyRowsParams{BrandID: 2, ProductID: i8(7), VehicleID: i8(8)},
			"brand_ids = 2 AND product_id = 7 AND vehicle_id = 8", true},
		{"empty org list", db.ListWarrantyRowsParams{BrandID: 2, OrgIds: []int64{}}, "", false},
		{"no brand", db.ListWarrantyRowsParams{}, "", false},
	}
	for _, tc := range cases {
		got, ok := indexFilter(tc.p)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeWarrantyIndex struct {
	row db.GetWarrantyForIndexRow
	err error
}

func (f fakeWarrantyIndex) ListWarrantiesForIndex(context.Context) ([]db.ListWarrantiesForIndexRow, error) {
	return []db.ListWarrantiesForIndexRow{db.ListWarrantiesForIndexRow(f.row)}, f.err
}

func (f fakeWarrantyIndex) GetWarrantyForIndex(context.Context, uuid.UUID) (db.GetWarrantyForIndexRow, error) {
	return f.row, f.err
}

func TestWarrantiesSearchAdapter(t *testing.T) {
	id := uuid.New()
	row := db.GetWarrantyForIndexRow{
		Uuid: id, PublicCode: "OLX-ABCD-1234", OrganizationID: 5, BrandID: 2, Status: "active",
		HolderUserID: 3, VehicleID: 4, ProductID: 7, ServiceNo: "SRV-1", ServiceCreatedBy: pgtype.Int8{Int64: 9, Valid: true},
		VehiclePlate: pgtype.Text{String: "34 ABC 123", Valid: true}, VehiclePlateNormalized: pgtype.Text{String: "34ABC123", Valid: true},
		VehicleVin: pgtype.Text{String: "WVWZZZ1JZXW000001", Valid: true}, ProductSku: "PPF-1", ProductName: "PPF Gloss",
		HolderStatus: "active", HolderName: "Ayse", HolderSurname: "Kaya", HolderPhone: pgtype.Text{String: "+905551112233", Valid: true},
	}
	a := NewSearchAdapter(fakeWarrantyIndex{row: row})
	if s := a.Spec(); !s.ListScoped || s.ID != searchengine.SpecWarranties {
		t.Fatalf("spec = %+v", s)
	}
	doc, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "OLX-ABCD-1234" || doc.CustomerUserID != 3 || doc.CreatedByUserID != 9 || doc.ProductID != 7 || doc.Status != "active" {
		t.Fatalf("doc = %+v", doc)
	}
	for _, k := range []string{"OLX-ABCD-1234", "SRV-1", "34ABC123", "WVWZZZ1JZXW000001", "Ayse Kaya", "905551112233"} {
		if !slices.Contains(doc.Keywords, k) {
			t.Errorf("keyword %q missing in %v", k, doc.Keywords)
		}
	}
	row.HolderStatus = "anonymized"
	doc, _ = NewSearchAdapter(fakeWarrantyIndex{row: row}).Document(context.Background(), id.String())
	for _, k := range doc.Keywords {
		if k == "Ayse Kaya" || k == "34ABC123" || k == "+905551112233" || k == "WVWZZZ1JZXW000001" {
			t.Fatalf("anonymized holder leaks %q: %v", k, doc.Keywords)
		}
	}
	if !slices.Contains(doc.Keywords, "OLX-ABCD-1234") {
		t.Fatalf("public code must stay searchable: %v", doc.Keywords)
	}
	if _, err := NewSearchAdapter(fakeWarrantyIndex{err: pgx.ErrNoRows}).Document(context.Background(), id.String()); !errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("missing warranty err = %v", err)
	}
}
