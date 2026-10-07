package tools

import (
	"fmt"
	"strings"
	"unicode"
)

// Output limits: a list tool returns at most MaxRows rows and a result is
// at most MaxResultBytes; anything larger asks the model to narrow down.
const (
	MaxRows        = 50
	DefaultRows    = 20
	MaxResultBytes = 16 * 1024
)

// Field length caps for free text from stored records fed to the model.
const (
	maxNameChars = 120
	maxTextChars = 300
)

// ExportNotice answers requests for a full list or a CSV / XLSX file: the
// tools never build files (TEC-386); the panel list screens export through
// the io engine.
const ExportNotice = "Tools do not create CSV / XLSX files: for a complete list or a file, " +
	"use Export on the matching list screen in the panel."

// List is the common envelope of list results. Total is the number of
// matching records; Truncated tells the model there are more than it got.
type List[T any] struct {
	Items     []T    `json:"items"`
	Returned  int    `json:"returned"`
	Total     int64  `json:"total"`
	Truncated bool   `json:"truncated,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

// NewList builds the list envelope.
func NewList[T any](items []T, total int64) List[T] {
	if items == nil {
		items = []T{}
	}
	l := List[T]{Items: items, Returned: len(items), Total: total}
	if total > int64(len(items)) {
		l.Truncated = true
		l.Hint = fmt.Sprintf("Showing %d of %d records. Narrow the search (more specific query or filters) instead of paging. ",
			len(items), total) + ExportNotice
	}
	return l
}

// limitArg clamps a requested row count to 1..MaxRows (DefaultRows when 0).
func limitArg(n int) int32 {
	switch {
	case n <= 0:
		return DefaultRows
	case n > MaxRows:
		return MaxRows
	}
	return int32(n)
}

// capResult replaces a result over MaxResultBytes with a narrowing request.
func capResult(r Result) Result {
	if len(r.Content) <= MaxResultBytes {
		return r
	}
	return ErrorResult(CodeTooLarge, fmt.Sprintf(
		"The result is too large (%d bytes, limit %d). Narrow the request: use a more specific query, filters or a lower limit.",
		len(r.Content), MaxResultBytes))
}

// dataText prepares untrusted free text from stored records for a tool
// result: control characters and line breaks become spaces (so a note
// cannot fake message structure), whitespace collapses and the text is
// clipped to max runes. The value is JSON-encoded as a field, never
// concatenated into prose.
func dataText(s string, max int) string {
	var sb strings.Builder
	sb.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			if !space {
				sb.WriteByte(' ')
				space = true
			}
			continue
		}
		space = false
		sb.WriteRune(r)
	}
	out := strings.TrimSpace(sb.String())
	rs := []rune(out)
	if len(rs) <= max {
		return out
	}
	return strings.TrimSpace(string(rs[:max])) + "…"
}

// textPtr applies dataText to an optional value.
func textPtr(s *string, max int) *string {
	if s == nil {
		return nil
	}
	v := dataText(*s, max)
	return &v
}
