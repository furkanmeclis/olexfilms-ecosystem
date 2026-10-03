package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-209: the vehicles index filter pins the brand and the organizations
// of an organization relative scope (customer_organizations).
func TestVehicleIndexFilter(t *testing.T) {
	cases := []struct {
		name string
		c    Caller
		user pgtype.Int8
		want string
		ok   bool
	}{
		{"brand", scopedCaller(rbac.ScopeBrand, nil), pgtype.Int8{}, "brand_ids = 2", true},
		{"subtree + owner", scopedCaller(rbac.ScopeSubtree, []int64{5, 7}), pgtype.Int8{Int64: 3, Valid: true},
			"brand_ids = 2 AND organization_ids IN [5, 7] AND customer_user_id = 3", true},
		{"customer scope", scopedCaller(rbac.ScopeCustomer, nil), pgtype.Int8{}, "", false},
		{"no brand", Caller{}, pgtype.Int8{}, "", false},
	}
	for _, tc := range cases {
		got, ok := vehicleIndexFilter(tc.c, tc.user)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeVehicleIndex struct {
	row db.GetVehicleForIndexRow
	err error
}

func (f fakeVehicleIndex) ListVehiclesForIndex(context.Context) ([]db.ListVehiclesForIndexRow, error) {
	return []db.ListVehiclesForIndexRow{db.ListVehiclesForIndexRow(f.row)}, f.err
}

func (f fakeVehicleIndex) GetVehicleForIndex(context.Context, uuid.UUID) (db.GetVehicleForIndexRow, error) {
	return f.row, f.err
}

func TestVehicleSearchAdapter(t *testing.T) {
	id := uuid.New()
	row := db.GetVehicleForIndexRow{
		Uuid: id, UserID: 3, BrandID: 2, Plate: pgtype.Text{String: "34 ABC 123", Valid: true},
		PlateNormalized: pgtype.Text{String: "34ABC123", Valid: true}, Vin: pgtype.Text{String: "WVWZZZ1JZXW000001", Valid: true},
		ModelYear: pgtype.Int2{Int16: 2020, Valid: true}, CarBrandName: pgtype.Text{String: "VW", Valid: true},
		CarModelName: pgtype.Text{String: "Golf", Valid: true}, OwnerName: "Ali", OwnerSurname: "Can",
		OwnerPhone: pgtype.Text{String: "+905550000001", Valid: true}, OrganizationIds: []int64{5, 6},
	}
	a := NewVehicleSearchAdapter(fakeVehicleIndex{row: row})
	if s := a.Spec(); !s.ListScoped || s.ID != searchengine.SpecVehicles {
		t.Fatalf("spec = %+v", s)
	}
	doc, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "34 ABC 123" || doc.Subtitle != "VW Golf 2020 · Ali Can" || doc.CustomerUserID != 3 ||
		!slices.Equal(doc.OrganizationIDs, []int64{5, 6}) || !slices.Equal(doc.BrandIDs, []int64{2}) {
		t.Fatalf("doc = %+v", doc)
	}
	for _, k := range []string{"34ABC123", "WVWZZZ1JZXW000001", "905550000001"} {
		if !slices.Contains(doc.Keywords, k) {
			t.Errorf("keyword %q missing in %v", k, doc.Keywords)
		}
	}
	// Deleted vehicles and anonymized owners are not returned: skip.
	if _, err := NewVehicleSearchAdapter(fakeVehicleIndex{err: pgx.ErrNoRows}).Document(context.Background(), id.String()); !errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("err = %v", err)
	}
}

// A customer change refreshes the customer's record documents too.
func TestIndexCustomerRecordsNoStore(t *testing.T) {
	rec := &recIndexer{}
	s := &Service{}
	s.SetSearchIndexer(rec)
	s.indexCustomerRecords(context.Background(), uuid.New()) // no queries: no panic, nothing
	if len(rec.upserts) != 0 {
		t.Fatalf("upserts = %v", rec.upserts)
	}
}
