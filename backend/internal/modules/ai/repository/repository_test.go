package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-383 acceptance. Every test runs in one rolled-back transaction.

type fixture struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	tx     pgx.Tx
	q      *db.Queries
	store  *Store
	brand  db.Brand
	center db.Organization
	dealer db.Organization
	user   db.User
	other  db.User
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &fixture{ctx: ctx, pool: pool, tx: tx, q: db.New(tx), store: New(tx)}
	if f.brand, err = f.q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatalf("brand: %v", err)
	}
	if f.center, err = f.q.GetBrandCenter(ctx, f.brand.ID); err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	if f.dealer, err = f.q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "tec383-dealer-" + suffix, Name: "TEC383 dealer", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true},
		BrandID: f.brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	}); err != nil {
		t.Fatalf("dealer: %v", err)
	}
	user := func(name string) db.User {
		u, err := f.q.CreateUser(ctx, db.CreateUserParams{
			PasswordHash: "x", Name: "TEC383", Surname: name, Status: "active",
			Email: pgtype.Text{String: "tec383-" + name + "-" + suffix + "@example.test", Valid: true},
		})
		if err != nil {
			t.Fatalf("user %s: %v", name, err)
		}
		return u
	}
	f.user, f.other = user("owner"), user("other")
	return f
}

func (f *fixture) action(t *testing.T, key string, expires time.Duration) db.AiPendingAction {
	t.Helper()
	a, err := f.q.CreateAIPendingAction(f.ctx, db.CreateAIPendingActionParams{
		OrganizationID: f.dealer.ID, BrandID: f.brand.ID, UserID: f.user.ID,
		Source: model.SourcePanel, SourceRef: pgtype.Text{String: "conv-1", Valid: true},
		ToolUseID: "toolu_" + key, ToolName: "create_task",
		Input: []byte(`{"title":"x"}`), Preview: []byte(`{}`),
		IdempotencyKey: "tec383-" + key + fmt.Sprint(time.Now().UnixNano()),
		ExpiresAt:      pgtype.Timestamptz{Time: time.Now().Add(expires), Valid: true},
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	return a
}

// Acceptance: pending → executing is a single CAS; the second call gets no
// row.
func TestClaimPendingActionCAS(t *testing.T) {
	f := newFixture(t)
	a := f.action(t, "cas", time.Hour)

	if _, ok, err := f.store.ClaimPendingAction(f.ctx, a.Uuid, f.dealer.ID, f.other.ID, nil, nil); err != nil || ok {
		t.Fatalf("foreign user claimed: ok=%v err=%v", ok, err)
	}
	got, ok, err := f.store.ClaimPendingAction(f.ctx, a.Uuid, f.dealer.ID, f.user.ID, nil, nil)
	if err != nil || !ok || got.Status != model.ActionExecuting {
		t.Fatalf("first claim: ok=%v status=%q err=%v", ok, got.Status, err)
	}
	if _, ok, err := f.store.ClaimPendingAction(f.ctx, a.Uuid, f.dealer.ID, f.user.ID, nil, nil); err != nil || ok {
		t.Fatalf("second claim must get 0 rows: ok=%v err=%v", ok, err)
	}
	tag, err := f.tx.Exec(f.ctx, `UPDATE ai_pending_actions SET status = 'executing'
		WHERE uuid = $1 AND status = 'pending'`, a.Uuid)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("raw CAS after claim: rows=%d err=%v", tag.RowsAffected(), err)
	}

	done, err := f.q.ResolveAIPendingAction(f.ctx, db.ResolveAIPendingActionParams{
		ID: got.ID, Status: model.ActionConfirmed, Result: []byte(`{"ok":true}`),
	})
	if err != nil || done.Status != model.ActionConfirmed || !done.ResolvedAt.Valid {
		t.Fatalf("resolve: %+v %v", done, err)
	}
	if _, err := f.q.ResolveAIPendingAction(f.ctx, db.ResolveAIPendingActionParams{
		ID: got.ID, Status: model.ActionFailed,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second resolve: %v", err)
	}

	// An expired action is never claimed; the cleanup marks it expired.
	old := f.action(t, "old", time.Hour)
	if _, err := f.tx.Exec(f.ctx, `UPDATE ai_pending_actions
		SET created_at = NOW() - interval '2 hours', expires_at = NOW() - interval '1 minute' WHERE id = $1`, old.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := f.store.ClaimPendingAction(f.ctx, old.Uuid, f.dealer.ID, f.user.ID, nil, nil); err != nil || ok {
		t.Fatalf("expired claimed: ok=%v err=%v", ok, err)
	}
	if n, err := f.q.ExpireAIPendingActions(f.ctx, pgtype.Timestamptz{Time: time.Now(), Valid: true}); err != nil || n < 1 {
		t.Fatalf("expire: n=%d err=%v", n, err)
	}

	// A duplicate idempotency key is refused.
	if _, err := f.q.CreateAIPendingAction(f.ctx, db.CreateAIPendingActionParams{
		OrganizationID: f.dealer.ID, BrandID: f.brand.ID, UserID: f.user.ID, Source: model.SourcePanel,
		ToolUseID: "toolu_dup", ToolName: "create_task", Input: []byte(`{}`), Preview: []byte(`{}`),
		IdempotencyKey: a.IdempotencyKey,
		ExpiresAt:      pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err == nil {
		t.Fatal("duplicate idempotency key accepted")
	}
}

// Acceptance: the ledger insert raises ai_usage_monthly in the same
// transaction; rolling it back removes both.
func TestRecordUsageUpdatesMonthlyInSameTx(t *testing.T) {
	f := newFixture(t)
	uid := f.user.ID
	u := Usage{
		OrganizationID: f.dealer.ID, BrandID: f.brand.ID, Pool: model.PoolOrg, UserID: &uid,
		Channel: model.UsageChannelPanel, Purpose: model.PurposeChat, Model: "claude-sonnet-5-5",
		InputTokens: 100, OutputTokens: 40, CacheReadTokens: 1000, CacheWriteTokens: 10,
	}
	now := time.Now()
	row, month, err := f.store.RecordUsage(f.ctx, u)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if row.QuotaTokens != 150 || month.QuotaTokens != 150 || month.RequestCount != 1 ||
		month.CacheReadTokens != 1000 || month.Period != Period(now) {
		t.Fatalf("first record: row=%+v month=%+v", row, month)
	}
	if _, month, err = f.store.RecordUsage(f.ctx, u); err != nil || month.QuotaTokens != 300 || month.RequestCount != 2 {
		t.Fatalf("second record: %+v %v", month, err)
	}
	if got, err := f.store.MonthlyTokens(f.ctx, f.dealer.ID, model.PoolOrg, now); err != nil || got != 300 {
		t.Fatalf("monthly tokens = %d, %v", got, err)
	}
	// The system pool of the center is a separate projection row.
	sys := u
	sys.OrganizationID, sys.Pool, sys.UserID, sys.Channel = f.center.ID, model.PoolSystem, nil, model.UsageChannelWhatsApp
	if _, month, err := f.store.RecordUsage(f.ctx, sys); err != nil || month.QuotaTokens != 150 {
		t.Fatalf("system pool: %+v %v", month, err)
	}
	if got, _ := f.store.MonthlyTokens(f.ctx, f.center.ID, model.PoolOrg, now); got != 0 {
		t.Fatalf("center org pool = %d, want 0", got)
	}

	// Same transaction: a rolled-back caller transaction leaves neither the
	// ledger row nor the projection increment.
	inner, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, m, err := RecordUsageTx(f.ctx, db.New(inner), u); err != nil || m.QuotaTokens != 450 {
		t.Fatalf("tx record: %+v %v", m, err)
	}
	if err := inner.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.store.MonthlyTokens(f.ctx, f.dealer.ID, model.PoolOrg, now); got != 300 {
		t.Fatalf("after rollback monthly = %d, want 300", got)
	}
	var n int64
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM ai_usage WHERE organization_id = $1`, f.dealer.ID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("ledger rows = %d, %v", n, err)
	}

	// A failing ledger insert writes no projection either.
	bad := u
	bad.Channel = "sms"
	if _, _, err := f.store.RecordUsage(f.ctx, bad); err == nil {
		t.Fatal("invalid channel accepted")
	}
	if got, _ := f.store.MonthlyTokens(f.ctx, f.dealer.ID, model.PoolOrg, now); got != 300 {
		t.Fatalf("after failed insert monthly = %d", got)
	}

	// The ledger is append-only.
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sp.Exec(f.ctx, `UPDATE ai_usage SET input_tokens = 0 WHERE id = $1`, row.ID); err == nil {
		t.Fatal("ai_usage update accepted")
	}
	_ = sp.Rollback(f.ctx)
}

// Acceptance: the usage list rejects sort fields outside the whitelist and
// orders equal values by id in the sort direction.
func TestListUsageSortAndFilters(t *testing.T) {
	f := newFixture(t)
	uid, oid := f.user.ID, f.other.ID
	org := []int64{f.dealer.ID}
	rec := func(user *int64, purpose, mdl string, in int64) int64 {
		row, _, err := f.store.RecordUsage(f.ctx, Usage{
			OrganizationID: f.dealer.ID, BrandID: f.brand.ID, Pool: model.PoolOrg, UserID: user,
			Channel: model.UsageChannelPanel, Purpose: purpose, Model: mdl, InputTokens: in,
		})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		return row.ID
	}
	a := rec(&uid, model.PurposeChat, "m-a", 50)
	b := rec(&uid, model.PurposeChat, "m-a", 50)
	c := rec(&oid, model.PurposeTitle, "m-b", 10)
	d := rec(&uid, model.PurposeChat, "m-a", 50)

	if _, _, err := f.store.ListUsage(f.ctx, UsageFilter{
		OrganizationIDs: org, Sort: []apiquery.SortField{{Field: "user_id"}}, Limit: 10,
	}); err == nil {
		t.Fatal("sort outside the whitelist accepted")
	} else {
		var ve *apiquery.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("want *apiquery.ValidationError, got %T %v", err, err)
		}
	}

	ids := func(f UsageFilter, store *Store, ctx context.Context) ([]int64, int64) {
		t.Helper()
		rows, total, err := store.ListUsage(ctx, f)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		out := make([]int64, len(rows))
		for i, r := range rows {
			out[i] = r.ID
		}
		return out, total
	}
	eq := func(name string, got []int64, want ...int64) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
	got, total := ids(UsageFilter{OrganizationIDs: org, Sort: []apiquery.SortField{{Field: "tokens"}}, Limit: 10}, f.store, f.ctx)
	eq("tokens asc", got, c, a, b, d)
	if total != 4 {
		t.Fatalf("total = %d", total)
	}
	got, _ = ids(UsageFilter{OrganizationIDs: org, Sort: []apiquery.SortField{{Field: "tokens", Desc: true}}, Limit: 10}, f.store, f.ctx)
	eq("tokens desc", got, d, b, a, c)
	// Stable pages over equal values.
	p1, _ := ids(UsageFilter{OrganizationIDs: org, Sort: []apiquery.SortField{{Field: "model"}}, Limit: 2}, f.store, f.ctx)
	p2, _ := ids(UsageFilter{OrganizationIDs: org, Sort: []apiquery.SortField{{Field: "model"}}, Limit: 2, Offset: 2}, f.store, f.ctx)
	eq("model pages", append(p1, p2...), a, b, d, c)
	// Default -created_at (one transaction: equal timestamps, id desc).
	got, _ = ids(UsageFilter{OrganizationIDs: org, Limit: 10}, f.store, f.ctx)
	eq("default", got, d, c, b, a)

	// Filters.
	got, total = ids(UsageFilter{OrganizationIDs: org, UserIDs: []int64{oid}, Limit: 10}, f.store, f.ctx)
	eq("user", got, c)
	if total != 1 {
		t.Fatalf("filtered total = %d", total)
	}
	got, _ = ids(UsageFilter{OrganizationIDs: org, Purposes: []string{model.PurposeChat}, Models: []string{"m-a"},
		Channels: []string{model.UsageChannelPanel}, Pools: []string{model.PoolOrg}, Limit: 10}, f.store, f.ctx)
	eq("purpose+model", got, d, b, a)
	lo, hi := int64(11), int64(50)
	got, _ = ids(UsageFilter{OrganizationIDs: org, TokensMin: &lo, TokensMax: &hi, Limit: 10}, f.store, f.ctx)
	eq("tokens range", got, d, b, a)
	future := time.Now().Add(time.Hour)
	got, _ = ids(UsageFilter{OrganizationIDs: org, Created: apiquery.TimeRange{From: &future}, Limit: 10}, f.store, f.ctx)
	eq("created_from", got)
	got, _ = ids(UsageFilter{OrganizationIDs: org, BrandID: &f.brand.ID, Created: apiquery.TimeRange{Before: &future}, Limit: 10}, f.store, f.ctx)
	eq("created_to", got, d, c, b, a)
}

// The conversation list: own conversations of a channel, q in the title,
// sort whitelist and -updated_at default.
func TestListConversations(t *testing.T) {
	f := newFixture(t)
	conv := func(user int64, channel, title string) db.AiConversation {
		c, err := f.q.CreateAIConversation(f.ctx, db.CreateAIConversationParams{
			OrganizationID: f.dealer.ID, BrandID: f.brand.ID, UserID: user, Channel: channel, Title: title,
		})
		if err != nil {
			t.Fatalf("conversation: %v", err)
		}
		return c
	}
	beta := conv(f.user.ID, model.ChannelPanel, "Beta stok")
	alpha := conv(f.user.ID, model.ChannelPanel, "Alpha randevu")
	gamma := conv(f.user.ID, model.ChannelPanel, "Gamma stok")
	conv(f.other.ID, model.ChannelPanel, "Foreign")
	conv(f.user.ID, model.ChannelPortal, "Portal")
	deleted := conv(f.user.ID, model.ChannelPanel, "Deleted")
	if n, err := f.q.SoftDeleteAIConversation(f.ctx, db.SoftDeleteAIConversationParams{
		Uuid: deleted.Uuid, OrganizationID: f.dealer.ID, UserID: f.user.ID,
	}); err != nil || n != 1 {
		t.Fatalf("soft delete: %d %v", n, err)
	}
	if _, err := f.q.CreateAIMessage(f.ctx, db.CreateAIMessageParams{
		ConversationID: beta.ID, OrganizationID: f.dealer.ID, BrandID: f.brand.ID,
		Role: model.RoleUser, Status: model.MessageComplete, Content: []byte(`[]`), Ui: []byte(`[]`),
	}); err != nil {
		t.Fatalf("message: %v", err)
	}
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(sp).CreateAIMessage(f.ctx, db.CreateAIMessageParams{
		ConversationID: beta.ID, OrganizationID: f.center.ID, BrandID: f.brand.ID,
		Role: model.RoleUser, Status: model.MessageComplete, Content: []byte(`[]`), Ui: []byte(`[]`),
	}); err == nil {
		t.Fatal("message with a foreign organization accepted")
	}
	_ = sp.Rollback(f.ctx)

	list := func(q string, sort ...apiquery.SortField) []string {
		t.Helper()
		rows, total, err := f.store.ListConversations(f.ctx, ConversationFilter{
			OrganizationID: f.dealer.ID, UserID: f.user.ID, Channel: model.ChannelPanel, Q: q, Sort: sort, Limit: 10,
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if int(total) != len(rows) {
			t.Fatalf("total %d != %d rows", total, len(rows))
		}
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.Title
		}
		return out
	}
	// Default -updated_at; one transaction gives equal timestamps, so the id
	// tiebreak (desc) decides.
	if got := fmt.Sprint(list("")); got != fmt.Sprint([]string{gamma.Title, alpha.Title, beta.Title}) {
		t.Fatalf("default sort = %s", got)
	}
	if got := fmt.Sprint(list("", apiquery.SortField{Field: "title"})); got != fmt.Sprint([]string{alpha.Title, beta.Title, gamma.Title}) {
		t.Fatalf("title sort = %s", got)
	}
	if got := fmt.Sprint(list("STOK", apiquery.SortField{Field: "title", Desc: true})); got != fmt.Sprint([]string{gamma.Title, beta.Title}) {
		t.Fatalf("q = %s", got)
	}
	if _, _, err := f.store.ListConversations(f.ctx, ConversationFilter{
		OrganizationID: f.dealer.ID, UserID: f.user.ID, Channel: model.ChannelPanel,
		Sort: []apiquery.SortField{{Field: "message_count"}}, Limit: 10,
	}); err == nil {
		t.Fatal("sort outside the whitelist accepted")
	}
}

// The migration seeds the platform singleton with the documented defaults.
func TestAISettingsDefaults(t *testing.T) {
	f := newFixture(t)
	s, err := f.q.GetAISettings(f.ctx)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if s.DefaultMonthlyTokenQuota != 2000000 || s.DefaultModel == "" || s.FastModel == "" || string(s.ToolToggles) != "{}" {
		t.Fatalf("defaults: %+v", s)
	}
	if _, err := f.tx.Exec(f.ctx, `INSERT INTO ai_settings (id) VALUES (2)`); err == nil {
		t.Fatal("second ai_settings row accepted")
	}
}

// TEC-387: a confirmation with edits stores the edited input and preview in
// the same compare-and-set; a new message cancels only the user's pending
// cards of that conversation.
func TestClaimWithEditsAndCancelForSource(t *testing.T) {
	f := newFixture(t)
	a := f.action(t, "edit", time.Hour)
	got, ok, err := f.store.ClaimPendingAction(f.ctx, a.Uuid, f.dealer.ID, f.user.ID,
		[]byte(`{"title":"edited"}`), []byte(`{"summary":"edited"}`))
	if err != nil || !ok || string(got.Input) != `{"title": "edited"}` || string(got.Preview) != `{"summary": "edited"}` {
		t.Fatalf("claim with edits: ok=%v input=%s preview=%s err=%v", ok, got.Input, got.Preview, err)
	}

	open1, open2 := f.action(t, "src1", time.Hour), f.action(t, "src2", time.Hour)
	executing := f.action(t, "src3", time.Hour)
	if _, ok, _ := f.store.ClaimPendingAction(f.ctx, executing.Uuid, f.dealer.ID, f.user.ID, nil, nil); !ok {
		t.Fatal("claim")
	}
	params := db.CancelAIPendingActionsForSourceParams{
		Source: model.SourcePanel, SourceRef: pgtype.Text{String: "conv-1", Valid: true},
		OrganizationID: f.dealer.ID, UserID: f.other.ID,
	}
	if rows, err := f.store.CancelPendingActionsForSource(f.ctx, params); err != nil || len(rows) != 0 {
		t.Fatalf("other user's message cancelled %d cards (%v)", len(rows), err)
	}
	params.UserID = f.user.ID
	rows, err := f.store.CancelPendingActionsForSource(f.ctx, params)
	if err != nil || len(rows) != 2 {
		t.Fatalf("cancel for source: %d rows, err %v", len(rows), err)
	}
	for _, r := range rows {
		if (r.Uuid != open1.Uuid && r.Uuid != open2.Uuid) || r.Status != model.ActionCancelled || !r.ResolvedAt.Valid {
			t.Fatalf("cancelled row: %+v", r)
		}
	}
	if list, _ := f.store.ListPendingActions(f.ctx, f.dealer.ID, f.user.ID); len(list) != 0 {
		t.Fatalf("pending after cancel: %d", len(list))
	}
}

// TEC-387 acceptance: the sweep queries move each row once; running them
// again changes nothing.
func TestSweepQueriesTransitionOnce(t *testing.T) {
	f := newFixture(t)
	pending := f.action(t, "exp", time.Minute)
	running := f.action(t, "stale", time.Hour)
	if _, ok, _ := f.store.ClaimPendingAction(f.ctx, running.Uuid, f.dealer.ID, f.user.ID, nil, nil); !ok {
		t.Fatal("claim")
	}
	later := time.Now().Add(2 * time.Minute)

	if _, ok, err := f.store.ExpirePendingAction(f.ctx, pending.ID, time.Now().Add(-time.Hour)); err != nil || ok {
		t.Fatalf("expired a card before its expiry: ok=%v err=%v", ok, err)
	}
	n, err := f.store.ExpirePendingActions(f.ctx, later)
	if err != nil || n < 1 {
		t.Fatalf("first expire: %d %v", n, err)
	}
	if n, err := f.store.ExpirePendingActions(f.ctx, later); err != nil || n != 0 {
		t.Fatalf("second expire changed %d rows (%v)", n, err)
	}
	if _, ok, _ := f.store.ExpirePendingAction(f.ctx, pending.ID, later); ok {
		t.Fatal("single expire after the sweep must find nothing")
	}

	stale, err := f.store.FailStalePendingActions(f.ctx, later, "outcome unknown")
	if err != nil || len(stale) != 1 || stale[0].Uuid != running.Uuid || stale[0].Status != model.ActionFailed ||
		stale[0].Error.String != "outcome unknown" || !stale[0].ResolvedAt.Valid {
		t.Fatalf("first fail-stale: %+v %v", stale, err)
	}
	if again, err := f.store.FailStalePendingActions(f.ctx, later, "outcome unknown"); err != nil || len(again) != 0 {
		t.Fatalf("second fail-stale changed %d rows (%v)", len(again), err)
	}
	row, ok, err := f.store.PendingActionForUser(f.ctx, pending.Uuid, f.dealer.ID, f.user.ID)
	if err != nil || !ok || row.Status != model.ActionExpired {
		t.Fatalf("pending row: %+v ok=%v err=%v", row, ok, err)
	}
	if _, ok, _ := f.store.PendingActionForUser(f.ctx, pending.Uuid, f.dealer.ID, f.other.ID); ok {
		t.Fatal("another user's lookup found the action")
	}
	byKey, ok, err := f.store.PendingActionByKey(f.ctx, pending.IdempotencyKey)
	if err != nil || !ok || byKey.Uuid != pending.Uuid {
		t.Fatalf("by key: %+v %v %v", byKey, ok, err)
	}
}
