package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	repo   *repository.Repository
	brand  db.Brand
	center db.Organization
	seq    int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	return &fixture{ctx: ctx, pool: pool, q: q, repo: repository.New(pool), brand: brand, center: center}
}

func (f *fixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t501-%s-%d-%d", name, time.Now().UnixNano(), f.seq),
		Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ,
		ParentID:       pgtype.Int8{Int64: parent, Valid: true},
		BrandID:        f.brand.ID,
		Currency:       "TRY",
		Locale:         "tr",
		Timezone:       "Europe/Istanbul",
		Settings:       []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("numeric %s: %v", s, err)
	}
	return n
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func date(y int, m time.Month, d int) pgtype.Date {
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func (f *fixture) invoiceParams(t *testing.T, buyer db.Organization, number string, source uuid.UUID, status string, issue pgtype.Date, payable string) db.CreateEinvoiceParams {
	t.Helper()
	arg := db.CreateEinvoiceParams{
		Uuid:               uuid.New(),
		OrganizationID:     f.center.ID,
		BrandID:            f.brand.ID,
		Number:             number,
		Profile:            "EARSIVFATURA",
		InvoiceType:        "SATIS",
		SourceType:         "manual",
		SourceUuid:         source,
		BuyerOrgID:         pgtype.Int8{Int64: buyer.ID, Valid: true},
		Buyer:              []byte(fmt.Sprintf(`{"legal_name":%q}`, buyer.Name)),
		Seller:             []byte(`{"legal_name":"Olex Merkez"}`),
		Lines:              []byte(`[{"name":"Hizmet","quantity":1}]`),
		Currency:           "TRY",
		RateSnapshot:       []byte(`{"base":"TRY","quote":"TRY","rate":"1"}`),
		LineExtension:      numeric(t, payable),
		TaxExclusive:       numeric(t, payable),
		TaxTotal:           numeric(t, "0"),
		Payable:            numeric(t, payable),
		TaxBreakdown:       []byte(`[]`),
		ValidationStatus:   "valid",
		ValidationMessages: []byte(`[]`),
		Status:             status,
		IssueDate:          issue,
	}
	if status == "archived" {
		arg.XmlStorageKey = text("einvoice/test.xml")
		arg.XmlSha256 = text("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	}
	return arg
}

func TestRepository_IncrementCounterConcurrent(t *testing.T) {
	f := newFixture(t)
	org := f.org(t, "counter", "distributor", f.center.ID)

	const calls = 50
	got := make(chan int64, calls)
	errs := make(chan error, calls)
	var wg sync.WaitGroup
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, err := f.repo.IncrementCounter(f.ctx, db.IncrementEinvoiceCounterParams{
				OrganizationID: org.ID, BrandID: f.brand.ID, Series: "T51", Year: 2501,
			})
			if err != nil {
				errs <- err
				return
			}
			got <- row.LastNo
			if row.Number == "" || len(row.Number) != 16 {
				errs <- fmt.Errorf("number = %q", row.Number)
			}
		}()
	}
	wg.Wait()
	close(got)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	numbers := make([]int64, 0, calls)
	for n := range got {
		numbers = append(numbers, n)
	}
	sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
	for i, n := range numbers {
		if want := int64(i + 1); n != want {
			t.Fatalf("counter[%d] = %d, want %d; all=%v", i, n, want, numbers)
		}
	}
}

func TestRepository_ArchiveConstraintsAndListSort(t *testing.T) {
	f := newFixture(t)
	buyer := f.org(t, "buyer", "distributor", f.center.ID)
	source := uuid.New()

	archived, err := f.repo.Create(f.ctx, f.invoiceParams(
		t, buyer, "T512501000000001", source, "archived", date(2026, time.October, 1), "100",
	))
	if err != nil {
		t.Fatalf("create archived: %v", err)
	}
	_, err = f.pool.Exec(f.ctx, `DELETE FROM einvoices WHERE id = $1`, archived.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("delete archived: got %v, want check violation", err)
	}

	_, err = f.repo.Create(f.ctx, f.invoiceParams(
		t, buyer, "T512501000000002", source, "draft", date(2026, time.October, 2), "110",
	))
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_einvoices_source_active" {
		t.Fatalf("duplicate active source: got %v", err)
	}

	inv2, err := f.repo.Create(f.ctx, f.invoiceParams(
		t, buyer, "T512501000000003", uuid.New(), "draft", date(2026, time.October, 3), "120",
	))
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	inv3, err := f.repo.Create(f.ctx, f.invoiceParams(
		t, buyer, "T512501000000004", uuid.New(), "draft", date(2026, time.October, 3), "90",
	))
	if err != nil {
		t.Fatalf("create third: %v", err)
	}

	rows, err := f.repo.List(f.ctx, db.ListEinvoicesParams{
		BrandID: f.brand.ID, BuyerOrgIds: []int64{buyer.ID},
		SortKey: "issue_date", SortDesc: true, RowLimit: 10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 || rows[0].ID != inv3.ID || rows[1].ID != inv2.ID || rows[2].ID != archived.ID {
		t.Fatalf("sort/tiebreak rows = %+v, want ids [%d %d %d]", rows, inv3.ID, inv2.ID, archived.ID)
	}
	if rows[0].TotalCount != 3 {
		t.Fatalf("total_count = %d, want 3", rows[0].TotalCount)
	}
}
