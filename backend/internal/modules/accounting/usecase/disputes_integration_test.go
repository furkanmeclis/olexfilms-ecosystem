package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-174 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18). The ledger is append-only, so every fixture carries a unique
// suffix and source UUID; all organizations keep TRY so no rate is needed.

type disputeEnv struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	poster *posting.Poster
	svc    *acc.Service
	brand  int64
	dist   db.Organization // seller, parent of dealer
	dealer db.Organization // buyer, disputes
	other  db.Organization // another distributor of the brand
	suffix string
}

func newDisputeEnv(t *testing.T) *disputeEnv {
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
	out := outbox.NewStore(pool, q)
	poster := posting.New(q, out, nil)
	e := &disputeEnv{
		ctx: ctx, pool: pool, q: q, poster: poster,
		svc:    acc.New(pool, q, poster, nil).WithOutbox(out),
		suffix: fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	e.brand = brand.ID
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("olex center: %v", err)
	}
	e.dist = e.org(t, "dist", "distributor", center.ID)
	e.dealer = e.org(t, "dealer", "dealer", e.dist.ID)
	e.other = e.org(t, "other", "distributor", center.ID)
	return e
}

func (e *disputeEnv) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := e.q.CreateOrganization(e.ctx, db.CreateOrganizationParams{
		Slug: "t174-" + name + "-" + e.suffix, Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: e.brand, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

// caller is a managed-scope principal of o (dealer and distributor roles
// hold accounting.read/dispute/resolve at managed).
func caller(o db.Organization, perm string) acc.Caller {
	return acc.Caller{
		Org: orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, OrgType: o.Type, BrandID: o.BrandID},
		Filter: scopefilter.Filter{
			Permission: perm, Scope: rbac.ScopeManaged, OrgID: o.ID, OrgIDs: []int64{o.ID},
		},
	}
}

// sale posts a distributor → dealer sale and returns the dealer's row.
func (e *disputeEnv) sale(t *testing.T, amount string) (posting.Source, db.FinanceEntry) {
	t.Helper()
	src := posting.Source{Type: "order", UUID: uuid.New()}
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	res, err := e.poster.PostHierarchicalSaleTx(e.ctx, tx, posting.Sale{
		Source: src, SellerOrgID: e.dist.ID, BuyerOrgID: e.dealer.ID,
		Amount: amount, Currency: "TRY", RateDate: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("sale: %v", err)
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatal(err)
	}
	return src, res.Buyer.Entry
}

func (e *disputeEnv) balance(t *testing.T, owner, counterparty int64) string {
	t.Helper()
	var bal string
	if err := e.pool.QueryRow(e.ctx, `
		SELECT b.balance::text FROM cari_account_balances b
		JOIN cari_accounts c ON c.id = b.cari_id
		WHERE c.organization_id = $1 AND c.counterparty_org_id = $2`, owner, counterparty).Scan(&bal); err != nil {
		t.Fatalf("balance: %v", err)
	}
	return bal
}

func (e *disputeEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (e *disputeEnv) open(t *testing.T, entry db.FinanceEntry) acc.Dispute {
	t.Helper()
	d, err := e.svc.OpenDispute(e.ctx, caller(e.dealer, rbac.PermAccountingDispute),
		acc.DisputeInput{EntryUUID: entry.Uuid, Reason: "Fiyat yanlış yazılmış"})
	if err != nil {
		t.Fatalf("open dispute: %v", err)
	}
	return d
}

// The dealer disputes a row its distributor posted; the distributor lists
// and reads it, another distributor does not (404); a second open dispute
// on the same row is refused; the opened event and audit row are written.
func TestDispute_OpenVisibilityAndDuplicate(t *testing.T) {
	e := newDisputeEnv(t)
	src, entry := e.sale(t, "1000.00")
	d := e.open(t, entry)
	if d.Status != acc.DisputeOpen || d.Entry.UUID != entry.Uuid || d.SourceUUID != src.UUID ||
		d.CounterpartyOrganization.UUID != e.dist.Uuid || d.Organization.UUID != e.dealer.Uuid {
		t.Fatalf("dispute = %+v", d)
	}

	items, total, err := e.svc.ListDisputes(e.ctx, caller(e.dist, rbac.PermAccountingRead),
		acc.DisputeFilter{Statuses: []string{acc.DisputeOpen}, OrganizationUUIDs: []uuid.UUID{e.dealer.Uuid}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].UUID != d.UUID {
		t.Fatalf("distributor list = %d %+v", total, items)
	}
	if _, err := e.svc.GetDispute(e.ctx, caller(e.dist, rbac.PermAccountingRead), d.UUID); err != nil {
		t.Fatalf("distributor detail: %v", err)
	}
	if _, err := e.svc.GetDispute(e.ctx, caller(e.dealer, rbac.PermAccountingRead), d.UUID); err != nil {
		t.Fatalf("dealer detail: %v", err)
	}
	if _, err := e.svc.GetDispute(e.ctx, caller(e.other, rbac.PermAccountingRead), d.UUID); !errors.Is(err, acc.ErrDisputeNotFound) {
		t.Fatalf("other distributor detail err = %v, want not found", err)
	}
	_, total, err = e.svc.ListDisputes(e.ctx, caller(e.other, rbac.PermAccountingRead),
		acc.DisputeFilter{OrganizationUUIDs: []uuid.UUID{e.dealer.Uuid}, Limit: 50})
	if err != nil || total != 0 {
		t.Fatalf("other distributor list = %d, %v", total, err)
	}
	// The other distributor cannot resolve it either.
	if _, err := e.svc.ResolveDispute(e.ctx, caller(e.other, rbac.PermAccountingResolve), d.UUID,
		acc.ResolveInput{Resolution: acc.ResolutionReversal}); !errors.Is(err, acc.ErrDisputeNotFound) {
		t.Fatalf("other distributor resolve err = %v", err)
	}

	_, err = e.svc.OpenDispute(e.ctx, caller(e.dealer, rbac.PermAccountingDispute),
		acc.DisputeInput{EntryUUID: entry.Uuid, Reason: "ikinci itiraz"})
	if !errors.Is(err, acc.ErrDisputeAlreadyOpen) {
		t.Fatalf("second open err = %v, want already open", err)
	}

	if n := e.count(t, `SELECT COUNT(*) FROM outbox_events
		WHERE event_name = 'accounting.dispute_opened' AND payload->'data'->>'dispute_uuid' = $1`, d.UUID.String()); n != 1 {
		t.Fatalf("opened events = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM activity_events
		WHERE action = 'accounting.dispute_opened' AND resource = 'accounting_dispute' AND resource_uuid = $1`, d.UUID); n != 1 {
		t.Fatalf("opened audit rows = %d", n)
	}
}

// Resolution by reversal: the source is reversed on both ledgers
// (reversal_of_id rows) and the balances return to zero.
func TestDispute_ResolveByReversal(t *testing.T) {
	e := newDisputeEnv(t)
	_, entry := e.sale(t, "1000.00")
	if got := e.balance(t, e.dealer.ID, e.dist.ID); got != "-1000.00" {
		t.Fatalf("dealer balance before = %s", got)
	}
	d := e.open(t, entry)

	// The dealer is not the parent: its own dispute is not addressed to it.
	if _, err := e.svc.ResolveDispute(e.ctx, caller(e.dealer, rbac.PermAccountingResolve), d.UUID,
		acc.ResolveInput{Resolution: acc.ResolutionReversal}); !errors.Is(err, acc.ErrDisputeNotFound) {
		t.Fatalf("dealer resolve err = %v", err)
	}

	got, err := e.svc.ResolveDispute(e.ctx, caller(e.dist, rbac.PermAccountingResolve), d.UUID,
		acc.ResolveInput{Resolution: acc.ResolutionReversal, Note: "Kayıt iptal edildi"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Status != acc.DisputeResolvedReversal || got.ReversalEntryUUID == nil || got.ResolvedAt == nil {
		t.Fatalf("resolved = %+v", got)
	}
	rev, err := e.q.GetFinanceEntryByUUID(e.ctx, *got.ReversalEntryUUID)
	if err != nil {
		t.Fatal(err)
	}
	if !rev.ReversalOfID.Valid || rev.ReversalOfID.Int64 != entry.ID {
		t.Fatalf("reversal row = %+v", rev)
	}
	if b := e.balance(t, e.dealer.ID, e.dist.ID); b != "0.00" {
		t.Fatalf("dealer balance after = %s", b)
	}
	if b := e.balance(t, e.dist.ID, e.dealer.ID); b != "0.00" {
		t.Fatalf("distributor balance after = %s", b)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM outbox_events
		WHERE event_name = 'accounting.dispute_resolved' AND payload->'data'->>'dispute_uuid' = $1`, d.UUID.String()); n != 1 {
		t.Fatalf("resolved events = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM activity_events
		WHERE action = 'accounting.dispute_resolved' AND resource_uuid = $1`, d.UUID); n != 1 {
		t.Fatalf("resolved audit rows = %d", n)
	}

	// Final: a second resolution is refused, and the row cannot be changed
	// in the database either.
	if _, err := e.svc.ResolveDispute(e.ctx, caller(e.dist, rbac.PermAccountingResolve), d.UUID,
		acc.ResolveInput{Resolution: acc.ResolutionReject, Note: "x"}); !errors.Is(err, acc.ErrDisputeClosed) {
		t.Fatalf("second resolve err = %v", err)
	}
	_, err = e.pool.Exec(e.ctx, `UPDATE accounting_disputes SET resolution_note = 'changed' WHERE uuid = $1`, d.UUID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
		t.Fatalf("update of a final dispute err = %v, want 23001", err)
	}
	// The reversed row can no longer be disputed.
	if _, err := e.svc.OpenDispute(e.ctx, caller(e.dealer, rbac.PermAccountingDispute),
		acc.DisputeInput{EntryUUID: entry.Uuid, Reason: "tekrar"}); !errors.Is(err, acc.ErrNotDisputable) {
		t.Fatalf("dispute of a reversed row err = %v", err)
	}
}

// Resolution by revision: the reversal and the corrected repost (revision
// 2, both ledgers) are written in one transaction.
func TestDispute_ResolveByRevision(t *testing.T) {
	e := newDisputeEnv(t)
	src, entry := e.sale(t, "1000.00")
	d := e.open(t, entry)

	got, err := e.svc.ResolveDispute(e.ctx, caller(e.dist, rbac.PermAccountingResolve), d.UUID,
		acc.ResolveInput{Resolution: acc.ResolutionRevision, CorrectedAmount: "800.00", Note: "İskonto unutulmuş"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Status != acc.DisputeResolvedRevision || got.CorrectedAmount == nil || *got.CorrectedAmount != "800.00" ||
		got.ReversalEntryUUID == nil || got.RevisionEntryUUID == nil {
		t.Fatalf("resolved = %+v", got)
	}
	rev, err := e.q.GetFinanceEntryByUUID(e.ctx, *got.ReversalEntryUUID)
	if err != nil {
		t.Fatal(err)
	}
	repost, err := e.q.GetFinanceEntryByUUID(e.ctx, *got.RevisionEntryUUID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.ReversalOfID.Int64 != entry.ID || repost.Revision != 2 || repost.ReversalOfID.Valid ||
		repost.OrganizationID != e.dealer.ID || repost.Role != entry.Role || posting.FormatNumeric(repost.Amount) != "800.00" {
		t.Fatalf("reversal %+v / repost %+v", rev, repost)
	}
	// NOW() is the transaction start: equal timestamps prove one transaction.
	if !rev.CreatedAt.Time.Equal(repost.CreatedAt.Time) || !got.ResolvedAt.Equal(repost.CreatedAt.Time) {
		t.Fatalf("not one transaction: reversal %v, repost %v, resolved %v",
			rev.CreatedAt.Time, repost.CreatedAt.Time, got.ResolvedAt)
	}
	// Seller side is revised too: 2 originals, 2 reversals, 2 reposts.
	if n := e.count(t, `SELECT COUNT(*) FROM finance_entries WHERE source_uuid = $1`, src.UUID); n != 6 {
		t.Fatalf("source rows = %d, want 6", n)
	}
	if b := e.balance(t, e.dealer.ID, e.dist.ID); b != "-800.00" {
		t.Fatalf("dealer balance = %s", b)
	}
	if b := e.balance(t, e.dist.ID, e.dealer.ID); b != "800.00" {
		t.Fatalf("distributor balance = %s", b)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM outbox_events
		WHERE event_name = 'accounting.dispute_resolved' AND payload->'data'->>'dispute_uuid' = $1`, d.UUID.String()); n != 1 {
		t.Fatalf("resolved events = %d", n)
	}
	// The revised row can be disputed again (one open dispute per row).
	if _, err := e.svc.OpenDispute(e.ctx, caller(e.dealer, rbac.PermAccountingDispute),
		acc.DisputeInput{EntryUUID: repost.Uuid, Reason: "hala yanlış"}); err != nil {
		t.Fatalf("dispute of the revision: %v", err)
	}
}

// Rejection needs a note, posts nothing and writes the rejected event and
// audit row.
func TestDispute_Reject(t *testing.T) {
	e := newDisputeEnv(t)
	src, entry := e.sale(t, "1000.00")
	d := e.open(t, entry)

	var ve *acc.ValidationError
	if _, err := e.svc.ResolveDispute(e.ctx, caller(e.dist, rbac.PermAccountingResolve), d.UUID,
		acc.ResolveInput{Resolution: acc.ResolutionReject}); !errors.As(err, &ve) || ve.Field != "note" {
		t.Fatalf("reject without note err = %v", err)
	}
	got, err := e.svc.ResolveDispute(e.ctx, caller(e.dist, rbac.PermAccountingResolve), d.UUID,
		acc.ResolveInput{Resolution: acc.ResolutionReject, Note: "Fatura doğru"})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got.Status != acc.DisputeRejected || got.ResolutionNote == nil || got.ReversalEntryUUID != nil {
		t.Fatalf("rejected = %+v", got)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM finance_entries WHERE source_uuid = $1`, src.UUID); n != 2 {
		t.Fatalf("source rows = %d, want 2", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM outbox_events
		WHERE event_name = 'accounting.dispute_rejected' AND payload->'data'->>'dispute_uuid' = $1`, d.UUID.String()); n != 1 {
		t.Fatalf("rejected events = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM activity_events
		WHERE action = 'accounting.dispute_rejected' AND resource_uuid = $1`, d.UUID); n != 1 {
		t.Fatalf("rejected audit rows = %d", n)
	}
	// After a rejection the row may be disputed again.
	e.open(t, entry)
}

// The distributor cannot dispute its own sale row (seller side: the cari
// counterparty is its child, not its parent).
func TestDispute_SellerRowNotDisputable(t *testing.T) {
	e := newDisputeEnv(t)
	src, _ := e.sale(t, "500.00")
	seller, err := e.q.GetFinanceEntryBySource(e.ctx, db.GetFinanceEntryBySourceParams{
		OrganizationID: e.dist.ID, SourceType: src.Type, SourceUuid: src.UUID, Role: posting.RoleSale, Revision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.OpenDispute(e.ctx, caller(e.dist, rbac.PermAccountingDispute),
		acc.DisputeInput{EntryUUID: seller.Uuid, Reason: "x"})
	if !errors.Is(err, acc.ErrNotDisputable) {
		t.Fatalf("seller row err = %v", err)
	}
	// A database-level insert bypassing the use case is refused as well.
	_, err = e.pool.Exec(e.ctx, `
		INSERT INTO accounting_disputes (organization_id, brand_id, counterparty_org_id, entry_id, source_type, source_uuid, reason)
		VALUES ($1, $2, $3, $4, 'order', $5, 'x')`, e.dist.ID, e.brand, e.dealer.ID, seller.ID, src.UUID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("raw insert err = %v, want 23514", err)
	}
}
