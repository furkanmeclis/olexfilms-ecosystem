package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func scopedCaller(scope rbac.Scope, orgIDs []int64) Caller {
	return Caller{
		UserID: 9,
		Org:    orgctx.Scope{InternalID: 5, Name: "Tech Oto", BrandID: 2},
		Filter: scopefilter.Filter{Scope: scope, OrgIDs: orgIDs, BrandID: 2},
	}
}

// TEC-164: the index filter always pins the domain brand (K20) and adds the
// organizations of an organization relative scope (customer_organizations).
func TestIndexFilter(t *testing.T) {
	cases := []struct {
		name   string
		c      Caller
		status []string
		want   string
		ok     bool
	}{
		{"brand", scopedCaller(rbac.ScopeBrand, nil), nil, "brand_ids = 2", true},
		{"all still brand", scopedCaller(rbac.ScopeAll, nil), nil, "brand_ids = 2", true},
		{"subtree", scopedCaller(rbac.ScopeSubtree, []int64{5, 7, 8}), nil, "brand_ids = 2 AND organization_ids IN [5, 7, 8]", true},
		{"managed with status", scopedCaller(rbac.ScopeManaged, []int64{5}), []string{"active"},
			`brand_ids = 2 AND organization_ids IN [5] AND status = "active"`, true},
		{"several statuses", scopedCaller(rbac.ScopeBrand, nil), []string{"active", "pending"},
			`brand_ids = 2 AND status IN ["active", "pending"]`, true},
		{"customer scope", scopedCaller(rbac.ScopeCustomer, nil), nil, "", false},
		{"no brand", Caller{Filter: scopefilter.Filter{Scope: rbac.ScopeBrand}}, nil, "", false},
	}
	for _, tc := range cases {
		got, ok := indexFilter(tc.c, tc.status)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeIndexStore struct {
	row db.GetCustomerForIndexRow
	err error
}

func (f fakeIndexStore) ListCustomersForIndex(context.Context) ([]db.ListCustomersForIndexRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []db.ListCustomersForIndexRow{db.ListCustomersForIndexRow(f.row)}, nil
}

func (f fakeIndexStore) GetCustomerForIndex(context.Context, uuid.UUID) (db.GetCustomerForIndexRow, error) {
	return f.row, f.err
}

func TestSearchAdapterDocument(t *testing.T) {
	id := uuid.New()
	row := db.GetCustomerForIndexRow{
		Uuid: id, Name: "Ahmet", Surname: "Yilmaz", Status: StatusActive,
		PhoneE164:       pgtype.Text{String: "+905551234567", Valid: true},
		Email:           pgtype.Text{String: "a@example.com", Valid: true},
		CompanyName:     pgtype.Text{String: "Yilmaz Ltd", Valid: true},
		OrganizationIds: []int64{5, 7}, BrandIds: []int64{2},
	}
	a := NewSearchAdapter(fakeIndexStore{row: row})
	spec := a.Spec()
	if !spec.ListScoped || spec.BrandScoped || spec.TenantScoped {
		t.Fatalf("spec scope flags = %+v", spec)
	}
	doc, err := a.Document(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != id.String() || doc.Title != "Ahmet Yilmaz" || doc.Subtitle != "+905551234567 · Yilmaz Ltd" {
		t.Fatalf("doc = %+v", doc)
	}
	if len(doc.OrganizationIDs) != 2 || len(doc.BrandIDs) != 1 || doc.BrandIDs[0] != 2 || doc.Status != StatusActive {
		t.Fatalf("filter fields = %+v", doc)
	}
	want := map[string]bool{"+905551234567": false, "905551234567": false, "a@example.com": false, "Yilmaz Ltd": false}
	for _, k := range doc.Keywords {
		if _, ok := want[k]; ok {
			want[k] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("keyword %q missing in %v", k, doc.Keywords)
		}
	}
	docs, err := a.ListAll(context.Background())
	if err != nil || len(docs) != 1 || docs[0].ID != id.String() {
		t.Fatalf("ListAll = %v, %v", docs, err)
	}
}

// Anonymized, merged or unlinked customers are not returned by the index
// query: the adapter answers ErrSkipDocument so the indexer deletes them.
func TestSearchAdapterSkipsNonIndexable(t *testing.T) {
	a := NewSearchAdapter(fakeIndexStore{err: pgx.ErrNoRows})
	if _, err := a.Document(context.Background(), uuid.NewString()); !errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("err = %v, want ErrSkipDocument", err)
	}
	if _, err := a.Document(context.Background(), "nope"); err == nil || errors.Is(err, searchengine.ErrSkipDocument) {
		t.Fatalf("invalid uuid err = %v", err)
	}
}

type recIndexer struct{ upserts, deletes []string }

func (r *recIndexer) EnqueueUpsert(_ context.Context, spec, id string) {
	r.upserts = append(r.upserts, spec+"/"+id)
}

func (r *recIndexer) EnqueueDelete(_ context.Context, spec, id string) {
	r.deletes = append(r.deletes, spec+"/"+id)
}

func TestIndexCustomer(t *testing.T) {
	rec := &recIndexer{}
	s := &Service{}
	s.indexCustomer(context.Background(), uuid.New()) // no indexer: no panic
	s.SetSearchIndexer(rec)
	id := uuid.New()
	s.indexCustomer(context.Background(), id)
	s.indexCustomer(context.Background(), uuid.Nil)
	if len(rec.upserts) != 1 || rec.upserts[0] != SearchSpec+"/"+id.String() {
		t.Fatalf("upserts = %v", rec.upserts)
	}
}

func TestExportScope(t *testing.T) {
	if got, err := ExportScope(scopedCaller(rbac.ScopeBrand, nil)); err != nil || got != ScopeBrand {
		t.Fatalf("brand: %q %v", got, err)
	}
	if got, err := ExportScope(scopedCaller(rbac.ScopeSubtree, []int64{5, 7})); err != nil || got != "5,7" {
		t.Fatalf("subtree: %q %v", got, err)
	}
	if _, err := ExportScope(scopedCaller(rbac.ScopeCustomer, nil)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("customer scope: %v", err)
	}
}

func TestListExportRow(t *testing.T) {
	phone := "+905551234567"
	row := listExportRow(CustomerSummary{
		Name: "Ahmet", Surname: "Yilmaz", Phone: &phone, Status: StatusActive, Type: TypeIndividual,
		CreatedAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC),
	}, i18n.Locale("en"))
	if row["name"] != "Ahmet Yilmaz" || row["phone"] != phone || row["email"] != "" || row["created_at"] != "2026-10-02" {
		t.Fatalf("row = %v", row)
	}
	if row["status"] != "Active" {
		t.Fatalf("status label = %v", row["status"])
	}
}

// TEC-164: customer.created carries the recipient, brand and portal link,
// never the phone; customers without a phone or with WhatsApp off get none.
func TestCreatedEventAndWelcomeWanted(t *testing.T) {
	user := db.User{ID: 42, Uuid: uuid.New(), Name: "Ahmet", Surname: "Yilmaz",
		PhoneE164: pgtype.Text{String: "+905551234567", Valid: true}}
	ev := CreatedEvent(scopedCaller(rbac.ScopeManaged, []int64{5}), user, "https://olexfilms.app/portal")
	if ev.Name != events.CustomerCreated || ev.TenantID == nil || *ev.TenantID != 5 {
		t.Fatalf("event = %+v", ev)
	}
	p := ev.Payload
	if p["customer_user_id"] != int64(42) || p["brand_id"] != int64(2) || p["portal_url"] != "https://olexfilms.app/portal" ||
		p["organization_name"] != "Tech Oto" || p["customer_name"] != "Ahmet Yilmaz" || p["has_phone"] != true {
		t.Fatalf("payload = %v", p)
	}
	for k, v := range p {
		if v == user.PhoneE164.String {
			t.Fatalf("payload %s leaks the phone", k)
		}
	}
	if !welcomeWanted(user, profilePatch{}) {
		t.Fatal("phone, default prefs: want welcome")
	}
	if welcomeWanted(db.User{ID: 1}, profilePatch{}) {
		t.Fatal("no phone: want no welcome")
	}
	off := profilePatch{Prefs: Of(map[string]bool{"whatsapp": false})}
	if welcomeWanted(user, off) {
		t.Fatal("whatsapp off: want no welcome")
	}
	s := &Service{}
	s.SetPortalURL("https://olexfilms.app/")
	if s.PortalURL() != "https://olexfilms.app/portal" {
		t.Fatalf("portal = %q", s.PortalURL())
	}
	s.SetPortalURL(" ")
	if s.PortalURL() != "" {
		t.Fatalf("empty portal = %q", s.PortalURL())
	}
}
