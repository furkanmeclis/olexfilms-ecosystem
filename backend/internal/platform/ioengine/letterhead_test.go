package ioengine

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

func TestLetterheadFromOrganization(t *testing.T) {
	org := db.Organization{
		Name:             "  Wash Co  ",
		Address:          "Atatürk Cad. 12",
		District:         "Kadıköy",
		City:             "İstanbul",
		Phone:            "0216 000 00 00",
		Email:            "hello@wash.co",
		Website:          "https://wash.co",
		Tagline:          "Clean cars",
		FooterText:       "Thanks",
		PrimaryColor:     "#445566",
		PaperSize:        "Letter",
		InvoiceTaxOffice: pgtype.Text{String: " Kadıköy ", Valid: true},
		InvoiceVkn:       pgtype.Text{String: "1234567890", Valid: true},
		InvoiceTckn:      pgtype.Text{String: "11111111111", Valid: true},
	}
	layout := db.AppSetting{PrimaryColor: "#112233", PaperSize: "A4"}
	lh := LetterheadFromOrganization(org, layout)
	if lh.CompanyName != "Wash Co" {
		t.Fatalf("name = %q", lh.CompanyName)
	}
	if lh.Address != "Atatürk Cad. 12, Kadıköy, İstanbul" {
		t.Fatalf("address = %q", lh.Address)
	}
	if lh.Phone != "0216 000 00 00" {
		t.Fatalf("phone = %q", lh.Phone)
	}
	if lh.PrimaryColor != "#445566" {
		t.Fatalf("color = %q", lh.PrimaryColor)
	}
	if lh.PaperSize != "Letter" {
		t.Fatalf("paper = %q", lh.PaperSize)
	}
	if lh.Email != "hello@wash.co" || lh.Tagline != "Clean cars" || lh.Website != "https://wash.co" || lh.FooterText != "Thanks" {
		t.Fatalf("identity extras: %+v", lh)
	}
	// TEC-772: the footer carries the tax office and VKN, never the TCKN.
	if lh.TaxOffice != "Kadıköy" || lh.TaxNumber != "1234567890" {
		t.Fatalf("tax = %q / %q", lh.TaxOffice, lh.TaxNumber)
	}
}

func TestLetterheadFromOrganizationLayoutDefaults(t *testing.T) {
	lh := LetterheadFromOrganization(db.Organization{Name: "A"}, db.AppSetting{})
	if lh.PrimaryColor != "#0F172A" {
		t.Fatalf("default color = %q", lh.PrimaryColor)
	}
	if lh.PaperSize != "A4" {
		t.Fatalf("default paper = %q", lh.PaperSize)
	}
}

func TestLetterheadFooterHTMLFor(t *testing.T) {
	lh := &Letterhead{FooterText: "Thanks <3", Address: "Atatürk Cad. 12", Phone: "0216", Email: "a@b.co",
		Website: "wash.co", TaxOffice: "Kadıköy", TaxNumber: "1234567890"}
	tr := LetterheadFooterHTMLFor(lh, "tr")
	for _, want := range []string{`<footer class="doc-footer">`, "Thanks &lt;3", "Atatürk Cad. 12 · 0216 · a@b.co",
		"VD: Kadıköy · VKN: 1234567890", `<div class="doc-footer-end">wash.co</div>`} {
		if !strings.Contains(tr, want) {
			t.Errorf("tr footer missing %q:\n%s", want, tr)
		}
	}
	if en := LetterheadFooterHTMLFor(lh, "en"); !strings.Contains(en, "Tax office: Kadıköy · Tax no.: 1234567890") {
		t.Errorf("en footer labels: %s", en)
	}
	if LetterheadFooterHTML(lh) != tr {
		t.Error("LetterheadFooterHTML is not the Turkish footer")
	}
	if got := LetterheadFooterHTMLFor(&Letterhead{TaxNumber: "1"}, "de"); !strings.Contains(got, "Steuernr.: 1") || strings.Contains(got, "Finanzamt") {
		t.Errorf("number-only footer: %s", got)
	}
	if got := LetterheadFooterHTMLFor(&Letterhead{CompanyName: "Only name"}, "tr"); got != "" {
		t.Errorf("empty footer = %q", got)
	}
	if LetterheadFooterHTMLFor(nil, "tr") != "" {
		t.Error("nil letterhead rendered a footer")
	}
}
