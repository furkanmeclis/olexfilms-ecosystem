package apiquery

import (
	"fmt"
	"strings"
)

// Detail is a field-level validation detail (compatible with pkg/response.Detail).
type Detail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// ValidationError is returned when sort/include/fields fail whitelist checks.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	if len(e.Details) == 1 {
		return e.Details[0].Message
	}
	return fmt.Sprintf("validation failed (%d details)", len(e.Details))
}

// SortColumns maps API field names to sort keys (trusted literals).
type SortColumns map[string]string

// AllowedNames is a set of allowed include or field names.
type AllowedNames map[string]struct{}

// NewAllowed builds an AllowedNames set.
func NewAllowed(names ...string) AllowedNames {
	out := make(AllowedNames, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		out[n] = struct{}{}
	}
	return out
}

// ValidateSort ensures every sort field is in the whitelist.
func ValidateSort(sorts []SortField, allowed SortColumns) error {
	if len(sorts) == 0 {
		return nil
	}
	if len(allowed) == 0 {
		return &ValidationError{Details: []Detail{{
			Field: "sort", Message: "sort is not supported for this resource", Code: "unknown",
		}}}
	}
	var details []Detail
	for _, s := range sorts {
		if _, ok := allowed[s.Field]; !ok {
			details = append(details, Detail{
				Field:   "sort",
				Message: fmt.Sprintf("unknown sort field %q", s.Field),
				Code:    "unknown",
			})
		}
	}
	if len(details) > 0 {
		return &ValidationError{Details: details}
	}
	return nil
}

// PrimarySort returns the first sort field when present and allowed.
func PrimarySort(sorts []SortField, allowed SortColumns) (field string, desc bool, ok bool) {
	if len(sorts) == 0 {
		return "", false, false
	}
	if _, exists := allowed[sorts[0].Field]; !exists {
		return "", false, false
	}
	return sorts[0].Field, sorts[0].Desc, true
}
