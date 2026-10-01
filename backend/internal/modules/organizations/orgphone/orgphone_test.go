package orgphone

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		raw, country, want string
		err                bool
	}{
		{"", "TR", "", false},
		{"   ", "", "", false},
		{"0212 555 12 34", "TR", "+902125551234", false},
		{"(0532) 123 45 67", "", "+905321234567", false}, // empty country falls back to TR
		{"+90 532 123 45 67", "DE", "+905321234567", false},
		{"0049 30 1234567", "TR", "+49301234567", false},
		{"030 1234567", "DE", "+49301234567", false},
		{"020 123 4567", "NL", "+31201234567", false},
		{"12", "TR", "", true},
		{"call us", "TR", "", true},
	}
	for _, c := range cases {
		got, err := Normalize(c.raw, c.country)
		if c.err {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Normalize(%q, %q) err = %v, want ErrInvalid", c.raw, c.country, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("Normalize(%q, %q) = %q, %v; want %q", c.raw, c.country, got, err, c.want)
		}
	}
}

type fakeStore struct {
	rows     []db.ListOrganizationsWithRawPhoneRow
	resolved map[int64]string
}

func (f *fakeStore) ListOrganizationsWithRawPhone(context.Context) ([]db.ListOrganizationsWithRawPhoneRow, error) {
	var out []db.ListOrganizationsWithRawPhoneRow
	for _, r := range f.rows {
		if _, done := f.resolved[r.ID]; !done {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) ResolveOrganizationRawPhone(_ context.Context, arg db.ResolveOrganizationRawPhoneParams) (int64, error) {
	f.resolved[arg.ID] = arg.PhoneE164
	return 1, nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func TestRunDryRunAndIdempotent(t *testing.T) {
	store := &fakeStore{resolved: map[int64]string{}, rows: []db.ListOrganizationsWithRawPhoneRow{
		{ID: 1, Name: "tr", PhoneRaw: text("0212 555 12 34"), CountryIso2: text("TR")},
		{ID: 2, Name: "de", PhoneRaw: text("030 1234567"), CountryIso2: text("DE")},
		{ID: 3, Name: "bad", PhoneRaw: text("ask reception"), CountryIso2: text("TR")},
		{ID: 4, Name: "api", Phone: "+905321234567", PhoneRaw: text("0532"), CountryIso2: text("TR")},
	}}
	ctx := context.Background()

	dry, err := Run(ctx, store, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.resolved) != 0 {
		t.Fatalf("dry run wrote %v", store.resolved)
	}
	if dry.Scanned != 4 || len(dry.Converted) != 2 || len(dry.Superseded) != 1 || len(dry.Unresolved) != 1 {
		t.Fatalf("dry report = %+v", dry)
	}

	first, err := Run(ctx, store, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Converted) != 2 || store.resolved[1] != "+902125551234" || store.resolved[2] != "+49301234567" {
		t.Fatalf("first run = %+v, resolved %v", first, store.resolved)
	}
	if _, ok := store.resolved[3]; ok {
		t.Fatal("unparseable phone must stay in phone_raw")
	}
	if _, ok := store.resolved[4]; !ok {
		t.Fatal("superseded row must have phone_raw cleared")
	}

	second, err := Run(ctx, store, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Scanned != 1 || len(second.Converted) != 0 || len(second.Unresolved) != 1 || second.Unresolved[0].Raw != "ask reception" {
		t.Fatalf("second run = %+v", second)
	}
}
