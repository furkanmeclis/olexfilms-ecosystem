package ioengine

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// CSVSafe neutralises spreadsheet formula injection: a value starting with
// =, +, -, @, tab or CR gets a leading apostrophe, so a spreadsheet shows it
// as text instead of evaluating it.
func CSVSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// safeCell is CSVSafe for exported string cells; a plain number such as a
// negative balance ("-120.50", "-1.234,50") cannot be a formula and is kept.
func safeCell(v string) string {
	if v == "" || !strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return v
	}
	if isPlainNumber(v) {
		return v
	}
	return "'" + v
}

func isPlainNumber(v string) bool {
	s := strings.TrimLeft(v, "+-")
	if s == "" || len(v)-len(s) > 1 {
		return false
	}
	digits := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits = true
		case r == '.' || r == ',' || r == ' ':
		default:
			return false
		}
	}
	return digits
}

// DefaultMaxImportRows is the row limit of an import file (TEC-757);
// adapters override it with RowLimiter.
const DefaultMaxImportRows = 5000

// CodeTooManyRows is the stable API error code of a file over the limit.
const CodeTooManyRows = "IMPORT_TOO_MANY_ROWS"

// MaxImportRows is the row limit of an adapter.
func MaxImportRows(a ResourceAdapter) int {
	if l, ok := a.(RowLimiter); ok && l.MaxImportRows() > 0 {
		return l.MaxImportRows()
	}
	return DefaultMaxImportRows
}

// RowLimitError rejects an import file with more rows than the limit.
type RowLimitError struct {
	Rows, Max int
	Locale    i18n.Locale
}

func (e *RowLimitError) Error() string {
	return i18n.TranslateParams(e.Locale, "imports.error.too_many_rows", map[string]string{
		"rows": strconv.Itoa(e.Rows), "max": strconv.Itoa(e.Max),
	})
}

// APICode is the stable error code (response.Coded).
func (e *RowLimitError) APICode() string { return CodeTooManyRows }

// CheckRowLimit returns a *RowLimitError when rows exceed the limit.
func CheckRowLimit(a ResourceAdapter, rows int, locale i18n.Locale) error {
	if limit := MaxImportRows(a); rows > limit {
		return &RowLimitError{Rows: rows, Max: limit, Locale: locale}
	}
	return nil
}
