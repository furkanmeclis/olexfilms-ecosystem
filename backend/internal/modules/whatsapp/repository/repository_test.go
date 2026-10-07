package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-393 acceptance. Every test runs in one rolled-back transaction; the
// conversations of a test share a unique phone prefix, which the q filter
// uses to keep other rows of the database out of the list assertions.

type fixture struct {
	ctx    context.Context
	tx     pgx.Tx
	q      *db.Queries
	store  *Store
	prefix string
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	// 9 unique digits; each conversation appends two more (E.164 max 15).
	prefix := fmt.Sprintf("+90%09d", time.Now().UnixNano()%1_000_000_000)
	return &fixture{ctx: ctx, tx: tx, q: db.New(tx), store: New(tx), prefix: prefix}
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f *fixture) conversation(t *testing.T, name string, lastMessage *time.Time) db.Conversation {
	t.Helper()
	f.seq++
	c, err := f.q.UpsertConversation(f.ctx, db.UpsertConversationParams{
		Channel: "whatsapp", ContactE164: fmt.Sprintf("%s%02d", f.prefix, f.seq),
		ContactName: pgtype.Text{String: name, Valid: true}, LastMessageAt: tsNarg(lastMessage),
	})
	if err != nil {
		t.Fatalf("conversation %s: %v", name, err)
	}
	return c
}

func (f *fixture) message(t *testing.T, convID int64, senderType string, at time.Time) db.Message {
	t.Helper()
	f.seq++
	direction := "in"
	if senderType != model.SenderContact {
		direction = "out"
	}
	m, err := f.q.InsertMessage(f.ctx, db.InsertMessageParams{
		ConversationID: convID, Channel: "whatsapp", Direction: direction, SenderType: senderType,
		ExternalID: fmt.Sprintf("tec393-%s-%d", f.prefix, f.seq), Body: pgtype.Text{String: "hi", Valid: true},
		Status: "received",
	})
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	f.exec(t, `UPDATE messages SET created_at = $2 WHERE id = $1`, m.ID, at)
	m.CreatedAt = pgtype.Timestamptz{Time: at, Valid: true}
	return m
}

func ids(rows []db.Conversation) []int64 {
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func eqIDs(t *testing.T, label string, got []db.Conversation, want ...db.Conversation) {
	t.Helper()
	g, w := ids(got), ids(want)
	if fmt.Sprint(g) != fmt.Sprint(w) {
		t.Fatalf("%s: got %v, want %v", label, g, w)
	}
}

func TestConversationListSortFilterTiebreak(t *testing.T) {
	f := newFixture(t)
	t1 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	a := f.conversation(t, "Ayşe Yılmaz", &t1)
	b := f.conversation(t, "Bora", &t2)
	c := f.conversation(t, "Cem", nil)
	d := f.conversation(t, "Deniz", &t2)

	user, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "TEC393", Surname: "Atanan", Status: "active",
		Email: pgtype.Text{String: "tec393-" + f.prefix[1:] + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	created := t1.Add(-24 * time.Hour)
	f.exec(t, `UPDATE conversations SET created_at = $1 WHERE id = ANY($2)`, created, []int64{a.ID, b.ID, c.ID, d.ID})
	f.exec(t, `UPDATE conversations SET unread_count = 2 WHERE id = ANY($1)`, []int64{a.ID, d.ID})
	if _, err := f.q.SetConversationStatus(f.ctx, db.SetConversationStatusParams{ID: b.ID, Status: model.StatusClosed}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.SetConversationStatus(f.ctx, db.SetConversationStatusParams{ID: d.ID, Status: model.StatusPending}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.SetConversationAIMode(f.ctx, db.SetConversationAIModeParams{ID: d.ID, AiMode: model.AIModeOff}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.AssignConversation(f.ctx, db.AssignConversationParams{
		ID: d.ID, AssignedUserID: pgtype.Int8{Int64: user.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.SetConversationIdentity(f.ctx, db.SetConversationIdentityParams{
		ID: c.ID, IdentityKind: model.IdentityVisitor, IdentityResolvedAt: pgtype.Timestamptz{Time: t1, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.SetConversationIdentity(f.ctx, db.SetConversationIdentityParams{
		ID: a.ID, IdentityKind: model.IdentityCustomer, IdentityUserID: pgtype.Int8{Int64: user.ID, Valid: true},
		IdentityResolvedAt: pgtype.Timestamptz{Time: t1, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	list := func(label string, fl ConversationFilter, want ...db.Conversation) {
		t.Helper()
		if fl.Q == "" {
			fl.Q = f.prefix
		}
		fl.Limit = 50
		rows, total, err := f.store.ListConversations(f.ctx, fl)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		eqIDs(t, label, rows, want...)
		if total != int64(len(want)) {
			t.Fatalf("%s: total %d, want %d", label, total, len(want))
		}
	}
	sortBy := func(s string) []apiquery.SortField {
		if s[0] == '-' {
			return []apiquery.SortField{{Field: s[1:], Desc: true}}
		}
		return []apiquery.SortField{{Field: s}}
	}

	// Default -last_message_at: equal times by id desc, empty last.
	list("default", ConversationFilter{}, d, b, a, c)
	list("last_message_at asc", ConversationFilter{Sort: sortBy("last_message_at")}, a, b, d, c)
	list("unread_count asc", ConversationFilter{Sort: sortBy("unread_count")}, b, c, a, d)
	list("unread_count desc", ConversationFilter{Sort: sortBy("-unread_count")}, d, a, c, b)
	list("created_at asc (tiebreak)", ConversationFilter{Sort: sortBy("created_at")}, a, b, c, d)
	list("created_at desc (tiebreak)", ConversationFilter{Sort: sortBy("-created_at")}, d, c, b, a)

	var verr *apiquery.ValidationError
	if _, _, err := f.store.ListConversations(f.ctx, ConversationFilter{Sort: sortBy("contact_name"), Limit: 10}); !errors.As(err, &verr) {
		t.Fatalf("unknown sort: %v", err)
	}

	yes, no := true, false
	list("status", ConversationFilter{Statuses: []string{model.StatusClosed, model.StatusPending}}, d, b)
	list("unread=true", ConversationFilter{Unread: &yes}, d, a)
	list("unread=false", ConversationFilter{Unread: &no}, b, c)
	list("ai_mode", ConversationFilter{AIModes: []string{model.AIModeOff}}, d)
	list("identity_kind", ConversationFilter{IdentityKinds: []string{model.IdentityVisitor, model.IdentityCustomer}}, a, c)
	list("assigned_user", ConversationFilter{AssignedUserIDs: []int64{user.ID}}, d)
	before := t2.Add(time.Second)
	list("last_message range", ConversationFilter{LastMessage: apiquery.TimeRange{From: &t2, Before: &before}}, d, b)
	list("channel", ConversationFilter{Channel: "sms"})

	// q: contact name, identity user name, phone digits.
	only := func(label, q string, want ...db.Conversation) {
		t.Helper()
		rows, _, err := f.store.ListConversations(f.ctx, ConversationFilter{Q: q, Limit: 50,
			Statuses: []string{model.StatusOpen, model.StatusPending, model.StatusClosed}})
		if err != nil {
			t.Fatal(err)
		}
		mine := rows[:0]
		for _, r := range rows {
			for _, w := range want {
				if r.ID == w.ID {
					mine = append(mine, r)
				}
			}
		}
		eqIDs(t, label, mine, want...)
	}
	only("q contact name", "ayşe yıl", a)
	only("q phone digits", f.prefix[1:]+"03", c)
	only("q identity user", "TEC393 Atanan", a)
	// Paging keeps the order and the total.
	rows, total, err := f.store.ListConversations(f.ctx, ConversationFilter{Q: f.prefix, Limit: 2, Offset: 2})
	if err != nil || total != 4 {
		t.Fatalf("page: total=%d err=%v", total, err)
	}
	eqIDs(t, "page 2", rows, a, c)
}

func TestConversationCountersAndDefaults(t *testing.T) {
	f := newFixture(t)
	conv := f.conversation(t, "Sayaç", nil)
	if conv.Status != model.StatusOpen || conv.AiMode != model.AIModeAuto ||
		conv.IdentityKind != model.IdentityUnknown || conv.UnreadCount != 0 {
		t.Fatalf("defaults = %+v", conv)
	}
	at := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	if _, err := f.q.SetConversationStatus(f.ctx, db.SetConversationStatusParams{ID: conv.ID, Status: model.StatusClosed}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		var err error
		conv, err = f.q.TouchConversationInbound(f.ctx, db.TouchConversationInboundParams{
			ID: conv.ID, At: pgtype.Timestamptz{Time: at.Add(time.Duration(i) * time.Minute), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if conv.UnreadCount != 2 || conv.Status != model.StatusOpen ||
		!conv.LastInboundAt.Time.Equal(at.Add(time.Minute)) || !conv.LastMessageAt.Time.Equal(at.Add(time.Minute)) {
		t.Fatalf("after inbound = unread %d status %s inbound %v last %v",
			conv.UnreadCount, conv.Status, conv.LastInboundAt.Time, conv.LastMessageAt.Time)
	}
	// An older outbound time never moves last_message_at back.
	conv, err := f.q.TouchConversationOutbound(f.ctx, db.TouchConversationOutboundParams{
		ID: conv.ID, At: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil || !conv.LastMessageAt.Time.Equal(at.Add(time.Minute)) || conv.UnreadCount != 2 {
		t.Fatalf("after outbound = %+v %v", conv, err)
	}
	if conv, err = f.q.MarkConversationRead(f.ctx, conv.ID); err != nil || conv.UnreadCount != 0 {
		t.Fatalf("read = %d %v", conv.UnreadCount, err)
	}

	// AI pause: paused_until only with paused; expired pauses resume.
	conv, err = f.q.SetConversationAIMode(f.ctx, db.SetConversationAIModeParams{
		ID: conv.ID, AiMode: model.AIModePaused, AiPausedUntil: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil || !conv.AiPausedUntil.Valid {
		t.Fatalf("pause = %+v %v", conv, err)
	}
	if n, err := f.q.ResumeExpiredAIPauses(f.ctx, pgtype.Timestamptz{Time: at.Add(time.Second), Valid: true}); err != nil || n < 1 {
		t.Fatalf("resume = %d %v", n, err)
	}
	if conv, _ = f.q.GetConversationByID(f.ctx, conv.ID); conv.AiMode != model.AIModeAuto || conv.AiPausedUntil.Valid {
		t.Fatalf("after resume = %s %v", conv.AiMode, conv.AiPausedUntil)
	}
	conv, err = f.q.SetConversationAIMode(f.ctx, db.SetConversationAIModeParams{
		ID: conv.ID, AiMode: model.AIModeOff, AiPausedUntil: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil || conv.AiPausedUntil.Valid {
		t.Fatalf("off keeps no pause time: %+v %v", conv, err)
	}
}

func TestMessageCursorTimeline(t *testing.T) {
	f := newFixture(t)
	conv := f.conversation(t, "Akış", nil)
	at := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	// m1 < m2 = m3 (same time, id tiebreak) < m4 < m5
	m1 := f.message(t, conv.ID, model.SenderContact, at)
	m2 := f.message(t, conv.ID, model.SenderContact, at.Add(time.Minute))
	m3 := f.message(t, conv.ID, model.SenderStaff, at.Add(time.Minute))
	m4 := f.message(t, conv.ID, model.SenderContact, at.Add(2*time.Minute))
	m5 := f.message(t, conv.ID, model.SenderContact, at.Add(3*time.Minute))

	var seen []int64
	var cur *MessageCursor
	for page := 0; page < 5; page++ {
		rows, next, err := f.store.ListMessagesBefore(f.ctx, conv.ID, cur, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			seen = append(seen, r.ID)
		}
		if next == nil {
			break
		}
		cur = next
	}
	want := []int64{m5.ID, m4.ID, m3.ID, m2.ID, m1.ID}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("timeline = %v, want %v", seen, want)
	}
	after, err := f.store.ListMessagesAfter(f.ctx, conv.ID, CursorOf(m2), 10)
	if err != nil {
		t.Fatal(err)
	}
	got := []int64{}
	for _, r := range after {
		got = append(got, r.ID)
	}
	if fmt.Sprint(got) != fmt.Sprint([]int64{m3.ID, m4.ID, m5.ID}) {
		t.Fatalf("after m2 = %v", got)
	}
}

func TestAIRunUniquePerTriggerAndPurge(t *testing.T) {
	f := newFixture(t)
	conv := f.conversation(t, "AI", nil)
	now := time.Now().UTC()
	trigger := f.message(t, conv.ID, model.SenderContact, now)
	run, ok, err := f.store.StartAIRun(f.ctx, db.CreateConversationAIRunParams{
		ConversationID: conv.ID, TriggerMessageID: trigger.ID, Model: "claude-sonnet-5-5",
	})
	if err != nil || !ok || run.Status != model.RunRunning {
		t.Fatalf("start = %+v %v %v", run, ok, err)
	}
	if _, ok, err := f.store.StartAIRun(f.ctx, db.CreateConversationAIRunParams{
		ConversationID: conv.ID, TriggerMessageID: trigger.ID,
	}); err != nil || ok {
		t.Fatalf("second run for the same trigger: ok=%v err=%v", ok, err)
	}
	finished, err := f.q.FinishConversationAIRun(f.ctx, db.FinishConversationAIRunParams{
		ID: run.ID, Status: model.RunCompleted, Stages: []byte(`[{"stage":"reply"}]`), ToolCalls: []byte(`[]`),
		Model: "claude-sonnet-5-5", InputTokens: 10, OutputTokens: 5,
		FinishedAt: pgtype.Timestamptz{Time: run.StartedAt.Time.Add(1500 * time.Millisecond), Valid: true},
	})
	if err != nil || finished.Status != model.RunCompleted || !finished.DurationMs.Valid || finished.DurationMs.Int32 < 1400 {
		t.Fatalf("finish = %+v %v", finished, err)
	}
	if _, err := f.q.FinishConversationAIRun(f.ctx, db.FinishConversationAIRunParams{
		ID: run.ID, Status: model.RunFailed, Stages: []byte(`[]`), ToolCalls: []byte(`[]`),
		FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("finish twice: %v", err)
	}
	reply := f.message(t, conv.ID, model.SenderAI, now)
	if _, err := f.q.SetMessageAIRun(f.ctx, db.SetMessageAIRunParams{ID: reply.ID, AiRunID: pgtype.Int8{Int64: run.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}

	recentTrigger := f.message(t, conv.ID, model.SenderContact, now)
	recent, _, err := f.store.StartAIRun(f.ctx, db.CreateConversationAIRunParams{
		ConversationID: conv.ID, TriggerMessageID: recentTrigger.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE conversation_ai_runs SET created_at = $2 WHERE id = $1`, run.ID,
		now.AddDate(0, 0, -model.AIRunRetentionDays-1))

	n, err := f.store.PurgeAIRunsBefore(f.ctx, now.AddDate(0, 0, -model.AIRunRetentionDays))
	if err != nil || n < 1 {
		t.Fatalf("purge = %d %v", n, err)
	}
	if _, err := f.q.GetConversationAIRunByUUID(f.ctx, run.Uuid); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("old run kept: %v", err)
	}
	if _, err := f.q.GetConversationAIRunByUUID(f.ctx, recent.Uuid); err != nil {
		t.Fatalf("recent run purged: %v", err)
	}
	kept, err := f.q.GetMessageByUUID(f.ctx, reply.Uuid)
	if err != nil || kept.AiRunID.Valid || kept.Body.String != "hi" {
		t.Fatalf("reply after purge = %+v %v", kept, err)
	}
}

func TestOptOutProjectionReflectsLastRecord(t *testing.T) {
	f := newFixture(t)
	phone := f.prefix + "99"
	if out, err := f.store.IsOptedOut(f.ctx, phone, model.OptOutScopeMarketing); err != nil || out {
		t.Fatalf("no entry = %v %v", out, err)
	}
	record := func(scope, action, source string) db.ContactOptOutState {
		t.Helper()
		row, state, err := f.store.RecordOptOut(f.ctx, OptOut{ContactE164: phone, Scope: scope, Action: action, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		if state.LastEntryID != row.ID || state.Source != source {
			t.Fatalf("state %+v does not follow entry %d", state, row.ID)
		}
		return state
	}
	if s := record(model.OptOutScopeMarketing, model.OptOutActionOut, model.OptOutSourceWhatsApp); !s.OptedOut {
		t.Fatal("out not projected")
	}
	if s := record(model.OptOutScopeMarketing, model.OptOutActionIn, model.OptOutSourcePortal); s.OptedOut {
		t.Fatal("opt back in not projected")
	}
	record(model.OptOutScopeMarketing, model.OptOutActionOut, model.OptOutSourcePanel)
	record(model.OptOutScopeAI, model.OptOutActionOut, model.OptOutSourceWhatsApp)
	record(model.OptOutScopeAI, model.OptOutActionIn, model.OptOutSourceWhatsApp)

	if out, _ := f.store.IsOptedOut(f.ctx, phone, model.OptOutScopeMarketing); !out {
		t.Fatal("marketing should be opted out (last entry)")
	}
	if out, _ := f.store.IsOptedOut(f.ctx, phone, model.OptOutScopeAI); out {
		t.Fatal("ai should be opted in (last entry)")
	}
	states, err := f.q.ListContactOptOutStates(f.ctx, phone)
	if err != nil || len(states) != 2 {
		t.Fatalf("states = %+v %v", states, err)
	}
	history, err := f.q.ListContactOptOutHistory(f.ctx, db.ListContactOptOutHistoryParams{ContactE164: phone, LimitCount: 10})
	if err != nil || len(history) != 5 {
		t.Fatalf("history = %d %v", len(history), err)
	}
	optedOut, err := f.q.ListOptedOutContacts(f.ctx, db.ListOptedOutContactsParams{
		Scope: model.OptOutScopeMarketing, Contacts: []string{phone, f.prefix + "98"},
	})
	if err != nil || fmt.Sprint(optedOut) != fmt.Sprint([]string{phone}) {
		t.Fatalf("opted out subset = %v %v", optedOut, err)
	}

	// The ledger is append-only.
	for _, stmt := range []string{
		`UPDATE contact_opt_outs SET action = 'in' WHERE contact_e164 = $1`,
		`DELETE FROM contact_opt_outs WHERE contact_e164 = $1`,
	} {
		sp, err := f.tx.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sp.Exec(f.ctx, stmt, phone); err == nil {
			t.Fatalf("%s: want append-only error", stmt)
		}
		_ = sp.Rollback(f.ctx)
	}
	// Invalid scope is rejected by the CHECK.
	sp, _ := f.tx.Begin(f.ctx)
	if _, _, err := New(sp).RecordOptOut(f.ctx, OptOut{ContactE164: phone, Scope: "sms", Action: "out", Source: "panel"}); err == nil {
		t.Fatal("invalid scope accepted")
	}
	_ = sp.Rollback(f.ctx)
}
