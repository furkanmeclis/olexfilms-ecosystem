package apiquery

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Filter helpers for list endpoints. Every helper reports bad input as
// *ValidationError (400 VALIDATION_ERROR) with the query parameter as field.

func invalid(field, msg string) *ValidationError {
	return &ValidationError{Details: []Detail{{Field: field, Message: msg, Code: "invalid"}}}
}

// CSVValues returns the values of a multi-value parameter. It accepts a
// comma list (status=a,b), repeated keys (status=a&status=b) or both;
// empties are dropped and duplicates removed (first occurrence wins).
func CSVValues(values url.Values, key string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range values[key] {
		for _, v := range SplitCSV(raw) {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// EnumList parses a multi-value enum filter (see CSVValues) and checks each
// value against allowed. It returns nil when the parameter is absent or
// empty, so the SQL side can treat a NULL array as "no filter".
func EnumList(values url.Values, key string, allowed ...string) ([]string, error) {
	vals := CSVValues(values, key)
	if len(vals) == 0 {
		return nil, nil
	}
	ok := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		ok[a] = struct{}{}
	}
	var details []Detail
	for _, v := range vals {
		if _, found := ok[v]; !found {
			details = append(details, Detail{
				Field:   key,
				Message: fmt.Sprintf("invalid value %q; allowed: %s", v, strings.Join(allowed, ", ")),
				Code:    "invalid",
			})
		}
	}
	if len(details) > 0 {
		return nil, &ValidationError{Details: details}
	}
	return vals, nil
}

// TimeRange is a parsed <prefix>_from / <prefix>_to pair. From is inclusive
// and Before is an exclusive upper bound, so SQL is always
// `col >= from AND col < before`.
type TimeRange struct {
	From   *time.Time
	Before *time.Time
}

// DateRange reads <prefix>_from and <prefix>_to. Each accepts YYYY-MM-DD
// (UTC midnight) or RFC3339. A date-only _to includes that whole day
// (Before = next midnight); an RFC3339 _to includes that instant
// (Before = instant + 1µs, the Postgres timestamp resolution). from after
// to is a validation error.
func DateRange(values url.Values, prefix string) (TimeRange, error) {
	var out TimeRange
	fromKey, toKey := prefix+"_from", prefix+"_to"
	if raw := strings.TrimSpace(values.Get(fromKey)); raw != "" {
		t, _, err := parseDateOrTime(raw)
		if err != nil {
			return TimeRange{}, invalid(fromKey, "must be YYYY-MM-DD or RFC3339")
		}
		out.From = &t
	}
	if raw := strings.TrimSpace(values.Get(toKey)); raw != "" {
		t, dateOnly, err := parseDateOrTime(raw)
		if err != nil {
			return TimeRange{}, invalid(toKey, "must be YYYY-MM-DD or RFC3339")
		}
		if dateOnly {
			t = t.AddDate(0, 0, 1)
		} else {
			t = t.Add(time.Microsecond)
		}
		out.Before = &t
	}
	if out.From != nil && out.Before != nil && !out.From.Before(*out.Before) {
		return TimeRange{}, invalid(fromKey, fmt.Sprintf("%s must not be after %s", fromKey, toKey))
	}
	return out, nil
}

func parseDateOrTime(raw string) (time.Time, bool, error) {
	if t, err := time.Parse(time.DateOnly, raw); err == nil {
		return t.UTC(), true, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false, err
	}
	return t.UTC(), false, nil
}

// NumberRange is a parsed <prefix>_min / <prefix>_max pair (both inclusive).
type NumberRange struct {
	Min *float64
	Max *float64
}

// NumRange reads <prefix>_min and <prefix>_max as finite decimal numbers.
// min greater than max is a validation error.
func NumRange(values url.Values, prefix string) (NumberRange, error) {
	var out NumberRange
	for _, side := range []struct {
		key string
		dst **float64
	}{{prefix + "_min", &out.Min}, {prefix + "_max", &out.Max}} {
		raw := strings.TrimSpace(values.Get(side.key))
		if raw == "" {
			continue
		}
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return NumberRange{}, invalid(side.key, "must be a number")
		}
		*side.dst = &n
	}
	if out.Min != nil && out.Max != nil && *out.Min > *out.Max {
		return NumberRange{}, invalid(prefix+"_min", fmt.Sprintf("%s_min must not exceed %s_max", prefix, prefix))
	}
	return out, nil
}

// Bool reads a true|false parameter; absent or empty returns nil.
func Bool(values url.Values, key string) (*bool, error) {
	switch strings.TrimSpace(values.Get(key)) {
	case "":
		return nil, nil
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	default:
		return nil, invalid(key, "must be true or false")
	}
}
