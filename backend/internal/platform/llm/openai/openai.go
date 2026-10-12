// Package openai is the llm.Provider driver for OpenAI-compatible Chat
// Completions APIs (TEC-636): OpenAI itself and every server that speaks the
// same wire format (OpenRouter, Google Gemini's OpenAI endpoint, Groq,
// DeepSeek, Together, Ollama, LM Studio, vLLM). It needs no SDK: one
// streaming POST /chat/completions per turn, read as server-sent events.
//
// The llm package keeps the Anthropic block shape; this driver maps it:
// system blocks → one system message, tool_use → assistant tool_calls,
// tool_result → role "tool" messages, images → data: URLs. 429 / 5xx and
// transport failures are retried with exponential backoff before the
// stream starts; 4xx never.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
)

// Defaults.
const (
	DefaultBaseURL        = "https://api.openai.com/v1"
	DefaultMaxTokens      = 8000
	DefaultRequestTimeout = 120 * time.Second
	DefaultMaxRetries     = 3
	DefaultRetryBaseDelay = time.Second
	maxRetryAfter         = 30 * time.Second
	maxErrorBody          = 64 << 10
	maxSSELine            = 4 << 20
)

// ErrorCategory is the errtrack "category" tag of provider failures (module ai).
const ErrorCategory = "ai.provider"

// Config builds the driver.
type Config struct {
	// APIKey is sent as a Bearer token; a local server may need none.
	APIKey string
	// BaseURL is the API root including the version path, e.g.
	// https://api.openai.com/v1, https://openrouter.ai/api/v1,
	// http://localhost:11434/v1 (Ollama). Empty = DefaultBaseURL.
	BaseURL string
	// DefaultModel is used when Request.Model is empty.
	DefaultModel string
	MaxTokens    int
	// RequestTimeout bounds one attempt including the whole stream.
	RequestTimeout time.Duration
	// MaxRetries counts retries after the first attempt (negative = none).
	MaxRetries     int
	RetryBaseDelay time.Duration
	HTTPClient     *http.Client
}

// Provider streams model turns from an OpenAI-compatible API.
type Provider struct {
	hc         *http.Client
	endpoint   string
	apiKey     string
	model      string
	maxTokens  int
	tokenField string
	timeout    time.Duration
	maxRetries int
	retryBase  time.Duration
}

// New returns the driver, or llm.Disabled when neither an API key nor a
// base URL is configured (a key alone targets api.openai.com; a base URL
// alone is a local server without authentication).
func New(cfg Config) llm.Provider {
	key := strings.TrimSpace(cfg.APIKey)
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if key == "" && base == "" {
		return llm.Disabled{}
	}
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		// No overall client timeout: streams are bounded by RequestTimeout.
		hc = &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 90 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
		}}
	}
	p := &Provider{
		hc:         hc,
		endpoint:   base + "/chat/completions",
		apiKey:     key,
		model:      cfg.DefaultModel,
		maxTokens:  cfg.MaxTokens,
		tokenField: maxTokensField(base),
		timeout:    cfg.RequestTimeout,
		maxRetries: cfg.MaxRetries,
		retryBase:  cfg.RetryBaseDelay,
	}
	if p.maxTokens <= 0 {
		p.maxTokens = DefaultMaxTokens
	}
	if p.timeout <= 0 {
		p.timeout = DefaultRequestTimeout
	}
	if p.maxRetries < 0 {
		p.maxRetries = 0
	}
	if p.retryBase <= 0 {
		p.retryBase = DefaultRetryBaseDelay
	}
	return p
}

// NewFromConfig builds the driver from env config (OPENAI_*, AI_*).
func NewFromConfig(c config.AIConfig) llm.Provider {
	return New(Config{
		APIKey:         c.OpenAIAPIKey,
		BaseURL:        c.OpenAIBaseURL,
		DefaultModel:   c.DefaultModel,
		MaxTokens:      c.MaxTokens,
		RequestTimeout: c.RequestTimeout,
		MaxRetries:     DefaultMaxRetries,
	})
}

// maxTokensField: OpenAI's own API rejects max_tokens on its newer models
// and wants max_completion_tokens; compatible servers know max_tokens.
func maxTokensField(base string) string {
	if u, err := url.Parse(base); err == nil && strings.EqualFold(u.Hostname(), "api.openai.com") {
		return "max_completion_tokens"
	}
	return "max_tokens"
}

// Name implements llm.Provider.
func (*Provider) Name() string { return "openai" }

// Enabled implements llm.Provider.
func (*Provider) Enabled() bool { return true }

// Stream implements llm.Provider. The request is sent synchronously so
// connection failures and API errors (after retries) are returned directly;
// failures inside the stream arrive as llm.EventError.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	body, err := p.buildBody(req)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, p.timeout)
		resp, err := p.post(actx, body)
		if err == nil && resp.StatusCode == http.StatusOK {
			ch := make(chan llm.Event, 16)
			go p.run(actx, cancel, resp.Body, ch)
			return ch, nil
		}
		retryAfter := ""
		if err == nil {
			retryAfter = resp.Header.Get("Retry-After")
			err = apiError(resp)
		}
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt >= p.maxRetries || !retryable(err) {
			report(ctx, err, attempt+1)
			return nil, err
		}
		if err := sleep(ctx, p.backoff(attempt, retryAfter)); err != nil {
			return nil, err
		}
	}
}

func (p *Provider) post(ctx context.Context, body []byte) (*http.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	return p.hc.Do(hreq)
}

// --- Request ------------------------------------------------------------------

type wireMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wirePart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *wireImageURL `json:"image_url,omitempty"`
}

type wireImageURL struct {
	URL string `json:"url"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireTool struct {
	Type     string          `json:"type"`
	Function wireFunctionDef `json:"function"`
}

type wireFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

func (p *Provider) buildBody(req llm.Request) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		return nil, errors.New("openai: model is required")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	msgs, err := toMessages(req.System, req.Messages)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"model":          model,
		"messages":       msgs,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
		p.tokenField:     maxTokens,
	}
	if len(req.Tools) > 0 {
		tools := make([]wireTool, 0, len(req.Tools))
		for _, t := range req.Tools {
			params := t.InputSchema
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, wireTool{Type: "function", Function: wireFunctionDef{
				Name: t.Name, Description: t.Description, Parameters: params,
			}})
		}
		body["tools"] = tools
	}
	return json.Marshal(body)
}

// toMessages maps the Anthropic-shaped conversation. Prompt-cache marks have
// no equivalent and are dropped (OpenAI caches long prefixes by itself).
func toMessages(system []llm.SystemBlock, in []llm.Message) ([]wireMessage, error) {
	var out []wireMessage
	var sys []string
	for _, b := range system {
		if t := strings.TrimSpace(b.Text); t != "" {
			sys = append(sys, b.Text)
		}
	}
	if len(sys) > 0 {
		out = append(out, wireMessage{Role: "system", Content: strings.Join(sys, "\n\n")})
	}
	for _, m := range in {
		switch m.Role {
		case llm.RoleUser:
			// Tool results answer the previous assistant tool_calls and
			// must come first, one "tool" message each.
			var parts []wirePart
			for _, b := range m.Content {
				switch b.Type {
				case llm.BlockToolResult:
					content := b.Content
					if b.IsError {
						content = "ERROR: " + content
					}
					out = append(out, wireMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: content})
				case llm.BlockText:
					if b.Text != "" {
						parts = append(parts, wirePart{Type: "text", Text: b.Text})
					}
				case llm.BlockImage:
					parts = append(parts, wirePart{Type: "image_url", ImageURL: &wireImageURL{
						URL: "data:" + b.MediaType + ";base64," + b.Data,
					}})
				default:
					return nil, fmt.Errorf("openai: unsupported user block type %q", b.Type)
				}
			}
			if len(parts) > 0 {
				out = append(out, wireMessage{Role: "user", Content: userContent(parts)})
			}
		case llm.RoleAssistant:
			msg := wireMessage{Role: "assistant"}
			var text strings.Builder
			for _, b := range m.Content {
				switch b.Type {
				case llm.BlockText:
					text.WriteString(b.Text)
				case llm.BlockToolUse:
					args := strings.TrimSpace(string(b.Input))
					if args == "" || !json.Valid([]byte(args)) {
						args = "{}"
					}
					msg.ToolCalls = append(msg.ToolCalls, wireToolCall{
						ID: b.ID, Type: "function", Function: wireFunction{Name: b.Name, Arguments: args},
					})
				default:
					return nil, fmt.Errorf("openai: unsupported assistant block type %q", b.Type)
				}
			}
			if text.Len() > 0 {
				msg.Content = text.String()
			}
			if msg.Content == nil && len(msg.ToolCalls) == 0 {
				continue
			}
			out = append(out, msg)
		default:
			return nil, fmt.Errorf("openai: unsupported role %q", m.Role)
		}
	}
	return out, nil
}

// userContent sends plain text as a string (accepted by every server) and
// switches to content parts only when an image is present.
func userContent(parts []wirePart) any {
	for _, p := range parts {
		if p.Type != "text" {
			return parts
		}
	}
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// --- Stream -------------------------------------------------------------------

type chunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int64 `json:"prompt_tokens"`
		CompletionTokens    int64 `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *errorBody `json:"error"`
}

type toolCall struct {
	id   string
	name string
	args strings.Builder
}

// run reads the SSE stream and closes ch. Tool calls are complete only at
// the end of the turn, so they are emitted then, followed by usage and stop.
func (p *Provider) run(ctx context.Context, cancel context.CancelFunc, body io.ReadCloser, ch chan<- llm.Event) {
	defer close(ch)
	defer cancel()
	defer func() { _ = body.Close() }()
	send := func(ev llm.Event) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}
	fail := func(err error) {
		report(ctx, err, 1)
		send(llm.Event{Type: llm.EventError, Err: err})
	}
	var (
		text   strings.Builder
		calls  = map[int]*toolCall{}
		finish string
		model  string
		usage  llm.Usage
		done   bool
	)
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64<<10), maxSSELine)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			done = true
			break
		}
		var c chunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			fail(fmt.Errorf("openai: decode stream chunk: %w", err))
			return
		}
		if c.Error != nil {
			fail(&Error{Type: c.Error.typeName(), Message: truncate(c.Error.Message)})
			return
		}
		if c.Model != "" {
			model = c.Model
		}
		if c.Usage != nil {
			usage = usageOf(c.Usage.PromptTokens, c.Usage.CompletionTokens, c.Usage.PromptTokensDetails)
		}
		for _, choice := range c.Choices {
			if d := choice.Delta.Content; d != "" {
				text.WriteString(d)
				if !send(llm.Event{Type: llm.EventTextDelta, Text: d}) {
					return
				}
			}
			for _, tc := range choice.Delta.ToolCalls {
				call := calls[tc.Index]
				if call == nil {
					call = &toolCall{}
					calls[tc.Index] = call
				}
				if tc.ID != "" {
					call.id = tc.ID
				}
				if tc.Function.Name != "" {
					call.name = tc.Function.Name
				}
				call.args.WriteString(tc.Function.Arguments)
			}
			if choice.FinishReason != "" {
				finish = choice.FinishReason
			}
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			send(llm.Event{Type: llm.EventError, Err: ctx.Err()})
			return
		}
		fail(fmt.Errorf("openai: read stream: %w", err))
		return
	}
	if !done && finish == "" {
		fail(errors.New("openai: stream ended without a finish reason"))
		return
	}
	msg := llm.Message{Role: llm.RoleAssistant}
	if text.Len() > 0 {
		msg.Content = append(msg.Content, llm.TextBlock(text.String()))
	}
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for n, i := range idx {
		block := toolUseBlock(calls[i], n)
		msg.Content = append(msg.Content, block)
		if !send(llm.Event{Type: llm.EventToolUse, Block: block}) {
			return
		}
	}
	if !send(llm.Event{Type: llm.EventUsage, Usage: usage}) {
		return
	}
	stop := stopReason(finish)
	if len(idx) > 0 && stop == llm.StopEndTurn {
		// Some servers answer "stop" although the turn calls tools.
		stop = llm.StopToolUse
	}
	send(llm.Event{Type: llm.EventMessageStop, StopReason: stop, Model: model, Message: msg})
}

// toolUseBlock maps an assembled tool call. Missing ids (some local servers)
// get a stable one; malformed arguments are kept as a JSON string so tool
// validation reports them back instead of running with {}.
func toolUseBlock(c *toolCall, n int) llm.Block {
	id := c.id
	if id == "" {
		id = "call_" + strconv.Itoa(n)
	}
	input := json.RawMessage(strings.TrimSpace(c.args.String()))
	switch {
	case len(input) == 0:
		input = json.RawMessage(`{}`)
	case !json.Valid(input):
		quoted, _ := json.Marshal(string(input))
		input = quoted
	}
	return llm.Block{Type: llm.BlockToolUse, ID: id, Name: c.name, Input: input}
}

// usageOf: prompt_tokens includes the cached part; llm.Usage keeps it apart
// like the Anthropic driver (input = uncached input).
func usageOf(prompt, completion int64, details *struct {
	CachedTokens int64 `json:"cached_tokens"`
}) llm.Usage {
	var cached int64
	if details != nil {
		cached = details.CachedTokens
	}
	if cached > prompt {
		cached = prompt
	}
	return llm.Usage{InputTokens: prompt - cached, OutputTokens: completion, CacheReadTokens: cached}
}

func stopReason(r string) string {
	switch r {
	case "stop", "":
		return llm.StopEndTurn
	case "tool_calls", "function_call":
		return llm.StopToolUse
	case "length":
		return llm.StopMaxTokens
	case "content_filter":
		return llm.StopRefusal
	}
	return llm.StopOther
}

// --- Errors -------------------------------------------------------------------

type errorBody struct {
	Message string          `json:"message"`
	Type    json.RawMessage `json:"type"`
	Code    json.RawMessage `json:"code"`
}

// typeName prefers the string "type", then the string "code".
func (e *errorBody) typeName() string {
	for _, raw := range []json.RawMessage{e.Type, e.Code} {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
	}
	return "api_error"
}

// Error is a provider failure safe to log (no secrets, message truncated).
type Error struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *Error) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("openai: %d %s: %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("openai: %s: %s", e.Type, e.Message)
}

// Retryable reports whether the call may succeed when repeated.
func (e *Error) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

func apiError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	e := &Error{StatusCode: resp.StatusCode, Type: http.StatusText(resp.StatusCode), Message: strings.TrimSpace(string(raw))}
	var body struct {
		Error *errorBody `json:"error"`
	}
	if json.Unmarshal(raw, &body) == nil && body.Error != nil {
		e.Type = body.Error.typeName()
		if body.Error.Message != "" {
			e.Message = body.Error.Message
		}
	}
	e.Message = truncate(e.Message)
	return e
}

func truncate(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

func retryable(err error) bool {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Retryable()
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	// Transport failures (reset, refused, attempt timeout) are transient.
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded)
}

// backoff is base·2^attempt, raised to the server's Retry-After (capped).
func (p *Provider) backoff(attempt int, retryAfter string) time.Duration {
	d := p.retryBase << attempt
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s > 0 {
		if ra := min(time.Duration(s)*time.Second, maxRetryAfter); ra > d {
			d = ra
		}
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func report(ctx context.Context, err error, attempts int) {
	tags := errtrack.Tags{"category": ErrorCategory, "attempts": strconv.Itoa(attempts), "provider": "openai"}
	var pe *Error
	if errors.As(err, &pe) {
		tags["status"] = strconv.Itoa(pe.StatusCode)
		tags["error_type"] = pe.Type
	}
	errtrack.Capture(ctx, errtrack.ModuleAI, err, tags)
}
