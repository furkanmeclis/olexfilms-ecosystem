package ioengine

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

func TestLetterheadFromOrganization(t *testing.T) {
	org := db.Organization{
		Name:         "  Wash Co  ",
		Address:      "Atatürk Cad. 12",
		District:     "Kadıköy",
		City:         "İstanbul",
		Phone:        "0216 000 00 00",
		Email:        "hello@wash.co",
		Website:      "https://wash.co",
		Tagline:      "Clean cars",
		FooterText:   "Thanks",
		PrimaryColor: "#445566",
		PaperSize:    "Letter",
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
