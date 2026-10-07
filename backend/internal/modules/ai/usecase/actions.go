// Package usecase holds the AI assistant use cases. actions.go (TEC-387,
// F4-01e, otopoly-go pattern) is the confirmation flow of write tools,
// shared by every channel: the panel / portal SSE chat (F4-01f), WhatsApp
// "EVET / HAYIR" (F4-02c) and the MCP panel approval (F4-03c).
//
//	write tool call → schema check → Propose (preview, nothing changes)
//	→ ai_pending_actions row (TTL 30 min) → the turn pauses, the caller
//	shows the card → Confirm (only the user who started it; edited fields
//	are validated again) → pending → executing in one UPDATE → permission,
//	module and scope are checked again → Run → confirmed | failed.
//
// Cancel, a new message in the same conversation (CancelForSource) and the
// periodic Sweep (expired pending, executing for over 10 minutes) resolve
// the rest. Every executed action is written to the activity log with the
// actor and via = ai.
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// PendingActionTTL is how long a confirmation card can be confirmed.
	PendingActionTTL = 30 * time.Minute
	// StaleExecutingAfter: an action executing for longer is considered
	// interrupted (the process stopped mid-run). Well above actionTimeout.
	StaleExecutingAfter = 10 * time.Minute
	// actionTimeout bounds one confirmed run.
	actionTimeout = 60 * time.Second
)

// Error codes of the confirmation API (named business rules).
const (
	// CodeActionExpired (422): the card expired before the confirmation.
	CodeActionExpired = "AI_ACTION_EXPIRED"
	// CodeActionResolved (409): the action was already confirmed,
	// cancelled or is running.
	CodeActionResolved = "AI_ACTION_RESOLVED"
)

// Errors of Confirm / Cancel.
var (
	// ErrActionNotFound: no such action of the caller (another user's
	// action is never distinguished from a missing one).
	ErrActionNotFound = errors.New("ai actions: not found")
	ErrActionExpired  = errors.New("ai actions: expired")
	ErrActionResolved = errors.New("ai actions: already resolved")
	// ErrActionForbidden: the user lost the permission, module or scope of
	// the tool before the confirmation; the action is failed.
	ErrActionForbidden = errors.New("ai actions: no longer allowed")
)

// ValidationError is an edited card field that does not fit the tool
// schema (400 VALIDATION_ERROR).
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

// ErrorStatus maps a Confirm / Cancel error to the HTTP status and error
// code the channels answer with; ok is false for internal errors.
func ErrorStatus(err error) (status int, code string, ok bool) {
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		return http.StatusBadRequest, "VALIDATION_ERROR", true
	case errors.Is(err, ErrActionNotFound):
		return http.StatusNotFound, "NOT_FOUND", true
	case errors.Is(err, ErrActionExpired):
		return http.StatusUnprocessableEntity, CodeActionExpired, true
	case errors.Is(err, ErrActionResolved):
		return http.StatusConflict, CodeActionResolved, true
	case errors.Is(err, ErrActionForbidden):
		return http.StatusForbidden, "FORBIDDEN", true
	}
	return http.StatusInternalServerError, "INTERNAL_ERROR", false
}

// Tool results fed back to the model for actions that did not run, and
// error texts stored on the row.
const (
	resultCancelled   = "Not executed: the user cancelled this action on the confirmation card."
	resultMovedOn     = "Not executed: the user sent a new message instead of confirming this action."
	resultExpired     = "Not executed: the confirmation card expired before the user confirmed it."
	resultForbidden   = "Not executed: the user is no longer allowed to perform this action."
	resultInternalErr = "The action failed with an internal error; tell the user it may not have been applied and to check the record in the app."
	errForbidden      = "permission revoked: the user is no longer allowed to run this tool"
	errInterrupted    = "outcome unknown: the action was interrupted while executing; check the record before retrying"
	maxErrorChars     = 1000
)

// ActionStore is the persistence of pending actions (ai/repository.Store).
type ActionStore interface {
	CreatePendingAction(ctx context.Context, p db.CreateAIPendingActionParams) (db.AiPendingAction, error)
	PendingActionByKey(ctx context.Context, key string) (db.AiPendingAction, bool, error)
	PendingActionForUser(ctx context.Context, id uuid.UUID, orgID, userID int64) (db.AiPendingAction, bool, error)
	ListPendingActions(ctx context.Context, orgID, userID int64) ([]db.AiPendingAction, error)
	ClaimPendingAction(ctx context.Context, id uuid.UUID, orgID, userID int64, input, preview []byte) (db.AiPendingAction, bool, error)
	ResolvePendingAction(ctx context.Context, p db.ResolveAIPendingActionParams) (db.AiPendingAction, error)
	CancelPendingAction(ctx context.Context, id uuid.UUID, orgID, userID int64) (db.AiPendingAction, bool, error)
	CancelPendingActionsForSource(ctx context.Context, p db.CancelAIPendingActionsForSourceParams) ([]db.AiPendingAction, error)
	ExpirePendingAction(ctx context.Context, id int64, now time.Time) (db.AiPendingAction, bool, error)
	ExpirePendingActions(ctx context.Context, now time.Time) (int64, error)
	FailStalePendingActions(ctx context.Context, staleBefore time.Time, errText string) ([]db.AiPendingAction, error)
}

// ToolGate is the tool registry surface of the flow (*tools.Registry).
type ToolGate interface {
	Get(name string) (tools.Tool, bool)
	Allowed(ctx context.Context, p tools.Principal, spec tools.Spec) (bool, error)
	Propose(ctx context.Context, p tools.Principal, name string, input json.RawMessage) (*tools.Proposal, *tools.Result, error)
	RunConfirmed(ctx context.Context, p tools.Principal, name string, input json.RawMessage) (tools.Result, error)
}

// ActivityRecorder records audit events (*activity.Recorder).
type ActivityRecorder interface {
	Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, r *http.Request)
}

// Actions is the confirmation flow.
type Actions struct {
	store    ActionStore
	tools    ToolGate
	activity ActivityRecorder
	now      func() time.Time
	log      *slog.Logger
}

// NewActions builds the flow. activity may be nil (tests).
func NewActions(store ActionStore, gate ToolGate, rec ActivityRecorder, log *slog.Logger) *Actions {
	if log == nil {
		log = slog.Default()
	}
	return &Actions{store: store, tools: gate, activity: rec, now: time.Now, log: log}
}

// SetClock replaces the clock (tests).
func (s *Actions) SetClock(now func() time.Time) { s.now = now }

// Card is a confirmation card: the "confirm" event of the chat and the row
// of the pending actions list.
type Card struct {
	ActionUUID uuid.UUID     `json:"action_uuid"`
	ToolUseID  string        `json:"tool_use_id"`
	ToolName   string        `json:"tool_name"`
	Source     string        `json:"source"`
	SourceRef  string        `json:"source_ref,omitempty"`
	Status     string        `json:"status"`
	Preview    tools.Preview `json:"preview"`
	ExpiresAt  time.Time     `json:"expires_at"`
	CreatedAt  time.Time     `json:"created_at"`
}

// Outcome is a resolved action. ToolResult goes back to the model as the
// tool_result of ToolUseID; Message and Link are for the card.
type Outcome struct {
	ActionUUID uuid.UUID    `json:"action_uuid"`
	ToolUseID  string       `json:"tool_use_id"`
	ToolName   string       `json:"tool_name"`
	Source     string       `json:"source"`
	SourceRef  string       `json:"source_ref,omitempty"`
	Status     string       `json:"status"`
	Message    string       `json:"message,omitempty"`
	Link       *tools.Link  `json:"link,omitempty"`
	ToolResult tools.Result `json:"-"`
}

// storedResult is ai_pending_actions.result.
type storedResult struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message,omitempty"`
	Link    *tools.Link `json:"link,omitempty"`
}

func cardOf(row db.AiPendingAction) Card {
	c := Card{
		ActionUUID: row.Uuid, ToolUseID: row.ToolUseID, ToolName: row.ToolName, Source: row.Source,
		SourceRef: row.SourceRef.String, Status: row.Status, ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time,
	}
	_ = json.Unmarshal(row.Preview, &c.Preview)
	return c
}

func outcomeOf(row db.AiPendingAction, res tools.Result) Outcome {
	o := Outcome{
		ActionUUID: row.Uuid, ToolUseID: row.ToolUseID, ToolName: row.ToolName, Source: row.Source,
		SourceRef: row.SourceRef.String, Status: row.Status, ToolResult: res, Link: res.Link,
	}
	if res.IsError {
		o.Message = strings.TrimPrefix(res.Content, "Not executed: ")
	}
	return o
}

// --- propose -----------------------------------------------------------------

// ProposeCall is one write-tool call of the model.
type ProposeCall struct {
	Principal tools.Principal
	// Source is model.Source*; SourceRef the conversation of the source
	// (ai_conversations uuid, WhatsApp conversation, MCP session).
	Source    string
	SourceRef string
	ToolUseID string
	ToolName  string
	Input     json.RawMessage
}

// ProposeOutcome: exactly one field is set. Result means the call was
// rejected (not allowed, invalid input, unknown record): the caller feeds
// it to the model as the tool_result and the turn goes on. Card means a
// confirmation card was stored: the turn pauses and the caller emits it.
type ProposeOutcome struct {
	Result *tools.Result `json:"result,omitempty"`
	Card   *Card         `json:"card,omitempty"`
}

var sources = []string{model.SourcePanel, model.SourcePortal, model.SourceWhatsApp, model.SourceMCP}

// Propose stores a write-tool call as a pending action. Nothing is
// written to the target module here. The same tool_use proposed twice
// (a retried turn) returns the existing card.
func (s *Actions) Propose(ctx context.Context, call ProposeCall) (ProposeOutcome, error) {
	if !slices.Contains(sources, call.Source) {
		return ProposeOutcome{}, fmt.Errorf("ai actions: unknown source %q", call.Source)
	}
	toolUseID := strings.TrimSpace(call.ToolUseID)
	if toolUseID == "" || len(toolUseID) > 128 {
		return ProposeOutcome{}, errors.New("ai actions: tool_use id is required (max 128 chars)")
	}
	p := call.Principal
	if p.Org == nil {
		// Write tools are panel tools; they need the active organization.
		r := tools.ErrorResult(tools.CodeToolNotAllowed, "TOOL_NOT_ALLOWED: the tool "+call.ToolName+
			" is not available to this user. Do not call it again; answer with the tools you have.")
		return ProposeOutcome{Result: &r}, nil
	}
	key := idempotencyKey(call.Source, p.Org.InternalID, p.Auth.UserInternal, call.SourceRef, toolUseID)
	if row, ok, err := s.store.PendingActionByKey(ctx, key); err != nil {
		return ProposeOutcome{}, err
	} else if ok && row.OrganizationID == p.Org.InternalID && row.UserID == p.Auth.UserInternal {
		c := cardOf(row)
		return ProposeOutcome{Card: &c}, nil
	}
	prop, res, err := s.tools.Propose(ctx, p, call.ToolName, call.Input)
	if res != nil && (err == nil || errors.Is(err, tools.ErrToolNotAllowed)) {
		return ProposeOutcome{Result: res}, nil
	}
	if err != nil {
		return ProposeOutcome{}, err
	}
	preview, err := json.Marshal(prop.Preview)
	if err != nil {
		return ProposeOutcome{}, fmt.Errorf("ai actions: preview: %w", err)
	}
	row, err := s.store.CreatePendingAction(ctx, db.CreateAIPendingActionParams{
		OrganizationID: p.Org.InternalID, BrandID: p.Org.BrandID, UserID: p.Auth.UserInternal,
		Source: call.Source, SourceRef: pgtype.Text{String: call.SourceRef, Valid: call.SourceRef != ""},
		ToolUseID: toolUseID, ToolName: call.ToolName, Input: prop.Input, Preview: preview,
		IdempotencyKey: key, ExpiresAt: pgtype.Timestamptz{Time: s.now().Add(PendingActionTTL), Valid: true},
	})
	if err != nil {
		return ProposeOutcome{}, fmt.Errorf("ai actions: create: %w", err)
	}
	c := cardOf(row)
	return ProposeOutcome{Card: &c}, nil
}

// idempotencyKey is unique per user and tool_use: a key of another user
// never collides with (or reveals) this one.
func idempotencyKey(source string, orgID, userID int64, ref, toolUseID string) string {
	k := fmt.Sprintf("%s:%d:%d:%s:%s", source, orgID, userID, ref, toolUseID)
	if len(k) > 200 {
		sum := sha256.Sum256([]byte(k))
		k = "sha256:" + hex.EncodeToString(sum[:])
	}
	return k
}

// ListPending returns the caller's open cards (pending actions screen).
func (s *Actions) ListPending(ctx context.Context, p tools.Principal) ([]Card, error) {
	if p.Org == nil {
		return []Card{}, nil
	}
	rows, err := s.store.ListPendingActions(ctx, p.Org.InternalID, p.Auth.UserInternal)
	if err != nil {
		return nil, err
	}
	out := make([]Card, 0, len(rows))
	for _, r := range rows {
		out = append(out, cardOf(r))
	}
	return out, nil
}

// --- confirm -----------------------------------------------------------------

// load returns the caller's own action; anything else is not found.
func (s *Actions) load(ctx context.Context, p tools.Principal, id uuid.UUID) (db.AiPendingAction, error) {
	if p.Org == nil {
		return db.AiPendingAction{}, ErrActionNotFound
	}
	row, ok, err := s.store.PendingActionForUser(ctx, id, p.Org.InternalID, p.Auth.UserInternal)
	if err != nil {
		return db.AiPendingAction{}, err
	}
	if !ok {
		return db.AiPendingAction{}, ErrActionNotFound
	}
	return row, nil
}

// checkPending reports whether the action can still be decided; a pending
// action past its expiry is moved to expired here.
func (s *Actions) checkPending(ctx context.Context, row db.AiPendingAction) error {
	switch row.Status {
	case model.ActionPending:
	case model.ActionExpired:
		return ErrActionExpired
	default:
		return ErrActionResolved
	}
	if row.ExpiresAt.Time.After(s.now()) {
		return nil
	}
	if _, _, err := s.store.ExpirePendingAction(ctx, row.ID, s.now()); err != nil {
		return err
	}
	return ErrActionExpired
}

// Confirm runs the caller's pending action exactly once. edits replace
// fields the card marks editable; the merged input is validated against
// the tool schema and previewed again (400 on failure, the card stays
// pending). Permission, module and scope are checked again: when they were
// revoked the action is failed and ErrActionForbidden returned. A failed
// run is an Outcome with status failed, not an error.
func (s *Actions) Confirm(ctx context.Context, p tools.Principal, id uuid.UUID, edits map[string]any) (Outcome, error) {
	row, err := s.load(ctx, p, id)
	if err != nil {
		return Outcome{}, err
	}
	if err := s.checkPending(ctx, row); err != nil {
		return Outcome{}, err
	}
	allowed, err := s.allowed(ctx, p, row.ToolName)
	if err != nil {
		return Outcome{}, err
	}
	if !allowed {
		return s.failForbidden(ctx, p, row)
	}
	var input, preview []byte
	if len(edits) > 0 {
		input, preview, err = s.applyEdits(ctx, p, row, edits)
		if errors.Is(err, tools.ErrToolNotAllowed) {
			return s.failForbidden(ctx, p, row)
		}
		if err != nil {
			return Outcome{}, err
		}
	}
	claimed, ok, err := s.store.ClaimPendingAction(ctx, row.Uuid, row.OrganizationID, row.UserID, input, preview)
	if err != nil {
		return Outcome{}, err
	}
	if !ok {
		return Outcome{}, s.claimLost(ctx, p, id)
	}
	return s.execute(ctx, p, claimed)
}

// claimLost explains a lost compare-and-set: another confirmation won,
// the action was cancelled, or it expired in between.
func (s *Actions) claimLost(ctx context.Context, p tools.Principal, id uuid.UUID) error {
	row, err := s.load(ctx, p, id)
	if err != nil {
		return err
	}
	if err := s.checkPending(ctx, row); err != nil {
		return err
	}
	return ErrActionResolved
}

func (s *Actions) allowed(ctx context.Context, p tools.Principal, name string) (bool, error) {
	t, ok := s.tools.Get(name)
	if !ok || t.Spec().Kind != tools.KindWrite {
		return false, nil
	}
	return s.tools.Allowed(ctx, p, t.Spec())
}

// failForbidden claims the action and fails it because the user lost the
// right to run it.
func (s *Actions) failForbidden(ctx context.Context, p tools.Principal, row db.AiPendingAction) (Outcome, error) {
	claimed, ok, err := s.store.ClaimPendingAction(ctx, row.Uuid, row.OrganizationID, row.UserID, nil, nil)
	if err != nil {
		return Outcome{}, err
	}
	if !ok {
		return Outcome{}, s.claimLost(ctx, p, row.Uuid)
	}
	res := tools.ErrorResult(tools.CodeToolNotAllowed, resultForbidden)
	out, err := s.finish(ctx, p, claimed, model.ActionFailed, res, errForbidden)
	if err != nil {
		return Outcome{}, err
	}
	return out, ErrActionForbidden
}

// execute runs a claimed action with the activity origin via = ai.
func (s *Actions) execute(ctx context.Context, p tools.Principal, row db.AiPendingAction) (Outcome, error) {
	base := context.WithoutCancel(ctx)
	runCtx := activity.WithOrigin(base, activity.Origin{
		Via: "ai", ActionUUID: row.Uuid.String(), ConversationUUID: row.SourceRef.String,
	})
	runCtx, cancel := context.WithTimeout(runCtx, actionTimeout)
	res, err := s.tools.RunConfirmed(runCtx, p, row.ToolName, row.Input)
	cancel()
	if errors.Is(err, tools.ErrToolNotAllowed) {
		out, ferr := s.finish(base, p, row, model.ActionFailed, tools.ErrorResult(tools.CodeToolNotAllowed, resultForbidden), errForbidden)
		if ferr != nil {
			return Outcome{}, ferr
		}
		return out, ErrActionForbidden
	}
	status, errText := model.ActionConfirmed, ""
	if err != nil {
		s.log.ErrorContext(ctx, "ai_action_failed", "action", row.Uuid, "tool", row.ToolName, "err", err)
		res, errText = tools.ErrorResult(tools.CodeToolFailed, resultInternalErr), err.Error()
	}
	if res.IsError {
		status = model.ActionFailed
		if errText == "" {
			errText = res.Content
		}
	}
	return s.finish(base, p, row, status, res, errText)
}

// finish resolves a claimed action and records it in the activity log.
func (s *Actions) finish(ctx context.Context, p tools.Principal, row db.AiPendingAction, status string, res tools.Result, errText string) (Outcome, error) {
	stored := storedResult{OK: status == model.ActionConfirmed, Link: res.Link}
	if res.IsError {
		stored.Message = strings.TrimPrefix(res.Content, "Not executed: ")
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return Outcome{}, err
	}
	params := db.ResolveAIPendingActionParams{ID: row.ID, Status: status, Result: raw}
	if errText != "" {
		params.Error = pgtype.Text{String: truncate(errText, maxErrorChars), Valid: true}
	}
	resolved, err := s.store.ResolvePendingAction(ctx, params)
	if err != nil {
		return Outcome{}, fmt.Errorf("ai actions: resolve: %w", err)
	}
	s.record(ctx, p, resolved)
	return outcomeOf(resolved, res), nil
}

func (s *Actions) record(ctx context.Context, p tools.Principal, row db.AiPendingAction) {
	if s.activity == nil {
		return
	}
	ctx = activity.WithOrigin(ctx, activity.Origin{Via: "ai", ActionUUID: row.Uuid.String(), ConversationUUID: row.SourceRef.String})
	actor := p.Auth.UserInternal
	action := "ai.action.confirmed"
	if row.Status == model.ActionFailed {
		action = "ai.action.failed"
	}
	payload := map[string]any{"tool": row.ToolName, "source": row.Source, "status": row.Status,
		"organization_id": row.OrganizationID}
	s.activity.Record(ctx, &actor, action, "ai_action", &row.Uuid, payload, nil)
}

// applyEdits merges the edited card fields into the stored input, checks
// the result against the tool schema and previews it again.
func (s *Actions) applyEdits(ctx context.Context, p tools.Principal, row db.AiPendingAction, edits map[string]any) (json.RawMessage, json.RawMessage, error) {
	var current tools.Preview
	_ = json.Unmarshal(row.Preview, &current)
	merged, err := mergeEdits(row.Input, current.Edit, edits)
	if err != nil {
		return nil, nil, err
	}
	t, ok := s.tools.Get(row.ToolName)
	if !ok {
		return nil, nil, tools.ErrToolNotAllowed
	}
	if err := tools.Validate(t.Spec().InputSchema, merged); err != nil {
		var ve *tools.ValidationError
		if errors.As(err, &ve) {
			return nil, nil, &ValidationError{Field: strings.TrimPrefix(ve.Path, "."), Message: ve.Message}
		}
		return nil, nil, &ValidationError{Message: err.Error()}
	}
	prop, res, err := s.tools.Propose(ctx, p, row.ToolName, merged)
	if errors.Is(err, tools.ErrToolNotAllowed) {
		return nil, nil, err
	}
	if res != nil && err == nil {
		return nil, nil, &ValidationError{Message: res.Content}
	}
	if err != nil {
		return nil, nil, err
	}
	preview, err := json.Marshal(prop.Preview)
	if err != nil {
		return nil, nil, err
	}
	return prop.Input, preview, nil
}

// mergeEdits writes edits into the input. Only fields the card lists are
// editable; an empty value clears an optional field. Types are left to the
// schema check, except numbers typed as text.
func mergeEdits(input json.RawMessage, allowed []tools.EditField, edits map[string]any) (json.RawMessage, error) {
	m := map[string]any{}
	if err := json.Unmarshal(input, &m); err != nil {
		return nil, fmt.Errorf("ai actions: stored input: %w", err)
	}
	byKey := make(map[string]tools.EditField, len(allowed))
	for _, f := range allowed {
		byKey[f.Key] = f
	}
	for key, v := range edits {
		f, ok := byKey[key]
		if !ok {
			return nil, &ValidationError{Field: key, Message: "cannot be edited"}
		}
		if s, isStr := v.(string); isStr {
			v = strings.TrimSpace(s)
		}
		if v == nil || v == "" {
			if f.Required {
				return nil, &ValidationError{Field: key, Message: "is required"}
			}
			delete(m, key)
			continue
		}
		if s, isStr := v.(string); isStr && f.Type == tools.EditNumber {
			n, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, &ValidationError{Field: key, Message: "must be a number"}
			}
			v = n
		}
		switch v.(type) {
		case string, float64, bool, json.Number:
		default:
			return nil, &ValidationError{Field: key, Message: "has an invalid value"}
		}
		if f.Type == tools.EditSelect && len(f.Options) > 0 && !slices.Contains(f.Options, fmt.Sprint(v)) {
			return nil, &ValidationError{Field: key, Message: "is not one of the options"}
		}
		m[key] = v
	}
	return json.Marshal(m)
}

// --- cancel ------------------------------------------------------------------

// Cancel resolves the caller's pending action as cancelled; nothing runs.
func (s *Actions) Cancel(ctx context.Context, p tools.Principal, id uuid.UUID) (Outcome, error) {
	row, err := s.load(ctx, p, id)
	if err != nil {
		return Outcome{}, err
	}
	if err := s.checkPending(ctx, row); err != nil {
		return Outcome{}, err
	}
	cancelled, ok, err := s.store.CancelPendingAction(ctx, row.Uuid, row.OrganizationID, row.UserID)
	if err != nil {
		return Outcome{}, err
	}
	if !ok {
		return Outcome{}, s.claimLost(ctx, p, id)
	}
	return outcomeOf(cancelled, tools.ErrorResult("", resultCancelled)), nil
}

// CancelForSource cancels the user's open cards of one conversation: the
// user sent a new message instead of confirming. The outcomes carry the
// tool_result each unanswered tool_use gets.
func (s *Actions) CancelForSource(ctx context.Context, p tools.Principal, source, sourceRef string) ([]Outcome, error) {
	if p.Org == nil || sourceRef == "" {
		return nil, nil
	}
	rows, err := s.store.CancelPendingActionsForSource(ctx, db.CancelAIPendingActionsForSourceParams{
		Source: source, SourceRef: pgtype.Text{String: sourceRef, Valid: true},
		OrganizationID: p.Org.InternalID, UserID: p.Auth.UserInternal,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Outcome, 0, len(rows))
	for _, r := range rows {
		out = append(out, outcomeOf(r, tools.ErrorResult("", resultMovedOn)))
	}
	return out, nil
}

// ExpiredResult is the tool_result of an action that expired (the chat
// feeds it to the model when the conversation resumes).
func ExpiredResult() tools.Result { return tools.ErrorResult("", resultExpired) }

// --- sweep -------------------------------------------------------------------

// SweepResult counts one sweep.
type SweepResult struct {
	Expired int64
	Failed  int
}

// Sweep is the periodic job: pending actions past their expiry become
// expired and actions executing for over StaleExecutingAfter become failed
// ("outcome unknown"). Both are conditional UPDATEs, so a second run (or
// two workers) changes nothing.
func (s *Actions) Sweep(ctx context.Context) (SweepResult, error) {
	now := s.now()
	expired, err := s.store.ExpirePendingActions(ctx, now)
	if err != nil {
		return SweepResult{}, fmt.Errorf("ai actions: expire: %w", err)
	}
	stale, err := s.store.FailStalePendingActions(ctx, now.Add(-StaleExecutingAfter), errInterrupted)
	if err != nil {
		return SweepResult{Expired: expired}, fmt.Errorf("ai actions: fail stale: %w", err)
	}
	for _, r := range stale {
		s.log.WarnContext(ctx, "ai_action_interrupted", "action", r.Uuid, "tool", r.ToolName)
	}
	return SweepResult{Expired: expired, Failed: len(stale)}, nil
}

// SweepTask is Sweep for the worker (queue.AIActionSweepFunc).
func (s *Actions) SweepTask(ctx context.Context) error {
	res, err := s.Sweep(ctx)
	if err != nil {
		return err
	}
	if res.Expired > 0 || res.Failed > 0 {
		s.log.InfoContext(ctx, "ai_actions_swept", "expired", res.Expired, "failed", res.Failed)
	}
	return nil
}

func truncate(s string, maxRunes int) string {
	if r := []rune(s); len(r) > maxRunes {
		return string(r[:maxRunes])
	}
	return s
}
