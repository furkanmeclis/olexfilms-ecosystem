package usecase

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func publicRow(now time.Time) db.GetPublicWarrantyByCodeRow {
	return db.GetPublicWarrantyByCodeRow{
		PublicCode: "AbCdEfGhIjKlMnOpQrSt_-", Status: "active",
		StartAt: ts(now.AddDate(0, -1, 0)), EndAt: ts(now.Add(36 * time.Hour)),
		ProductName: "Film X", OrganizationName: "Dealer A", OrganizationCity: "kadikoy",
		OrganizationProvince: "İstanbul",
		CarBrandUuid:         uuid.New(), CarBrandName: "BMW", CarBrandHasLogo: true, CarModelName: "M3",
		ServiceModelYear: pgtype.Int2{Int16: 2024, Valid: true},
		ServicePlate:     txt("34 ABC 112"), ServicePlateCountry: txt("TR"),
		ServiceVin:   txt("WVWZZZ1JZ3W386752"),
		VehiclePlate: txt("06 X 99"), VehiclePlateCountry: txt("TR"),
	}
}

func TestBuildPublicWarranty(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	w := BuildPublicWarranty(publicRow(now), PublicWarrantyBrand{Name: "Olex", Slug: "olex"}, now)
	if w.Status != "active" || w.DaysRemaining != 2 {
		t.Fatalf("status %s days %d", w.Status, w.DaysRemaining)
	}
	if w.Vehicle.PlateMasked == nil || *w.Vehicle.PlateMasked != "34 *** 12" {
		t.Fatalf("plate = %v", w.Vehicle.PlateMasked)
	}
	if w.Vehicle.VINLast4 == nil || *w.Vehicle.VINLast4 != "6752" {
		t.Fatalf("vin = %v", w.Vehicle.VINLast4)
	}
	if w.Dealer.City != "İstanbul" || w.Vehicle.BrandLogoUUID == nil || *w.Vehicle.ModelYear != 2024 {
		t.Fatalf("projection = %+v", w)
	}
}

func TestBuildPublicWarrantyStatus(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	row := publicRow(now)
	row.EndAt = ts(now.Add(-time.Minute)) // cron has not run yet
	if w := BuildPublicWarranty(row, PublicWarrantyBrand{}, now); w.Status != "expired" || w.DaysRemaining != 0 {
		t.Fatalf("past end: %s %d", w.Status, w.DaysRemaining)
	}
	row = publicRow(now)
	row.Status = "void"
	if w := BuildPublicWarranty(row, PublicWarrantyBrand{}, now); w.Status != "void" || w.DaysRemaining != 0 {
		t.Fatalf("void: %s %d", w.Status, w.DaysRemaining)
	}
	// No service plate: the vehicle plate is used; no VIN, no logo.
	row = publicRow(now)
	row.ServicePlate, row.ServiceVin, row.CarBrandHasLogo = pgtype.Text{}, pgtype.Text{}, false
	w := BuildPublicWarranty(row, PublicWarrantyBrand{}, now)
	if w.Vehicle.PlateMasked == nil || *w.Vehicle.PlateMasked != "06 *** 99" || w.Vehicle.VINLast4 != nil || w.Vehicle.BrandLogoUUID != nil {
		t.Fatalf("fallback = %+v", w.Vehicle)
	}
}

// The public JSON never carries personal data keys nor the clear plate/VIN.
func TestPublicWarrantyJSONHasNoPII(t *testing.T) {
	now := time.Now()
	b, err := json.Marshal(BuildPublicWarranty(publicRow(now), PublicWarrantyBrand{Name: "Olex", Slug: "olex"}, now))
	if err != nil {
		t.Fatal(err)
	}
	if keys := FindPIIKeys(b); len(keys) > 0 {
		t.Fatalf("personal data keys %v in %s", keys, b)
	}
	if keys := FindPIIKeys([]byte(`{"a":[{"Phone":"x"}],"vin":"y"}`)); len(keys) != 2 {
		t.Fatalf("FindPIIKeys missed keys: %v", keys)
	}
	for _, clear := range []string{"34ABC112", "34 ABC 112", "WVWZZZ1JZ3W386752"} {
		if strings.Contains(string(b), clear) {
			t.Fatalf("clear value %q in %s", clear, b)
		}
	}
}

func TestValidPublicCode(t *testing.T) {
	for code, want := range map[string]bool{
		"AbCdEfGhIjKlMnOpQrSt_-": true,
		"abc":                    false,
		"DS7K2M9QX4":             true, // old hub service number format
		"SRV-2025-00042":         true,
		"1234":                   true,
		"AbCdEfGhIjKl MnOpQrSt":  false,
		"AbCdEfGhIjKl+nOpQrSt/=": false,
		strings.Repeat("a", 33):  false,
	} {
		if got := ValidPublicCode(code); got != want {
			t.Errorf("ValidPublicCode(%q) = %v", code, got)
		}
	}
}

// TEC-248: the anonymous PDF is built from the public projection only; the
// full plate, full VIN and holder never reach the document.
func TestPublicCertificateHTMLHasNoPII(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	w := BuildPublicWarranty(publicRow(now), PublicWarrantyBrand{Name: "Olex", Slug: "olex"}, now)
	doc, err := PublicCertificateHTML(w, PublicVerifyURL("https://olexfilms.app/", w.PublicCode), time.UTC, "tr", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"34 ABC 112", "ABC", "WVWZZZ1JZ3W386752", "06 X 99"} {
		if strings.Contains(doc, banned) {
			t.Errorf("document carries %q", banned)
		}
	}
	for _, want := range []string{"34 *** 12", "6752", "Film X", "Dealer A", "https://olexfilms.app/garanti/" + w.PublicCode} {
		if !strings.Contains(doc, want) {
			t.Errorf("document lacks %q", want)
		}
	}
}
