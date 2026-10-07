// Package anthropic is the llm.Provider driver for the Anthropic Messages API
// (official Go SDK, streaming). System prompt and tool definitions carry
// prompt-cache breakpoints; 429/529/5xx are retried with exponential backoff
// before the first event, 400/401 never.
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
)

// Defaults (AI_MAX_TOKENS, AI_REQUEST_TIMEOUT).
const (
	DefaultMaxTokens      = 8000
	DefaultRequestTimeout = 120 * time.Second
	DefaultMaxRetries     = 3
	DefaultRetryBaseDelay = time.Second
	maxRetryAfter         = 30 * time.Second
)

// ErrorCategory is the errtrack "category" tag of provider failures (module ai).
const ErrorCategory = "ai.provider"

// Config builds the driver. An empty APIKey yields llm.Disabled.
type Config struct {
	APIKey  string
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

// Provider streams model turns from the Messages API.
type Provider struct {
	client     sdk.Client
	model      string
	maxTokens  int
	timeout    time.Duration
	maxRetries int
	retryBase  time.Duration
}

// New returns the Anthropic driver, or llm.Disabled when no API key is set.
func New(cfg Config) llm.Provider {
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		return llm.Disabled{}
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
	opts := []option.RequestOption{
		option.WithAPIKey(key),
		option.WithHTTPClient(hc),
		// Retries are ours (status filter, backoff, errtrack).
		option.WithMaxRetries(0),
	}
	if base := strings.TrimSpace(cfg.BaseURL); base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	p := &Provider{
		client:     sdk.NewClient(opts...),
		model:      cfg.DefaultModel,
		maxTokens:  cfg.MaxTokens,
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

// NewFromConfig builds the driver from env config (ANTHROPIC_*, AI_*).
func NewFromConfig(c config.AIConfig) llm.Provider {
	return New(Config{
		APIKey:         c.APIKey,
		BaseURL:        c.BaseURL,
		DefaultModel:   c.DefaultModel,
		MaxTokens:      c.MaxTokens,
		RequestTimeout: c.RequestTimeout,
		MaxRetries:     DefaultMaxRetries,
	})
}

// Name implements llm.Provider.
func (*Provider) Name() string { return "anthropic" }

// Enabled implements llm.Provider.
func (*Provider) Enabled() bool { return true }

// Stream implements llm.Provider. The request is sent and its first event
// read synchronously so connection failures and API errors (after retries)
// are returned directly.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	params, err := p.buildParams(req)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, p.timeout)
		stream := p.client.Messages.NewStreaming(actx, params)
		if stream.Next() {
			ch := make(chan llm.Event, 16)
			go p.run(actx, cancel, stream, ch)
			return ch, nil
		}
		err := stream.Err()
		_ = stream.Close()
		cancel()
		if err == nil {
			err = errors.New("anthropic: empty stream")
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt >= p.maxRetries || !retryable(err) {
			err = wrapErr(err)
			report(ctx, err, attempt+1)
			return nil, err
		}
		if err := sleep(ctx, p.backoff(attempt, err)); err != nil {
			return nil, err
		}
	}
}

// run consumes the stream (its first event is already current) and closes ch.
func (p *Provider) run(ctx context.Context, cancel context.CancelFunc, stream *ssestream.Stream[sdk.MessageStreamEventUnion], ch chan<- llm.Event) {
	defer close(ch)
	defer cancel()
	defer func() { _ = stream.Close() }()
	send := func(ev llm.Event) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}
	fail := func(err error) {
		err = wrapErr(err)
		report(ctx, err, 1)
		send(llm.Event{Type: llm.EventError, Err: err})
	}
	msg := sdk.Message{}
	for {
		ev := stream.Current()
		if err := msg.Accumulate(ev); err != nil {
			fail(fmt.Errorf("anthropic: accumulate: %w", err))
			return
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				if !send(llm.Event{Type: llm.EventTextDelta, Text: ev.Delta.Text}) {
					return
				}
			}
		case "content_block_stop":
			if i := int(ev.Index); i < len(msg.Content) && msg.Content[i].Type == "tool_use" {
				if !send(llm.Event{Type: llm.EventToolUse, Block: toolUseBlock(msg.Content[i])}) {
					return
				}
			}
		case "message_stop":
			if !send(llm.Event{Type: llm.EventUsage, Usage: usageOf(msg.Usage)}) {
				return
			}
			send(llm.Event{
				Type:       llm.EventMessageStop,
				StopReason: stopReason(msg.StopReason),
				Model:      string(msg.Model),
				Message:    messageOf(msg),
			})
			return
		}
		if !stream.Next() {
			break
		}
	}
	if err := stream.Err(); err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			send(llm.Event{Type: llm.EventError, Err: err})
			return
		}
		fail(err)
		return
	}
	fail(errors.New("anthropic: stream ended without message_stop"))
}

func (p *Provider) buildParams(req llm.Request) (sdk.MessageNewParams, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		return sdk.MessageNewParams{}, errors.New("anthropic: model is required")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	params := sdk.MessageNewParams{
		Model:     sdk.Model(model),
		MaxTokens: int64(maxTokens),
	}
	params.System = systemBlocks(req.System)
	for i, t := range req.Tools {
		tool := toolParam(t)
		if i == len(req.Tools)-1 {
			tool.CacheControl = sdk.NewCacheControlEphemeralParam()
		}
		params.Tools = append(params.Tools, sdk.ToolUnionParam{OfTool: &tool})
	}
	if req.CacheMessages {
		params.CacheControl = sdk.NewCacheControlEphemeralParam()
	}
	msgs, err := toMessages(req.Messages)
	if err != nil {
		return params, err
	}
	params.Messages = msgs
	return params, nil
}

// systemBlocks places cache breakpoints on blocks marked Cache, or on the
// last block when none is marked.
func systemBlocks(in []llm.SystemBlock) []sdk.TextBlockParam {
	marked := false
	for _, b := range in {
		if b.Cache && strings.TrimSpace(b.Text) != "" {
			marked = true
		}
	}
	var out []sdk.TextBlockParam
	for _, b := range in {
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		block := sdk.TextBlockParam{Text: b.Text}
		if b.Cache {
			block.CacheControl = sdk.NewCacheControlEphemeralParam()
		}
		out = append(out, block)
	}
	if !marked && len(out) > 0 {
		out[len(out)-1].CacheControl = sdk.NewCacheControlEphemeralParam()
	}
	return out
}

func toolParam(t llm.ToolDef) sdk.ToolParam {
	props, _ := t.InputSchema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	var required []string
	switch r := t.InputSchema["required"].(type) {
	case []string:
		required = r
	case []any:
		for _, v := range r {
			if s, ok := v.(string); ok {
				required = append(required, s)
			}
		}
	}
	extras := map[string]any{}
	for k, v := range t.InputSchema {
		switch k {
		case "type", "properties", "required":
		default:
			extras[k] = v
		}
	}
	tool := sdk.ToolParam{
		Name: t.Name,
		InputSchema: sdk.ToolInputSchemaParam{
			Properties: props,
			Required:   required,
		},
	}
	if len(extras) > 0 {
		tool.InputSchema.ExtraFields = extras
	}
	if t.Description != "" {
		tool.Description = sdk.String(t.Description)
	}
	return tool
}

func toMessages(in []llm.Message) ([]sdk.MessageParam, error) {
	out := make([]sdk.MessageParam, 0, len(in))
	for _, m := range in {
		blocks := make([]sdk.ContentBlockParamUnion, 0, len(m.Content))
		for _, b := range m.Content {
			switch b.Type {
			case llm.BlockText:
				if b.Text != "" {
					blocks = append(blocks, sdk.NewTextBlock(b.Text))
				}
			case llm.BlockImage:
				blocks = append(blocks, sdk.NewImageBlockBase64(b.MediaType, b.Data))
			case llm.BlockToolUse:
				input := b.Input
				if len(input) == 0 || !json.Valid(input) {
					input = json.RawMessage(`{}`)
				}
				blocks = append(blocks, sdk.NewToolUseBlock(b.ID, input, b.Name))
			case llm.BlockToolResult:
				blocks = append(blocks, sdk.NewToolResultBlock(b.ToolUseID, b.Content, b.IsError))
			default:
				return nil, fmt.Errorf("anthropic: unsupported block type %q", b.Type)
			}
		}
		if len(blocks) == 0 {
			continue
		}
		switch m.Role {
		case llm.RoleUser:
			out = append(out, sdk.NewUserMessage(blocks...))
		case llm.RoleAssistant:
			out = append(out, sdk.NewAssistantMessage(blocks...))
		default:
			return nil, fmt.Errorf("anthropic: unsupported role %q", m.Role)
		}
	}
	return out, nil
}

// toolUseBlock maps a finished tool_use block. Malformed input is kept as a
// JSON string so tool validation reports it back instead of running with {}.
func toolUseBlock(cb sdk.ContentBlockUnion) llm.Block {
	input := json.RawMessage(strings.TrimSpace(string(cb.Input)))
	switch {
	case len(input) == 0:
		input = json.RawMessage(`{}`)
	case !json.Valid(input):
		quoted, _ := json.Marshal(string(input))
		input = quoted
	}
	return llm.Block{Type: llm.BlockToolUse, ID: cb.ID, Name: cb.Name, Input: input}
}

func messageOf(msg sdk.Message) llm.Message {
	out := llm.Message{Role: llm.RoleAssistant}
	for _, cb := range msg.Content {
		switch cb.Type {
		case "text":
			if cb.Text != "" {
				out.Content = append(out.Content, llm.TextBlock(cb.Text))
			}
		case "tool_use":
			out.Content = append(out.Content, toolUseBlock(cb))
		}
	}
	return out
}

func usageOf(u sdk.Usage) llm.Usage {
	return llm.Usage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

func stopReason(r sdk.StopReason) string {
	switch r {
	case sdk.StopReasonEndTurn, sdk.StopReasonStopSequence:
		return llm.StopEndTurn
	case sdk.StopReasonToolUse:
		return llm.StopToolUse
	case sdk.StopReasonMaxTokens:
		return llm.StopMaxTokens
	case sdk.StopReasonRefusal:
		return llm.StopRefusal
	}
	return llm.StopOther
}

// Error is a provider failure safe to log (no secrets, message truncated).
type Error struct {
	StatusCode int
	Type       string
	Message    string
	Err        error
}

func (e *Error) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("anthropic: %d %s: %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("anthropic: %s: %s", e.Type, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable reports whether the call may succeed when repeated.
func (e *Error) Retryable() bool { return retryableStatus(e.StatusCode, e.Type) }

func wrapErr(err error) error {
	var apiErr *sdk.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	status, typ := apiStatus(apiErr)
	msg := strings.TrimSpace(apiErr.RawJSON())
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(msg), &body) == nil && body.Error.Message != "" {
		msg = body.Error.Message
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return &Error{StatusCode: status, Type: typ, Message: msg, Err: err}
}

// apiStatus returns the effective status: an SSE error event arrives on a
// 200 response and is mapped from its error type.
func apiStatus(e *sdk.Error) (int, string) {
	typ := string(e.Type())
	status := e.StatusCode
	if status < 400 {
		switch typ {
		case "overloaded_error":
			status = 529
		case "rate_limit_error":
			status = http.StatusTooManyRequests
		case "api_error":
			status = http.StatusInternalServerError
		}
	}
	return status, typ
}

func retryableStatus(status int, typ string) bool {
	switch {
	case status == http.StatusTooManyRequests, status >= 500:
		return true
	case status == 0:
		return typ == "overloaded_error" || typ == "rate_limit_error" || typ == "api_error"
	}
	return false
}

func retryable(err error) bool {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		return retryableStatus(apiStatus(apiErr))
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	// Transport failures (reset, refused, attempt timeout) are transient.
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded)
}

// backoff is base·2^attempt, raised to the server's Retry-After (capped).
func (p *Provider) backoff(attempt int, err error) time.Duration {
	d := p.retryBase << attempt
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) && apiErr.Response != nil {
		if s, perr := strconv.Atoi(strings.TrimSpace(apiErr.Response.Header.Get("Retry-After"))); perr == nil && s > 0 {
			if ra := min(time.Duration(s)*time.Second, maxRetryAfter); ra > d {
				d = ra
			}
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
	tags := errtrack.Tags{"category": ErrorCategory, "attempts": strconv.Itoa(attempts)}
	var pe *Error
	if errors.As(err, &pe) {
		tags["status"] = strconv.Itoa(pe.StatusCode)
		tags["error_type"] = pe.Type
	}
	errtrack.Capture(ctx, errtrack.ModuleAI, err, tags)
}
