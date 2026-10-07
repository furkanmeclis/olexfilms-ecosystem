package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	legal "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ ChatStore = (*repository.Store)(nil)

// --- fakes -------------------------------------------------------------------

// chatStore keeps conversations, messages and the usage projection in
// memory.
type chatStore struct {
	mu       sync.Mutex
	now      func() time.Time
	settings db.AiSetting
	orgs     map[int64]db.AiOrgSetting
	monthly  map[string]int64
	usage    []repository.Usage
	events   []events.Event
	convs    []*db.AiConversation
	msgs     []*db.AiMessage
	nextID   int64
}

func newChatStore(now func() time.Time) *chatStore {
	return &chatStore{
		now: now, orgs: map[int64]db.AiOrgSetting{}, monthly: map[string]int64{},
		settings: db.AiSetting{DefaultModel: "claude-sonnet-5-5", FastModel: "claude-haiku-4-5",
			DefaultMonthlyTokenQuota: 2_000_000, SystemPoolMonthlyQuota: 5_000_000},
	}
}

func monthKey(orgID int64, pool, period string) string {
	return fmt.Sprintf("%s:%s:%d", pool, period, orgID)
}

func (s *chatStore) Settings(context.Context) (db.AiSetting, error) { return s.settings, nil }

func (s *chatStore) OrgSettings(_ context.Context, orgID int64) (db.AiOrgSetting, bool, error) {
	o, ok := s.orgs[orgID]
	return o, ok, nil
}

func (s *chatStore) MonthlyTokens(_ context.Context, orgID int64, pool string, at time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.monthly[monthKey(orgID, pool, repository.Period(at))], nil
}

func (s *chatStore) RecordUsageEvents(_ context.Context, u repository.Usage, _ repository.EventEnqueuer,
	build func(db.AiUsage, db.AiUsageMonthly) []events.Event) (db.AiUsage, db.AiUsageMonthly, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	quota := u.InputTokens + u.OutputTokens + u.CacheWriteTokens
	period := repository.Period(s.now())
	k := monthKey(u.OrganizationID, u.Pool, period)
	s.monthly[k] += quota
	s.usage = append(s.usage, u)
	row := db.AiUsage{OrganizationID: u.OrganizationID, BrandID: u.BrandID, Pool: u.Pool, Purpose: u.Purpose, QuotaTokens: quota}
	month := db.AiUsageMonthly{OrganizationID: u.OrganizationID, BrandID: u.BrandID, Pool: u.Pool, Period: period, QuotaTokens: s.monthly[k]}
	if build != nil {
		s.events = append(s.events, build(row, month)...)
	}
	return row, month, nil
}

func (s *chatStore) ListConversations(_ context.Context, f repository.ConversationFilter) ([]db.AiConversation, int64, error) {
	if _, err := apiquery.ResolveSort(f.Sort, repository.ConversationSort); err != nil {
		return nil, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []db.AiConversation
	for _, c := range s.convs {
		if c.OrganizationID == f.OrganizationID && c.UserID == f.UserID && c.Channel == f.Channel && !c.DeletedAt.Valid {
			out = append(out, *c)
		}
	}
	return out, int64(len(out)), nil
}

func (s *chatStore) CreateConversation(_ context.Context, p db.CreateAIConversationParams) (db.AiConversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	ts := pgtype.Timestamptz{Time: s.now(), Valid: true}
	c := &db.AiConversation{ID: s.nextID, Uuid: uuid.New(), OrganizationID: p.OrganizationID, BrandID: p.BrandID,
		UserID: p.UserID, Channel: p.Channel, Title: p.Title, CreatedAt: ts, UpdatedAt: ts}
	s.convs = append(s.convs, c)
	return *c, nil
}

func (s *chatStore) conv(id int64) *db.AiConversation {
	for _, c := range s.convs {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (s *chatStore) ConversationForUser(_ context.Context, id uuid.UUID, orgID, userID int64, channel string) (db.AiConversation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.convs {
		if c.Uuid == id && c.OrganizationID == orgID && c.UserID == userID && c.Channel == channel && !c.DeletedAt.Valid {
			return *c, true, nil
		}
	}
	return db.AiConversation{}, false, nil
}

func (s *chatStore) RenameConversation(_ context.Context, id int64, title string) (db.AiConversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.conv(id)
	c.Title = title
	return *c, nil
}

func (s *chatStore) DeleteConversation(_ context.Context, id uuid.UUID, orgID, userID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.convs {
		if c.Uuid == id && c.OrganizationID == orgID && c.UserID == userID && !c.DeletedAt.Valid {
			c.DeletedAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
			return true, nil
		}
	}
	return false, nil
}

func (s *chatStore) TouchConversation(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conv(id).MessageCount++
	return nil
}

func (s *chatStore) CreateMessage(_ context.Context, p db.CreateAIMessageParams) (db.AiMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	m := &db.AiMessage{ID: s.nextID, Uuid: uuid.New(), ConversationID: p.ConversationID, OrganizationID: p.OrganizationID,
		BrandID: p.BrandID, Role: p.Role, Status: p.Status, Content: p.Content, Ui: p.Ui}
	s.msgs = append(s.msgs, m)
	return *m, nil
}

func (s *chatStore) FinishMessage(_ context.Context, p db.FinishAIMessageParams) (db.AiMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.msgs {
		if m.ID == p.ID && m.Status == model.MessagePending {
			m.Status, m.Content, m.Ui, m.Model, m.Error = p.Status, p.Content, p.Ui, p.Model, p.Error
			m.InputTokens, m.OutputTokens = p.InputTokens, p.OutputTokens
			return *m, nil
		}
	}
	return db.AiMessage{}, errors.New("not pending")
}

func (s *chatStore) Messages(_ context.Context, conversationID int64) ([]db.AiMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []db.AiMessage
	for _, m := range s.msgs {
		if m.ConversationID == conversationID {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (s *chatStore) ChatContext(context.Context, int64, int64) (db.GetAIChatContextRow, error) {
	return db.GetAIChatContextRow{UserName: "Ayşe", UserSurname: "Kaya", UserLocale: "tr", OrgName: "Merkez",
		OrgType: "center", OrgLocale: "tr", OrgTimezone: "Europe/Istanbul", BrandName: "Olex"}, nil
}

func (s *chatStore) BrandCenter(_ context.Context, brandID int64) (db.Organization, error) {
	return db.Organization{ID: 10, BrandID: brandID, Type: "center"}, nil
}

type fakeFeatures map[int64]bool

func (f fakeFeatures) Enabled(_ context.Context, orgID int64, _ string) (bool, error) {
	return f[orgID], nil
}

type fakeConsents struct{ required map[int64]bool }

func (f fakeConsents) AIConsentRequired(_ context.Context, userID int64, _ i18n.Locale) (*legal.Text, error) {
	if f.required[userID] {
		return &legal.Text{Kind: legal.KindAIGuidelines, Locale: "tr", Version: 3, Body: "Yönerge"}, nil
	}
	return nil, nil
}

type countingLimiter struct {
	mu    sync.Mutex
	hits  map[string]int
	limit int
}

func (l *countingLimiter) Allow(_ context.Context, action, subject string, limit int, _ time.Duration) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[action+subject]++
	if l.limit > 0 {
		limit = l.limit
	}
	return l.hits[action+subject] <= limit, time.Minute
}

// stockTool is a panel read tool.
type stockTool struct{}

func (stockTool) Spec() tools.Spec {
	return tools.Spec{Name: "stock_summary", Description: "test read tool", Kind: tools.KindRead, Realm: tools.RealmPanel,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}}
}

func (stockTool) Run(context.Context, tools.Env, json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: `{"rolls":42}`}, nil
}

// vehiclesTool is a customer realm read tool.
type vehiclesTool struct{}

func (vehiclesTool) Spec() tools.Spec {
	return tools.Spec{Name: "my_vehicles", Description: "test customer tool", Kind: tools.KindRead, Realm: tools.RealmCustomer,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}}
}

func (vehiclesTool) Run(context.Context, tools.Env, json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: `{"items":[]}`}, nil
}

// --- harness -----------------------------------------------------------------

type chatHarness struct {
	*harness
	store    *chatStore
	llm      *fake.Provider
	features fakeFeatures
	consents fakeConsents
	limiter  *countingLimiter
	chat     *Chat
	caller   Caller
	conv     Conversation
}

const testUser = int64(7)

func newChatHarness(t *testing.T, turns ...fake.Turn) *chatHarness {
	h := &chatHarness{harness: newHarness(t, nil)}
	h.reg.Register(stockTool{})
	h.reg.Register(vehiclesTool{})
	clock := func() time.Time { return h.now }
	h.store = newChatStore(clock)
	h.llm = fake.New(turns...)
	h.features = fakeFeatures{10: true}
	h.consents = fakeConsents{required: map[int64]bool{}}
	h.limiter = &countingLimiter{hits: map[string]int{}}
	h.chat = NewChat(ChatDeps{
		Store: h.store, Tools: h.reg, Actions: h.actions, Provider: h.llm,
		Models:   llm.Models{Default: "claude-sonnet-5-5", Fast: "claude-haiku-4-5"},
		Features: h.features, Consents: h.consents, Limiter: h.limiter,
	})
	h.chat.SetClock(clock)
	p := user(testUser, rbac.PermAIUse, rbac.PermAIActionsConfirm, rbac.PermTasksWrite)
	h.caller = Caller{Auth: p.Auth, Channel: model.ChannelPanel, Org: p.Org}
	conv, err := h.chat.Create(h.ctx, h.caller, "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	h.conv = conv
	return h
}

type sse struct {
	name string
	data any
}

func (h *chatHarness) send(text string) ([]sse, error) {
	h.t.Helper()
	turn, err := h.chat.PrepareMessage(h.ctx, h.caller, h.conv.UUID, text)
	if err != nil {
		return nil, err
	}
	return h.run(turn), nil
}

func (h *chatHarness) run(turn *Turn) []sse {
	h.t.Helper()
	var out []sse
	if err := h.chat.Run(h.ctx, turn, func(name string, data any) { out = append(out, sse{name, data}) }); err != nil {
		h.t.Fatalf("run: %v", err)
	}
	return out
}

func names(evs []sse) []string {
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		if len(out) > 0 && e.name == EventTextDelta && out[len(out)-1] == EventTextDelta {
			continue
		}
		out = append(out, e.name)
	}
	return out
}

func find(evs []sse, name string) (sse, bool) {
	for _, e := range evs {
		if e.name == name {
			return e, true
		}
	}
	return sse{}, false
}

func wantChatStatus(t *testing.T, err error, status int, code string) {
	t.Helper()
	got, gotCode, ok := ChatErrorStatus(err)
	if !ok || got != status || gotCode != code {
		t.Fatalf("err %v: want %d %s, got %d %s", err, status, code, got, gotCode)
	}
}

func usage(in, out int64) llm.Usage { return llm.Usage{InputTokens: in, OutputTokens: out} }

// --- acceptance --------------------------------------------------------------

// A text answer streams message_start → text_delta… → message_done, then
// the fast model titles the new conversation; both calls are booked.
func TestChatTextTurnEventsInOrder(t *testing.T) {
	h := newChatHarness(t,
		fake.Text("Stokta 42 rulo var.", usage(100, 10)),
		fake.Text("Stok durumu", usage(20, 3)),
	)
	evs, err := h.send("Stokta kaç rulo var?")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{EventMessageStart, EventTextDelta, EventMessageDone, EventTitle}
	if got := names(evs); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	var text strings.Builder
	for _, e := range evs {
		if e.name == EventTextDelta {
			text.WriteString(e.data.(map[string]any)["text"].(string))
		}
	}
	if text.String() != "Stokta 42 rulo var." {
		t.Fatalf("text = %q", text.String())
	}
	done, _ := find(evs, EventMessageDone)
	if done.data.(map[string]any)["status"] != model.MessageComplete {
		t.Fatalf("done = %+v", done.data)
	}
	reqs := h.llm.Requests()
	if len(reqs) != 2 || reqs[0].Model != "claude-sonnet-5-5" || reqs[1].Model != "claude-haiku-4-5" {
		t.Fatalf("requests = %d %+v", len(reqs), reqs)
	}
	sys := reqs[0].System
	if len(sys) < 2 || !strings.Contains(sys[0].Text, "Use only the tools you are given") ||
		!strings.Contains(sys[len(sys)-1].Text, "UI language: tr") || !strings.Contains(sys[len(sys)-1].Text, "Europe/Istanbul") {
		t.Fatalf("system prompt = %+v", sys)
	}
	if len(h.store.usage) != 2 || h.store.usage[0].Purpose != model.PurposeChat || h.store.usage[1].Purpose != model.PurposeTitle ||
		h.store.usage[0].Pool != model.PoolOrg || h.store.usage[0].Channel != model.UsageChannelPanel {
		t.Fatalf("usage = %+v", h.store.usage)
	}
	detail, err := h.chat.Get(h.ctx, h.caller, h.conv.UUID)
	if err != nil || detail.Title != "Stok durumu" || len(detail.Messages) != 2 ||
		detail.Messages[1].Status != model.MessageComplete || detail.Messages[1].UI[0].Text != "Stokta 42 rulo var." {
		t.Fatalf("detail = %+v %v", detail, err)
	}
}

// A write tool stops the turn at the confirm event without writing; the
// confirmation runs it once, emits action and continues the turn with the
// tool result.
func TestChatWriteToolConfirmResumes(t *testing.T) {
	h := newChatHarness(t,
		fake.ToolCall("toolu_w1", "create_note", map[string]any{"title": "Ara"}, usage(100, 20)),
		fake.Text("Not oluşturuldu.", usage(120, 8)),
	)
	h.conv, _ = h.chat.Create(h.ctx, h.caller, "Notlar") // titled: no title call

	evs, err := h.send("Yarın için not ekle: Ara")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{EventMessageStart, EventToolStart, EventConfirm, EventMessageDone}
	if got := names(evs); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if h.writes.Load() != 0 {
		t.Fatal("a proposal wrote")
	}
	confirm, _ := find(evs, EventConfirm)
	card := confirm.data.(Card)
	if card.ToolName != "create_note" || card.Status != model.ActionPending || card.SourceRef != h.conv.UUID.String() {
		t.Fatalf("card = %+v", card)
	}
	done, _ := find(evs, EventMessageDone)
	if done.data.(map[string]any)["stop_reason"] != "confirm" {
		t.Fatalf("done = %+v", done.data)
	}

	turn, err := h.chat.PrepareAction(h.ctx, h.caller, card.ActionUUID, true, nil)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	evs = h.run(turn)
	want = []string{EventAction, EventMessageStart, EventTextDelta, EventMessageDone}
	if got := names(evs); !slices.Equal(got, want) {
		t.Fatalf("continuation events = %v, want %v", got, want)
	}
	if out := evs[0].data.(Outcome); out.Status != model.ActionConfirmed || out.ToolUseID != "toolu_w1" {
		t.Fatalf("action = %+v", out)
	}
	if h.writes.Load() != 1 {
		t.Fatalf("writes = %d", h.writes.Load())
	}
	reqs := h.llm.Requests()
	last := reqs[len(reqs)-1].Messages
	tr := last[len(last)-1]
	if tr.Role != llm.RoleUser || tr.Content[0].Type != llm.BlockToolResult || tr.Content[0].ToolUseID != "toolu_w1" ||
		tr.Content[0].IsError || !strings.Contains(tr.Content[0].Content, `"ok":true`) {
		t.Fatalf("continuation request ends with %+v", tr)
	}
	// A second confirmation is refused before any stream.
	if _, err := h.chat.PrepareAction(h.ctx, h.caller, card.ActionUUID, true, nil); err == nil {
		t.Fatal("second confirm accepted")
	} else {
		wantChatStatus(t, err, http.StatusConflict, CodeActionResolved)
	}
}

// A user who has not accepted the AI guidelines gets 428 and no model call.
func TestChatConsentRequired(t *testing.T) {
	h := newChatHarness(t, fake.Text("x", usage(1, 1)))
	h.consents.required[testUser] = true
	_, err := h.send("Merhaba")
	wantChatStatus(t, err, http.StatusPreconditionRequired, CodeConsentRequired)
	var ce *ConsentRequiredError
	if !errors.As(err, &ce) || ce.Text.Version != 3 {
		t.Fatalf("consent text = %+v", err)
	}
	if n := len(h.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d", n)
	}
	st, err := h.chat.Status(h.ctx, h.caller)
	if err != nil || !st.ConsentRequired || st.Consent == nil {
		t.Fatalf("status = %+v %v", st, err)
	}
}

// An organization whose monthly quota is used up gets 403
// AI_QUOTA_EXCEEDED and the model is not called.
func TestChatQuotaExceededNoModelCall(t *testing.T) {
	h := newChatHarness(t, fake.Text("x", usage(1, 1)))
	h.store.orgs[10] = db.AiOrgSetting{OrganizationID: 10, Enabled: true, MonthlyTokenQuota: pgtype.Int8{Int64: 1000, Valid: true}}
	h.store.monthly[monthKey(10, model.PoolOrg, repository.Period(h.now))] = 1000
	_, err := h.send("Merhaba")
	wantChatStatus(t, err, http.StatusForbidden, CodeQuotaExceeded)
	if n := len(h.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d", n)
	}
	st, _ := h.chat.Status(h.ctx, h.caller)
	if st.Quota.Limit != 1000 || st.Quota.Remaining == nil || *st.Quota.Remaining != 0 {
		t.Fatalf("status quota = %+v", st.Quota)
	}
}

// Spending the quota mid-turn ends the turn with quota_exceeded before
// the next model call; crossing 80 % and 100 % writes one threshold event
// each.
func TestChatQuotaExceededMidTurn(t *testing.T) {
	h := newChatHarness(t,
		fake.ToolCall("toolu_r1", "stock_summary", map[string]any{}, usage(900, 200)),
		fake.Text("never", usage(1, 1)),
	)
	h.store.orgs[10] = db.AiOrgSetting{OrganizationID: 10, Enabled: true, MonthlyTokenQuota: pgtype.Int8{Int64: 1000, Valid: true}}
	evs, err := h.send("Stok?")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{EventMessageStart, EventToolStart, EventToolResult, EventQuotaExceeded, EventMessageDone}
	if got := names(evs); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if n := len(h.llm.Requests()); n != 1 {
		t.Fatalf("model calls = %d, want 1", n)
	}
	var thresholds []int64
	for _, ev := range h.store.events {
		if ev.Name != events.AIQuotaThreshold {
			t.Fatalf("event %s", ev.Name)
		}
		thresholds = append(thresholds, ev.Payload["threshold"].(int64))
	}
	if !slices.Equal(thresholds, []int64{80, 100}) {
		t.Fatalf("thresholds = %v", thresholds)
	}
}

// The ninth model call of a turn is never made.
func TestChatModelCallLimit(t *testing.T) {
	var turns []fake.Turn
	for i := range MaxModelCalls + 1 {
		turns = append(turns, fake.ToolCall("toolu_loop_"+string(rune('a'+i)), "stock_summary", map[string]any{}, usage(10, 1)))
	}
	h := newChatHarness(t, turns...)
	evs, err := h.send("Döngü")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(h.llm.Requests()); n != MaxModelCalls || h.llm.Remaining() != 1 {
		t.Fatalf("model calls = %d remaining = %d", n, h.llm.Remaining())
	}
	e, ok := find(evs, EventError)
	if !ok || e.data.(map[string]any)["code"] != StreamTurnLimit {
		t.Fatalf("error event = %+v", e)
	}
	done, _ := find(evs, EventMessageDone)
	if done.data.(map[string]any)["status"] != model.MessageError {
		t.Fatalf("done = %+v", done.data)
	}
}

// An organization without the ai_assistant module gets 403
// FEATURE_DISABLED on every endpoint; so does an organization switched off
// in ai_org_settings.
func TestChatFeatureDisabled(t *testing.T) {
	h := newChatHarness(t, fake.Text("x", usage(1, 1)))
	h.features[10] = false
	_, err := h.send("Merhaba")
	wantChatStatus(t, err, http.StatusForbidden, "FEATURE_DISABLED")
	_, _, err = h.chat.List(h.ctx, h.caller, ListFilter{Limit: 20})
	wantChatStatus(t, err, http.StatusForbidden, "FEATURE_DISABLED")

	h.features[10] = true
	h.store.orgs[10] = db.AiOrgSetting{OrganizationID: 10, Enabled: false}
	_, err = h.send("Merhaba")
	wantChatStatus(t, err, http.StatusForbidden, "FEATURE_DISABLED")
	if st, _ := h.chat.Status(h.ctx, h.caller); st.Enabled {
		t.Fatal("status enabled for a disabled organization")
	}
	if n := len(h.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d", n)
	}
}

// Portal customers get the customer tool set only (no panel tools) and
// book on the brand center's system pool.
func TestChatCustomerRealmHasNoPanelTools(t *testing.T) {
	h := newChatHarness(t, fake.Text("Merhaba!", usage(10, 2)), fake.Text("Selam", usage(1, 1)))
	h.caller = Caller{Auth: authctx.Principal{UserID: uuid.New(), UserInternal: 9, Realm: "portal"},
		Channel: model.ChannelPortal, BrandID: 1}
	conv, err := h.chat.Create(h.ctx, h.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	h.conv = conv
	if _, err := h.send("Merhaba"); err != nil {
		t.Fatal(err)
	}
	req := h.llm.Requests()[0]
	var got []string
	for _, d := range req.Tools {
		got = append(got, d.Name)
	}
	if !slices.Equal(got, []string{"my_vehicles"}) {
		t.Fatalf("customer tools = %v", got)
	}
	if !strings.Contains(req.System[0].Text, "Customer assistant") {
		t.Fatal("customer prompt missing")
	}
	if u := h.store.usage[0]; u.Pool != model.PoolSystem || u.Channel != model.UsageChannelPortal || u.OrganizationID != 10 {
		t.Fatalf("usage = %+v", u)
	}
	// The panel channel does not see the portal conversation.
	panel := user(testUser, rbac.PermAIUse)
	if _, err := h.chat.Get(h.ctx, Caller{Auth: panel.Auth, Channel: model.ChannelPanel, Org: panel.Org}, conv.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("panel get portal conversation = %v", err)
	}
}

// Permission, message size, conversation size and the rate limit.
func TestChatLimits(t *testing.T) {
	h := newChatHarness(t, fake.Text("x", usage(1, 1)))
	noPerm := user(testUser)
	_, err := h.chat.PrepareMessage(h.ctx, Caller{Auth: noPerm.Auth, Channel: model.ChannelPanel, Org: noPerm.Org}, h.conv.UUID, "x")
	wantChatStatus(t, err, http.StatusForbidden, "FORBIDDEN")

	_, err = h.send(strings.Repeat("a", MaxMessageChars+1))
	wantChatStatus(t, err, http.StatusBadRequest, "VALIDATION_ERROR")
	if _, err := h.chat.PrepareMessage(h.ctx, h.caller, h.conv.UUID, strings.Repeat("ş", MaxMessageChars)); err != nil {
		t.Fatalf("8000 characters refused: %v", err)
	}

	h.store.convs[0].MessageCount = MaxConversationMessages - 1
	_, err = h.send("x")
	wantChatStatus(t, err, http.StatusUnprocessableEntity, CodeConversationLimit)

	h.store.convs[0].MessageCount = 0
	h.limiter.limit = 1
	h.limiter.hits = map[string]int{"ai_turn7": 1}
	_, err = h.send("x")
	wantChatStatus(t, err, http.StatusTooManyRequests, "RATE_LIMITED")

	_, _, err = h.chat.List(h.ctx, h.caller, ListFilter{Sort: []apiquery.SortField{{Field: "tokens"}}})
	wantChatStatus(t, err, http.StatusBadRequest, "VALIDATION_ERROR")
	if n := len(h.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d", n)
	}
}

// A new message instead of a confirmation cancels the open card; its
// tool_use is answered in the new user message.
func TestChatNewMessageCancelsOpenCard(t *testing.T) {
	h := newChatHarness(t,
		fake.ToolCall("toolu_w2", "create_note", map[string]any{"title": "Ara"}, usage(10, 2)),
		fake.Text("Tamam, iptal edildi.", usage(10, 2)),
	)
	h.conv, _ = h.chat.Create(h.ctx, h.caller, "Notlar")
	evs, err := h.send("Not ekle: Ara")
	if err != nil {
		t.Fatal(err)
	}
	confirm, _ := find(evs, EventConfirm)
	card := confirm.data.(Card)
	if _, err := h.send("Vazgeçtim"); err != nil {
		t.Fatal(err)
	}
	if row := h.harness.store.row(t, card.ActionUUID); row.Status != model.ActionCancelled {
		t.Fatalf("card status = %s", row.Status)
	}
	msgs := h.llm.Requests()[1].Messages
	lastUser := msgs[len(msgs)-1]
	if lastUser.Content[0].Type != llm.BlockToolResult || lastUser.Content[0].ToolUseID != "toolu_w2" || !lastUser.Content[0].IsError {
		t.Fatalf("new message does not answer the card: %+v", lastUser.Content)
	}
	if h.writes.Load() != 0 {
		t.Fatal("cancelled card wrote")
	}
}

// A cancelled ctx (client gone) stores the turn as cancelled.
func TestChatCancelledContextStoresCancelled(t *testing.T) {
	h := newChatHarness(t, fake.Text("x", usage(1, 1)))
	turn, err := h.chat.PrepareMessage(h.ctx, h.caller, h.conv.UUID, "Merhaba")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(h.ctx)
	cancel()
	if err := h.chat.Run(ctx, turn, nil); err != nil {
		t.Fatal(err)
	}
	msgs, _ := h.store.Messages(h.ctx, h.store.convs[0].ID)
	if len(msgs) != 2 || msgs[1].Status != model.MessageCancelled {
		t.Fatalf("messages = %+v", msgs)
	}
}

func TestSanitizeHistoryMergesPausedTurn(t *testing.T) {
	in := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("q")}},
		{Role: llm.RoleAssistant, Content: []llm.Block{
			{Type: llm.BlockToolUse, ID: "a", Name: "stock_summary"}, {Type: llm.BlockToolUse, ID: "b", Name: "create_note"}}},
		{Role: llm.RoleUser, Content: []llm.Block{llm.ToolResultBlock("a", "ok", false)}},
		{Role: llm.RoleUser, Content: []llm.Block{llm.ToolResultBlock("b", "done", false)}},
		{Role: llm.RoleAssistant, Content: []llm.Block{{Type: llm.BlockToolUse, ID: "c", Name: "create_note"}}},
	}
	out := sanitizeHistory(in)
	if len(out) != 5 || len(out[2].Content) != 2 || out[2].Content[1].ToolUseID != "b" ||
		out[4].Role != llm.RoleUser || out[4].Content[0].ToolUseID != "c" || !out[4].Content[0].IsError {
		t.Fatalf("sanitized = %+v", out)
	}
}
