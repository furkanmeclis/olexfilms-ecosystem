package usecase

// chat.go (TEC-388, F4-01f) is the panel and portal assistant chat: the
// conversations, the pre-checks of a turn and the agent loop streamed as
// server-sent events. Both channels share it; the realm decides the
// organization, the quota pool and the tool set:
//
//	panel  → the active organization, its own quota (pool org), panel tools
//	portal → the brand center, the system pool (QUESTIONS S1/S17), customer
//	         tools
//
// Pre-checks of a turn, in order: module (ai_assistant + ai_org_settings)
// → permission (ai.use; portal: the customer realm) → provider → AI
// guidelines consent (428) → quota (403) → rate limit (429) → message and
// conversation limits. A turn calls the model at most MaxModelCalls times;
// a write tool stops it at the confirmation card and the confirm / cancel
// endpoints resume it from there.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	legal "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Limits of the chat.
const (
	// MaxMessageChars caps a user message (characters).
	MaxMessageChars = 8000
	// MaxConversationMessages caps the stored messages of a conversation.
	MaxConversationMessages = 400
	// MaxModelCalls caps the model calls of one turn (tool loop).
	MaxModelCalls = 8
	// TurnRateLimit turns per user in TurnRateWindow (messages, confirms
	// and cancels).
	TurnRateLimit  = 30
	TurnRateWindow = 5 * time.Minute
	// MaxTitleChars is the title column size.
	MaxTitleChars = 200

	titleTimeout   = 20 * time.Second
	titleMaxTokens = 64
	fallbackTitle  = 60
)

// Error codes of the chat endpoints.
const (
	CodeConsentRequired   = "AI_CONSENT_REQUIRED"
	CodeQuotaExceeded     = "AI_QUOTA_EXCEEDED"
	CodeUnavailable       = "AI_UNAVAILABLE"
	CodeConversationLimit = "AI_CONVERSATION_LIMIT"
)

// Errors of the chat.
var (
	ErrFeatureDisabled = errors.New("ai chat: module disabled")
	ErrForbidden       = errors.New("ai chat: not allowed")
	ErrUnavailable     = errors.New("ai chat: provider unavailable")
	ErrQuotaExceeded   = errors.New("ai chat: monthly quota exceeded")
	// ErrNotFound: no such conversation of the caller.
	ErrNotFound          = errors.New("ai chat: conversation not found")
	ErrConversationLimit = errors.New("ai chat: conversation is full")
)

// ConsentRequiredError: the user has not accepted the current AI
// guidelines (428); Text is the version to accept.
type ConsentRequiredError struct{ Text legal.Text }

func (e *ConsentRequiredError) Error() string { return "ai chat: AI guidelines consent required" }

// RateLimitedError: too many turns (429).
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "ai chat: rate limited" }

// ChatErrorStatus maps a chat error to the HTTP status and error code;
// ok is false for internal errors. Action errors map through ErrorStatus.
func ChatErrorStatus(err error) (status int, code string, ok bool) {
	var ce *ConsentRequiredError
	var re *RateLimitedError
	var ve *ValidationError
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &ce):
		return http.StatusPreconditionRequired, CodeConsentRequired, true
	case errors.As(err, &re):
		return http.StatusTooManyRequests, "RATE_LIMITED", true
	case errors.As(err, &ve), errors.As(err, &qe):
		return http.StatusBadRequest, "VALIDATION_ERROR", true
	case errors.Is(err, ErrFeatureDisabled):
		return http.StatusForbidden, "FEATURE_DISABLED", true
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, "FORBIDDEN", true
	case errors.Is(err, ErrQuotaExceeded):
		return http.StatusForbidden, CodeQuotaExceeded, true
	case errors.Is(err, ErrUnavailable):
		return http.StatusServiceUnavailable, CodeUnavailable, true
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND", true
	case errors.Is(err, ErrConversationLimit):
		return http.StatusUnprocessableEntity, CodeConversationLimit, true
	}
	return ErrorStatus(err)
}

// SSE event names.
const (
	EventMessageStart  = "message_start"
	EventTextDelta     = "text_delta"
	EventToolStart     = "tool_start"
	EventToolResult    = "tool_result"
	EventConfirm       = "confirm"
	EventAction        = "action"
	EventQuotaExceeded = "quota_exceeded"
	EventError         = "error"
	EventMessageDone   = "message_done"
	EventTitle         = "title"
)

// Error event codes (the "code" of an error event and of a ui error block).
const (
	StreamProviderError = "provider_error"
	StreamTurnLimit     = "turn_limit"
	StreamRefusal       = "refusal"
	StreamMaxTokens     = "max_tokens"
	StreamQuotaExceeded = "quota_exceeded"
)

// Emitter delivers one server-sent event.
type Emitter func(event string, data any)

// --- dependencies ------------------------------------------------------------

// ChatStore is the chat persistence (ai/repository.Store).
type ChatStore interface {
	Settings(ctx context.Context) (db.AiSetting, error)
	OrgSettings(ctx context.Context, orgID int64) (db.AiOrgSetting, bool, error)
	MonthlyTokens(ctx context.Context, orgID int64, pool string, at time.Time) (int64, error)
	RecordUsageEvents(ctx context.Context, u repository.Usage, enq repository.EventEnqueuer,
		build func(db.AiUsage, db.AiUsageMonthly) []events.Event) (db.AiUsage, db.AiUsageMonthly, error)
	ListConversations(ctx context.Context, f repository.ConversationFilter) ([]db.AiConversation, int64, error)
	CreateConversation(ctx context.Context, p db.CreateAIConversationParams) (db.AiConversation, error)
	ConversationForUser(ctx context.Context, id uuid.UUID, orgID, userID int64, channel string) (db.AiConversation, bool, error)
	RenameConversation(ctx context.Context, id int64, title string) (db.AiConversation, error)
	DeleteConversation(ctx context.Context, id uuid.UUID, orgID, userID int64) (bool, error)
	TouchConversation(ctx context.Context, id int64) error
	CreateMessage(ctx context.Context, p db.CreateAIMessageParams) (db.AiMessage, error)
	FinishMessage(ctx context.Context, p db.FinishAIMessageParams) (db.AiMessage, error)
	Messages(ctx context.Context, conversationID int64) ([]db.AiMessage, error)
	ChatContext(ctx context.Context, userID, orgID int64) (db.GetAIChatContextRow, error)
	BrandCenter(ctx context.Context, brandID int64) (db.Organization, error)
}

// ChatTools is the tool registry surface of the loop (*tools.Registry).
type ChatTools interface {
	Available(ctx context.Context, p tools.Principal) ([]tools.Tool, error)
	Call(ctx context.Context, p tools.Principal, name string, input json.RawMessage) (tools.Result, error)
}

// FeatureChecker reports whether a module is on (features.Service).
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// ConsentChecker is the AI guidelines gate (legal usecase.Service).
type ConsentChecker interface {
	AIConsentRequired(ctx context.Context, userID int64, locale i18n.Locale) (*legal.Text, error)
}

// RateLimiter is a fixed-window limiter (ratelimit.Limiter).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// ChatDeps wires the chat.
type ChatDeps struct {
	Store    ChatStore
	Tools    ChatTools
	Actions  *Actions
	Provider llm.Provider
	Models   llm.Models
	Features FeatureChecker
	Consents ConsentChecker
	// Limiter nil disables the rate limit.
	Limiter RateLimiter
	// Outbox writes the quota threshold events; nil skips them.
	Outbox repository.EventEnqueuer
	Log    *slog.Logger
}

// Chat is the assistant chat use case.
type Chat struct {
	ChatDeps
	now func() time.Time
}

// NewChat builds the chat.
func NewChat(d ChatDeps) *Chat {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Provider == nil {
		d.Provider = llm.Disabled{}
	}
	return &Chat{ChatDeps: d, now: time.Now}
}

// SetClock replaces the clock (tests).
func (c *Chat) SetClock(now func() time.Time) { c.now = now }

// --- caller and session ------------------------------------------------------

// Caller is who chats. Panel callers carry the active organization;
// portal callers the brand of the request domain.
type Caller struct {
	Auth    authctx.Principal
	Channel string
	Org     *orgctx.Scope
	Brand   *brandctx.Brand
	// AcceptLanguage is the request header (K10 fallback).
	AcceptLanguage string
}

// session is the resolved realm of a caller.
type session struct {
	caller  Caller
	orgID   int64
	brandID int64
	pool    string
	source  string
	channel string
	// usage is the ai_usage channel when it differs from channel
	// (WhatsApp turns, agent.go).
	usage string
	tools tools.Principal
}

func (c *Chat) session(ctx context.Context, caller Caller) (*session, error) {
	s := &session{caller: caller, channel: caller.Channel}
	switch caller.Channel {
	case model.ChannelPanel:
		if caller.Org == nil {
			return nil, ErrForbidden
		}
		s.orgID, s.brandID = caller.Org.InternalID, caller.Org.BrandID
		s.pool, s.source = model.PoolOrg, model.SourcePanel
		s.tools = tools.Principal{Auth: caller.Auth, Org: caller.Org, Realm: tools.RealmPanel}
	case model.ChannelPortal:
		if caller.Brand == nil || caller.Brand.ID <= 0 {
			return nil, ErrForbidden
		}
		center, err := c.Store.BrandCenter(ctx, caller.Brand.ID)
		if err != nil {
			return nil, fmt.Errorf("ai chat: brand center: %w", err)
		}
		s.orgID, s.brandID = center.ID, center.BrandID
		s.pool, s.source = model.PoolSystem, model.SourcePortal
		s.tools = tools.Principal{Auth: caller.Auth, Realm: tools.RealmCustomer, Brand: caller.Brand}
	default:
		return nil, fmt.Errorf("ai chat: unknown channel %q", caller.Channel)
	}
	return s, nil
}

// gate checks the module and the permission (every chat endpoint).
func (c *Chat) gate(ctx context.Context, s *session) error {
	if on, err := c.moduleOn(ctx, s.orgID); err != nil {
		return err
	} else if !on {
		return ErrFeatureDisabled
	}
	if !c.allowed(s) {
		return ErrForbidden
	}
	return nil
}

func (c *Chat) moduleOn(ctx context.Context, orgID int64) (bool, error) {
	if c.Features != nil {
		on, err := c.Features.Enabled(ctx, orgID, features.ModuleAIAssistant)
		if err != nil || !on {
			return false, err
		}
	}
	os, ok, err := c.Store.OrgSettings(ctx, orgID)
	if err != nil {
		return false, err
	}
	return !ok || os.Enabled, nil
}

func (c *Chat) allowed(s *session) bool {
	if s.channel == model.ChannelPortal {
		return jwt.NormalizeAudience(s.caller.Auth.Realm) == jwt.AudiencePortal
	}
	return s.caller.Auth.HasPermission(rbac.PermAIUse)
}

// quota returns the monthly limit (0 = unlimited) and the tokens used.
func (c *Chat) quota(ctx context.Context, s *session, settings db.AiSetting) (limit, used int64, err error) {
	if s.pool == model.PoolSystem {
		limit = settings.SystemPoolMonthlyQuota
	} else {
		limit = settings.DefaultMonthlyTokenQuota
		os, ok, err := c.Store.OrgSettings(ctx, s.orgID)
		if err != nil {
			return 0, 0, err
		}
		if ok && os.MonthlyTokenQuota.Valid {
			limit = os.MonthlyTokenQuota.Int64
		}
	}
	used, err = c.Store.MonthlyTokens(ctx, s.orgID, s.pool, c.now())
	return limit, used, err
}

func exceeded(limit, used int64) bool { return limit > 0 && used >= limit }

// facts resolves the prompt facts and the user's locale / time zone (K10).
func (c *Chat) facts(ctx context.Context, s *session) (sessionFacts, *time.Location, error) {
	row, err := c.Store.ChatContext(ctx, s.caller.Auth.UserInternal, s.orgID)
	if err != nil {
		return sessionFacts{}, nil, fmt.Errorf("ai chat: context: %w", err)
	}
	res := i18n.Resolve(i18n.Sources{
		UserLocale: row.UserLocale, UserTimezone: row.UserTimezone,
		OrgLocale: row.OrgLocale, OrgTimezone: row.OrgTimezone,
		CenterLocale: row.CenterLocale, CenterTimezone: row.CenterTimezone,
		AcceptLanguage: s.caller.AcceptLanguage,
	})
	loc, err := time.LoadLocation(res.Timezone)
	if err != nil {
		loc = time.UTC
	}
	f := sessionFacts{
		Brand: row.BrandName, Org: row.OrgName, OrgType: row.OrgType,
		User:   strings.TrimSpace(row.UserName + " " + row.UserSurname),
		Locale: string(res.Locale), Timezone: res.Timezone,
		Customer: s.channel == model.ChannelPortal,
	}
	if s.caller.Org != nil {
		f.Role = s.caller.Org.MemberRole
	}
	return f, loc, nil
}

// --- conversations -----------------------------------------------------------

// Conversation is a conversation of the list and detail.
type Conversation struct {
	UUID          uuid.UUID  `json:"uuid"`
	Title         string     `json:"title"`
	MessageCount  int32      `json:"message_count"`
	LastMessageAt *time.Time `json:"last_message_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func conversationOf(r db.AiConversation) Conversation {
	out := Conversation{UUID: r.Uuid, Title: r.Title, MessageCount: r.MessageCount,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.LastMessageAt.Valid {
		t := r.LastMessageAt.Time
		out.LastMessageAt = &t
	}
	return out
}

// UIBlock is one rendered block of a message (ai_messages.ui).
type UIBlock struct {
	// Type is text, tool, confirm, action or error.
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	ID      string   `json:"id,omitempty"`
	Name    string   `json:"name,omitempty"`
	Status  string   `json:"status,omitempty"`
	Card    *Card    `json:"card,omitempty"`
	Action  *Outcome `json:"action,omitempty"`
	Code    string   `json:"code,omitempty"`
	Message string   `json:"message,omitempty"`
}

// Message is a stored message for the UI (API blocks are not exposed).
type Message struct {
	UUID      uuid.UUID `json:"uuid"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	UI        []UIBlock `json:"ui"`
	CreatedAt time.Time `json:"created_at"`
}

// ConversationDetail is a conversation with its messages.
type ConversationDetail struct {
	Conversation
	Messages []Message `json:"messages"`
}

// ListFilter is the conversation list query.
type ListFilter struct {
	Q             string
	Created       apiquery.TimeRange
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// List returns the caller's conversations of the channel.
func (c *Chat) List(ctx context.Context, caller Caller, f ListFilter) ([]Conversation, int64, error) {
	s, err := c.open(ctx, caller)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := c.Store.ListConversations(ctx, repository.ConversationFilter{
		OrganizationID: s.orgID, UserID: caller.Auth.UserInternal, Channel: s.channel,
		Q: f.Q, Created: f.Created, Sort: f.Sort, Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Conversation, 0, len(rows))
	for _, r := range rows {
		out = append(out, conversationOf(r))
	}
	return out, total, nil
}

func (c *Chat) open(ctx context.Context, caller Caller) (*session, error) {
	s, err := c.session(ctx, caller)
	if err != nil {
		return nil, err
	}
	return s, c.gate(ctx, s)
}

// Create starts an empty conversation.
func (c *Chat) Create(ctx context.Context, caller Caller, title string) (Conversation, error) {
	s, err := c.open(ctx, caller)
	if err != nil {
		return Conversation{}, err
	}
	title, err = cleanTitle(title, false)
	if err != nil {
		return Conversation{}, err
	}
	row, err := c.Store.CreateConversation(ctx, db.CreateAIConversationParams{
		OrganizationID: s.orgID, BrandID: s.brandID, UserID: caller.Auth.UserInternal, Channel: s.channel, Title: title,
	})
	if err != nil {
		return Conversation{}, err
	}
	return conversationOf(row), nil
}

func cleanTitle(title string, required bool) (string, error) {
	title = strings.Join(strings.Fields(title), " ")
	if required && title == "" {
		return "", &ValidationError{Field: "title", Message: "is required"}
	}
	if utf8.RuneCountInString(title) > MaxTitleChars {
		return "", &ValidationError{Field: "title", Message: fmt.Sprintf("must be at most %d characters", MaxTitleChars)}
	}
	return title, nil
}

func (c *Chat) conversation(ctx context.Context, s *session, id uuid.UUID) (db.AiConversation, error) {
	row, ok, err := c.Store.ConversationForUser(ctx, id, s.orgID, s.caller.Auth.UserInternal, s.channel)
	if err != nil {
		return db.AiConversation{}, err
	}
	if !ok {
		return db.AiConversation{}, ErrNotFound
	}
	return row, nil
}

// Get returns a conversation with its messages.
func (c *Chat) Get(ctx context.Context, caller Caller, id uuid.UUID) (ConversationDetail, error) {
	s, err := c.open(ctx, caller)
	if err != nil {
		return ConversationDetail{}, err
	}
	conv, err := c.conversation(ctx, s, id)
	if err != nil {
		return ConversationDetail{}, err
	}
	rows, err := c.Store.Messages(ctx, conv.ID)
	if err != nil {
		return ConversationDetail{}, err
	}
	out := ConversationDetail{Conversation: conversationOf(conv), Messages: make([]Message, 0, len(rows))}
	for _, r := range rows {
		m := Message{UUID: r.Uuid, Role: r.Role, Status: r.Status, UI: []UIBlock{}, CreatedAt: r.CreatedAt.Time}
		_ = json.Unmarshal(r.Ui, &m.UI)
		out.Messages = append(out.Messages, m)
	}
	return out, nil
}

// Rename sets the title of a conversation.
func (c *Chat) Rename(ctx context.Context, caller Caller, id uuid.UUID, title string) (Conversation, error) {
	s, err := c.open(ctx, caller)
	if err != nil {
		return Conversation{}, err
	}
	title, err = cleanTitle(title, true)
	if err != nil {
		return Conversation{}, err
	}
	conv, err := c.conversation(ctx, s, id)
	if err != nil {
		return Conversation{}, err
	}
	row, err := c.Store.RenameConversation(ctx, conv.ID, title)
	if err != nil {
		return Conversation{}, err
	}
	return conversationOf(row), nil
}

// Delete soft-deletes a conversation.
func (c *Chat) Delete(ctx context.Context, caller Caller, id uuid.UUID) error {
	s, err := c.open(ctx, caller)
	if err != nil {
		return err
	}
	if _, err := c.conversation(ctx, s, id); err != nil {
		return err
	}
	ok, err := c.Store.DeleteConversation(ctx, id, s.orgID, caller.Auth.UserInternal)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// --- status ------------------------------------------------------------------

// Quota is the monthly token pool of the caller.
type Quota struct {
	Period string `json:"period"`
	// Limit 0 means unlimited; Remaining is then null.
	Limit     int64  `json:"limit"`
	Used      int64  `json:"used"`
	Remaining *int64 `json:"remaining"`
}

// Status is GET /v1/ai/status.
type Status struct {
	// Enabled: the module is on for the organization and a provider is
	// configured.
	Enabled bool `json:"enabled"`
	// Allowed: the caller may use the assistant (permission / realm).
	Allowed         bool        `json:"allowed"`
	ConsentRequired bool        `json:"consent_required"`
	Consent         *legal.Text `json:"consent,omitempty"`
	Quota           Quota       `json:"quota"`
}

// Status reports whether the assistant can be used and the quota left.
func (c *Chat) Status(ctx context.Context, caller Caller) (Status, error) {
	s, err := c.session(ctx, caller)
	if err != nil {
		return Status{}, err
	}
	on, err := c.moduleOn(ctx, s.orgID)
	if err != nil {
		return Status{}, err
	}
	out := Status{Enabled: on && c.Provider.Enabled(), Allowed: c.allowed(s)}
	settings, err := c.Store.Settings(ctx)
	if err != nil {
		return Status{}, err
	}
	limit, used, err := c.quota(ctx, s, settings)
	if err != nil {
		return Status{}, err
	}
	out.Quota = Quota{Period: repository.Period(c.now()), Limit: limit, Used: used}
	if limit > 0 {
		r := max(limit-used, 0)
		out.Quota.Remaining = &r
	}
	if out.Allowed {
		f, _, err := c.facts(ctx, s)
		if err != nil {
			return Status{}, err
		}
		txt, err := c.consent(ctx, s, f)
		if err != nil {
			return Status{}, err
		}
		out.ConsentRequired, out.Consent = txt != nil, txt
	}
	return out, nil
}

func (c *Chat) consent(ctx context.Context, s *session, f sessionFacts) (*legal.Text, error) {
	if c.Consents == nil {
		return nil, nil
	}
	locale, _ := i18n.Parse(f.Locale)
	return c.Consents.AIConsentRequired(ctx, s.caller.Auth.UserInternal, locale)
}

// --- turn --------------------------------------------------------------------

// Turn is a checked, ready to stream turn: a new user message, or the
// continuation after a confirmed / cancelled action.
type Turn struct {
	s        *session
	conv     db.AiConversation
	settings db.AiSetting
	facts    sessionFacts
	loc      *time.Location
	limit    int64
	text     string
	outcome  *Outcome
	// ref is the pending action source_ref when the turn has no
	// ai_conversations row (WhatsApp, agent.go); readOnly hides the write
	// tools; channelPrompt is appended to the cached system prompt.
	ref           string
	readOnly      bool
	channelPrompt string
}

// sourceRef is the conversation reference of the turn's pending actions.
func (t *Turn) sourceRef() string {
	if t.ref != "" {
		return t.ref
	}
	return t.conv.Uuid.String()
}

// ConversationUUID returns the conversation of the turn.
func (t *Turn) ConversationUUID() uuid.UUID { return t.conv.Uuid }

// prepare runs the pre-checks shared by messages and actions. checkQuota
// is false for actions: a confirmation still runs on a spent quota, only
// the continuation stops.
func (c *Chat) prepare(ctx context.Context, caller Caller, checkQuota bool) (*Turn, error) {
	s, err := c.open(ctx, caller)
	if err != nil {
		return nil, err
	}
	if !c.Provider.Enabled() {
		return nil, ErrUnavailable
	}
	f, loc, err := c.facts(ctx, s)
	if err != nil {
		return nil, err
	}
	if txt, err := c.consent(ctx, s, f); err != nil {
		return nil, err
	} else if txt != nil {
		return nil, &ConsentRequiredError{Text: *txt}
	}
	settings, err := c.Store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	limit, used, err := c.quota(ctx, s, settings)
	if err != nil {
		return nil, err
	}
	if checkQuota && exceeded(limit, used) {
		return nil, ErrQuotaExceeded
	}
	if c.Limiter != nil {
		ok, retry := c.Limiter.Allow(ctx, "ai_turn", strconv.FormatInt(caller.Auth.UserInternal, 10), TurnRateLimit, TurnRateWindow)
		if !ok {
			return nil, &RateLimitedError{RetryAfter: retry}
		}
	}
	return &Turn{s: s, settings: settings, facts: f, loc: loc, limit: limit}, nil
}

// PrepareMessage checks a new user message before the stream opens, so
// every refusal is a regular HTTP error.
func (c *Chat) PrepareMessage(ctx context.Context, caller Caller, convID uuid.UUID, content string) (*Turn, error) {
	t, err := c.prepare(ctx, caller, true)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(content)
	if text == "" {
		return nil, &ValidationError{Field: "content", Message: "is required"}
	}
	if utf8.RuneCountInString(text) > MaxMessageChars {
		return nil, &ValidationError{Field: "content", Message: fmt.Sprintf("must be at most %d characters", MaxMessageChars)}
	}
	conv, err := c.conversation(ctx, t.s, convID)
	if err != nil {
		return nil, err
	}
	// A turn stores the user message and the answer.
	if conv.MessageCount+2 > MaxConversationMessages {
		return nil, ErrConversationLimit
	}
	t.conv, t.text = conv, text
	return t, nil
}

// PrepareAction confirms (or cancels) the caller's card of this channel
// and returns the continuation turn. The action runs here, before the
// stream opens; its errors are regular HTTP errors (ErrorStatus).
func (c *Chat) PrepareAction(ctx context.Context, caller Caller, actionID uuid.UUID, confirm bool, edits map[string]any) (*Turn, error) {
	t, err := c.prepare(ctx, caller, false)
	if err != nil {
		return nil, err
	}
	if t.s.channel == model.ChannelPanel && !caller.Auth.HasPermission(rbac.PermAIActionsConfirm) {
		return nil, ErrForbidden
	}
	card, err := c.Actions.Lookup(ctx, t.s.tools, actionID)
	if err != nil {
		return nil, err
	}
	convID, perr := uuid.Parse(card.SourceRef)
	if card.Source != t.s.source || perr != nil {
		return nil, ErrActionNotFound
	}
	conv, err := c.conversation(ctx, t.s, convID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrActionNotFound
	}
	if err != nil {
		return nil, err
	}
	var out Outcome
	if confirm {
		out, err = c.Actions.Confirm(ctx, t.s.tools, actionID, edits)
	} else {
		out, err = c.Actions.Cancel(ctx, t.s.tools, actionID)
	}
	if err != nil {
		return nil, err
	}
	t.conv, t.outcome = conv, &out
	return t, nil
}

// turnState collects one turn.
type turnState struct {
	emit     Emitter
	ui       []UIBlock
	usage    llm.Usage
	model    string
	proposed bool
}

func (ts *turnState) appendText(text string) {
	if n := len(ts.ui); n > 0 && ts.ui[n-1].Type == "text" {
		ts.ui[n-1].Text += text
		return
	}
	ts.ui = append(ts.ui, UIBlock{Type: "text", Text: text})
}

func (ts *turnState) tool(id string) *UIBlock {
	for i := range ts.ui {
		if ts.ui[i].Type == "tool" && ts.ui[i].ID == id {
			return &ts.ui[i]
		}
	}
	return nil
}

func (ts *turnState) fail(code, message string) {
	ts.ui = append(ts.ui, UIBlock{Type: "error", Code: code, Message: message})
	ts.emit(EventError, map[string]any{"code": code, "message": message})
}

// Run streams a prepared turn. Model and tool failures are error events;
// only persistence failures are returned. A cancelled ctx (the client went
// away) stops the loop and stores the turn as cancelled.
func (c *Chat) Run(ctx context.Context, t *Turn, emit Emitter) error {
	if emit == nil {
		emit = func(string, any) {}
	}
	persist := context.WithoutCancel(ctx)
	s, conv := t.s, t.conv

	rows, err := c.Store.Messages(persist, conv.ID)
	if err != nil {
		return err
	}
	history := historyFromRows(rows)

	var (
		lead          []llm.Message // stored with the assistant row (resume)
		userMessageID *uuid.UUID
		ts            = &turnState{emit: emit}
	)
	if t.outcome != nil {
		o := *t.outcome
		emit(EventAction, o)
		ts.ui = append(ts.ui, UIBlock{Type: "action", Action: &o})
		lead = []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{o.ToolResult.Block(o.ToolUseID)}}}
	} else {
		// A new message instead of a confirmation cancels the open cards;
		// their tool_uses are answered in this user message.
		var blocks []llm.Block
		moved, err := c.Actions.CancelForSource(persist, s.tools, s.source, conv.Uuid.String())
		if err != nil {
			return err
		}
		for _, o := range moved {
			blocks = append(blocks, o.ToolResult.Block(o.ToolUseID))
		}
		blocks = append(blocks, turnContext(c.now(), t.loc), llm.TextBlock(t.text))
		userMsg := llm.Message{Role: llm.RoleUser, Content: blocks}
		row, err := c.storeMessage(persist, t, model.RoleUser, model.MessageComplete, []llm.Message{userMsg},
			[]UIBlock{{Type: "text", Text: t.text}})
		if err != nil {
			return err
		}
		userMessageID = &row.Uuid
		history = append(history, userMsg)
	}

	assistant, err := c.storeMessage(persist, t, model.RoleAssistant, model.MessagePending, nil, nil)
	if err != nil {
		return err
	}
	start := map[string]any{"conversation_uuid": conv.Uuid, "message_uuid": assistant.Uuid}
	if userMessageID != nil {
		start["user_message_uuid"] = *userMessageID
	}
	emit(EventMessageStart, start)

	turnMsgs, status, stop, errText := c.loop(ctx, t, ts, append(history, lead...))
	if ctx.Err() != nil && status == model.MessageComplete {
		status = model.MessageCancelled
	}
	content, _ := json.Marshal(nonNil(append(lead, turnMsgs...)))
	ui, _ := json.Marshal(nonNilUI(ts.ui))
	fin := db.FinishAIMessageParams{
		ID: assistant.ID, Status: status, Content: content, Ui: ui, Model: truncate(ts.model, 128),
		InputTokens: ts.usage.InputTokens, OutputTokens: ts.usage.OutputTokens,
		CacheReadTokens: ts.usage.CacheReadTokens, CacheWriteTokens: ts.usage.CacheWriteTokens,
	}
	if errText != "" {
		fin.Error = pgtype.Text{String: truncate(errText, maxErrorChars), Valid: true}
	}
	if _, err := c.Store.FinishMessage(persist, fin); err != nil {
		return fmt.Errorf("ai chat: finish message: %w", err)
	}
	emit(EventMessageDone, map[string]any{
		"message_uuid": assistant.Uuid, "status": status, "stop_reason": stop, "usage": ts.usage,
	})

	if t.text != "" && strings.TrimSpace(conv.Title) == "" && status == model.MessageComplete && ctx.Err() == nil {
		if title := c.title(ctx, t, lastAssistantText(turnMsgs)); title != "" {
			if _, err := c.Store.RenameConversation(persist, conv.ID, title); err == nil {
				emit(EventTitle, map[string]any{"conversation_uuid": conv.Uuid, "title": title})
			} else {
				c.Log.WarnContext(ctx, "ai_title_store_failed", "err", err)
			}
		}
	}
	return nil
}

func (c *Chat) storeMessage(ctx context.Context, t *Turn, role, status string, msgs []llm.Message, ui []UIBlock) (db.AiMessage, error) {
	content, err := json.Marshal(nonNil(msgs))
	if err != nil {
		return db.AiMessage{}, err
	}
	uiRaw, err := json.Marshal(nonNilUI(ui))
	if err != nil {
		return db.AiMessage{}, err
	}
	row, err := c.Store.CreateMessage(ctx, db.CreateAIMessageParams{
		ConversationID: t.conv.ID, OrganizationID: t.conv.OrganizationID, BrandID: t.conv.BrandID,
		Role: role, Status: status, Content: content, Ui: uiRaw,
	})
	if err != nil {
		return db.AiMessage{}, fmt.Errorf("ai chat: store message: %w", err)
	}
	if err := c.Store.TouchConversation(ctx, t.conv.ID); err != nil {
		return db.AiMessage{}, fmt.Errorf("ai chat: touch conversation: %w", err)
	}
	return row, nil
}

// loop is the agent loop: at most MaxModelCalls model calls, tools in
// between, a stop at the first confirmation card. It returns the turn's
// API messages, the message status, the last stop reason and the error
// text to store.
func (c *Chat) loop(ctx context.Context, t *Turn, ts *turnState, history []llm.Message) ([]llm.Message, string, string, string) {
	s := t.s
	persist := context.WithoutCancel(ctx)
	ts.model = c.Models.ResolveDefault(t.settings.DefaultModel)

	available, err := c.Tools.Available(ctx, s.tools)
	if err != nil {
		c.Log.ErrorContext(ctx, "ai_tools_available_failed", "err", err)
		ts.fail(StreamProviderError, "The assistant is not available right now.")
		return nil, model.MessageError, "", err.Error()
	}
	byName := make(map[string]tools.Tool, len(available))
	defs := make([]llm.ToolDef, 0, len(available))
	for _, tl := range available {
		if t.readOnly && tl.Spec().Kind == tools.KindWrite {
			continue
		}
		byName[tl.Spec().Name] = tl
		defs = append(defs, tl.Spec().Def())
	}
	system := []llm.SystemBlock{{Text: systemPrompt(t.facts, t.settings.ExtraInstructions), Cache: true}}
	if p := strings.TrimSpace(t.channelPrompt); p != "" {
		system = append(system, llm.SystemBlock{Text: p, Cache: true})
	}
	if k := knowledgePrompt(t.settings.KnowledgeText); k != "" {
		system = append(system, llm.SystemBlock{Text: k, Cache: true})
	}
	system = append(system, llm.SystemBlock{Text: sessionPrompt(t.facts)})

	var (
		turnMsgs []llm.Message
		stop     string
	)
	for call := 0; ; call++ {
		if call >= MaxModelCalls {
			ts.fail(StreamTurnLimit, "The assistant needed too many steps for this request. Ask a narrower question.")
			return turnMsgs, model.MessageError, stop, StreamTurnLimit
		}
		if _, used, err := c.quota(persist, s, t.settings); err != nil {
			c.Log.ErrorContext(ctx, "ai_quota_check_failed", "err", err)
		} else if exceeded(t.limit, used) {
			ts.ui = append(ts.ui, UIBlock{Type: "error", Code: StreamQuotaExceeded, Message: "Monthly AI token quota exceeded."})
			ts.emit(EventQuotaExceeded, map[string]any{"limit": t.limit, "used": used})
			return turnMsgs, model.MessageError, stop, StreamQuotaExceeded
		}
		if ctx.Err() != nil {
			return turnMsgs, model.MessageCancelled, stop, ""
		}
		req := llm.Request{
			Model: ts.model, System: system, Tools: defs, CacheMessages: true,
			Messages: trimHistory(sanitizeHistory(append(append([]llm.Message(nil), history...), turnMsgs...))),
		}
		resp, err := c.stream(ctx, req, ts)
		if resp.Usage != (llm.Usage{}) {
			c.record(persist, t, model.PurposeChat, firstNonEmpty(resp.Model, ts.model), resp.Usage)
			ts.usage.Add(resp.Usage)
		}
		if err != nil {
			if ctx.Err() != nil {
				return turnMsgs, model.MessageCancelled, stop, ""
			}
			c.Log.WarnContext(ctx, "ai_provider_error", "err", err, "org_id", s.orgID)
			ts.fail(StreamProviderError, "The AI provider returned an error. Try again.")
			return turnMsgs, model.MessageError, stop, err.Error()
		}
		if resp.Model != "" {
			ts.model = resp.Model
		}
		if len(resp.Message.Content) > 0 {
			turnMsgs = append(turnMsgs, resp.Message)
		}
		stop = resp.StopReason
		switch resp.StopReason {
		case llm.StopToolUse:
			uses := resp.Message.ToolUses()
			if len(uses) == 0 {
				return turnMsgs, model.MessageComplete, stop, ""
			}
			var results []llm.Block
			pause := false
			for _, tu := range uses {
				block, paused := c.execTool(ctx, t, ts, byName, tu)
				if block != nil {
					results = append(results, *block)
				}
				pause = pause || paused
			}
			if len(results) > 0 {
				turnMsgs = append(turnMsgs, llm.Message{Role: llm.RoleUser, Content: results})
			}
			if pause {
				return turnMsgs, model.MessageComplete, "confirm", ""
			}
			if ctx.Err() != nil {
				return turnMsgs, model.MessageCancelled, stop, ""
			}
		case llm.StopRefusal:
			ts.fail(StreamRefusal, "The model declined to answer this request.")
			return turnMsgs, model.MessageComplete, stop, ""
		case llm.StopMaxTokens:
			ts.fail(StreamMaxTokens, "The answer was cut off because it reached the maximum length.")
			return turnMsgs, model.MessageComplete, stop, ""
		default:
			return turnMsgs, model.MessageComplete, stop, ""
		}
	}
}

// stream runs one model call, forwarding text deltas and tool starts.
func (c *Chat) stream(ctx context.Context, req llm.Request, ts *turnState) (llm.Response, error) {
	ch, err := c.Provider.Stream(ctx, req)
	if err != nil {
		return llm.Response{}, err
	}
	var (
		resp llm.Response
		ferr error
	)
	for ev := range ch {
		switch ev.Type {
		case llm.EventTextDelta:
			ts.appendText(ev.Text)
			ts.emit(EventTextDelta, map[string]any{"text": ev.Text})
		case llm.EventToolUse:
			if ts.tool(ev.Block.ID) == nil {
				ts.ui = append(ts.ui, UIBlock{Type: "tool", ID: ev.Block.ID, Name: ev.Block.Name, Status: "running"})
				ts.emit(EventToolStart, map[string]any{"id": ev.Block.ID, "name": ev.Block.Name})
			}
		case llm.EventUsage:
			resp.Usage = ev.Usage
		case llm.EventMessageStop:
			resp.Message, resp.StopReason, resp.Model = ev.Message, ev.StopReason, ev.Model
		case llm.EventError:
			if ferr == nil {
				ferr = ev.Err
			}
		}
	}
	if ferr == nil && resp.StopReason == "" {
		ferr = errors.New("llm: stream ended without message_stop")
	}
	return resp, ferr
}

// Tool results of calls that were not run.
const (
	resultOneCard = "Not executed: only one change can be proposed at a time. Wait for the user to decide on the open confirmation card."
)

// execTool runs one tool call. It returns the tool_result block (nil when
// the call became a confirmation card) and whether the turn pauses.
func (c *Chat) execTool(ctx context.Context, t *Turn, ts *turnState, byName map[string]tools.Tool, tu llm.Block) (*llm.Block, bool) {
	if ts.tool(tu.ID) == nil {
		ts.ui = append(ts.ui, UIBlock{Type: "tool", ID: tu.ID, Name: tu.Name, Status: "running"})
		ts.emit(EventToolStart, map[string]any{"id": tu.ID, "name": tu.Name})
	}
	finish := func(res tools.Result) *llm.Block {
		if b := ts.tool(tu.ID); b != nil {
			b.Status = "done"
			if res.IsError {
				b.Status, b.Code = "error", res.Code
			}
		}
		ev := map[string]any{"id": tu.ID, "name": tu.Name, "ok": !res.IsError}
		if res.Code != "" {
			ev["code"] = res.Code
		}
		ts.emit(EventToolResult, ev)
		block := res.Block(tu.ID)
		return &block
	}
	if tl, ok := byName[tu.Name]; ok && tl.Spec().Kind == tools.KindWrite {
		if ts.proposed {
			return finish(tools.ErrorResult("", resultOneCard)), false
		}
		out, err := c.Actions.Propose(ctx, ProposeCall{
			Principal: t.s.tools, Source: t.s.source, SourceRef: t.sourceRef(),
			ToolUseID: tu.ID, ToolName: tu.Name, Input: tu.Input,
		})
		if err != nil {
			c.Log.ErrorContext(ctx, "ai_propose_failed", "tool", tu.Name, "err", err)
			return finish(tools.ErrorResult(tools.CodeToolFailed, "Could not prepare the confirmation. Tell the user to try again later.")), false
		}
		if out.Result != nil {
			return finish(*out.Result), false
		}
		ts.proposed = true
		if b := ts.tool(tu.ID); b != nil {
			b.Status = "pending"
		}
		card := *out.Card
		ts.ui = append(ts.ui, UIBlock{Type: "confirm", ID: tu.ID, Card: &card})
		ts.emit(EventConfirm, card)
		return nil, true
	}
	// Read tools (and unknown names: Call answers TOOL_NOT_ALLOWED).
	res, err := c.Tools.Call(ctx, t.s.tools, tu.Name, tu.Input)
	if err != nil && !errors.Is(err, tools.ErrToolNotAllowed) && !errors.Is(err, tools.ErrConfirmationRequired) {
		c.Log.WarnContext(ctx, "ai_tool_failed", "tool", tu.Name, "err", err)
	}
	return finish(res), false
}

// record books one model call on the ledger and its projection; crossing
// 80 % / 100 % of the quota writes ai.quota.threshold in the same
// transaction. A booking failure is logged, never shown.
func (c *Chat) record(ctx context.Context, t *Turn, purpose, modelID string, u llm.Usage) {
	s := t.s
	var userID *int64
	if id := s.caller.Auth.UserInternal; id > 0 {
		userID = &id
	}
	usageChannel := model.UsageChannelPanel
	switch {
	case s.usage != "":
		usageChannel = s.usage
	case s.channel == model.ChannelPortal:
		usageChannel = model.UsageChannelPortal
	}
	_, _, err := c.Store.RecordUsageEvents(ctx, repository.Usage{
		OrganizationID: s.orgID, BrandID: s.brandID, Pool: s.pool, UserID: userID,
		Channel: usageChannel, Purpose: purpose, Model: truncate(firstNonEmpty(modelID, "unknown"), 128),
		InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens,
	}, c.Outbox, func(row db.AiUsage, month db.AiUsageMonthly) []events.Event {
		return thresholdEvents(t.limit, row, month)
	})
	if err != nil {
		c.Log.ErrorContext(ctx, "ai_usage_record_failed", "err", err, "org_id", s.orgID)
	}
}

// QuotaThresholds are the notified percentages of the monthly quota.
var QuotaThresholds = []int64{80, 100}

// thresholdEvents returns an ai.quota.threshold event per threshold the
// booked row crossed. The projection only grows within a month, so each
// threshold is crossed (and notified) once per pool and month.
func thresholdEvents(limit int64, row db.AiUsage, month db.AiUsageMonthly) []events.Event {
	if limit <= 0 || row.QuotaTokens <= 0 {
		return nil
	}
	after := month.QuotaTokens
	before := after - row.QuotaTokens
	var out []events.Event
	for _, pct := range QuotaThresholds {
		if before*100 < limit*pct && after*100 >= limit*pct {
			ev := events.New(events.AIQuotaThreshold).WithTenant(month.OrganizationID).WithPayload(map[string]any{
				"organization_id": month.OrganizationID, "brand_id": month.BrandID, "pool": month.Pool,
				"period": month.Period, "threshold": pct, "used_tokens": after, "quota_tokens": limit,
			})
			out = append(out, ev)
		}
	}
	return out
}

// title asks the fast model for a short title; it falls back to the first
// words of the message.
func (c *Chat) title(ctx context.Context, t *Turn, answer string) string {
	fallback := truncate(strings.TrimSpace(strings.SplitN(t.text, "\n", 2)[0]), fallbackTitle)
	ctx, cancel := context.WithTimeout(ctx, titleTimeout)
	defer cancel()
	prompt := "User: " + truncate(t.text, 500)
	if answer != "" {
		prompt += "\nAssistant: " + truncate(answer, 500)
	}
	fast := c.Models.ResolveFast(t.settings.FastModel)
	resp, err := llm.Complete(ctx, c.Provider, llm.Request{
		Model: fast, MaxTokens: titleMaxTokens, System: []llm.SystemBlock{{Text: titlePrompt}},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock(prompt)}}},
	})
	if resp.Usage != (llm.Usage{}) {
		c.record(context.WithoutCancel(ctx), t, model.PurposeTitle, firstNonEmpty(resp.Model, fast), resp.Usage)
	}
	if err != nil {
		c.Log.InfoContext(ctx, "ai_title_fallback", "err", err)
		return fallback
	}
	title := strings.TrimSpace(strings.SplitN(strings.TrimSpace(resp.Message.Text()), "\n", 2)[0])
	title = strings.Trim(title, " \"'`*#.")
	if title == "" {
		return fallback
	}
	return truncate(title, 80)
}

func lastAssistantText(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleAssistant {
			if txt := strings.TrimSpace(msgs[i].Text()); txt != "" {
				return txt
			}
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func nonNil(m []llm.Message) []llm.Message {
	if m == nil {
		return []llm.Message{}
	}
	return m
}

func nonNilUI(u []UIBlock) []UIBlock {
	if u == nil {
		return []UIBlock{}
	}
	return u
}
