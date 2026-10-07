package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// swapLLM lets a test replace the provider between requests.
type swapLLM struct {
	mu sync.Mutex
	p  llm.Provider
}

func (s *swapLLM) set(p llm.Provider) { s.mu.Lock(); s.p = p; s.mu.Unlock() }
func (s *swapLLM) get() llm.Provider  { s.mu.Lock(); defer s.mu.Unlock(); return s.p }
func (*swapLLM) Name() string         { return "swap" }
func (*swapLLM) Enabled() bool        { return true }
func (s *swapLLM) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	return s.get().Stream(ctx, req)
}

// hangingLLM streams one text delta and then waits for the request
// context, like a slow model when the browser tab closes.
type hangingLLM struct{ started chan struct{} }

func (*hangingLLM) Name() string  { return "hanging" }
func (*hangingLLM) Enabled() bool { return true }
func (h *hangingLLM) Stream(ctx context.Context, _ llm.Request) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 2)
	go func() {
		defer close(ch)
		ch <- llm.Event{Type: llm.EventTextDelta, Text: "Merhaba "}
		close(h.started)
		<-ctx.Done()
		ch <- llm.Event{Type: llm.EventError, Err: ctx.Err()}
	}()
	return ch, nil
}

type aiChatEnv struct {
	*itest
	llm  *swapLLM
	http *httptest.Server
}

func newAIChatIntegration(t *testing.T) *aiChatEnv {
	t.Helper()
	sw := &swapLLM{p: fake.New()}
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.LLM = sw })
	srv := httptest.NewServer(it.handler)
	t.Cleanup(srv.Close)
	return &aiChatEnv{itest: it, llm: sw, http: srv}
}

func (e *aiChatEnv) enableAI(org db.Organization) {
	e.t.Helper()
	admin, _ := e.user("t388-admin", rbac.RoleSuperAdmin)
	ctx := context.Background()
	if _, err := e.srv.features.SetByAdmin(ctx, admin.ID, org.ID, features.ModuleAIAssistant, true); err != nil {
		e.t.Fatalf("enable ai_assistant: %v", err)
	}
	e.t.Cleanup(func() { _, _ = e.srv.features.ClearByAdmin(context.Background(), org.ID, features.ModuleAIAssistant) })
}

// acceptGuidelines answers the pending AI guidelines on the panel or
// portal consent endpoints.
func (e *aiChatEnv) acceptGuidelines(tok, prefix string) {
	e.t.Helper()
	code, env := e.do("GET", prefix+"/consents/pending?locale=tr", hostOlex, tok, nil)
	if code != http.StatusOK {
		e.t.Fatalf("pending: %d %s", code, errCode(env))
	}
	var out struct {
		Items []pendingText `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &out)
	for _, p := range out.Items {
		if code, env := e.do("POST", prefix+"/consents", hostOlex, tok, map[string]any{
			"kind": p.Kind, "locale": p.Locale, "version": p.Version, "accepted": true,
		}); code != http.StatusCreated {
			e.t.Fatalf("accept: %d %s", code, errCode(env))
		}
	}
}

func (e *aiChatEnv) post(ctx context.Context, tok, path string, body any) *http.Response {
	e.t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", e.http.URL+path, bytes.NewReader(raw))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("POST %s: %v", path, err)
	}
	return res
}

// sseEvents reads a complete event stream and returns the event names and
// the data of each.
func sseEvents(t *testing.T, r io.Reader) ([]string, []json.RawMessage) {
	t.Helper()
	var (
		names []string
		datas []json.RawMessage
		sc    = bufio.NewScanner(r)
	)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			names = append(names, strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: "):
			datas = append(datas, json.RawMessage(strings.TrimPrefix(line, "data: ")))
		}
	}
	return names, datas
}

func (e *aiChatEnv) createConversation(tok, prefix string) string {
	e.t.Helper()
	code, env := e.do("POST", prefix+"/ai/conversations", hostOlex, tok, map[string]any{})
	if code != http.StatusCreated {
		e.t.Fatalf("create conversation: %d %s", code, errCode(env))
	}
	var c struct {
		UUID string `json:"uuid"`
	}
	_ = json.Unmarshal(env.Data, &c)
	return c.UUID
}

// TEC-388 handler acceptance on the real server: the module gate, the
// guidelines consent (428 until accepted on the new panel consent
// endpoints), the SSE response headers and event order, usage booking, the
// conversation list contract (unknown sort 400) and a client disconnect
// that cancels the turn and stores the message as cancelled.
func TestIntegrationAIChatSSE(t *testing.T) {
	e := newAIChatIntegration(t)
	ctx := context.Background()
	dealer := e.org("t388-dealer", "dealer", e.brandCenter("olex"))
	owner, pw := e.user("t388-owner")
	e.member(dealer, owner, "owner")
	tok := e.loginOrg(owner, pw, dealer)

	// Module off: 403 FEATURE_DISABLED.
	if code, env := e.do("GET", "/v1/ai/conversations", hostOlex, tok, nil); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("module off = %d %s", code, errCode(env))
	}
	e.enableAI(dealer)

	// List contract: unknown sort is 400.
	if code, env := e.do("GET", "/v1/ai/conversations?sort=-tokens", hostOlex, tok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("unknown sort = %d %s", code, errCode(env))
	}
	conv := e.createConversation(tok, "/v1")

	// Guidelines not accepted: 428 before any stream or model call.
	e.llm.set(fake.New(fake.Text("Merhaba, size nasıl yardımcı olabilirim?", llm.Usage{InputTokens: 120, OutputTokens: 9}),
		fake.Text("Selamlaşma", llm.Usage{InputTokens: 30, OutputTokens: 3})))
	if code, env := e.do("POST", "/v1/ai/conversations/"+conv+"/messages", hostOlex, tok, map[string]string{"content": "Merhaba"}); code != http.StatusPreconditionRequired || errCode(env) != "AI_CONSENT_REQUIRED" {
		t.Fatalf("no consent = %d %s", code, errCode(env))
	}
	e.acceptGuidelines(tok, "/v1")
	var st struct {
		Enabled         bool `json:"enabled"`
		ConsentRequired bool `json:"consent_required"`
		Quota           struct {
			Limit int64 `json:"limit"`
		} `json:"quota"`
	}
	code, env := e.do("GET", "/v1/ai/status", hostOlex, tok, nil)
	_ = json.Unmarshal(env.Data, &st)
	if code != http.StatusOK || !st.Enabled || st.ConsentRequired || st.Quota.Limit <= 0 {
		t.Fatalf("status = %d %+v", code, st)
	}

	res := e.post(ctx, tok, "/v1/ai/conversations/"+conv+"/messages", map[string]string{"content": "Merhaba"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") ||
		res.Header.Get("Cache-Control") != "no-cache" || res.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("headers = %v", res.Header)
	}
	names, _ := sseEvents(t, res.Body)
	_ = res.Body.Close()
	if len(names) < 4 || names[0] != "message_start" || names[1] != "text_delta" ||
		names[len(names)-2] != "message_done" || names[len(names)-1] != "title" {
		t.Fatalf("events = %v", names)
	}
	var booked int64
	if err := e.pool.QueryRow(ctx, `SELECT COALESCE(SUM(quota_tokens), 0) FROM ai_usage WHERE organization_id = $1 AND pool = 'org'`,
		dealer.ID).Scan(&booked); err != nil || booked != 120+9+30+3 {
		t.Fatalf("booked = %d %v", booked, err)
	}
	code, env = e.do("GET", "/v1/ai/conversations?sort=title&q=Selam", hostOlex, tok, nil)
	var page struct {
		Items []struct {
			UUID         string `json:"uuid"`
			Title        string `json:"title"`
			MessageCount int    `json:"message_count"`
		} `json:"items"`
		Total int `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &page)
	if code != http.StatusOK || page.Total != 1 || page.Items[0].UUID != conv || page.Items[0].Title != "Selamlaşma" || page.Items[0].MessageCount != 2 {
		t.Fatalf("list = %d %+v", code, page)
	}

	// The client disconnects mid-answer: the turn's context is cancelled
	// and the assistant message is stored as cancelled.
	hang := &hangingLLM{started: make(chan struct{})}
	e.llm.set(hang)
	cctx, cancel := context.WithCancel(ctx)
	res = e.post(cctx, tok, "/v1/ai/conversations/"+conv+"/messages", map[string]string{"content": "Uzun bir cevap ver"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("second stream status = %d", res.StatusCode)
	}
	select {
	case <-hang.started:
	case <-time.After(10 * time.Second):
		t.Fatal("model call did not start")
	}
	cancel()
	_ = res.Body.Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var status string
		err := e.pool.QueryRow(ctx, `SELECT m.status FROM ai_messages m JOIN ai_conversations c ON c.id = m.conversation_id
			WHERE c.uuid = $1::uuid AND m.role = 'assistant' ORDER BY m.id DESC LIMIT 1`, conv).Scan(&status)
		if err != nil {
			t.Fatal(err)
		}
		if status == "cancelled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("assistant message status = %s, want cancelled", status)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Rename, then delete: the conversation is gone for its owner.
	if code, env := e.do("PATCH", "/v1/ai/conversations/"+conv, hostOlex, tok, map[string]string{"title": "Yeni ad"}); code != http.StatusOK {
		t.Fatalf("rename = %d %s", code, errCode(env))
	}
	if code, _ := e.do("DELETE", "/v1/ai/conversations/"+conv, hostOlex, tok, nil); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	if code, _ := e.do("GET", "/v1/ai/conversations/"+conv, hostOlex, tok, nil); code != http.StatusNotFound {
		t.Fatalf("get deleted = %d", code)
	}
}

// TEC-388: a portal customer chats on /v1/portal/ai with the customer tool
// set only (no panel tools) on the brand center's system pool; panel
// tokens are refused there and portal tokens on /v1/ai.
func TestIntegrationAIChatPortal(t *testing.T) {
	e := newAIChatIntegration(t)
	ctx := context.Background()
	center := e.brandCenter("olex")
	e.enableAI(center)
	cust, cpw := e.user("t388-customer", rbac.RoleCustomer)
	tok := e.tokensFrom(e.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": cust.Email.String, "password": cpw, "realm": "portal",
	})).AccessToken
	if code, _ := e.do("GET", "/v1/ai/conversations", hostOlex, tok, nil); code != http.StatusForbidden && code != http.StatusUnauthorized {
		t.Fatalf("portal token on panel route = %d", code)
	}
	e.acceptGuidelines(tok, "/v1/portal")
	conv := e.createConversation(tok, "/v1/portal")

	scripted := fake.New(fake.Text("Merhaba!", llm.Usage{InputTokens: 50, OutputTokens: 4}), fake.Text("Selam", llm.Usage{InputTokens: 5, OutputTokens: 1}))
	e.llm.set(scripted)
	res := e.post(ctx, tok, "/v1/portal/ai/conversations/"+conv+"/messages", map[string]string{"content": "Merhaba"})
	names, _ := sseEvents(t, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || len(names) == 0 || names[0] != "message_start" {
		t.Fatalf("portal stream = %d %v", res.StatusCode, names)
	}
	reqs := scripted.Requests()
	if len(reqs) == 0 {
		t.Fatal("no model call")
	}
	realm := map[string]aitools.Realm{}
	for _, tl := range e.srv.aiTools.All() {
		realm[tl.Spec().Name] = tl.Spec().Realm
	}
	customerTools := 0
	for _, d := range reqs[0].Tools {
		switch realm[d.Name] {
		case aitools.RealmCustomer:
			customerTools++
		default:
			t.Fatalf("%s tool %s offered to a customer", realm[d.Name], d.Name)
		}
	}
	if customerTools == 0 {
		t.Fatal("no customer tools offered")
	}
	var pool string
	if err := e.pool.QueryRow(ctx, `SELECT pool FROM ai_usage WHERE user_id = $1 AND channel = 'portal' LIMIT 1`, cust.ID).Scan(&pool); err != nil || pool != "system" {
		t.Fatalf("portal usage pool = %q %v", pool, err)
	}
}
