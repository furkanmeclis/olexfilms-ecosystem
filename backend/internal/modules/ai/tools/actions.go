package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// Write tools (TEC-387, F4-01e) never run from Call: Propose validates the
// input, resolves references and builds the confirmation card without
// changing anything; the actions use case stores it as an
// ai_pending_actions row and runs it with RunConfirmed only after the user
// who started it confirms.

// CodeConfirmationRequired: a write tool was called directly (Call) instead
// of going through the confirmation card.
const CodeConfirmationRequired = "CONFIRMATION_REQUIRED"

// ErrConfirmationRequired is returned by Call for write tools.
var ErrConfirmationRequired = errors.New("ai tools: write tool needs confirmation")

// Preview is the confirmation card of a proposed write. Action is the tool
// name; the UI resolves ai.actions.<action> and ai.actions.fields.<key>.
// Summary is a short sentence in the principal's locale (TEC-461) for the
// card and the text channels (WhatsApp, MCP); SummaryArgs fills the
// ai.actions.summary.<action> template of the backend catalog, so a
// channel can render it in another locale (LocalizedSummary).
type Preview struct {
	Action      string            `json:"action"`
	Summary     string            `json:"summary"`
	SummaryArgs map[string]string `json:"summary_args,omitempty"`
	Fields      []Field           `json:"fields"`
	// Edit lists the fields the user may change on the card before
	// confirming; Key is the tool input property it writes.
	Edit []EditField `json:"edit,omitempty"`
	// Warnings are i18n keys (ai.actions.warnings.<key>).
	Warnings []string `json:"warnings,omitempty"`
}

// summaryKeyPrefix + action is the catalog key of a preview summary.
const summaryKeyPrefix = "ai.actions.summary."

// LocalizedSummary renders the summary in the locale; a locale outside the
// catalogs gets en. A preview without a catalog template keeps Summary.
func (pv Preview) LocalizedSummary(locale i18n.Locale) string {
	key := summaryKeyPrefix + pv.Action
	if pv.Action == "" || pv.SummaryArgs == nil || i18n.Translate(locale, key) == key {
		return pv.Summary
	}
	return i18n.TranslateParams(locale, key, pv.SummaryArgs)
}

// FieldLabel is the localized label of a preview field key; a key outside
// the catalog is shown with spaces for underscores.
func FieldLabel(locale i18n.Locale, key string) string {
	k := "ai.actions.fields." + key
	if label := i18n.Translate(locale, k); label != k {
		return label
	}
	return strings.ReplaceAll(key, "_", " ")
}

// Field is one "label: value" row of a preview.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Edit field types.
const (
	EditText     = "text"
	EditTextarea = "textarea"
	EditDate     = "date"
	EditTime     = "time"
	EditSelect   = "select"
	EditNumber   = "number"
)

// EditField is an editable input of the confirmation card.
type EditField struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Value    string   `json:"value"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// Link points the UI at the record a confirmed action created or changed.
type Link struct {
	Kind string `json:"kind"`
	UUID string `json:"uuid"`
}

// Proposal is a validated, reference-resolved write call. Input is what
// Run receives on confirmation (the model input, normalized).
type Proposal struct {
	Input   json.RawMessage
	Preview Preview
}

// ActionTool is a write tool.
type ActionTool interface {
	Tool
	// Propose checks the input and builds the card without changing
	// anything. A user-correctable problem is an error Result (returned
	// to the model); a returned error is an internal failure.
	Propose(ctx context.Context, env Env, input json.RawMessage) (Proposal, *Result, error)
}

// Propose checks, validates and previews one write-tool call. Exactly one
// of the proposal and the result is set: the result is an error for the
// model (not allowed, invalid input, record not found). The error is
// ErrToolNotAllowed (next to the result) or an internal failure.
func (r *Registry) Propose(ctx context.Context, p Principal, name string, input json.RawMessage) (*Proposal, *Result, error) {
	at, input, ctx, res, err := r.prepareWrite(ctx, p, name, input)
	if res != nil || err != nil {
		return nil, res, err
	}
	prop, bad, err := at.Propose(ctx, r.env(p), input)
	if err != nil {
		r.log.ErrorContext(ctx, "ai tool propose failed", "tool", name, "err", err)
		f := failed()
		return nil, &f, err
	}
	if bad != nil {
		return nil, bad, nil
	}
	prop.Preview.Action = name
	prop.Preview.Summary = prop.Preview.LocalizedSummary(p.Locale)
	if len(prop.Input) == 0 {
		prop.Input = input
	}
	return &prop, nil, nil
}

// RunConfirmed executes a write tool whose card the user confirmed. It
// checks the permission, organization type, module, toggle and brand again
// (ErrToolNotAllowed) and validates the stored input before Run. Only the
// actions use case calls it, after claiming the pending action.
func (r *Registry) RunConfirmed(ctx context.Context, p Principal, name string, input json.RawMessage) (res Result, err error) {
	at, input, ctx, bad, err := r.prepareWrite(ctx, p, name, input)
	if err != nil {
		return *bad, err
	}
	if bad != nil {
		return *bad, nil
	}
	defer func() {
		if rec := recover(); rec != nil {
			r.log.ErrorContext(ctx, "ai tool panic", "tool", name, "panic", rec)
			res, err = failed(), fmt.Errorf("ai tools: %s panicked: %v", name, rec)
		}
	}()
	res, err = at.Run(ctx, r.env(p), input)
	if err != nil {
		r.log.ErrorContext(ctx, "ai tool failed", "tool", name, "err", err)
		return failed(), err
	}
	return capResult(res), nil
}

// prepareWrite is the shared gate of Propose and RunConfirmed: the tool
// must exist, be a write tool, be allowed for the principal and the input
// must match its schema.
func (r *Registry) prepareWrite(ctx context.Context, p Principal, name string, input json.RawMessage) (ActionTool, json.RawMessage, context.Context, *Result, error) {
	t, ok := r.byName[name]
	at, isAction := t.(ActionTool)
	if !ok || !isAction || t.Spec().Kind != KindWrite {
		res := notAllowed(name)
		return nil, nil, ctx, &res, ErrToolNotAllowed
	}
	spec := t.Spec()
	allowed, err := r.Allowed(ctx, p, spec)
	if err != nil {
		res := failed()
		return nil, nil, ctx, &res, err
	}
	if !allowed {
		res := notAllowed(name)
		return nil, nil, ctx, &res, ErrToolNotAllowed
	}
	if err := Validate(spec.InputSchema, input); err != nil {
		res := ErrorResult(CodeInvalidInput, "invalid input for "+name+": "+err.Error()+
			". Fix the arguments and call the tool again.")
		return nil, nil, ctx, &res, nil
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	if _, ok := brandctx.From(ctx); !ok && p.Org != nil {
		ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: p.Org.BrandID, Slug: p.Org.BrandSlug, Status: "active"})
	}
	return at, input, ctx, nil, nil
}

func (r *Registry) env(p Principal) Env {
	return Env{Principal: p, Now: r.now(), Location: r.loc}
}

func confirmationRequired(name string) Result {
	return ErrorResult(CodeConfirmationRequired, "The tool "+name+" changes data and needs the user's confirmation; "+
		"it cannot be run directly.")
}

// marshalInput encodes a normalized tool input.
func marshalInput(v any) (json.RawMessage, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("ai tools: encode input: %w", err)
	}
	return raw, nil
}

// trimmed trims s and cuts it to maxRunes.
func trimmed(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxRunes {
		return string(r[:maxRunes])
	}
	return s
}
