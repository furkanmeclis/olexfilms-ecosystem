// Package llm is the LLM provider abstraction of the AI assistant (F4,
// design §AI asistan). Messages use the Anthropic block shape (text /
// image / tool_use / tool_result) and a provider streams one model turn as a
// channel of events. Drivers: anthropic (Messages API, official Go SDK) and
// fake (scripted, for tests). The API key comes only from env; without it
// the provider is disabled and every call answers ErrUnavailable (the HTTP
// layer maps that to 503 AI_UNAVAILABLE).
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

// Roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Block types.
const (
	BlockText       = "text"
	BlockImage      = "image"
	BlockToolUse    = "tool_use"
	BlockToolResult = "tool_result"
)

// Normalized stop reasons.
const (
	StopEndTurn   = "end_turn"
	StopToolUse   = "tool_use"
	StopMaxTokens = "max_tokens"
	StopRefusal   = "refusal"
	StopOther     = "other"
)

// ErrUnavailable is returned when no provider is configured (no
// ANTHROPIC_API_KEY). Callers answer 503 AI_UNAVAILABLE.
var ErrUnavailable = errors.New("llm: provider unavailable")

// Block is one content block of a message.
type Block struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// image: base64 data with its media type (image/jpeg|png|gif|webp).
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// TextBlock builds a text block.
func TextBlock(text string) Block { return Block{Type: BlockText, Text: text} }

// ToolResultBlock builds a tool_result block.
func ToolResultBlock(toolUseID, content string, isError bool) Block {
	return Block{Type: BlockToolResult, ToolUseID: toolUseID, Content: content, IsError: isError}
}

// Message is a single conversation turn.
type Message struct {
	Role    string  `json:"role"`
	Content []Block `json:"content"`
}

// ToolUses returns the tool_use blocks of a message.
func (m Message) ToolUses() []Block {
	var out []Block
	for _, b := range m.Content {
		if b.Type == BlockToolUse {
			out = append(out, b)
		}
	}
	return out
}

// Text concatenates the text blocks of a message.
func (m Message) Text() string {
	var sb strings.Builder
	for _, b := range m.Content {
		if b.Type == BlockText {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// SystemBlock is one system prompt segment. Cache marks the end of the
// stable, cacheable prefix; when no block sets it the last block carries the
// cache breakpoint.
type SystemBlock struct {
	Text  string
	Cache bool
}

// ToolDef is a function tool offered to the model. InputSchema is a JSON
// Schema object ({"type":"object","properties":{...},"required":[...]}).
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Request is one model turn.
type Request struct {
	// Model is the resolved model id (see Models); empty uses the driver
	// default.
	Model    string
	System   []SystemBlock
	Messages []Message
	// Tools always get a cache breakpoint on the last definition.
	Tools []ToolDef
	// MaxTokens <= 0 uses the driver default (AI_MAX_TOKENS).
	MaxTokens int
	// CacheMessages also caches the conversation so far (automatic
	// breakpoint on the last cacheable block).
	CacheMessages bool
}

// Usage is the token accounting of one call. Quota is charged on real usage.
type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

// Add accumulates another usage record.
func (u *Usage) Add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadTokens += o.CacheReadTokens
	u.CacheWriteTokens += o.CacheWriteTokens
}

// EventType enumerates stream events.
type EventType int

// Stream event types. A successful stream ends with EventUsage then
// EventMessageStop; a failed one ends with EventError. The channel is closed
// after the last event.
const (
	// EventTextDelta carries a text fragment in Text.
	EventTextDelta EventType = iota + 1
	// EventToolUse carries a complete tool_use block (input parsed) in Block.
	EventToolUse
	// EventUsage carries the final token usage of the turn.
	EventUsage
	// EventMessageStop carries StopReason, Model and the assembled Message.
	EventMessageStop
	// EventError carries a mid-stream failure in Err.
	EventError
)

func (t EventType) String() string {
	switch t {
	case EventTextDelta:
		return "text_delta"
	case EventToolUse:
		return "tool_use"
	case EventUsage:
		return "usage"
	case EventMessageStop:
		return "message_stop"
	case EventError:
		return "error"
	}
	return fmt.Sprintf("event(%d)", int(t))
}

// Event is one streamed provider event.
type Event struct {
	Type       EventType
	Text       string
	Block      Block
	Usage      Usage
	StopReason string
	Model      string
	Message    Message
	Err        error
}

// Provider is an LLM backend.
type Provider interface {
	// Name is the driver name (anthropic, fake, disabled).
	Name() string
	// Enabled reports whether calls can succeed (credentials present).
	Enabled() bool
	// Stream runs one model turn. Errors before the first event (no key,
	// rejected request, retries exhausted) are returned directly; later
	// failures arrive as EventError. The channel is closed at the end and
	// must be drained (or ctx cancelled).
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}

// Response is a fully collected turn.
type Response struct {
	Message    Message
	StopReason string
	Model      string
	Usage      Usage
}

// Collect drains a stream into a Response, returning the first EventError.
func Collect(ch <-chan Event) (Response, error) {
	var (
		resp Response
		err  error
	)
	for ev := range ch {
		switch ev.Type {
		case EventUsage:
			resp.Usage = ev.Usage
		case EventMessageStop:
			resp.Message = ev.Message
			resp.StopReason = ev.StopReason
			resp.Model = ev.Model
		case EventError:
			if err == nil {
				err = ev.Err
			}
		}
	}
	if err == nil && resp.StopReason == "" {
		err = errors.New("llm: stream ended without message_stop")
	}
	return resp, err
}

// Complete runs a turn and collects it (titles, language detection, triage).
func Complete(ctx context.Context, p Provider, req Request) (Response, error) {
	ch, err := p.Stream(ctx, req)
	if err != nil {
		return Response{}, err
	}
	return Collect(ch)
}

// Disabled is the provider used when ANTHROPIC_API_KEY is empty.
type Disabled struct{}

// Name implements Provider.
func (Disabled) Name() string { return "disabled" }

// Enabled implements Provider.
func (Disabled) Enabled() bool { return false }

// Stream implements Provider.
func (Disabled) Stream(context.Context, Request) (<-chan Event, error) {
	return nil, ErrUnavailable
}

// Models resolves the chat (default) and fast model ids: env values
// (AI_MODEL_DEFAULT / AI_MODEL_FAST) are the fallback for empty
// ai_settings.default_model / fast_model, and only ids in Allowed
// (AI_ALLOWED_MODELS) are used.
type Models struct {
	Default string
	Fast    string
	Allowed []string
}

// ModelsFromConfig reads AI_MODEL_DEFAULT, AI_MODEL_FAST and AI_ALLOWED_MODELS.
func ModelsFromConfig(c config.AIConfig) Models {
	return Models{Default: c.DefaultModel, Fast: c.FastModel, Allowed: c.AllowedModels}
}

// IsAllowed reports whether model is on the allow list. An empty list
// allows only the env default and fast models.
func (m Models) IsAllowed(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	if len(m.Allowed) == 0 {
		return model == m.Default || model == m.Fast
	}
	return slices.Contains(m.Allowed, model)
}

// ResolveDefault returns the chat model for an ai_settings.default_model
// value: empty or not allowed falls back to the env default.
func (m Models) ResolveDefault(configured string) string {
	return m.resolve(configured, m.Default)
}

// ResolveFast returns the fast model (titles, language detection, triage)
// for an ai_settings.fast_model value: empty or not allowed falls back to
// the env value.
func (m Models) ResolveFast(configured string) string {
	return m.resolve(configured, m.Fast)
}

func (m Models) resolve(configured, fallback string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" && m.IsAllowed(configured) {
		return configured
	}
	return fallback
}
