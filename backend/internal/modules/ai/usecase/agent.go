package usecase

// agent.go (TEC-396, F4-02c) is the channel adapter of the agent loop for
// channels without an ai_conversations row: the WhatsApp pipeline keeps its
// own conversation (conversations / messages) and gate, and runs one turn
// here. The loop, the tool set of the realm, the confirmation card
// (Actions.Propose), the quota re-check before every model call and the
// usage booking are exactly those of the panel / portal chat.

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
)

// ChannelWhatsApp marks WhatsApp turns in the prompt facts.
const ChannelWhatsApp = "whatsapp"

// whatsAppPrompt is the channel part of the cached system prompt.
const whatsAppPrompt = `# WhatsApp channel
- You are answering on WhatsApp. Keep answers short (a few sentences), plain text; WhatsApp formatting only (*bold*, _italic_), no headings, tables, HTML or Markdown links (write the URL itself).
- Several user messages sent in a row arrive together in one turn: answer them together.
- A change you propose is shown to the user as a text question; they confirm by replying YES or cancel by replying NO in their language.
- Images the user sent are attached; voice messages and documents cannot be read.`

// AgentFacts are the prompt facts of an agent turn.
type AgentFacts struct {
	Brand    string
	Org      string
	OrgType  string
	User     string
	Role     string
	Locale   string
	Timezone string
	Customer bool
	Visitor  bool
}

// AgentInput is one turn of a channel adapter.
type AgentInput struct {
	Principal tools.Principal
	// OrgID / BrandID / Pool are the quota owner: the user's organization
	// (pool org) or the brand center (pool system).
	OrgID   int64
	BrandID int64
	Pool    string
	// Source / SourceRef are the pending action origin (model.Source*, the
	// channel conversation uuid).
	Source    string
	SourceRef string
	Facts     AgentFacts
	// ReadOnly hides the write tools (read_only organization, inactive
	// user).
	ReadOnly bool
	// History is the earlier conversation; Content the new user message
	// (the current time context is prepended here).
	History []llm.Message
	Content []llm.Block
}

// AgentToolCall summarizes one tool call of the turn (no payloads).
type AgentToolCall struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Code   string `json:"code,omitempty"`
}

// AgentResult is the outcome of an agent turn.
type AgentResult struct {
	// Text is the assistant text of the turn.
	Text string
	// Card is set when a write tool became a confirmation card.
	Card *Card
	// Status is model.MessageComplete or model.MessageError.
	Status     string
	StopReason string
	// ErrorCode is a Stream* code (provider_error, turn_limit, refusal,
	// max_tokens, quota_exceeded); Error the stored error text.
	ErrorCode string
	Error     string
	Model     string
	Usage     llm.Usage
	ToolCalls []AgentToolCall
	// QuotaExceeded: the pool was already spent; no model call was made.
	QuotaExceeded bool
}

// QuotaExceeded reports whether the monthly pool of in is spent (limit 0
// = unlimited). It is the pre-check of a channel before any model call.
func (c *Chat) QuotaExceeded(ctx context.Context, orgID int64, pool string) (bool, error) {
	settings, err := c.Store.Settings(ctx)
	if err != nil {
		return false, err
	}
	limit, used, err := c.quota(ctx, &session{orgID: orgID, pool: pool}, settings)
	if err != nil {
		return false, err
	}
	return exceeded(limit, used), nil
}

// ModuleOn reports whether the assistant is on for the organization
// (ai_assistant module and ai_org_settings.enabled).
func (c *Chat) ModuleOn(ctx context.Context, orgID int64) (bool, error) {
	return c.moduleOn(ctx, orgID)
}

// ProviderEnabled reports whether a model provider is configured.
func (c *Chat) ProviderEnabled() bool { return c.Provider.Enabled() }

// RunAgent runs one turn of the agent loop for a channel adapter. Open
// confirmation cards of the same source are cancelled first (a new
// message instead of a decision). Model and tool failures are reported in
// the result; only setup failures (settings, quota) are errors.
func (c *Chat) RunAgent(ctx context.Context, in AgentInput) (AgentResult, error) {
	if !c.Provider.Enabled() {
		return AgentResult{}, ErrUnavailable
	}
	settings, err := c.Store.Settings(ctx)
	if err != nil {
		return AgentResult{}, err
	}
	s := &session{
		caller:  Caller{Auth: in.Principal.Auth, Channel: ChannelWhatsApp},
		orgID:   in.OrgID,
		brandID: in.BrandID,
		pool:    in.Pool,
		source:  in.Source,
		channel: ChannelWhatsApp,
		usage:   model.UsageChannelWhatsApp,
		tools:   in.Principal,
	}
	limit, used, err := c.quota(ctx, s, settings)
	if err != nil {
		return AgentResult{}, err
	}
	if exceeded(limit, used) {
		return AgentResult{Status: model.MessageError, ErrorCode: StreamQuotaExceeded, QuotaExceeded: true}, nil
	}
	loc, err := time.LoadLocation(in.Facts.Timezone)
	if err != nil || in.Facts.Timezone == "" {
		loc = time.UTC
	}
	f := in.Facts
	t := &Turn{
		s: s, settings: settings, loc: loc, limit: limit,
		facts: sessionFacts{
			Brand: f.Brand, Org: f.Org, OrgType: f.OrgType, User: f.User, Role: f.Role,
			Locale: f.Locale, Timezone: loc.String(), Customer: f.Customer, Visitor: f.Visitor, Channel: ChannelWhatsApp,
		},
		ref: in.SourceRef, readOnly: in.ReadOnly, channelPrompt: whatsAppPrompt,
	}
	if c.Actions != nil && in.Source != "" && in.SourceRef != "" {
		if _, err := c.Actions.CancelForSource(ctx, s.tools, in.Source, in.SourceRef); err != nil {
			return AgentResult{}, err
		}
	}
	content := append([]llm.Block{turnContext(c.now(), loc)}, in.Content...)
	history := append(append([]llm.Message(nil), in.History...), llm.Message{Role: llm.RoleUser, Content: content})

	ts := &turnState{emit: func(string, any) {}}
	_, status, stop, errText := c.loop(ctx, t, ts, history)
	out := AgentResult{Status: status, StopReason: stop, Error: errText, Model: ts.model, Usage: ts.usage}
	var texts []string
	for _, b := range ts.ui {
		switch b.Type {
		case "text":
			if txt := strings.TrimSpace(b.Text); txt != "" {
				texts = append(texts, txt)
			}
		case "confirm":
			if b.Card != nil {
				card := *b.Card
				out.Card = &card
			}
		case "tool":
			out.ToolCalls = append(out.ToolCalls, AgentToolCall{Name: b.Name, Status: b.Status, Code: b.Code})
		case "error":
			out.ErrorCode = b.Code
			if b.Code == StreamQuotaExceeded {
				out.QuotaExceeded = true
			}
		}
	}
	out.Text = strings.Join(texts, "\n\n")
	return out, nil
}
