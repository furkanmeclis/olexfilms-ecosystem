package pipeline_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	airepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	legal "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/pipeline"
	wausecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	wafake "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/fake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- fakes -------------------------------------------------------------------

// fakeSettings are the pipeline system settings.
type fakeSettings struct {
	pause int
	url   string
}

func (s *fakeSettings) WhatsAppAIStaffPauseMinutes(context.Context) int { return s.pause }
func (s *fakeSettings) WhatsAppAIGuidelinesURL(context.Context) string  { return s.url }

// fakeAccess grants ai.use (panel) to everybody.
type fakeAccess struct{}

func (fakeAccess) ResolveStoredAccess(context.Context, int64, *uuid.UUID) (authusecase.Access, error) {
	return authusecase.Access{Permissions: []string{rbac.PermAIUse, rbac.PermAIActionsConfirm}}, nil
}

// writeTool is a panel write tool that counts its confirmed runs.
type writeTool struct{ runs *atomic.Int32 }

func (writeTool) Spec() aitools.Spec {
	return aitools.Spec{
		Name: "create_test_task", Description: "Create a task.", Kind: aitools.KindWrite, Realm: aitools.RealmPanel,
		InputSchema: map[string]any{
			"type": "object", "required": []any{"title"},
			"properties": map[string]any{"title": map[string]any{"type": "string"}},
		},
	}
}

func (t writeTool) Run(context.Context, aitools.Env, json.RawMessage) (aitools.Result, error) {
	t.runs.Add(1)
	return aitools.Result{Content: "Task created."}, nil
}

func (writeTool) Propose(_ context.Context, _ aitools.Env, input json.RawMessage) (aitools.Proposal, *aitools.Result, error) {
	var in struct{ Title string }
	_ = json.Unmarshal(input, &in)
	return aitools.Proposal{Input: input, Preview: aitools.Preview{
		Summary: "Create the task " + in.Title + ".", Fields: []aitools.Field{{Key: "title", Value: in.Title}},
	}}, nil, nil
}

// cardTool previews like the real create_task tool: its summary is the
// ai.actions.summary.create_task catalog template (TEC-461).
type cardTool struct{ writeTool }

func (cardTool) Spec() aitools.Spec {
	s := writeTool{}.Spec()
	s.Name = "create_task"
	return s
}

func (cardTool) Propose(_ context.Context, _ aitools.Env, input json.RawMessage) (aitools.Proposal, *aitools.Result, error) {
	var in struct{ Title string }
	_ = json.Unmarshal(input, &in)
	return aitools.Proposal{Input: input, Preview: aitools.Preview{
		SummaryArgs: map[string]string{"title": in.Title, "subject": "Olex Merkez"},
		Fields:      []aitools.Field{{Key: "title", Value: in.Title}},
	}}, nil, nil
}

// --- fixture -------------------------------------------------------------------

type fixture struct {
	t        *testing.T
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	brand    db.Brand
	center   db.Organization
	llm      *fake.Provider
	settings *fakeSettings
	visitor  *fakeVisitorSettings
	toolRuns atomic.Int32
	pipe     *pipeline.Pipeline
	now      time.Time
	seq      atomic.Int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
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
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	f := &fixture{t: t, ctx: ctx, tx: tx, q: db.New(tx), llm: fake.New(), settings: &fakeSettings{pause: 30},
		visitor: &fakeVisitorSettings{}, now: time.Now()}
	if f.brand, err = f.q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatal(err)
	}
	if f.center, err = f.q.GetBrandCenter(ctx, f.brand.ID); err != nil {
		t.Fatal(err)
	}
	// The run reads the shared quota settings: no quota unless a test sets
	// one (the row lock is held until rollback).
	f.exec(`UPDATE ai_settings SET default_monthly_token_quota = 0, system_pool_monthly_quota = 0, tool_toggles = '{}'::jsonb`)

	models := llm.Models{Default: "claude-sonnet-5-5", Fast: "claude-haiku-4-5"}
	reg := aitools.NewRegistry(nil)
	reg.Register(writeTool{runs: &f.toolRuns})
	reg.Register(cardTool{writeTool{runs: &f.toolRuns}})
	// TEC-397: the visitor lead tool and a visitor tool leaking prices.
	reg.Register(pipeline.RequestDealerContact{})
	reg.Register(priceProbe{})
	store := airepo.New(tx)
	actions := aiusecase.NewActions(store, reg, nil, nil)
	chat := aiusecase.NewChat(aiusecase.ChatDeps{Store: store, Tools: reg, Actions: actions, Provider: f.llm, Models: models})
	msgs := wausecase.NewMessaging(wausecase.MessagingDeps{Queries: f.q, Tx: tx, Provider: &wafake.Provider{}})
	f.pipe = pipeline.New(pipeline.Deps{
		Queries: f.q, Agent: chat, Actions: actions,
		Identity: wausecase.NewIdentityResolver(f.q, nil, f.llm, models, nil),
		Sender:   msgs, Consents: legal.New(f.q), Access: fakeAccess{}, Settings: f.settings,
		DefaultBrandSlug: "olex",
		Dealers:          orgusecase.New(nil, f.q),
		Leads: leadsusecase.NewApplications(tx, f.q, geo.New(nil, f.q), allModules{},
			fakeBools{}, nil),
		VisitorSettings: f.visitor, FrontendURL: "https://app.example.test/",
	})
	f.pipe.SetClock(func() time.Time { return f.now })
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *fixture) uniq() string {
	return fmt.Sprintf("%07d", (time.Now().UnixNano()/1000+f.seq.Add(1))%10_000_000)
}

func (f *fixture) phone() string { return "+90533" + f.uniq() }

// conversation opens a Turkish conversation for phone.
func (f *fixture) conversation(phone string) db.Conversation {
	f.t.Helper()
	c, err := f.q.UpsertConversation(f.ctx, db.UpsertConversationParams{Channel: whatsapp.ChannelWhatsApp, ContactE164: phone})
	if err != nil {
		f.t.Fatal(err)
	}
	if c, err = f.q.SetConversationLocale(f.ctx, db.SetConversationLocaleParams{ID: c.ID, Locale: pgtype.Text{String: "tr", Valid: true}}); err != nil {
		f.t.Fatal(err)
	}
	return c
}

// consented marks a visitor conversation as having accepted the guidelines.
func (f *fixture) consented(c db.Conversation) db.Conversation {
	f.t.Helper()
	c, err := f.q.SetConversationAIConsent(f.ctx, db.SetConversationAIConsentParams{
		ID: c.ID, AiConsentAt: pgtype.Timestamptz{Time: f.now, Valid: true},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}

func (f *fixture) message(c db.Conversation, direction, sender, body string, media any) db.Message {
	f.t.Helper()
	p := db.InsertMessageParams{
		ConversationID: c.ID, Channel: whatsapp.ChannelWhatsApp, Direction: direction, SenderType: sender,
		ExternalID: "tec396-" + f.uniq() + uuid.NewString()[:8], Status: "received",
	}
	if body != "" {
		p.Body = pgtype.Text{String: body, Valid: true}
	}
	if media != nil {
		p.Media, _ = json.Marshal(media)
	}
	m, err := f.q.InsertMessage(f.ctx, p)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

func (f *fixture) inbound(c db.Conversation, body string) db.Message {
	return f.message(c, "in", model.SenderContact, body, nil)
}

func (f *fixture) process(c db.Conversation) {
	f.t.Helper()
	if err := f.pipe.Process(f.ctx, c.Uuid); err != nil {
		f.t.Fatalf("process: %v", err)
	}
}

// outgoing returns the queued AI / system messages, oldest first.
func (f *fixture) outgoing(c db.Conversation) []db.Message {
	f.t.Helper()
	rows, err := f.q.ListConversationMessagesBefore(f.ctx, db.ListConversationMessagesBeforeParams{ConversationID: c.ID, LimitCount: 200})
	if err != nil {
		f.t.Fatal(err)
	}
	var out []db.Message
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Direction == "out" && rows[i].SenderType != model.SenderStaff {
			out = append(out, rows[i])
		}
	}
	return out
}

func (f *fixture) runs(c db.Conversation) []db.ConversationAiRun {
	f.t.Helper()
	rows, err := f.q.ListConversationAIRuns(f.ctx, db.ListConversationAIRunsParams{ConversationID: c.ID, LimitCount: 50})
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func (f *fixture) user(phone string) db.User {
	f.t.Helper()
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "TEC396", Surname: f.uniq(), Status: "active",
		Email:     pgtype.Text{String: "tec396-" + f.uniq() + "@example.test", Valid: true},
		PhoneE164: pgtype.Text{String: phone, Valid: true},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return u
}

// panelUser is a dealer staff member who already accepted the guidelines.
func (f *fixture) panelUser(phone string) (db.User, db.Organization) {
	f.t.Helper()
	u := f.user(phone)
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: "tec396-" + f.uniq(), Name: "TEC396 Bayi", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true},
		BrandID: f.brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{OrganizationID: o.ID, UserID: u.ID, Role: "staff"}); err != nil {
		f.t.Fatal(err)
	}
	f.accept(u)
	return u, o
}

func (f *fixture) accept(u db.User) {
	f.t.Helper()
	txt, err := legal.New(f.q).AIConsentRequired(f.ctx, u.ID, "tr")
	if err != nil || txt == nil {
		f.t.Fatalf("guidelines text: %v %v", txt, err)
	}
	if _, err := legal.New(f.q).Decide(f.ctx, u.ID, legal.DecideInput{Kind: legal.KindAIGuidelines, Locale: txt.Locale, Version: txt.Version, Accepted: true}); err != nil {
		f.t.Fatal(err)
	}
}

func requestText(req llm.Request) string {
	var sb strings.Builder
	for _, m := range req.Messages {
		for _, b := range m.Content {
			sb.WriteString(b.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func bodies(msgs []db.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Body.String
	}
	return out
}

// --- acceptance ------------------------------------------------------------------

// Two messages a few seconds apart (one debounced task) are answered in a
// single AI turn; a second delivery of the task adds no run.
func TestTwoMessagesOneTurn(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.inbound(c, "Merhaba")
	f.inbound(c, "Seramik kaplama ne kadar sürer?")
	f.llm.Push(fake.Text("Genellikle bir gün sürer.", llm.Usage{InputTokens: 100, OutputTokens: 20}))

	f.process(c)
	f.process(c) // the same task again

	reqs := f.llm.Requests()
	if len(reqs) != 1 {
		t.Fatalf("model calls = %d, want 1", len(reqs))
	}
	txt := requestText(reqs[0])
	if !strings.Contains(txt, "Merhaba") || !strings.Contains(txt, "Seramik kaplama") {
		t.Fatalf("turn does not carry both messages:\n%s", txt)
	}
	out := f.outgoing(c)
	if len(out) != 1 || out[0].SenderType != model.SenderAI || out[0].Body.String != "Genellikle bir gün sürer." {
		t.Fatalf("outgoing = %v", bodies(out))
	}
	runs := f.runs(c)
	if len(runs) != 1 || runs[0].Status != model.RunCompleted || runs[0].InputTokens != 100 {
		t.Fatalf("runs = %+v", runs)
	}
	if !out[0].AiRunID.Valid || out[0].AiRunID.Int64 != runs[0].ID {
		t.Fatalf("answer not linked to the run: %+v", out[0].AiRunID)
	}
}

// The same event twice gives one run: the trigger message is UNIQUE.
func TestSameTriggerSingleRun(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	m := f.inbound(c, "Selam")
	if _, err := f.q.CreateConversationAIRun(f.ctx, db.CreateConversationAIRunParams{ConversationID: c.ID, TriggerMessageID: m.ID}); err != nil {
		t.Fatal(err)
	}
	// A concurrent run already holds the trigger: nothing new is created.
	if _, err := f.q.CreateConversationAIRun(f.ctx, db.CreateConversationAIRunParams{ConversationID: c.ID, TriggerMessageID: m.ID}); err != pgx.ErrNoRows {
		t.Fatalf("second run for the trigger: %v", err)
	}
	f.process(c)
	if n := len(f.runs(c)); n != 1 || len(f.llm.Requests()) != 0 {
		t.Fatalf("runs = %d, model calls = %d", n, len(f.llm.Requests()))
	}
}

// A staff message 10 minutes ago keeps the AI quiet (30 minute pause).
func TestStaffRecentlyActiveSkips(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.message(c, "out", model.SenderStaff, "Merhaba, ben Ayşe.", nil)
	f.inbound(c, "Teşekkürler")
	f.now = time.Now().Add(10 * time.Minute)

	f.process(c)

	if len(f.llm.Requests()) != 0 || len(f.outgoing(c)) != 0 {
		t.Fatalf("AI answered: calls %d, out %v", len(f.llm.Requests()), bodies(f.outgoing(c)))
	}
	runs := f.runs(c)
	if len(runs) != 1 || runs[0].Status != model.RunSkipped || !strings.Contains(string(runs[0].Stages), pipeline.ReasonStaffActive) {
		t.Fatalf("runs = %+v", runs)
	}

	// After the pause the AI answers again.
	f.inbound(c, "Bir sorum var")
	f.now = time.Now().Add(31 * time.Minute)
	f.llm.Push(fake.Text("Buyurun.", llm.Usage{}))
	f.process(c)
	if len(f.llm.Requests()) != 1 {
		t.Fatalf("model calls after the pause = %d", len(f.llm.Requests()))
	}
}

// Without consent only the guidelines question goes out; EVET records the
// consent and answers the question that waited.
func TestConsentGateAndEvet(t *testing.T) {
	f := newFixture(t)
	phone := f.phone()
	u := f.user(phone)
	f.exec(`INSERT INTO customer_profiles (user_id) VALUES ($1)`, u.ID)
	f.settings.url = "https://olex.test/yonerge"
	c := f.conversation(phone)
	f.inbound(c, "Garantim ne zaman bitiyor?")

	f.process(c)

	out := f.outgoing(c)
	if len(f.llm.Requests()) != 0 {
		t.Fatalf("model called before consent")
	}
	if len(out) != 1 || out[0].SenderType != model.SenderSystem ||
		!strings.Contains(out[0].Body.String, "EVET") || !strings.Contains(out[0].Body.String, "https://olex.test/yonerge") {
		t.Fatalf("consent prompt = %v", bodies(out))
	}

	f.inbound(c, "Evet")
	f.llm.Push(fake.Text("Garantiniz 2027'de bitiyor.", llm.Usage{InputTokens: 10}))
	f.process(c)

	reqs := f.llm.Requests()
	if len(reqs) != 1 || !strings.Contains(requestText(reqs[0]), "Garantim ne zaman bitiyor?") {
		t.Fatalf("question not answered after EVET: %d calls", len(reqs))
	}
	if strings.Contains(requestText(reqs[0]), "\nEvet\n") {
		t.Fatalf("the consent answer was sent to the model")
	}
	var accepted bool
	var ua string
	if err := f.tx.QueryRow(f.ctx, `SELECT accepted, COALESCE(user_agent, '') FROM consents WHERE user_id = $1 AND kind = 'ai_guidelines'`, u.ID).Scan(&accepted, &ua); err != nil {
		t.Fatalf("consent row: %v", err)
	}
	if !accepted || ua != "whatsapp" {
		t.Fatalf("consent accepted=%v source=%q", accepted, ua)
	}
	conv, _ := f.q.GetConversationByID(f.ctx, c.ID)
	if !conv.AiConsentAt.Valid {
		t.Fatal("ai_consent_at not set")
	}
	out = f.outgoing(c)
	if last := out[len(out)-1]; last.SenderType != model.SenderAI || last.Body.String != "Garantiniz 2027'de bitiyor." {
		t.Fatalf("answer = %v", bodies(out))
	}
	// Customer usage goes to the center's system pool on the whatsapp channel.
	var pool, channel string
	var org int64
	if err := f.tx.QueryRow(f.ctx, `SELECT pool, channel, organization_id FROM ai_usage WHERE user_id = $1 ORDER BY id DESC LIMIT 1`, u.ID).Scan(&pool, &channel, &org); err != nil {
		t.Fatal(err)
	}
	if pool != aimodel.PoolSystem || channel != aimodel.UsageChannelWhatsApp || org != f.center.ID {
		t.Fatalf("usage pool=%s channel=%s org=%d", pool, channel, org)
	}
}

// DUR writes the marketing opt-out without calling the model.
func TestStopKeywordOptsOut(t *testing.T) {
	f := newFixture(t)
	phone := f.phone()
	c := f.consented(f.conversation(phone))
	f.inbound(c, "DUR")

	f.process(c)

	if len(f.llm.Requests()) != 0 {
		t.Fatal("model called for DUR")
	}
	st, err := f.q.GetContactOptOutState(f.ctx, db.GetContactOptOutStateParams{ContactE164: phone, Scope: model.OptOutScopeMarketing})
	if err != nil || !st.OptedOut || st.Source != model.OptOutSourceWhatsApp {
		t.Fatalf("opt-out state %+v %v", st, err)
	}
	if out := f.outgoing(c); len(out) != 1 || out[0].SenderType != model.SenderSystem {
		t.Fatalf("confirmation = %v", bodies(out))
	}
}

// İNSAN pauses the AI and moves the conversation to pending.
func TestHumanKeywordHandsOver(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.inbound(c, "İnsan")

	f.process(c)

	conv, _ := f.q.GetConversationByID(f.ctx, c.ID)
	if conv.AiMode != model.AIModePaused || conv.Status != model.StatusPending || len(f.llm.Requests()) != 0 {
		t.Fatalf("conversation %s/%s, calls %d", conv.AiMode, conv.Status, len(f.llm.Requests()))
	}
	f.inbound(c, "Orada mısınız?")
	f.process(c)
	if len(f.llm.Requests()) != 0 {
		t.Fatal("paused conversation answered by the AI")
	}
}

// A write tool becomes a text card; EVET runs it exactly once, HAYIR
// cancels the next one.
func TestWriteToolConfirmAndCancel(t *testing.T) {
	f := newFixture(t)
	phone := f.phone()
	f.panelUser(phone)
	c := f.conversation(phone)

	f.inbound(c, "Yarın için Ahmet'i arama görevi oluştur")
	f.llm.Push(fake.ToolCall("tu-1", "create_test_task", map[string]any{"title": "Ahmet'i ara"}, llm.Usage{InputTokens: 5}))
	f.process(c)

	out := f.outgoing(c)
	if len(out) != 1 || !strings.Contains(out[0].Body.String, "EVET") || !strings.Contains(out[0].Body.String, "Ahmet'i ara") {
		t.Fatalf("card = %v", bodies(out))
	}
	if f.toolRuns.Load() != 0 {
		t.Fatal("tool ran before confirmation")
	}

	f.inbound(c, "EVET")
	f.process(c)
	f.process(c) // redelivered task
	if f.toolRuns.Load() != 1 {
		t.Fatalf("confirmed runs = %d, want 1", f.toolRuns.Load())
	}
	if len(f.llm.Requests()) != 1 {
		t.Fatalf("model called for the confirmation: %d", len(f.llm.Requests()))
	}
	var status string
	if err := f.tx.QueryRow(f.ctx, `SELECT status FROM ai_pending_actions WHERE source = 'whatsapp' AND source_ref = $1 AND tool_use_id = 'tu-1'`, c.Uuid.String()).Scan(&status); err != nil || status != aimodel.ActionConfirmed {
		t.Fatalf("action status %q %v", status, err)
	}

	f.inbound(c, "Bir görev daha: Mehmet'i ara")
	f.llm.Push(fake.ToolCall("tu-2", "create_test_task", map[string]any{"title": "Mehmet'i ara"}, llm.Usage{}))
	f.process(c)
	f.inbound(c, "hayır")
	f.process(c)
	if f.toolRuns.Load() != 1 {
		t.Fatalf("cancelled action ran: %d", f.toolRuns.Load())
	}
	if err := f.tx.QueryRow(f.ctx, `SELECT status FROM ai_pending_actions WHERE source = 'whatsapp' AND source_ref = $1 AND tool_use_id = 'tu-2'`, c.Uuid.String()).Scan(&status); err != nil || status != aimodel.ActionCancelled {
		t.Fatalf("action status %q %v", status, err)
	}
	out = f.outgoing(c)
	if last := out[len(out)-1].Body.String; !strings.Contains(last, "iptal") {
		t.Fatalf("cancel reply = %q", last)
	}
}

// TEC-461: the WhatsApp confirmation card (summary and field labels) is
// in the user's language: tr for a Turkish user, ar for an Arabic one.
func TestWriteCardInUserLanguage(t *testing.T) {
	cases := []struct {
		locale string
		want   []string
		absent []string
	}{
		{locale: "tr", want: []string{`*Olex Merkez için "Ahmet'i ara" görevi oluşturulsun.*`, "• Başlık: Ahmet'i ara", "EVET"},
			absent: []string{"Create the task", "• title:"}},
		{locale: "ar", want: []string{`*إنشاء المهمة "Ahmet'i ara" بخصوص Olex Merkez.*`, "• العنوان: Ahmet'i ara"},
			absent: []string{"Create the task", "görevi oluşturulsun", "• title:"}},
	}
	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			f := newFixture(t)
			phone := f.phone()
			u, _ := f.panelUser(phone)
			f.exec(`UPDATE users SET locale = $1 WHERE id = $2`, tc.locale, u.ID)
			c := f.conversation(phone)

			f.inbound(c, "Ahmet'i arama görevi oluştur")
			f.llm.Push(fake.ToolCall("tu-1", "create_task", map[string]any{"title": "Ahmet'i ara"}, llm.Usage{InputTokens: 5}))
			f.process(c)

			out := f.outgoing(c)
			if len(out) != 1 {
				t.Fatalf("messages = %v", bodies(out))
			}
			body := out[0].Body.String
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Fatalf("card lacks %q:\n%s", w, body)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(body, a) {
					t.Fatalf("card has %q:\n%s", a, body)
				}
			}
			// The stored card (panel list, MCP) carries the same summary.
			var summary string
			if err := f.tx.QueryRow(f.ctx, `SELECT preview->>'summary' FROM ai_pending_actions WHERE source = 'whatsapp' AND source_ref = $1`,
				c.Uuid.String()).Scan(&summary); err != nil || !strings.Contains(body, "*"+summary+"*") {
				t.Fatalf("stored summary %q %v", summary, err)
			}
		})
	}
}

// A voice message gets the fixed reply; the model is not called.
func TestVoiceMessageFixedReply(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.message(c, "in", model.SenderContact, "", map[string]any{"type": "audio", "mime_type": "audio/ogg"})

	f.process(c)

	if len(f.llm.Requests()) != 0 {
		t.Fatal("model called for a voice message")
	}
	out := f.outgoing(c)
	if len(out) != 1 || !strings.Contains(out[0].Body.String, "Sesli mesaj") {
		t.Fatalf("voice reply = %v", bodies(out))
	}
}

// A spent system pool sends the fixed text, hands over and never calls the
// model.
func TestQuotaExceededNoModelCall(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.exec(`UPDATE ai_settings SET system_pool_monthly_quota = 5`)
	if _, _, err := airepo.New(f.tx).RecordUsage(f.ctx, airepo.Usage{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, Pool: aimodel.PoolSystem, Channel: aimodel.UsageChannelWhatsApp,
		Purpose: aimodel.PurposeChat, Model: "claude-sonnet-5-5", InputTokens: 10,
	}); err != nil {
		t.Fatal(err)
	}
	f.inbound(c, "Merhaba")

	f.process(c)

	if len(f.llm.Requests()) != 0 {
		t.Fatal("model called with a spent quota")
	}
	conv, _ := f.q.GetConversationByID(f.ctx, c.ID)
	if conv.AiMode != model.AIModePaused || conv.Status != model.StatusPending || !conv.AiPausedUntil.Valid {
		t.Fatalf("not handed over: %s/%s", conv.AiMode, conv.Status)
	}
	if out := f.outgoing(c); len(out) != 1 || out[0].SenderType != model.SenderSystem {
		t.Fatalf("quota reply = %v", bodies(out))
	}
}

// A provider error answers with the localized "could not process" text and
// fails the run.
func TestProviderErrorFailureText(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.inbound(c, "Merhaba")
	f.llm.Push(fake.Turn{Err: llm.ErrUnavailable})

	f.process(c)

	out := f.outgoing(c)
	if len(out) != 1 || !strings.Contains(out[0].Body.String, "işleyemedik") {
		t.Fatalf("failure reply = %v", bodies(out))
	}
	if runs := f.runs(c); runs[0].Status != model.RunFailed {
		t.Fatalf("run status %s", runs[0].Status)
	}
}

// A long Markdown answer is converted and split into ≤ 4000 character parts.
func TestLongAnswerSplit(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.inbound(c, "Detaylı anlat")
	long := "**Başlık**\n\n" + strings.Repeat("Uzun bir cümle burada. ", 300)
	f.llm.Push(fake.Text(long, llm.Usage{}))

	f.process(c)

	out := f.outgoing(c)
	if len(out) < 2 {
		t.Fatalf("parts = %d", len(out))
	}
	for _, m := range out {
		if n := len([]rune(m.Body.String)); n > pipeline.MaxPartRunes {
			t.Fatalf("part of %d runes", n)
		}
	}
	if !strings.HasPrefix(out[0].Body.String, "*Başlık*") {
		t.Fatalf("markdown not converted: %q", out[0].Body.String[:20])
	}
}
