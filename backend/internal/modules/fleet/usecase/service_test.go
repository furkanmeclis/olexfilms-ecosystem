package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// noDB fails the test on any database use: input rules must be decided
// before a query runs.
type noDB struct{ t *testing.T }

func (n noDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	n.t.Error("unexpected Exec")
	return pgconn.CommandTag{}, errors.New("no db")
}

func (n noDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	n.t.Error("unexpected Query")
	return nil, errors.New("no db")
}

func (n noDB) QueryRow(context.Context, string, ...any) pgx.Row {
	n.t.Error("unexpected QueryRow")
	return nil
}

func (n noDB) Begin(context.Context) (pgx.Tx, error) {
	n.t.Error("unexpected Begin")
	return nil, errors.New("no db")
}

var dealerOrg = db.Organization{ID: 1, Type: "dealer", BrandID: 1, Currency: "TRY", Timezone: "Europe/Istanbul"}

// Acceptance: a VKN/TCKN with a wrong checksum is a 422 rule error from the
// usecase, decided without touching the database.
func TestCreateFleetInvalidTaxNumber(t *testing.T) {
	svc := New(noDB{t})
	for _, tax := range []string{"1089325651", "10000000147", "00000000146", "12345"} {
		_, err := svc.CreateFleet(context.Background(), CreateFleetInput{
			Opener: dealerOrg, LegalName: "Filo A.Ş.", TaxNumber: tax,
		})
		var re *RuleError
		if !errors.Is(err, ErrInvalidTaxNumber) || !errors.As(err, &re) {
			t.Fatalf("%s: err = %v, want ErrInvalidTaxNumber", tax, err)
		}
		if re.Status() != http.StatusUnprocessableEntity || re.Code != model.CodeInvalidTaxNumber {
			t.Fatalf("%s: rule = %d %s", tax, re.Status(), re.Code)
		}
	}
}

// Other bad fields are 400 validation errors, also without a query.
func TestCreateFleetValidation(t *testing.T) {
	svc := New(noDB{t})
	cases := map[string]CreateFleetInput{
		"tax_number":       {LegalName: "x"},
		"legal_name":       {TaxNumber: "1089325650", LegalName: "  "},
		"contact_phone":    {TaxNumber: "1089325650", LegalName: "x", ContactPhone: "12"},
		"billing_email":    {TaxNumber: "1089325650", LegalName: "x", BillingEmail: "not-an-email"},
		"report_frequency": {TaxNumber: "1089325650", LegalName: "x", ReportFrequency: "weekly"},
		"report_locale":    {TaxNumber: "1089325650", LegalName: "x", ReportLocale: "xx"},
	}
	for field, in := range cases {
		in.Opener = dealerOrg
		_, err := svc.CreateFleet(context.Background(), in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != field {
			t.Fatalf("%s: err = %v", field, err)
		}
	}
	if _, err := svc.CreateFleet(context.Background(), CreateFleetInput{
		Opener: db.Organization{Type: "center"}, TaxNumber: "1089325650", LegalName: "x",
	}); !errors.Is(err, ErrNotLinkable) {
		t.Fatalf("center opener: err = %v", err)
	}
}

// Opening a fleet creates the fleet organization outside the tree, its
// profile with the defaults and the opener's active link; the same tax
// number in the brand is ErrFleetExists.
func TestCreateFleet(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	org := func(name, typ string, parent int64) db.Organization {
		o, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: "t472-uc-" + name + "-" + suffix, Name: name, Status: "active",
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true}, BrandID: brand.ID,
			Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("org %s: %v", name, err)
		}
		return o
	}
	dist := org("dist", "distributor", center.ID)
	dealer := org("dealer", "dealer", dist.ID)

	// The usecase transaction is a savepoint of the test transaction.
	svc := New(tx)
	// A tax number unique to this run (10 digits, valid checksum).
	tax := validVKN(t, suffix)
	got, err := svc.CreateFleet(ctx, CreateFleetInput{
		Opener: dealer, LegalName: "Test Filo A.Ş.", TaxNumber: tax,
		ContactPhone: "+90 532 000 00 00", BillingEmail: "fatura@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Organization.Type != "fleet" || got.Organization.ParentID.Valid || got.Organization.Name != "Test Filo A.Ş." {
		t.Fatalf("fleet org = %+v", got.Organization)
	}
	if got.Profile.ReportFrequency != model.ReportMonthly || got.Profile.ReportLocale != "tr" ||
		got.Profile.ContactPhone.String != "+905320000000" || got.Profile.TaxNumber != tax {
		t.Fatalf("profile = %+v", got.Profile)
	}
	if got.Link.Status != model.LinkActive || got.Link.DealerOrgID != dealer.ID || got.Link.CreatedByOrgID != dealer.ID {
		t.Fatalf("link = %+v", got.Link)
	}
	dist2 := org("dist2", "distributor", center.ID)
	if _, err := svc.CreateFleet(ctx, CreateFleetInput{Opener: dist2, LegalName: "Kopya", TaxNumber: tax}); !errors.Is(err, ErrFleetExists) {
		t.Fatalf("same tax number: err = %v, want ErrFleetExists", err)
	}
}

// validVKN derives a checksum-valid VKN from seed digits.
func validVKN(t *testing.T, seed string) string {
	t.Helper()
	base := seed[len(seed)-9:]
	for d := 0; d <= 9; d++ {
		if s := fmt.Sprintf("%s%d", base, d); model.ValidVKN(s) {
			return s
		}
	}
	t.Fatalf("no VKN for %s", base)
	return ""
}
