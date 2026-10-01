package db_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-171: database-level guards of the accounting ledger (migration
// 000047). Everything runs inside one transaction that is rolled back; each
// failing statement runs in its own savepoint.

type accountingFixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	center  db.Organization
	account db.FinanceAccount
	cari    db.CariAccount
}

func newAccountingFixture(t *testing.T) *accountingFixture {
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	q := db.New(tx)
	center := brandCenter(t, ctx, q, "olex")
	other := brandCenter(t, ctx, q, "glorian")
	account, err := q.CreateFinanceAccount(ctx, db.CreateFinanceAccountParams{
		OrganizationID: center.ID, BrandID: center.BrandID, Type: "cash",
		Name: "t171-cash", Currency: center.Currency, Active: true,
	})
	if err != nil {
		t.Fatalf("finance account: %v", err)
	}
	cari, err := q.CreateCariForOrgIfMissing(ctx, db.CreateCariForOrgIfMissingParams{
		OrganizationID: center.ID, BrandID: center.BrandID,
		CounterpartyOrgID: pgtype.Int8{Int64: other.ID, Valid: true}, Currency: center.Currency,
	})
	if err != nil {
		t.Fatalf("cari account: %v", err)
	}
	return &accountingFixture{ctx: ctx, tx: tx, q: q, center: center, account: account, cari: cari}
}

func brandCenter(t *testing.T, ctx context.Context, q *db.Queries, slug string) db.Organization {
	t.Helper()
	brand, err := q.GetBrandBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("%s brand: %v", slug, err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("%s center: %v", slug, err)
	}
	return center
}

func (f *accountingFixture) entry(t *testing.T, direction, amount string, source uuid.UUID) db.InsertFinanceEntryParams {
	t.Helper()
	arg := db.InsertFinanceEntryParams{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID,
		CariID:    pgtype.Int8{Int64: f.cari.ID, Valid: true},
		Direction: direction, Category: "sales",
		OrigCurrency: f.center.Currency, OrigAmount: numeric(t, amount),
		Currency: f.center.Currency, Amount: numeric(t, amount),
		Rate:     numeric(t, "1"),
		RateDate: pgtype.Date{Time: time.Now().UTC().Truncate(24 * time.Hour), Valid: true},
		Role:     "main", Revision: 1,
	}
	if source != uuid.Nil {
		arg.SourceType = pgtype.Text{String: "order", Valid: true}
		arg.SourceUuid = pgtype.UUID{Bytes: source, Valid: true}
	}
	return arg
}

// expectCode runs fn in a savepoint and requires a PgError with one of codes.
func (f *accountingFixture) expectCode(t *testing.T, name string, fn func(sp pgx.Tx) error, codes ...string) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = fn(sp)
	_ = sp.Rollback(f.ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("%s: want SQLSTATE %v, got %v", name, codes, err)
	}
	for _, c := range codes {
		if pgErr.Code == c {
			return
		}
	}
	t.Fatalf("%s: want SQLSTATE %v, got %s (%s)", name, codes, pgErr.Code, pgErr.Message)
}

func numericFloat(t *testing.T, n pgtype.Numeric) float64 {
	t.Helper()
	v, err := n.Float64Value()
	if err != nil || !v.Valid {
		t.Fatalf("numeric %+v: %v", n, err)
	}
	return v.Float64
}

func TestAccountingSchemaConstraints(t *testing.T) {
	f := newAccountingFixture(t)
	ctx := f.ctx
	source := uuid.New()

	sale, err := f.q.InsertFinanceEntry(ctx, f.entry(t, "income", "100.00", source))
	if err != nil {
		t.Fatalf("insert entry: %v", err)
	}

	t.Run("update rejected", func(t *testing.T) {
		f.expectCode(t, "update", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE finance_entries SET description = 'x' WHERE id = $1`, sale.ID)
			return err
		}, "23001")
	})

	t.Run("delete rejected", func(t *testing.T) {
		f.expectCode(t, "delete", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM finance_entries WHERE id = $1`, sale.ID)
			return err
		}, "23001")
	})

	t.Run("duplicate source writes nothing", func(t *testing.T) {
		_, err := f.q.InsertFinanceEntry(ctx, f.entry(t, "income", "100.00", source))
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("second insert: want ErrNoRows, got %v", err)
		}
		got, err := f.q.GetFinanceEntryBySource(ctx, db.GetFinanceEntryBySourceParams{
			OrganizationID: f.center.ID, SourceType: "order", SourceUuid: source, Role: "main", Revision: 1,
		})
		if err != nil || got.ID != sale.ID {
			t.Fatalf("existing row = %d, %v; want %d", got.ID, err, sale.ID)
		}
		rows, err := f.q.ListFinanceEntriesBySource(ctx, db.ListFinanceEntriesBySourceParams{
			SourceType: "order", SourceUuid: source,
		})
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows by source = %d, %v", len(rows), err)
		}
	})

	t.Run("revision bump writes a new row", func(t *testing.T) {
		arg := f.entry(t, "income", "90.00", source)
		arg.Revision = 2
		if _, err := f.q.InsertFinanceEntry(ctx, arg); err != nil {
			t.Fatalf("revision 2: %v", err)
		}
	})

	t.Run("reversal FK enforced", func(t *testing.T) {
		f.expectCode(t, "missing reversal target", func(sp pgx.Tx) error {
			arg := f.entry(t, "income", "-100.00", uuid.Nil)
			_, err := sp.Exec(ctx, `INSERT INTO finance_entries (
					organization_id, brand_id, cari_id, direction, category, orig_currency, orig_amount,
					currency, amount, rate, rate_date, reversal_of_id)
				VALUES ($1, $2, $3, 'income', 'sales', $4, -100, $4, -100, 1, CURRENT_DATE, -1)`,
				arg.OrganizationID, arg.BrandID, arg.CariID, arg.Currency)
			return err
		}, "23503")
	})

	t.Run("reversal mirrors once", func(t *testing.T) {
		rev, err := f.q.InsertFinanceReversal(ctx, db.InsertFinanceReversalParams{
			ReversalOfID: sale.ID, OrganizationID: f.center.ID,
		})
		if err != nil {
			t.Fatalf("reversal: %v", err)
		}
		if !rev.ReversalOfID.Valid || rev.ReversalOfID.Int64 != sale.ID {
			t.Fatalf("reversal_of_id = %+v", rev.ReversalOfID)
		}
		if _, err := f.q.InsertFinanceReversal(ctx, db.InsertFinanceReversalParams{
			ReversalOfID: sale.ID, OrganizationID: f.center.ID,
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("second reversal: want ErrNoRows, got %v", err)
		}
		f.expectCode(t, "reversal with wrong amount", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO finance_entries (
					organization_id, brand_id, cari_id, direction, category, orig_currency, orig_amount,
					currency, amount, rate, rate_date, source_type, source_uuid, reversal_of_id)
				SELECT organization_id, brand_id, cari_id, direction, category, orig_currency, -1,
					currency, -1, rate, rate_date, source_type, source_uuid, id
				FROM finance_entries WHERE source_uuid = $1 AND revision = 2`, source)
			return err
		}, "23514")
	})

	t.Run("cari balance nets reversals", func(t *testing.T) {
		if _, err := f.q.InsertFinanceEntry(ctx, f.entry(t, "collection", "40.00", uuid.Nil)); err != nil {
			t.Fatalf("collection: %v", err)
		}
		bal, err := f.q.GetCariBalance(ctx, db.GetCariBalanceParams{CariID: f.cari.ID, OrganizationID: f.center.ID})
		if err != nil {
			t.Fatal(err)
		}
		// +100 (sale) -100 (reversal) +90 (revision 2) -40 (collection) = 50.
		if got := numericFloat(t, bal.Balance); got != 50 {
			t.Fatalf("cari balance = %v, want 50", got)
		}
		if bal.EntryCount != 4 {
			t.Fatalf("cari entry count = %d, want 4", bal.EntryCount)
		}
	})

	t.Run("charge cannot touch an account", func(t *testing.T) {
		f.expectCode(t, "charge with account", func(sp pgx.Tx) error {
			arg := f.entry(t, "charge", "10.00", uuid.Nil)
			arg.AccountID = pgtype.Int8{Int64: f.account.ID, Valid: true}
			_, err := db.New(sp).InsertFinanceEntry(ctx, arg)
			return err
		}, "23514")
	})

	t.Run("currency follows the organization", func(t *testing.T) {
		f.expectCode(t, "foreign ledger currency", func(sp pgx.Tx) error {
			arg := f.entry(t, "income", "10.00", uuid.Nil)
			arg.Currency = "XAU"
			arg.Rate = numeric(t, "2")
			arg.Amount = numeric(t, "20.00")
			_, err := db.New(sp).InsertFinanceEntry(ctx, arg)
			return err
		}, "23514")
	})
}
