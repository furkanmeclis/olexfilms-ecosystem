package anthropic_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/anthropic"
)

// sse renders Anthropic stream events.
func sse(events ...string) string {
	var sb strings.Builder
	for _, ev := range events {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(ev), &head); err != nil {
			panic(err)
		}
		fmt.Fprintf(&sb, "event: %s\ndata: %s\n\n", head.Type, ev)
	}
	return sb.String()
}

var toolTurn = sse(
	`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],"stop_reason":null,"usage":{"input_tokens":12,"output_tokens":1,"cache_creation_input_tokens":300,"cache_read_input_tokens":1500}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"ping"}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Bakıyorum"}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", bir saniye."}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"search_products","input":{}}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":""}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\": \"cam fi"}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"lmi\", \"limit\": 5}"}}`,
	`{"type":"content_block_stop","index":1}`,
	`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":57}}`,
	`{"type":"message_stop"}`,
)

var textTurn = sse(
	`{"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","model":"claude-haiku-4-5","content":[],"stop_reason":null,"usage":{"input_tokens":5,"output_tokens":1}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Tamam"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}`,
	`{"type":"message_stop"}`,
)

// server is a fake Anthropic Messages API: each call pops the next reply.
type server struct {
	t       *testing.T
	mu      sync.Mutex
	replies []reply
	bodies  [][]byte
	calls   atomic.Int32
	*httptest.Server
}

type reply struct {
	status int
	body   string
	header map[string]string
}

func newServer(t *testing.T, replies ...reply) *server {
	t.Helper()
	s := &server{t: t, replies: replies}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	if r.URL.Path != "/v1/messages" || r.Method != http.MethodPost {
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		return
	}
	if r.Header.Get("X-Api-Key") != "test-key" {
		http.Error(w, "missing key", http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.bodies = append(s.bodies, body)
	if len(s.replies) == 0 {
		s.mu.Unlock()
		http.Error(w, "script exhausted", http.StatusTeapot)
		return
	}
	rep := s.replies[0]
	s.replies = s.replies[1:]
	s.mu.Unlock()
	for k, v := range rep.header {
		w.Header().Set(k, v)
	}
	if rep.status == 0 || rep.status == http.StatusOK {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, rep.body)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rep.status)
	_, _ = io.WriteString(w, rep.body)
}

func apiError(typ, msg string) string {
	return fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":%q}}`, typ, msg)
}

func newProvider(s *server) llm.Provider {
	return anthropic.New(anthropic.Config{
		APIKey:         "test-key",
		BaseURL:        s.URL,
		DefaultModel:   "claude-sonnet-5-5",
		RetryBaseDelay: time.Millisecond,
		MaxRetries:     3,
	})
}

func baseRequest() llm.Request {
	return llm.Request{
		System: []llm.SystemBlock{{Text: "Sen Olexfilms asistanısın."}, {Text: "Kurallar..."}},
		Tools: []llm.ToolDef{
			{Name: "get_order", Description: "Sipariş", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"}}},
			{Name: "search_products", Description: "Ürün ara", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}, "additionalProperties": false}},
		},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{
			llm.TextBlock("Cam filmi var mı?"),
			{Type: llm.BlockImage, MediaType: "image/png", Data: "aGVsbG8="},
		}}},
	}
}

func drain(t *testing.T, ch <-chan llm.Event) []llm.Event {
	t.Helper()
	var out []llm.Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("stream did not finish")
		}
	}
}

func TestStreamParsesTextDeltasToolUseAndUsage(t *testing.T) {
	s := newServer(t, reply{body: toolTurn})
	ch, err := newProvider(s).Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	events := drain(t, ch)

	var types []string
	for _, ev := range events {
		types = append(types, ev.Type.String())
	}
	want := "text_delta,text_delta,tool_use,usage,message_stop"
	if got := strings.Join(types, ","); got != want {
		t.Fatalf("event order = %s, want %s", got, want)
	}
	if events[0].Text != "Bakıyorum" || events[1].Text != ", bir saniye." {
		t.Fatalf("text deltas = %q %q", events[0].Text, events[1].Text)
	}
	tool := events[2].Block
	if tool.Type != llm.BlockToolUse || tool.ID != "toolu_01" || tool.Name != "search_products" {
		t.Fatalf("tool_use block = %+v", tool)
	}
	var input struct {
		Q     string `json:"q"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(tool.Input, &input); err != nil || input.Q != "cam filmi" || input.Limit != 5 {
		t.Fatalf("tool input = %s (%v)", tool.Input, err)
	}
	wantUsage := llm.Usage{InputTokens: 12, OutputTokens: 57, CacheReadTokens: 1500, CacheWriteTokens: 300}
	if events[3].Usage != wantUsage {
		t.Fatalf("usage = %+v, want %+v", events[3].Usage, wantUsage)
	}
	stop := events[4]
	if stop.StopReason != llm.StopToolUse || stop.Model != "claude-sonnet-5-5" {
		t.Fatalf("message_stop = %+v", stop)
	}
	if stop.Message.Text() != "Bakıyorum, bir saniye." || len(stop.Message.ToolUses()) != 1 {
		t.Fatalf("assembled message = %+v", stop.Message)
	}
}

func TestStreamRequestCarriesCacheBreakpointsAndImage(t *testing.T) {
	s := newServer(t, reply{body: textTurn})
	resp, err := llm.Complete(context.Background(), newProvider(s), baseRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Text() != "Tamam" || resp.StopReason != llm.StopEndTurn {
		t.Fatalf("response = %+v", resp)
	}
	var body struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Stream    bool   `json:"stream"`
		System    []struct {
			Text         string          `json:"text"`
			CacheControl json.RawMessage `json:"cache_control"`
		} `json:"system"`
		Tools []struct {
			Name         string          `json:"name"`
			CacheControl json.RawMessage `json:"cache_control"`
			InputSchema  map[string]any  `json:"input_schema"`
		} `json:"tools"`
		Messages []struct {
			Content []struct {
				Type   string `json:"type"`
				Source struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				} `json:"source"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(s.bodies[0], &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if body.Model != "claude-sonnet-5-5" || body.MaxTokens != anthropic.DefaultMaxTokens || !body.Stream {
		t.Fatalf("model/max_tokens/stream = %s/%d/%v", body.Model, body.MaxTokens, body.Stream)
	}
	ephemeral := `{"type":"ephemeral"}`
	if len(body.System) != 2 || body.System[0].CacheControl != nil || string(body.System[1].CacheControl) != ephemeral {
		t.Fatalf("system cache breakpoint not on last block: %s", s.bodies[0])
	}
	if len(body.Tools) != 2 || body.Tools[0].CacheControl != nil || string(body.Tools[1].CacheControl) != ephemeral {
		t.Fatalf("tool cache breakpoint not on last tool: %s", s.bodies[0])
	}
	if body.Tools[1].InputSchema["additionalProperties"] != false {
		t.Fatalf("schema extras dropped: %v", body.Tools[1].InputSchema)
	}
	img := body.Messages[0].Content[1]
	if img.Type != "image" || img.Source.Type != "base64" || img.Source.MediaType != "image/png" || img.Source.Data != "aGVsbG8=" {
		t.Fatalf("image block = %+v", img)
	}
}

func TestStreamRetriesAfter529(t *testing.T) {
	s := newServer(t,
		reply{status: 529, body: apiError("overloaded_error", "Overloaded")},
		reply{status: http.StatusTooManyRequests, body: apiError("rate_limit_error", "slow down")},
		reply{body: textTurn},
	)
	resp, err := llm.Complete(context.Background(), newProvider(s), baseRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Text() != "Tamam" {
		t.Fatalf("text = %q", resp.Message.Text())
	}
	if got := s.calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3 (529, 429, ok)", got)
	}
}

func TestStreamGivesUpAfterThreeRetries(t *testing.T) {
	var replies []reply
	for range 5 {
		replies = append(replies, reply{status: http.StatusServiceUnavailable, body: apiError("api_error", "down")})
	}
	s := newServer(t, replies...)
	_, err := newProvider(s).Stream(context.Background(), baseRequest())
	var pe *anthropic.Error
	if !errors.As(err, &pe) || pe.StatusCode != http.StatusServiceUnavailable || !pe.Retryable() {
		t.Fatalf("err = %v", err)
	}
	if got := s.calls.Load(); got != 4 {
		t.Fatalf("calls = %d, want 4 (1 + 3 retries)", got)
	}
}

func TestStreamDoesNotRetryClientErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		typ    string
	}{
		{http.StatusBadRequest, "invalid_request_error"},
		{http.StatusUnauthorized, "authentication_error"},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			s := newServer(t,
				reply{status: tc.status, body: apiError(tc.typ, "nope")},
				reply{body: textTurn},
			)
			_, err := newProvider(s).Stream(context.Background(), baseRequest())
			var pe *anthropic.Error
			if !errors.As(err, &pe) || pe.StatusCode != tc.status || pe.Type != tc.typ || pe.Message != "nope" || pe.Retryable() {
				t.Fatalf("err = %#v", err)
			}
			if got := s.calls.Load(); got != 1 {
				t.Fatalf("calls = %d, want 1 (no retry)", got)
			}
		})
	}
}

func TestStreamMidStreamErrorEvent(t *testing.T) {
	body := sse(
		`{"type":"message_start","message":{"id":"msg_3","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],"stop_reason":null,"usage":{"input_tokens":5,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Yarım"}}`,
	) + "event: error\ndata: " + apiError("overloaded_error", "Overloaded") + "\n\n"
	s := newServer(t, reply{body: body})
	ch, err := newProvider(s).Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	events := drain(t, ch)
	last := events[len(events)-1]
	var pe *anthropic.Error
	if last.Type != llm.EventError || !errors.As(last.Err, &pe) || pe.StatusCode != 529 {
		t.Fatalf("last event = %+v", last)
	}
	if events[0].Type != llm.EventTextDelta || events[0].Text != "Yarım" {
		t.Fatalf("first event = %+v", events[0])
	}
	if got := s.calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 (no retry after output started)", got)
	}
}

func TestNewWithoutKeyIsUnavailable(t *testing.T) {
	p := anthropic.New(anthropic.Config{APIKey: "  ", DefaultModel: "claude-sonnet-5-5"})
	if p.Enabled() {
		t.Fatal("provider without key reports enabled")
	}
	if _, err := p.Stream(context.Background(), baseRequest()); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Stream err = %v, want ErrUnavailable", err)
	}
	if _, err := llm.Complete(context.Background(), p, baseRequest()); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Complete err = %v, want ErrUnavailable", err)
	}
	if p := anthropic.NewFromConfig(config.AIConfig{DefaultModel: "claude-sonnet-5-5"}); p.Enabled() || p.Name() != "disabled" {
		t.Fatalf("NewFromConfig without key = %s", p.Name())
	}
	if p := anthropic.NewFromConfig(config.AIConfig{APIKey: "k", DefaultModel: "claude-sonnet-5-5"}); !p.Enabled() || p.Name() != "anthropic" {
		t.Fatalf("NewFromConfig with key = %s", p.Name())
	}
}
