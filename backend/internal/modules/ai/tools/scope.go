package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// errOutOfScope marks a permission the principal lost between Available
// and Run (e.g. a scope that needs an organization); it surfaces as
// TOOL_NOT_ALLOWED.
var errOutOfScope = errors.New("ai tools: out of scope")

// resolveScope resolves the caller's reach for one permission exactly as
// middleware.RequireScope does for the matching list route.
func resolveScope(ctx context.Context, tree scopefilter.TreeReader, p Principal, perm string) (scopefilter.Filter, error) {
	f, err := scopefilter.Resolve(ctx, tree, p.Auth, p.Org, perm)
	if errors.Is(err, scopefilter.ErrForbidden) || errors.Is(err, scopefilter.ErrOrganizationRequired) {
		return scopefilter.Filter{}, errOutOfScope
	}
	return f, err
}

// decode validates (again, cheaply) and unmarshals the tool input.
func decode(spec Spec, raw json.RawMessage, dst any) *Result {
	if err := Decode(spec.InputSchema, raw, dst); err != nil {
		r := ErrorResult(CodeInvalidInput, "invalid input for "+spec.Name+": "+err.Error())
		return &r
	}
	return nil
}

// notFound is the result for a record that does not exist or is outside
// the caller's scope.
func notFound(what string) Result {
	return ErrorResult(CodeNotFound, what+" not found (it does not exist or is outside your access). Check the identifier.")
}

// invalidArg is the result for a use case validation error.
func invalidArg(msg string) Result {
	return ErrorResult(CodeInvalidInput, "invalid input: "+msg)
}

// parseID parses a UUID argument.
func parseID(s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(s))
	return id, err == nil
}

// compactPlate strips spaces and dashes ("34 ABC 123" -> "34ABC123").
func compactPlate(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(s)))
}

// schema helpers ----------------------------------------------------------

func object(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string, maxLen int) map[string]any {
	s := map[string]any{"type": "string", "description": desc}
	if maxLen > 0 {
		s["maxLength"] = maxLen
	}
	return s
}

func strMin(desc string, minLen, maxLen int) map[string]any {
	s := str(desc, maxLen)
	s["minLength"] = minLen
	return s
}

func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

func date(desc string) map[string]any {
	return map[string]any{"type": "string", "format": "date", "description": desc}
}

func limitProp() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": MaxRows,
		"description": "Maximum rows to return (default 20, max 50)."}
}

// errCases maps a module use case error to a model result: out of scope /
// forbidden -> TOOL_NOT_ALLOWED, not found -> NOT_FOUND, the module's
// validation error type -> INVALID_INPUT; anything else is an internal
// failure.
type errCases struct {
	tool      string
	what      string
	notFound  []error
	forbidden []error
	invalid   func(error) (string, bool)
}

func (c errCases) result(err error) (Result, error) {
	if errors.Is(err, errOutOfScope) {
		return notAllowed(c.tool), nil
	}
	for _, e := range c.forbidden {
		if errors.Is(err, e) {
			return notAllowed(c.tool), nil
		}
	}
	for _, e := range c.notFound {
		if errors.Is(err, e) {
			return notFound(c.what), nil
		}
	}
	if c.invalid != nil {
		if msg, ok := c.invalid(err); ok {
			return invalidArg(msg), nil
		}
	}
	return Result{}, err
}

// asError reports whether err wraps a T (a module's validation error type).
func asError[T error](err error) (string, bool) {
	var t T
	if errors.As(err, &t) {
		return t.Error(), true
	}
	return "", false
}
