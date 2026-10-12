package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
)

// sse writes chat completion chunks as server-sent events, ending with [DONE].
func sse(w http.ResponseWriter, chunks ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	for _, c := range chunks {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", c)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func newProvider(t *testing.T, srv *httptest.Server, key string) llm.Provider {
	t.Helper()
	return New(Config{
		APIKey: key, BaseURL: srv.URL + "/v1", DefaultModel: "test-model",
		MaxRetries: 2, RetryBaseDelay: time.Millisecond, HTTPClient: srv.Client(),
	})
}

func TestNewDisabledWithoutKeyOrBaseURL(t *testing.T) {
	if p := New(Config{}); p.Enabled() {
		t.Fatal("no key and no base URL must disable the provider")
	}
	if _, err := New(Config{}).Stream(context.Background(), llm.Request{}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("disabled stream err = %v", err)
	}
	if p := New(Config{BaseURL: "http://localhost:11434/v1"}); !p.Enabled() || p.Name() != "openai" {
		t.Fatal("a base URL alone (local server) must enable the provider")
	}
}

func TestMaxTokensField(t *testing.T) {
	if got := maxTokensField("https://api.openai.com/v1"); got != "max_completion_tokens" {
		t.Fatalf("openai field = %s", got)
	}
	if got := maxTokensField("http://localhost:11434/v1"); got != "max_tokens" {
		t.Fatalf("compatible field = %s", got)
	}
}

func TestStreamText(t *testing.T) {
	var body map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		sse(w,
			`{"model":"test-model-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Mer"}}]}`,
			`{"model":"test-model-1","choices":[{"index":0,"delta":{"content":"haba"}}]}`,
			`{"model":"test-model-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"model":"test-model-1","choices":[],"usage":{"prompt_tokens":120,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":100}}}`,
		)
	}))
	defer srv.Close()

	ch, err := newProvider(t, srv, "sk-test").Stream(context.Background(), llm.Request{
		System:   []llm.SystemBlock{{Text: "Sen bir asistansın."}, {Text: "Kısa cevap ver.", Cache: true}},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("selam")}}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var deltas []string
	var resp llm.Response
	for ev := range ch {
		switch ev.Type {
		case llm.EventTextDelta:
			deltas = append(deltas, ev.Text)
		case llm.EventUsage:
			resp.Usage = ev.Usage
		case llm.EventMessageStop:
			resp.Message, resp.StopReason, resp.Model = ev.Message, ev.StopReason, ev.Model
		case llm.EventError:
			t.Fatalf("event error: %v", ev.Err)
		}
	}
	if strings.Join(deltas, "|") != "Mer|haba" || resp.Message.Text() != "Merhaba" {
		t.Fatalf("deltas %v text %q", deltas, resp.Message.Text())
	}
	if resp.StopReason != llm.StopEndTurn || resp.Model != "test-model-1" {
		t.Fatalf("stop %q model %q", resp.StopReason, resp.Model)
	}
	if resp.Usage != (llm.Usage{InputTokens: 20, OutputTokens: 7, CacheReadTokens: 100}) {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if auth != "Bearer sk-test" {
		t.Fatalf("auth = %q", auth)
	}
	if body["model"] != "test-model" || body["stream"] != true || body["max_tokens"] != float64(DefaultMaxTokens) {
		t.Fatalf("body = %v", body)
	}
	msgs := body["messages"].([]any)
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "Sen bir asistansın.\n\nKısa cevap ver." {
		t.Fatalf("system = %v", sys)
	}
	if user := msgs[1].(map[string]any); user["role"] != "user" || user["content"] != "selam" {
		t.Fatalf("user = %v", user)
	}
}

func TestStreamToolCallsAssembled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sse(w,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"count_services","arguments":""}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"month\":"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"2026-10\"}"}},{"index":1,"function":{"name":"broken","arguments":"{oops"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		)
	}))
	defer srv.Close()

	ch, err := newProvider(t, srv, "k").Stream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("bu ay kaç hizmet?")}}},
		Tools:    []llm.ToolDef{{Name: "count_services", Description: "say", InputSchema: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var uses []llm.Block
	var stop llm.Event
	for ev := range ch {
		switch ev.Type {
		case llm.EventToolUse:
			uses = append(uses, ev.Block)
		case llm.EventMessageStop:
			stop = ev
		case llm.EventError:
			t.Fatalf("event error: %v", ev.Err)
		}
	}
	if len(uses) != 2 {
		t.Fatalf("tool uses = %+v", uses)
	}
	if uses[0].ID != "call_a" || uses[0].Name != "count_services" || string(uses[0].Input) != `{"month":"2026-10"}` {
		t.Fatalf("first tool use = %+v (%s)", uses[0], uses[0].Input)
	}
	// Missing id gets a stable one; malformed arguments stay a JSON string.
	if uses[1].ID != "call_1" || string(uses[1].Input) != `"{oops"` {
		t.Fatalf("second tool use = %+v (%s)", uses[1], uses[1].Input)
	}
	if stop.StopReason != llm.StopToolUse || len(stop.Message.ToolUses()) != 2 {
		t.Fatalf("stop = %+v", stop)
	}
}

func TestToMessagesToolResultsAndImages(t *testing.T) {
	msgs, err := toMessages(nil, []llm.Message{
		{Role: llm.RoleUser, Content: []llm.Block{
			llm.TextBlock("bu ne?"),
			{Type: llm.BlockImage, MediaType: "image/png", Data: "AAAA"},
		}},
		{Role: llm.RoleAssistant, Content: []llm.Block{
			llm.TextBlock("bakıyorum"),
			{Type: llm.BlockToolUse, ID: "call_1", Name: "lookup", Input: json.RawMessage(`{"q":"x"}`)},
		}},
		{Role: llm.RoleUser, Content: []llm.Block{
			llm.ToolResultBlock("call_1", "bulunamadı", true),
			llm.TextBlock("devam"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(msgs)
	got := string(raw)
	for _, want := range []string{
		`{"role":"user","content":[{"type":"text","text":"bu ne?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}`,
		`{"role":"assistant","content":"bakıyorum","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]}`,
		`{"role":"tool","content":"ERROR: bulunamadı","tool_call_id":"call_1"},{"role":"user","content":"devam"}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("messages\n%s\nmissing\n%s", got, want)
		}
	}
}

func TestStreamClientErrorNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`)
	}))
	defer srv.Close()
	_, err := newProvider(t, srv, "bad").Stream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("x")}}},
	})
	var pe *Error
	if !errors.As(err, &pe) || pe.StatusCode != 401 || pe.Type != "invalid_request_error" || !strings.Contains(pe.Message, "Incorrect API key") {
		t.Fatalf("err = %#v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("401 retried: %d calls", calls.Load())
	}
}

func TestStreamRetriesRateLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"slow down","code":429}}`)
			return
		}
		sse(w, `{"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	resp, err := llm.Complete(context.Background(), newProvider(t, srv, ""), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("x")}}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Message.Text() != "ok" || calls.Load() != 3 {
		t.Fatalf("text %q after %d calls", resp.Message.Text(), calls.Load())
	}
}

func TestStreamErrorEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sse(w,
			`{"choices":[{"index":0,"delta":{"content":"ya"}}]}`,
			`{"error":{"message":"model overloaded","type":"server_error"}}`,
		)
	}))
	defer srv.Close()
	_, err := llm.Complete(context.Background(), newProvider(t, srv, "k"), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("x")}}},
	})
	var pe *Error
	if !errors.As(err, &pe) || pe.Type != "server_error" {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamWithoutFinishIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ya\"}}]}\n\n")
	}))
	defer srv.Close()
	_, err := llm.Complete(context.Background(), newProvider(t, srv, "k"), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("x")}}},
	})
	if err == nil || !strings.Contains(err.Error(), "without a finish reason") {
		t.Fatalf("err = %v", err)
	}
}
