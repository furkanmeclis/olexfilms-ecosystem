package posting_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-173 acceptance: the source API against a migrated PostgreSQL
// (TEST_DATABASE_URL; CI runs PG18). Each call runs in its own transaction
// like a caller (TEC-169) would. finance_entries is append-only, so rows
// stay; every fixture carries a unique suffix and source UUID.

// Fixed rate fixture: EUR→UAH on the frozen day, and a different rate a
// few days later that a correct conversion must not pick.
var (
	frozenDay = time.Date(2001, 3, 5, 0, 0, 0, 0, time.UTC)
	laterDay  = time.Date(2001, 3, 9, 0, 0, 0, 0, time.UTC)
)

const (
	frozenRate = "45.1234"
	laterRate  = "50.0000"
	saleAmount = "1234.56"
	// 1234.56 × 45.1234 = 55707.544... → 55707.54
	wantUAH = "55707.54"
)

type env struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	p      *posting.Poster
	brand  int64
	center db.Organization
	dist   db.Organization // EUR seller
	dealer db.Organization // UAH buyer, child of dist
	suffix string
}

func newEnv(t *testing.T) *env {
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
	rates := fxrates.New(q, nil, nil)
	e := &env{ctx: ctx, pool: pool, q: q, p: posting.New(q, outbox.NewStore(pool, q), rates),
		suffix: fmt.Sprintf("%d", time.Now().UnixNano())}

	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	e.brand = brand.ID
	if e.center, err = q.GetBrandCenter(ctx, brand.ID); err != nil {
		t.Fatalf("olex center: %v", err)
	}
	e.dist = e.org(t, "dist", "distributor", e.center.ID, "EUR")
	e.dealer = e.org(t, "dealer", "dealer", e.dist.ID, "UAH")

	for _, r := range []struct {
		day  time.Time
		rate string
	}{{frozenDay, frozenRate}, {laterDay, laterRate}} {
		if err := q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: r.day, Valid: true}, Base: "EUR", Quote: "UAH",
			Rate: r.rate, Source: "manual", Note: pgtype.Text{String: "TEC-173 fixture", Valid: true},
		}); err != nil {
			t.Fatalf("rate fixture: %v", err)
		}
	}
	return e
}

func (e *env) org(t *testing.T, name, typ string, parent int64, currency string) db.Organization {
	t.Helper()
	o, err := e.q.CreateOrganization(e.ctx, db.CreateOrganizationParams{
		Slug: "t173-" + name + "-" + e.suffix, Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: e.brand, Currency: currency, Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

// inTx runs fn in its own transaction and commits when fn succeeds.
func inTx[T any](t *testing.T, e *env, fn func(tx pgx.Tx) (T, error)) (T, error) {
	t.Helper()
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	v, err := fn(tx)
	if err != nil {
		return v, err
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return v, nil
}

func (e *env) sale(src posting.Source) posting.Sale {
	return posting.Sale{
		Source: src, SellerOrgID: e.dist.ID, BuyerOrgID: e.dealer.ID,
		Amount: saleAmount, Currency: "EUR", RateDate: frozenDay,
		// The order's TRY snapshot (TEC-96): not the buyer's pair, so the
		// bridge resolves EUR→UAH on the same frozen day.
		RateSnapshot: &fxrates.Snapshot{Base: "EUR", Quote: "TRY", Rate: "0.6", RateDate: "2001-03-05", Source: "manual"},
	}
}

func (e *env) postSale(t *testing.T, s posting.Sale) posting.SaleResult {
	t.Helper()
	res, err := inTx(t, e, func(tx pgx.Tx) (posting.SaleResult, error) {
		return e.p.PostHierarchicalSaleTx(e.ctx, tx, s)
	})
	if err != nil {
		t.Fatalf("post sale: %v", err)
	}
	return res
}

func (e *env) balance(t *testing.T, owner, counterparty int64) string {
	t.Helper()
	c, err := e.q.GetCariAccountByCounterpartyOrg(e.ctx, db.GetCariAccountByCounterpartyOrgParams{
		OrganizationID: owner, CounterpartyOrgID: pgtype.Int8{Int64: counterparty, Valid: true},
	})
	if err != nil {
		t.Fatalf("cari %d/%d: %v", owner, counterparty, err)
	}
	b, err := e.q.GetCariBalance(e.ctx, db.GetCariBalanceParams{CariID: c.ID, OrganizationID: owner})
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return posting.FormatNumeric(b.Balance)
}

func (e *env) sourceRows(t *testing.T, src posting.Source) []db.FinanceEntry {
	t.Helper()
	rows, err := e.q.ListFinanceEntriesBySource(e.ctx, db.ListFinanceEntriesBySourceParams{
		SourceType: src.Type, SourceUuid: src.UUID,
	})
	if err != nil {
		t.Fatalf("source rows: %v", err)
	}
	return rows
}

// events counts outbox rows of the source by name.
func (e *env) events(t *testing.T, src posting.Source) map[string]int {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `
		SELECT event_name, COUNT(*) FROM outbox_events
		WHERE payload->'data'->>'source_uuid' = $1 GROUP BY event_name`, src.UUID.String())
	if err != nil {
		t.Fatalf("outbox: %v", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			t.Fatal(err)
		}
		out[name] = n
	}
	return out
}

func newSource() posting.Source { return posting.Source{Type: "order", UUID: uuid.New()} }

func TestHierarchicalSale_EURToUAH(t *testing.T) {
	e := newEnv(t)
	src := newSource()
	res := e.postSale(t, e.sale(src))
	if res.Replayed() {
		t.Fatal("first call replayed")
	}

	s := res.Seller.Entry
	if s.OrganizationID != e.dist.ID || s.Direction != "income" || s.Category != "sale" || !s.CariID.Valid {
		t.Fatalf("seller row: %+v", s)
	}
	if s.Currency != "EUR" || posting.FormatNumeric(s.Amount) != saleAmount ||
		s.OrigCurrency != "EUR" || posting.FormatNumeric(s.OrigAmount) != saleAmount ||
		posting.FormatRate(s.Rate) != "1.00000000" {
		t.Fatalf("seller amounts: %s %s (orig %s %s, rate %s)", posting.FormatNumeric(s.Amount), s.Currency,
			posting.FormatNumeric(s.OrigAmount), s.OrigCurrency, posting.FormatRate(s.Rate))
	}

	b := res.Buyer.Entry
	if b.OrganizationID != e.dealer.ID || b.Direction != "expense" || b.Category != "purchase" || !b.CariID.Valid {
		t.Fatalf("buyer row: %+v", b)
	}
	if b.Currency != "UAH" || posting.FormatNumeric(b.Amount) != wantUAH {
		t.Fatalf("buyer amount %s %s, want %s UAH", posting.FormatNumeric(b.Amount), b.Currency, wantUAH)
	}
	if b.OrigCurrency != "EUR" || posting.FormatNumeric(b.OrigAmount) != saleAmount {
		t.Fatalf("buyer original %s %s", posting.FormatNumeric(b.OrigAmount), b.OrigCurrency)
	}
	if posting.FormatRate(b.Rate) != "45.12340000" || b.RateDate.Time.Format(time.DateOnly) != "2001-03-05" {
		t.Fatalf("buyer rate %s on %s", posting.FormatRate(b.Rate), b.RateDate.Time.Format(time.DateOnly))
	}

	// Seller: the buyer owes; buyer: it owes the seller.
	if got := e.balance(t, e.dist.ID, e.dealer.ID); got != saleAmount {
		t.Fatalf("seller cari balance %s, want %s", got, saleAmount)
	}
	if got := e.balance(t, e.dealer.ID, e.dist.ID); got != "-"+wantUAH {
		t.Fatalf("buyer cari balance %s, want -%s", got, wantUAH)
	}
	if ev := e.events(t, src); ev["cari.charge_posted"] != 2 || len(ev) != 1 {
		t.Fatalf("outbox events: %v", ev)
	}
}

func TestHierarchicalSale_SecondCallWritesNothing(t *testing.T) {
	e := newEnv(t)
	src := newSource()
	first := e.postSale(t, e.sale(src))
	second := e.postSale(t, e.sale(src))
	if !second.Replayed() {
		t.Fatal("second call not replayed")
	}
	if second.Seller.Entry.ID != first.Seller.Entry.ID || second.Buyer.Entry.ID != first.Buyer.Entry.ID {
		t.Fatal("second call returned other rows")
	}
	if n := len(e.sourceRows(t, src)); n != 2 {
		t.Fatalf("source rows %d, want 2", n)
	}
	if ev := e.events(t, src); ev["cari.charge_posted"] != 2 {
		t.Fatalf("outbox events after replay: %v", ev)
	}

	// Same key, different amount: refused, nothing written.
	changed := e.sale(src)
	changed.Amount = "1.00"
	_, err := inTx(t, e, func(tx pgx.Tx) (posting.SaleResult, error) {
		return e.p.PostHierarchicalSaleTx(e.ctx, tx, changed)
	})
	if !errors.Is(err, posting.ErrIdempotencyConflict) {
		t.Fatalf("changed replay: want ErrIdempotencyConflict, got %v", err)
	}
}

func TestVoidBySource_BothSidesBackToZero(t *testing.T) {
	e := newEnv(t)
	src := newSource()
	e.postSale(t, e.sale(src))

	voided, err := inTx(t, e, func(tx pgx.Tx) (posting.VoidResult, error) {
		return e.p.VoidBySourceTx(e.ctx, tx, src, "order cancelled", nil)
	})
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	if len(voided.Reversals) != 2 {
		t.Fatalf("reversals %d, want 2 (seller and buyer)", len(voided.Reversals))
	}
	orgs := map[int64]bool{}
	for _, r := range voided.Reversals {
		orgs[r.OrganizationID] = true
	}
	if !orgs[e.dist.ID] || !orgs[e.dealer.ID] {
		t.Fatalf("void did not cascade to both organizations: %v", orgs)
	}
	if got := e.balance(t, e.dist.ID, e.dealer.ID); got != "0.00" {
		t.Fatalf("seller balance after void %s", got)
	}
	if got := e.balance(t, e.dealer.ID, e.dist.ID); got != "0.00" {
		t.Fatalf("buyer balance after void %s", got)
	}

	again, err := inTx(t, e, func(tx pgx.Tx) (posting.VoidResult, error) {
		return e.p.VoidBySourceTx(e.ctx, tx, src, "again", nil)
	})
	if err != nil || len(again.Reversals) != 0 {
		t.Fatalf("second void: %d reversals, err %v", len(again.Reversals), err)
	}
	if n := len(e.sourceRows(t, src)); n != 4 {
		t.Fatalf("source rows %d, want 4", n)
	}
	if ev := e.events(t, src); ev["cari.entry_voided"] != 2 {
		t.Fatalf("outbox events after void: %v", ev)
	}
}

func TestHierarchicalSale_PartialFailureRollsBack(t *testing.T) {
	e := newEnv(t)
	src := newSource()
	s := e.sale(src)
	// No EUR→UAH rate within the lookback of this day: the seller row is
	// written, the buyer side fails and the caller rolls the tx back.
	s.RateDate = time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	s.RateSnapshot = nil
	_, err := inTx(t, e, func(tx pgx.Tx) (posting.SaleResult, error) {
		return e.p.PostHierarchicalSaleTx(e.ctx, tx, s)
	})
	if !errors.Is(err, fxrates.ErrRateNotFound) {
		t.Fatalf("want ErrRateNotFound, got %v", err)
	}
	if n := len(e.sourceRows(t, src)); n != 0 {
		t.Fatalf("rows survived rollback: %d", n)
	}
	if _, err := e.q.GetCariAccountByCounterpartyOrg(e.ctx, db.GetCariAccountByCounterpartyOrgParams{
		OrganizationID: e.dist.ID, CounterpartyOrgID: pgtype.Int8{Int64: e.dealer.ID, Valid: true},
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cari survived rollback: %v", err)
	}
	if ev := e.events(t, src); len(ev) != 0 {
		t.Fatalf("events survived rollback: %v", ev)
	}
}

func TestHierarchicalSale_SellerMustBeParent(t *testing.T) {
	e := newEnv(t)
	s := e.sale(newSource())
	s.SellerOrgID, s.BuyerOrgID = e.dealer.ID, e.dist.ID // upward
	_, err := inTx(t, e, func(tx pgx.Tx) (posting.SaleResult, error) {
		return e.p.PostHierarchicalSaleTx(e.ctx, tx, s)
	})
	if !errors.Is(err, posting.ErrNotParent) {
		t.Fatalf("want ErrNotParent, got %v", err)
	}
}

func TestChargeAndIncome(t *testing.T) {
	e := newEnv(t)
	src := newSource()
	charge := posting.Entry{
		OrganizationID: e.dist.ID, Source: src, Category: "shipping", Amount: "20.00",
		Currency: "EUR", RateDate: frozenDay, CounterpartyOrgID: e.dealer.ID,
	}
	r, err := inTx(t, e, func(tx pgx.Tx) (posting.Result, error) { return e.p.Charge(e.ctx, tx, charge) })
	if err != nil || r.Entry.Direction != "charge" || r.Replayed {
		t.Fatalf("charge: %+v %v", r, err)
	}
	r2, err := inTx(t, e, func(tx pgx.Tx) (posting.Result, error) { return e.p.Charge(e.ctx, tx, charge) })
	if err != nil || !r2.Replayed || r2.Entry.ID != r.Entry.ID {
		t.Fatalf("charge replay: %+v %v", r2, err)
	}

	acc, err := e.q.CreateFinanceAccount(e.ctx, db.CreateFinanceAccountParams{
		OrganizationID: e.dist.ID, BrandID: e.brand, Type: "cash", Name: "Kasa " + e.suffix,
		Currency: "EUR", Active: true,
	})
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	income := posting.Entry{
		OrganizationID: e.dist.ID, Source: src, Role: "fee", Category: "service",
		Amount: "50", Currency: "EUR", AccountID: acc.ID,
	}
	r3, err := inTx(t, e, func(tx pgx.Tx) (posting.Result, error) { return e.p.PostIncome(e.ctx, tx, income) })
	if err != nil || r3.Entry.CariID.Valid || r3.Entry.AccountID.Int64 != acc.ID {
		t.Fatalf("income: %+v %v", r3, err)
	}
	if ev := e.events(t, src); ev["cari.charge_posted"] != 1 || ev["finance.entry_posted"] != 1 {
		t.Fatalf("events: %v", ev)
	}
	if got := e.balance(t, e.dist.ID, e.dealer.ID); got != "20.00" {
		t.Fatalf("cari after charge %s", got)
	}
}
