package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ValidationError describes why a tool input does not match its schema.
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + " " + e.Message
}

// Validate checks raw JSON input against the JSON-Schema subset used by tool
// definitions (ported from otopoly-go ai/tools/validate.go) (type, properties, required, additionalProperties, enum,
// minLength/maxLength, minimum/maximum, minItems/maxItems, items, format
// date|uuid). Unknown keywords are ignored.
func Validate(schema map[string]any, raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if !json.Valid(raw) {
		return &ValidationError{Message: "input is not valid JSON"}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return &ValidationError{Message: "input is not valid JSON"}
	}
	return validateValue(schema, v, "")
}

// Decode validates raw input and unmarshals it into dst.
func Decode(schema map[string]any, raw json.RawMessage, dst any) error {
	if err := Validate(schema, raw); err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return &ValidationError{Message: "input does not match the expected shape"}
	}
	return nil
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validateValue(schema map[string]any, v any, path string) error {
	if schema == nil {
		return nil
	}
	if enum, ok := schema["enum"].([]any); ok {
		if !inEnum(enum, v) {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must be one of %s", enumList(enum))}
		}
	}
	if enum, ok := schema["enum"].([]string); ok {
		anyEnum := make([]any, len(enum))
		for i, s := range enum {
			anyEnum[i] = s
		}
		if !inEnum(anyEnum, v) {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must be one of %s", enumList(anyEnum))}
		}
	}
	typ, _ := schema["type"].(string)
	switch typ {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return &ValidationError{Path: path, Message: "must be an object"}
		}
		props, _ := schema["properties"].(map[string]any)
		for _, req := range requiredList(schema["required"]) {
			if val, ok := obj[req]; !ok || val == nil {
				return &ValidationError{Path: join(path, req), Message: "is required"}
			}
		}
		if ap, ok := schema["additionalProperties"].(bool); ok && !ap {
			keys := make([]string, 0, len(obj))
			for k := range obj {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if _, known := props[k]; !known {
					return &ValidationError{Path: join(path, k), Message: "is not an allowed property"}
				}
			}
		}
		for k, val := range obj {
			sub, _ := props[k].(map[string]any)
			if sub == nil || val == nil {
				continue
			}
			if err := validateValue(sub, val, join(path, k)); err != nil {
				return err
			}
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return &ValidationError{Path: path, Message: "must be an array"}
		}
		if n, ok := number(schema["minItems"]); ok && float64(len(arr)) < n {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must have at least %d items", int(n))}
		}
		if n, ok := number(schema["maxItems"]); ok && float64(len(arr)) > n {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must have at most %d items", int(n))}
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, item := range arr {
				if err := validateValue(items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			return &ValidationError{Path: path, Message: "must be a string"}
		}
		n := len([]rune(s))
		if m, ok := number(schema["minLength"]); ok && float64(n) < m {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must be at least %d characters", int(m))}
		}
		if m, ok := number(schema["maxLength"]); ok && float64(n) > m {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must be at most %d characters", int(m))}
		}
		switch schema["format"] {
		case "date":
			if _, err := time.Parse("2006-01-02", s); err != nil || !dateRe.MatchString(s) {
				return &ValidationError{Path: path, Message: "must be a date in YYYY-MM-DD format"}
			}
		case "uuid":
			if _, err := uuid.Parse(s); err != nil {
				return &ValidationError{Path: path, Message: "must be a UUID"}
			}
		}
	case "integer", "number":
		num, ok := v.(json.Number)
		if !ok {
			return &ValidationError{Path: path, Message: "must be a " + typ}
		}
		f, err := num.Float64()
		if err != nil {
			return &ValidationError{Path: path, Message: "must be a " + typ}
		}
		if typ == "integer" && f != math.Trunc(f) {
			return &ValidationError{Path: path, Message: "must be an integer"}
		}
		if m, ok := number(schema["minimum"]); ok && f < m {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must be >= %v", m)}
		}
		if m, ok := number(schema["maximum"]); ok && f > m {
			return &ValidationError{Path: path, Message: fmt.Sprintf("must be <= %v", m)}
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return &ValidationError{Path: path, Message: "must be a boolean"}
		}
	}
	return nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func requiredList(v any) []string {
	switch r := v.(type) {
	case []string:
		return r
	case []any:
		out := make([]string, 0, len(r))
		for _, x := range r {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func inEnum(enum []any, v any) bool {
	for _, e := range enum {
		if fmt.Sprint(e) == fmt.Sprint(v) {
			return true
		}
	}
	return false
}

func enumList(enum []any) string {
	parts := make([]string, len(enum))
	for i, e := range enum {
		parts[i] = fmt.Sprint(e)
	}
	return strings.Join(parts, ", ")
}
