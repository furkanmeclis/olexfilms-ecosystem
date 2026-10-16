package ioengine

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/xuri/excelize/v2"
)

// ErrorReport is the downloadable result of an import job (TEC-757): the
// uploaded rows with their source headers, each with its row number,
// status and errors.
type ErrorReport struct {
	Headers []string
	Lines   []ReportLine
}

// ReportLine is one uploaded row of an error report.
type ReportLine struct {
	Index  int
	Cells  map[string]string
	Status string
	Errors []string
	Failed bool
}

const reportFailedFill = "FEE2E2"

// reportColumnKeys are the i18n keys of the report columns.
var reportColumnKeys = []string{"imports.report.col_row", "imports.report.col_status", "imports.report.col_error"}

// IsReportHeader reports whether h is a report column (row, status, error)
// in loc, so a re-uploaded report maps only the data columns.
func IsReportHeader(h string, loc i18n.Locale) bool {
	h = strings.TrimSpace(h)
	for _, k := range reportColumnKeys {
		if strings.EqualFold(h, i18n.Translate(loc, k)) {
			return true
		}
	}
	return false
}

// EncodeErrorReport writes the report as XLSX: the row, status and error
// columns first, then the source columns as text cells; failed rows come
// first and are highlighted. Cells are formula-safe.
func EncodeErrorReport(r ErrorReport, locale string) ([]byte, error) {
	loc := i18n.Normalize(locale)
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := sheetName(i18n.Translate(loc, "imports.report.sheet"), "Report")
	if err := f.SetSheetName(f.GetSheetName(0), sheet); err != nil {
		return nil, err
	}
	head, err := f.NewStyle(headerStyle(guideHeaderColor))
	if err != nil {
		return nil, err
	}
	text, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	failed, err := f.NewStyle(&excelize.Style{
		NumFmt: 49, Fill: excelize.Fill{Type: "pattern", Color: []string{reportFailedFill}, Pattern: 1},
	})
	if err != nil {
		return nil, err
	}
	headers := make([]string, 0, len(r.Headers)+3)
	for _, k := range reportColumnKeys {
		headers = append(headers, i18n.Translate(loc, k))
	}
	headers = append(headers, r.Headers...)
	widths := make([]float64, len(headers))
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr(sheet, cell, safeCell(h)); err != nil {
			return nil, err
		}
		widths[i] = float64(utf8.RuneCountInString(h)) + 4
	}
	last, _ := excelize.CoordinatesToCellName(len(headers), 1)
	if err := f.SetCellStyle(sheet, "A1", last, head); err != nil {
		return nil, err
	}
	lines := slices.Clone(r.Lines)
	slices.SortStableFunc(lines, func(a, b ReportLine) int {
		switch {
		case a.Failed == b.Failed:
			return a.Index - b.Index
		case a.Failed:
			return -1
		default:
			return 1
		}
	})
	for i, line := range lines {
		rowNum := i + 2
		values := make([]string, 0, len(headers))
		values = append(values, strconv.Itoa(line.Index), line.Status, strings.Join(line.Errors, "; "))
		for _, h := range r.Headers {
			values = append(values, line.Cells[h])
		}
		for c, v := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowNum)
			if c == 0 {
				if err := f.SetCellInt(sheet, cell, int64(line.Index)); err != nil {
					return nil, err
				}
			} else if err := f.SetCellStr(sheet, cell, safeCell(v)); err != nil {
				return nil, err
			}
			if w := float64(utf8.RuneCountInString(v)) + 2; w > widths[c] {
				widths[c] = w
			}
		}
		style := text
		if line.Failed {
			style = failed
		}
		first, _ := excelize.CoordinatesToCellName(1, rowNum)
		end, _ := excelize.CoordinatesToCellName(len(headers), rowNum)
		if err := f.SetCellStyle(sheet, first, end, style); err != nil {
			return nil, err
		}
	}
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		limit := 40.0
		if i == 2 {
			limit = 80
		}
		if err := f.SetColWidth(sheet, col, col, min(max(w, 8), limit)); err != nil {
			return nil, err
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return nil, err
	}
	if err := f.AutoFilter(sheet, fmt.Sprintf("A1:%s", last), nil); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ErrorReportFilename is "<source name without extension>-hatalar.xlsx".
func ErrorReportFilename(source string) string {
	base := strings.TrimSpace(source)
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	if base == "" {
		base = "import"
	}
	return base + "-hatalar.xlsx"
}
