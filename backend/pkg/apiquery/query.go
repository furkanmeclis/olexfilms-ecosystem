// Package apiquery parses standard list query parameters for HTTP APIs.
package apiquery

import (
	"net/url"
	"strconv"
	"strings"
)

const (
	// DefaultLimit is the default page size for list endpoints.
	DefaultLimit int32 = 20
	// MaxLimit is the maximum page size for list endpoints.
	MaxLimit int32 = 100
)

// SortField is one sort token (prefix "-" = descending).
type SortField struct {
	Field string
	Desc  bool
}

// Query holds normalized list query parameters.
type Query struct {
	Limit   int32
	Offset  int32
	Q       string
	Sort    []SortField
	Include []string
	Fields  []string
}

// Parse reads standard list query params from url.Values.
// Supported keys: limit, offset, q, sort, include, fields.
func Parse(values url.Values) Query {
	q := Query{
		Limit:  DefaultLimit,
		Offset: 0,
	}
	if v := strings.TrimSpace(values.Get("limit")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			q.Limit = int32(n)
		}
	}
	if v := strings.TrimSpace(values.Get("offset")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			q.Offset = int32(n)
		}
	}
	q.Limit, q.Offset = Clamp(q.Limit, q.Offset, DefaultLimit, MaxLimit)
	q.Q = strings.TrimSpace(values.Get("q"))
	q.Sort = ParseSort(values.Get("sort"))
	q.Include = SplitCSV(values.Get("include"))
	q.Fields = SplitCSV(values.Get("fields"))
	return q
}

// Clamp normalizes limit/offset.
func Clamp(limit, offset, def, max int32) (int32, int32) {
	if def <= 0 {
		def = DefaultLimit
	}
	if max <= 0 {
		max = MaxLimit
	}
	if limit <= 0 || limit > max {
		limit = def
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ParseSort parses sort tokens: "-created_at,name".
func ParseSort(raw string) []SortField {
	parts := SplitCSV(raw)
	if len(parts) == 0 {
		return nil
	}
	out := make([]SortField, 0, len(parts))
	for _, p := range parts {
		desc := false
		if strings.HasPrefix(p, "-") {
			desc = true
			p = strings.TrimPrefix(p, "-")
		}
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, SortField{Field: p, Desc: desc})
	}
	return out
}

// SplitCSV splits a comma-separated list, trimming empties.
func SplitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ForbiddenParams reports banned pagination/search aliases present in values.
func ForbiddenParams(values url.Values) []string {
	banned := []string{"page", "page_size", "per_page", "search"}
	var found []string
	for _, key := range banned {
		if _, ok := values[key]; ok {
			found = append(found, key)
		}
	}
	return found
}
