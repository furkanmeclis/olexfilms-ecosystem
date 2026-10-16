package ioengine

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// Stable row error codes of the preview check (TEC-757). The localized
// message is the i18n key "imports.error.<lower-case code without IMPORT_>".
const (
	CodeRequired       = "IMPORT_REQUIRED"
	CodeInvalidInteger = "IMPORT_INVALID_INTEGER"
	CodeInvalidNumber  = "IMPORT_INVALID_NUMBER"
	CodeInvalidBoolean = "IMPORT_INVALID_BOOLEAN"
	CodeInvalidValue   = "IMPORT_INVALID_VALUE"
	CodeTooLong        = "IMPORT_TOO_LONG"
	CodeOutOfRange     = "IMPORT_OUT_OF_RANGE"
	CodeNotFound       = "IMPORT_NOT_FOUND"
	CodeAlreadyExists  = "IMPORT_ALREADY_EXISTS"
	CodeNotAllowed     = "IMPORT_NOT_ALLOWED"
	CodeInvalidFormat  = "IMPORT_INVALID_FORMAT"
)

// RowErrorCodes lists the preview row error codes (catalog parity test).
var RowErrorCodes = []string{
	CodeRequired, CodeInvalidInteger, CodeInvalidNumber, CodeInvalidBoolean, CodeInvalidValue,
	CodeTooLong, CodeOutOfRange, CodeNotFound, CodeAlreadyExists, CodeNotAllowed, CodeInvalidFormat,
}

// RowErrorKey is the i18n key of a preview row error code.
func RowErrorKey(code string) string {
	return "imports.error." + strings.ToLower(strings.TrimPrefix(code, "IMPORT_"))
}

// NewRowError builds a localized row error of a field. params fill the
// message ({{field}} is the field label, set here).
func NewRowError(loc i18n.Locale, f ImportField, code string, params map[string]string) RowError {
	p := map[string]string{"field": i18n.Translate(loc, f.LabelKey)}
	for k, v := range params {
		p[k] = v
	}
	return RowError{Field: f.Key, Code: code, Error: i18n.TranslateParams(loc, RowErrorKey(code), p)}
}

// Cell reads a mapped cell (defaults fill an empty one), trimmed.
func Cell(row, defaults map[string]any, key string) string {
	if v := cellString(row, key); v != "" {
		return v
	}
	return cellString(defaults, key)
}

func cellString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// ValidateSchema checks a mapped row against the import fields: required
// cells, integer/decimal kinds, booleans and enums.
func ValidateSchema(fields []ImportField, row, defaults map[string]any, loc i18n.Locale) []RowError {
	var errs []RowError
	for _, f := range fields {
		v := Cell(row, defaults, f.Key)
		if v == "" {
			if f.Required {
				errs = append(errs, NewRowError(loc, f, CodeRequired, nil))
			}
			continue
		}
		switch {
		case f.Kind == FieldInteger:
			if _, ok := ParseInteger(v); !ok {
				errs = append(errs, NewRowError(loc, f, CodeInvalidInteger, map[string]string{"value": v}))
			}
		case f.Kind == FieldDecimal:
			if _, ok := ParseDecimal(v); !ok {
				errs = append(errs, NewRowError(loc, f, CodeInvalidNumber, map[string]string{"value": v}))
			}
		case f.Type == ColumnTypeBoolean:
			if _, ok := ParseBool(v, loc); !ok {
				errs = append(errs, NewRowError(loc, f, CodeInvalidBoolean, map[string]string{"value": v}))
			}
		}
		if len(f.Enum) > 0 && !containsFold(f.Enum, v) {
			errs = append(errs, NewRowError(loc, f, CodeInvalidValue, map[string]string{
				"value": v, "values": strings.Join(f.Enum, ", "),
			}))
		}
	}
	return errs
}

// CheckLength reports a CodeTooLong error when v is longer than max runes.
func CheckLength(loc i18n.Locale, f ImportField, v string, limit int) []RowError {
	if utf8.RuneCountInString(v) > limit {
		return []RowError{NewRowError(loc, f, CodeTooLong, map[string]string{"max": strconv.Itoa(limit)})}
	}
	return nil
}

// ParseInteger reads a whole number ("24", "24.0" from a numeric cell).
func ParseInteger(v string) (int64, bool) {
	v = strings.TrimSuffix(strings.TrimSpace(v), ".0")
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil
}

// ParseDecimal reads a decimal number ("15.5", "15,5").
func ParseDecimal(v string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v), ",", "."), 64)
	return n, err == nil
}

// ParseBool reads true/false, 1/0, yes/no, evet/hayır and the locale's
// common.yes / common.no.
func ParseBool(v string, loc i18n.Locale) (bool, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "1", "true", "yes", "evet", "aktif", "active", "x":
		return true, true
	case "0", "false", "no", "hayır", "hayir", "pasif", "inactive":
		return false, true
	}
	switch s {
	case strings.ToLower(i18n.Translate(loc, "common.yes")):
		return true, true
	case strings.ToLower(i18n.Translate(loc, "common.no")):
		return false, true
	}
	return false, false
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}
