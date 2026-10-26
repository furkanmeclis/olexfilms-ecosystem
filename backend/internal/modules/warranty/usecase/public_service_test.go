package usecase

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func publicServiceRows(now time.Time) (db.GetPublicServiceRow, []db.ListPublicServiceWarrantiesRow) {
	head := db.GetPublicServiceRow{
		ServiceNo: "1EVMEEI9", Package: txt(" Full body "),
		CompletedAt:      pgtype.Timestamptz{Time: now.AddDate(0, -2, 0), Valid: true},
		CreatedAt:        pgtype.Timestamptz{Time: now.AddDate(0, -3, 0), Valid: true},
		OrganizationName: "AFM Florya", OrganizationCity: "bakirkoy", OrganizationProvince: "İstanbul",
		CarBrandUuid: uuid.New(), CarBrandName: "BMW", CarBrandHasLogo: true, CarModelName: "X2",
		ServiceModelYear: pgtype.Int2{Int16: 2024, Valid: true},
		ServicePlate:     txt("34 AYS 984"), ServicePlateCountry: txt("TR"),
		ServiceVin: txt("WBAYH9100P5X12345"),
	}
	rows := []db.ListPublicServiceWarrantiesRow{
		{PublicCode: "1EVMEEI9", Status: "active", StartAt: ts(now.AddDate(0, -2, 0)), EndAt: ts(now.AddDate(5, 0, 0)),
			ProductName: "Olex PPF Gloss", CategoryName: "PPF", AppliedParts: []byte(`["hood","roof"]`)},
		{PublicCode: "AbCdEfGhIjKlMnOpQrSt_-", Status: "active", StartAt: ts(now.AddDate(-3, 0, 0)), EndAt: ts(now.Add(-time.Hour)),
			ProductName: "Olex Ceramic", CategoryName: "Seramik", AppliedParts: []byte(`[]`)},
		{PublicCode: "VoidCodeVoidCodeVoid01", Status: "void", StartAt: ts(now.AddDate(0, -2, 0)), EndAt: ts(now.AddDate(1, 0, 0)),
			ProductName: "Olex Glass", AppliedParts: nil},
	}
	return head, rows
}

func TestBuildPublicService(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	head, rows := publicServiceRows(now)
	p := BuildPublicService(head, rows, PublicWarrantyBrand{Name: "Olex", Slug: "olex"}, now)
	if p.ServiceNo != "1EVMEEI9" || p.Package == nil || *p.Package != "Full body" || !p.ServiceDate.Equal(head.CompletedAt.Time) {
		t.Fatalf("head = %+v", p)
	}
	if p.Dealer.Name != "AFM Florya" || p.Dealer.City != "İstanbul" {
		t.Fatalf("dealer = %+v", p.Dealer)
	}
	if p.Vehicle.PlateMasked == nil || *p.Vehicle.PlateMasked == "34 AYS 984" || p.Vehicle.VINLast4 == nil || *p.Vehicle.VINLast4 != "2345" {
		t.Fatalf("vehicle = %+v", p.Vehicle)
	}
	if len(p.Warranties) != 3 {
		t.Fatalf("warranties = %d", len(p.Warranties))
	}
	w0, w1, w2 := p.Warranties[0], p.Warranties[1], p.Warranties[2]
	if w0.Status != "active" || w0.DaysRemaining <= 0 || strings.Join(w0.Parts, ",") != "hood,roof" || w0.Category != "PPF" {
		t.Fatalf("w0 = %+v", w0)
	}
	if w1.Status != "expired" || w1.DaysRemaining != 0 || w1.Parts == nil {
		t.Fatalf("w1 = %+v", w1)
	}
	if w2.Status != "void" || w2.Parts == nil || len(w2.Parts) != 0 {
		t.Fatalf("w2 = %+v", w2)
	}
	if !p.HasActive() {
		t.Fatal("HasActive = false")
	}
	doc, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if keys := FindPIIKeys(doc); len(keys) != 0 {
		t.Fatalf("PII keys on the wire: %v", keys)
	}
	if strings.Contains(string(doc), "34 AYS 984") || strings.Contains(string(doc), "WBAYH9100P5X12345") {
		t.Fatal("unmasked plate or VIN on the wire")
	}
}

func TestPublicServiceCertificateHTML(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	head, rows := publicServiceRows(now)
	for _, slug := range []string{"olex", "other"} {
		p := BuildPublicService(head, rows, PublicWarrantyBrand{Name: "Brand", Slug: slug}, now)
		doc, err := PublicServiceCertificateHTML(p, "https://olexfilms.app/warranty/1EVMEEI9", nil, i18n.Locale("tr"), now)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"1EVMEEI9", "Olex PPF Gloss", "Olex Ceramic", "Olex Glass", "AFM Florya",
			"https://olexfilms.app/warranty/1EVMEEI9", "Aktif", "Süresi doldu", "Geçersiz"} {
			if !strings.Contains(doc, want) {
				t.Errorf("%s: document misses %q", slug, want)
			}
		}
		if strings.Contains(doc, "34 AYS 984") || strings.Contains(doc, "WBAYH9100P5X12345") {
			t.Errorf("%s: unmasked plate or VIN", slug)
		}
	}
}
