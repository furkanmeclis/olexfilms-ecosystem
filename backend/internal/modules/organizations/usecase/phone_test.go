package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
)

// TEC-159 (K29): organization phones are written in E.164, parsed with the
// organization's country.
func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		raw, iso2, want string
	}{
		{"", "TR", ""},
		{"0212 555 12 34", "TR", "+902125551234"},
		{"0212 555 12 34", "", "+902125551234"},
		{"030 1234567", "DE", "+49301234567"},
		{"+31 20 123 4567", "TR", "+31201234567"},
	}
	for _, c := range cases {
		got, err := normalizePhone(c.raw, c.iso2)
		if err != nil || got != c.want {
			t.Fatalf("normalizePhone(%q, %q) = %q, %v; want %q", c.raw, c.iso2, got, err, c.want)
		}
	}
	for _, bad := range []string{"12", "not a phone", "+0 123"} {
		if _, err := normalizePhone(bad, "TR"); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("normalizePhone(%q) err = %v, want ErrInvalidRequest", bad, err)
		}
	}
	if got := addressISO2(nil); got != "" {
		t.Fatalf("addressISO2(nil) = %q", got)
	}
	if got := addressISO2(&geo.Address{Country: db.Country{Iso2: "NL"}}); got != "NL" {
		t.Fatalf("addressISO2(NL) = %q", got)
	}
}

// The letterhead write path parses with the stored organization country and
// rejects an invalid number (database test, rolled back).
func TestPatchTenantSettingsPhoneE164(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	svc := New(pool, q)

	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE organizations SET country_id = (SELECT id FROM countries WHERE iso2 = 'DE'),
		province_id = NULL, district_id = NULL WHERE id = $1`, center.ID); err != nil {
		t.Fatal(err)
	}
	raw := "030 1234567"
	got, err := svc.PatchTenantSettings(ctx, center.ID, LetterheadPatch{Phone: &raw})
	if err != nil {
		t.Fatalf("PatchTenantSettings: %v", err)
	}
	if got.Phone != "+49301234567" {
		t.Fatalf("phone = %q, want +49301234567", got.Phone)
	}
	bad := "call reception"
	if _, err := svc.PatchTenantSettings(ctx, center.ID, LetterheadPatch{Phone: &bad}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid phone err = %v, want ErrInvalidRequest", err)
	}
}
