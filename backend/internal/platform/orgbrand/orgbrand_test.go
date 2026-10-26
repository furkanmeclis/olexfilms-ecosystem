package orgbrand

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func TestForCustomer(t *testing.T) {
	center := db.Organization{Type: "center", Name: "Olex Merkez", LogoObjectKey: text("org/logo.png"),
		ServicePointEnabled: true, ServicePointName: text("AFM Florya"), ServicePointLogoObjectKey: text("sp/logo.png")}
	got := ForCustomer(center)
	if got.Name != "AFM Florya" || got.LogoObjectKey.String != "sp/logo.png" {
		t.Fatalf("service point = %q %q", got.Name, got.LogoObjectKey.String)
	}
	noLogo := center
	noLogo.ServicePointLogoObjectKey = pgtype.Text{}
	if got := ForCustomer(noLogo); got.Name != "AFM Florya" || got.LogoObjectKey.String != "org/logo.png" {
		t.Fatalf("logo fallback = %q %q", got.Name, got.LogoObjectKey.String)
	}
	off := center
	off.ServicePointEnabled = false
	if Name(off) != "Olex Merkez" {
		t.Fatalf("disabled service point name = %q", Name(off))
	}
	dealer := center
	dealer.Type = "dealer"
	if Name(dealer) != "Olex Merkez" {
		t.Fatalf("a dealer never shows a service point: %q", Name(dealer))
	}
	if n, logo := Fields("center", "Olex Merkez", true, text("AFM Florya"), text("a"), pgtype.Text{}); n != "AFM Florya" || logo.String != "a" {
		t.Fatalf("Fields = %q %q", n, logo.String)
	}
}
