// Package tools is the AI assistant tool registry (TEC-385, F4-01c) shared
// by the panel chat, WhatsApp (F4-02c) and MCP (F4-03c).
//
// A tool declares a Spec (name, English model-facing description, JSON
// schema, kind, required permissions, allowed organization types, realm and
// feature module) and a Run function. Registry.Available lists the tools a
// principal may use and Registry.Call runs the very same check again before
// validating the input and calling Run, so a tool the model was never
// offered (or invents) answers TOOL_NOT_ALLOWED. Tools only call the module
// use cases with the caller's own scope (organization, subtree, brand K20,
// own / assigned); they never query the database directly.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
)

// Kind classifies a tool.
type Kind string

const (
	// KindRead tools only read data and run immediately.
	KindRead Kind = "read"
	// KindWrite tools change data; the chat loop routes them through the
	// confirmation card (F4-01e) instead of running them directly.
	KindWrite Kind = "write"
)

// Realm is the audience of a tool set.
type Realm string

const (
	// RealmPanel: panel users in an active organization (center,
	// distributor, dealer).
	RealmPanel Realm = "panel"
	// RealmCustomer: signed-in portal / WhatsApp customers (F4-01d).
	RealmCustomer Realm = "customer"
	// RealmVisitor: unauthenticated WhatsApp visitors (F4-01d).
	RealmVisitor Realm = "visitor"
)

// Organization types a panel tool may be limited to.
const (
	OrgCenter      = "center"
	OrgDistributor = "distributor"
	OrgDealer      = "dealer"
)

// Result codes.
const (
	// CodeToolNotAllowed: the tool does not exist or the principal may not
	// use it (permission, organization type, realm, module, platform toggle).
	CodeToolNotAllowed = "TOOL_NOT_ALLOWED"
	// CodeInvalidInput: the input does not match the tool schema.
	CodeInvalidInput = "INVALID_INPUT"
	// CodeNotFound: the record does not exist or is outside the caller's
	// scope (the two are never distinguished).
	CodeNotFound = "NOT_FOUND"
	// CodeTooLarge: the result exceeds MaxResultBytes.
	CodeTooLarge = "RESULT_TOO_LARGE"
	// CodeToolFailed: an internal error; details are logged, not shown.
	CodeToolFailed = "TOOL_FAILED"
)

// ErrToolNotAllowed is returned by Call (next to a TOOL_NOT_ALLOWED result)
// when the principal may not use the tool.
var ErrToolNotAllowed = errors.New("ai tools: tool not allowed")

// Spec describes a tool.
type Spec struct {
	Name string
	// Description is English text for the model.
	Description string
	// InputSchema is a JSON Schema object (see Validate for the subset).
	InputSchema map[string]any
	Kind        Kind
	// Permissions are all required.
	Permissions []string
	// OrgTypes limits panel tools to organization types (empty: any).
	OrgTypes []string
	Realm    Realm
	// Feature is the module key (features.Module*) that must be on for the
	// active organization; empty for none.
	Feature string
}

// Def converts a spec to the provider tool definition.
func (s Spec) Def() llm.ToolDef {
	return llm.ToolDef{Name: s.Name, Description: s.Description, InputSchema: s.InputSchema}
}

// Principal is who calls a tool: the authenticated user, the active
// organization (panel realm) and the realm of the channel.
type Principal struct {
	Auth authctx.Principal
	// Org is the active organization; nil outside the panel realm.
	Org   *orgctx.Scope
	Realm Realm
}

// Env is the execution environment of one tool call.
type Env struct {
	Principal Principal
	Now       time.Time
	Location  *time.Location
}

// Loc returns the business time zone (Europe/Istanbul when unset).
func (e Env) Loc() *time.Location {
	if e.Location != nil {
		return e.Location
	}
	return defaultLocation
}

var defaultLocation = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// Result is the outcome of a tool call; Content goes back to the model as
// the tool_result.
type Result struct {
	Content string
	IsError bool
	// Code is set on errors (Code* constants).
	Code string
	// Link is the record a confirmed write created or changed.
	Link *Link
}

// Block converts the result to a tool_result block.
func (r Result) Block(toolUseID string) llm.Block {
	return llm.ToolResultBlock(toolUseID, r.Content, r.IsError)
}

// Tool is an executable assistant tool.
type Tool interface {
	Spec() Spec
	// Run executes a validated input. Errors that the model should see are
	// returned as an error Result; a returned error is an internal failure.
	Run(ctx context.Context, env Env, input json.RawMessage) (Result, error)
}

// FeatureChecker reports whether a module is on for an organization
// (features.Service).
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// ToggleReader returns the platform tool switches
// (ai_settings.tool_toggles): a false value disables a tool, a missing name
// means enabled.
type ToggleReader interface {
	ToolToggles(ctx context.Context) (map[string]bool, error)
}

// Registry is the set of tools.
type Registry struct {
	byName   map[string]Tool
	features FeatureChecker
	toggles  ToggleReader
	now      func() time.Time
	loc      *time.Location
	log      *slog.Logger
}

// NewRegistry builds an empty registry. features may be nil only in tests
// (every module then counts as on).
func NewRegistry(features FeatureChecker) *Registry {
	return &Registry{byName: map[string]Tool{}, features: features, now: time.Now, log: slog.Default()}
}

// WithToggles sets the platform tool toggles.
func (r *Registry) WithToggles(t ToggleReader) *Registry { r.toggles = t; return r }

// WithClock sets the clock and business time zone (tests).
func (r *Registry) WithClock(now func() time.Time, loc *time.Location) *Registry {
	r.now, r.loc = now, loc
	return r
}

// WithLogger sets the logger for internal tool failures.
func (r *Registry) WithLogger(l *slog.Logger) *Registry {
	if l != nil {
		r.log = l
	}
	return r
}

// Register adds a tool; nil tools are skipped. A duplicate name panics
// (programming error at start-up).
func (r *Registry) Register(t Tool) {
	if t == nil {
		return
	}
	name := t.Spec().Name
	if _, dup := r.byName[name]; dup {
		panic("ai tools: duplicate tool " + name)
	}
	r.byName[name] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// All returns every registered tool sorted by name (a stable order keeps
// the prompt cache prefix identical between requests).
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.byName))
	for _, t := range r.byName {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec().Name < out[j].Spec().Name })
	return out
}

// Allowed reports whether the principal may use a tool. It is the single
// check behind both Available and Call.
func (r *Registry) Allowed(ctx context.Context, p Principal, spec Spec) (bool, error) {
	toggles, err := r.loadToggles(ctx)
	if err != nil {
		return false, err
	}
	return r.allowed(ctx, p, spec, toggles)
}

func (r *Registry) loadToggles(ctx context.Context) (map[string]bool, error) {
	if r.toggles == nil {
		return nil, nil
	}
	t, err := r.toggles.ToolToggles(ctx)
	if err != nil {
		return nil, fmt.Errorf("ai tools: toggles: %w", err)
	}
	return t, nil
}

func (r *Registry) allowed(ctx context.Context, p Principal, spec Spec, toggles map[string]bool) (bool, error) {
	if spec.Realm != p.Realm {
		return false, nil
	}
	if spec.Realm == RealmPanel && p.Org == nil {
		return false, nil
	}
	if len(spec.OrgTypes) > 0 && (p.Org == nil || !slices.Contains(spec.OrgTypes, p.Org.OrgType)) {
		return false, nil
	}
	// K20: a request made on one brand's domain never reaches another
	// brand's organization.
	if b, ok := brandctx.From(ctx); ok && p.Org != nil && b.ID != p.Org.BrandID {
		return false, nil
	}
	for _, perm := range spec.Permissions {
		if !p.Auth.HasPermission(perm) {
			return false, nil
		}
	}
	if on, set := toggles[spec.Name]; set && !on {
		return false, nil
	}
	if spec.Feature != "" && r.features != nil {
		if p.Org == nil {
			return false, nil
		}
		on, err := r.features.Enabled(ctx, p.Org.InternalID, spec.Feature)
		if err != nil {
			return false, fmt.Errorf("ai tools: feature %s: %w", spec.Feature, err)
		}
		if !on {
			return false, nil
		}
	}
	return true, nil
}

// Available returns the tools the principal may use, sorted by name.
func (r *Registry) Available(ctx context.Context, p Principal) ([]Tool, error) {
	toggles, err := r.loadToggles(ctx)
	if err != nil {
		return nil, err
	}
	var out []Tool
	for _, t := range r.All() {
		ok, err := r.allowed(ctx, p, t.Spec(), toggles)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// Defs returns the provider definitions of the available tools.
func (r *Registry) Defs(ctx context.Context, p Principal) ([]llm.ToolDef, error) {
	ts, err := r.Available(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]llm.ToolDef, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Spec().Def())
	}
	return out, nil
}

// Call checks, validates and runs one tool call. The Result is always
// suitable for the model. The error is ErrToolNotAllowed when the
// principal may not use the tool, or an internal failure (to log); a
// validation failure is only an error Result.
func (r *Registry) Call(ctx context.Context, p Principal, name string, input json.RawMessage) (res Result, err error) {
	t, ok := r.byName[name]
	if !ok {
		return notAllowed(name), ErrToolNotAllowed
	}
	spec := t.Spec()
	allowed, err := r.Allowed(ctx, p, spec)
	if err != nil {
		return failed(), err
	}
	if !allowed {
		return notAllowed(name), ErrToolNotAllowed
	}
	// TEC-387: a write tool only runs through the confirmation card
	// (Propose → actions use case → RunConfirmed).
	if spec.Kind == KindWrite {
		return confirmationRequired(name), ErrConfirmationRequired
	}
	if err := Validate(spec.InputSchema, input); err != nil {
		return ErrorResult(CodeInvalidInput, "invalid input for "+name+": "+err.Error()+
			". Fix the arguments and call the tool again."), nil
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	// Channels without a domain (WhatsApp, MCP) carry the organization's
	// brand, which the brand scoped use cases read (K20).
	if _, ok := brandctx.From(ctx); !ok && p.Org != nil {
		ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: p.Org.BrandID, Slug: p.Org.BrandSlug, Status: "active"})
	}
	defer func() {
		if rec := recover(); rec != nil {
			r.log.ErrorContext(ctx, "ai tool panic", "tool", name, "panic", rec)
			res, err = failed(), fmt.Errorf("ai tools: %s panicked: %v", name, rec)
		}
	}()
	env := Env{Principal: p, Now: r.now(), Location: r.loc}
	res, err = t.Run(ctx, env, input)
	if err != nil {
		r.log.ErrorContext(ctx, "ai tool failed", "tool", name, "err", err)
		return failed(), err
	}
	return capResult(res), nil
}

func notAllowed(name string) Result {
	return ErrorResult(CodeToolNotAllowed, "TOOL_NOT_ALLOWED: the tool "+name+
		" is not available to this user. Do not call it again; answer with the tools you have.")
}

func failed() Result {
	return ErrorResult(CodeToolFailed, "The tool failed because of an internal error. Tell the user to try again later.")
}

// ErrorResult is an error tool_result returned to the model.
func ErrorResult(code, msg string) Result {
	return Result{Content: msg, IsError: true, Code: code}
}

// JSONResult marshals v as compact JSON content.
func JSONResult(v any) (Result, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return Result{}, fmt.Errorf("ai tools: encode result: %w", err)
	}
	return Result{Content: string(raw)}, nil
}
