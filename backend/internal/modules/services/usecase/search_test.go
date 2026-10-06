package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-209: the index filter pins the domain brand (K20) and mirrors the
// SQL scope (organizations, own / assigned creator, customer, vehicle).
func TestServicesIndexFilter(t *testing.T) {
	c := Caller{Org: orgctx.Scope{BrandID: 2}}
	cases := []struct {
		name string
		c    Caller
		p    db.ListServicesInScopeParams
		want string
		ok   bool
	}{
		{"brand", c, db.ListServicesInScopeParams{}, "brand_ids = 2", true},
		{"subtree + status", c, db.ListServicesInScopeParams{OrgIds: []int64{5, 7}, Statuses: []string{"completed", "ready"}},
			`brand_ids = 2 AND organization_ids IN [5, 7] AND status IN ["completed", "ready"]`, true},
		{"own", c, db.ListServicesInScopeParams{OrgIds: []int64{5}, CreatedByUserID: pgtype.Int8{Int64: 9, Valid: true}},
			"brand_ids = 2 AND organization_ids IN [5] AND created_by_user_id = 9", true},
		{"customer + vehicle", c, db.ListServicesInScopeParams{CustomerUserID: pgtype.Int8{Int64: 3, Valid: true}, VehicleID: pgtype.Int8{Int64: 4, Valid: true}},
			"brand_ids = 2 AND customer_user_id = 3 AND vehicle_id = 4", true},
		{"empty org list", c, db.ListServicesInScopeParams{OrgIds: []int64{}}, "", false},
		{"no brand", Caller{}, db.ListServicesInScopeParams{}, "", false},
	}
	for _, tc := range cases {
		got, ok := indexFilter(tc.c, tc.p)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeServiceIndex struct {
	row db.GetServiceForIndexRow
	err error
}

func (f fakeServiceIndex) ListServicesForIndex(context.Context) ([]db.ListServicesForIndexRow, error) {
	return []db.ListServicesForIndexRow{db.ListServicesForIndexRow(f.row)}, f.err
}

func (f fakeServiceIndex) GetServiceForIndex(context.Context, uuid.UUID) (db.GetServiceForIndexRow, error) {
	return f.row, f.err
}

func TestServicesSearchAdapter(t *testing.T) {
	id := uuid.New()
	row := db.GetServiceForIndexRow{
		Uuid: id, ServiceNo: "SRV-1", OrganizationID: 5, BrandID: 2, Status: "completed",
		Plate: pgtype.Text{String: "34 ABC 123", Valid: true}, Vin: pgtype.Text{String: "WVWZZZ1JZXW000001", Valid: true},
		CustomerUserID: 3, VehicleID: 4, CreatedByUserID: pgtype.Int8{Int64: 9, Valid: true},
		CustomerStatus: "active", CustomerName: "Ahmet", CustomerSurname: "Yilmaz",
		CustomerPhone: pgtype.Text{String: "+905551234567", Valid: true}, CarBrandName: "VW", CarModelName: "Golf",
	}
	a := NewSearchAdapter(fakeServiceIndex{row: row})
	if s := a.Spec(); !s.ListScoped || s.ID != searchengine.SpecServices {
		t.Fatalf("spec = %+v", s)
	}
	doc, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "SRV-1" || doc.CreatedByUserID != 9 || doc.CustomerUserID != 3 || doc.VehicleID != 4 ||
		!slices.Equal(doc.OrganizationIDs, []int64{5}) || !slices.Equal(doc.BrandIDs, []int64{2}) {
		t.Fatalf("doc = %+v", doc)
	}
	for _, k := range []string{"34ABC123", "WVWZZZ1JZXW000001", "Ahmet Yilmaz", "905551234567"} {
		if !slices.Contains(doc.Keywords, k) {
			t.Errorf("keyword %q missing in %v", k, doc.Keywords)
		}
	}
	// K19: an anonymized customer's service keeps only its number.
	row.CustomerStatus = "anonymized"
	doc, err = NewSearchAdapter(fakeServiceIndex{row: row}).Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.Keywords, []string{"SRV-1"}) || doc.Subtitle != "" {
		t.Fatalf("anonymized doc leaks personal data: %+v", doc)
	}
	if _, err := NewSearchAdapter(fakeServiceIndex{err: pgx.ErrNoRows}).Document(context.Background(), id.String()); !errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("missing service err = %v", err)
	}
	docs, err := a.ListAll(context.Background())
	if err != nil || len(docs) != 1 || docs[0].ID != id.String() {
		t.Fatalf("ListAll = %v, %v", docs, err)
	}
}
