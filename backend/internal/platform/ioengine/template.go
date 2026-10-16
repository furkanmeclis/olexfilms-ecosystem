package ioengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/xuri/excelize/v2"
)

// Sample is the import template of a resource (TEC-757).
type Sample struct {
	Resource string
	Fields   []ImportField
	// Rows are example rows (adapter SampleRows hook).
	Rows []map[string]any
	// Lookups are live drop-down values per field key (SampleLookuper).
	Lookups map[string][]string
	// MaxRows is the row limit printed on the instructions sheet and the
	// height of the data validation ranges (0 = DefaultMaxImportRows).
	MaxRows int
}

// DataSheetName is the workbook defined name pointing at the data sheet of
// a template, so the parser skips the instructions sheet.
const DataSheetName = "OlxImportData"

// listsSheet is the hidden sheet holding the drop-down values.
const listsSheet = "_lists"

// Template header colors (required columns stand out).
const (
	headerRequiredColor = "B91C1C"
	headerOptionalColor = "334155"
	guideHeaderColor    = "0F172A"
)

// EncodeSample builds a sample file for download. CSV/TSV carry a UTF-8
// BOM; XLSX is the v2 template: an instructions sheet first, then the data
// sheet (styled header, required columns marked with *, frozen header,
// typed cells, drop-down lists from a hidden sheet).
func EncodeSample(format ImportFormat, locale string, s Sample) ([]byte, string, error) {
	ds := Dataset{Columns: fieldsToColumns(s.Fields), Rows: s.Rows}
	switch format {
	case ImportJSON:
		rows := s.Rows
		if rows == nil {
			rows = []map[string]any{}
		}
		b, err := json.MarshalIndent(rows, "", "  ")
		return b, "application/json", err
	case ImportCSV:
		b, err := EncodeCSV(ds, locale)
		return b, "text/csv; charset=utf-8", err
	case ImportTSV:
		b, err := encodeDelimited(ds, locale, '\t')
		return b, "text/tab-separated-values; charset=utf-8", err
	case ImportXLSX:
		b, err := encodeTemplateXLSX(s, i18n.Normalize(locale))
		return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", err
	default:
		return nil, "", fmt.Errorf("unsupported sample format")
	}
}

func fieldsToColumns(fields []ImportField) []Column {
	out := make([]Column, len(fields))
	for i, f := range fields {
		out[i] = Column{Key: f.Key, LabelKey: f.LabelKey, Type: f.Type, Required: f.Required}
	}
	return out
}

// TemplateHeader is the data sheet header of a field (required: " *").
func TemplateHeader(loc i18n.Locale, f ImportField) string {
	label := i18n.Translate(loc, f.LabelKey)
	if f.Required {
		return label + " *"
	}
	return label
}

func encodeTemplateXLSX(s Sample, loc i18n.Locale) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	maxRows := s.MaxRows
	if maxRows <= 0 {
		maxRows = DefaultMaxImportRows
	}
	guide := sheetName(i18n.Translate(loc, "imports.template.sheet_guide"), "Guide")
	data := sheetName(i18n.Translate(loc, "imports.template.sheet_data"), "Data")
	if data == guide {
		data = "Data"
	}
	if err := f.SetSheetName(f.GetSheetName(0), guide); err != nil {
		return nil, err
	}
	if _, err := f.NewSheet(data); err != nil {
		return nil, err
	}
	lists := map[string]string{}
	if err := writeTemplateLists(f, s, lists); err != nil {
		return nil, err
	}
	if err := writeTemplateData(f, data, s, loc, maxRows, lists); err != nil {
		return nil, err
	}
	if err := writeTemplateGuide(f, guide, data, s, loc, maxRows, lists); err != nil {
		return nil, err
	}
	if err := f.SetDefinedName(&excelize.DefinedName{
		Name: DataSheetName, RefersTo: quoteSheet(data) + "!$A$1",
	}); err != nil {
		return nil, err
	}
	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeTemplateLists writes the enum and lookup values to the hidden sheet,
// one column per field; lists maps field key -> source range.
func writeTemplateLists(f *excelize.File, s Sample, lists map[string]string) error {
	col := 0
	for _, field := range s.Fields {
		values := field.Enum
		if v, ok := s.Lookups[field.Key]; ok && len(v) > 0 {
			values = v
		}
		if len(values) == 0 {
			continue
		}
		if col == 0 {
			if _, err := f.NewSheet(listsSheet); err != nil {
				return err
			}
		}
		col++
		name, _ := excelize.ColumnNumberToName(col)
		if err := f.SetCellStr(listsSheet, name+"1", field.Key); err != nil {
			return err
		}
		for i, v := range values {
			// List values are string cells (never formulas) and must stay
			// byte-equal to the server values, so they are not prefixed.
			if err := f.SetCellStr(listsSheet, fmt.Sprintf("%s%d", name, i+2), v); err != nil {
				return err
			}
		}
		lists[field.Key] = fmt.Sprintf("%s!$%s$2:$%s$%d", quoteSheet(listsSheet), name, name, len(values)+1)
	}
	if col > 0 {
		return f.SetSheetVisible(listsSheet, false)
	}
	return nil
}

func writeTemplateData(f *excelize.File, sheet string, s Sample, loc i18n.Locale, maxRows int, lists map[string]string) error {
	reqStyle, err := f.NewStyle(headerStyle(headerRequiredColor))
	if err != nil {
		return err
	}
	optStyle, err := f.NewStyle(headerStyle(headerOptionalColor))
	if err != nil {
		return err
	}
	kindStyles := map[FieldKind]int{}
	for kind, numFmt := range map[FieldKind]int{FieldCode: 49, FieldInteger: 1, FieldDecimal: 2} {
		id, err := f.NewStyle(&excelize.Style{NumFmt: numFmt})
		if err != nil {
			return err
		}
		kindStyles[kind] = id
	}
	for i, field := range s.Fields {
		col, _ := excelize.ColumnNumberToName(i + 1)
		header := TemplateHeader(loc, field)
		width := float64(utf8.RuneCountInString(header)) + 4
		if style, ok := kindStyles[field.Kind]; ok {
			if err := f.SetColStyle(sheet, col, style); err != nil {
				return err
			}
		}
		cell := col + "1"
		if err := f.SetCellStr(sheet, cell, header); err != nil {
			return err
		}
		style := optStyle
		if field.Required {
			style = reqStyle
		}
		if err := f.SetCellStyle(sheet, cell, cell, style); err != nil {
			return err
		}
		for r, row := range s.Rows {
			v := strings.TrimSpace(fmt.Sprint(valueOr(row[field.Key])))
			if v == "" {
				continue
			}
			if w := float64(utf8.RuneCountInString(v)) + 2; w > width {
				width = w
			}
			if err := setTemplateCell(f, sheet, fmt.Sprintf("%s%d", col, r+2), field.Kind, v); err != nil {
				return err
			}
		}
		if err := f.SetColWidth(sheet, col, col, min(max(width, 12), 48)); err != nil {
			return err
		}
		if src, ok := lists[field.Key]; ok {
			dv := excelize.NewDataValidation(true)
			dv.Sqref = fmt.Sprintf("%s2:%s%d", col, col, maxRows+1)
			dv.SetSqrefDropList(src)
			// Enums are closed; live lookups only warn (the list may be
			// older than the data).
			style := excelize.DataValidationErrorStyleWarning
			if len(field.Enum) > 0 && len(s.Lookups[field.Key]) == 0 {
				style = excelize.DataValidationErrorStyleStop
			}
			dv.SetError(style, i18n.Translate(loc, "imports.template.invalid_title"),
				i18n.Translate(loc, "imports.template.invalid_list"))
			if err := f.AddDataValidation(sheet, dv); err != nil {
				return err
			}
		}
	}
	return f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
}

// setTemplateCell writes a typed example cell: numbers as numbers, the
// rest as (formula-safe) text.
func setTemplateCell(f *excelize.File, sheet, cell string, kind FieldKind, v string) error {
	switch kind {
	case FieldInteger, FieldDecimal:
		if n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil {
			return f.SetCellFloat(sheet, cell, n, -1, 64)
		}
	}
	return f.SetCellStr(sheet, cell, safeCell(v))
}

func writeTemplateGuide(f *excelize.File, sheet, dataSheet string, s Sample, loc i18n.Locale, maxRows int, lists map[string]string) error {
	title, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	if err != nil {
		return err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}
	head, err := f.NewStyle(headerStyle(guideHeaderColor))
	if err != nil {
		return err
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	if err != nil {
		return err
	}
	required, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: headerRequiredColor},
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	if err != nil {
		return err
	}
	row := 1
	put := func(col string, v string, style int) {
		cell := fmt.Sprintf("%s%d", col, row)
		_ = f.SetCellStr(sheet, cell, safeCell(v))
		if style != 0 {
			_ = f.SetCellStyle(sheet, cell, cell, style)
		}
	}
	params := map[string]string{
		"resource": i18n.ResourceLabel(loc, s.Resource), "sheet": dataSheet, "max": strconv.Itoa(maxRows),
	}
	put("A", i18n.TranslateParams(loc, "imports.template.title", params), title)
	row += 2
	for _, k := range []string{"intro", "step_required", "step_lists", "step_limit", "step_upload"} {
		put("A", i18n.TranslateParams(loc, "imports.template."+k, params), 0)
		row++
	}
	if notes := i18n.Translate(loc, "imports.template.notes."+s.Resource); notes != "imports.template.notes."+s.Resource {
		row++
		put("A", i18n.Translate(loc, "imports.template.notes"), bold)
		row++
		for _, line := range strings.Split(notes, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				put("A", line, 0)
				row++
			}
		}
	}
	row++
	for i, k := range []string{"col_column", "col_required", "col_type", "col_description", "col_values", "col_example"} {
		col, _ := excelize.ColumnNumberToName(i + 1)
		put(col, i18n.Translate(loc, "imports.template."+k), head)
	}
	row++
	for _, field := range s.Fields {
		req := i18n.Translate(loc, "common.no")
		reqStyle := wrap
		if field.Required {
			req, reqStyle = i18n.Translate(loc, "common.yes"), required
		}
		put("A", TemplateHeader(loc, field), reqStyle)
		put("B", req, reqStyle)
		put("C", i18n.Translate(loc, "imports.template.kind."+fieldKindKey(field)), wrap)
		desc := ""
		if field.HintKey != "" {
			desc = i18n.Translate(loc, field.HintKey)
		}
		put("D", desc, wrap)
		put("E", allowedValues(field, s.Lookups[field.Key], loc, lists), wrap)
		example := ""
		if len(s.Rows) > 0 {
			example = strings.TrimSpace(fmt.Sprint(valueOr(s.Rows[0][field.Key])))
		}
		put("F", example, wrap)
		row++
	}
	for col, w := range map[string]float64{"A": 30, "B": 10, "C": 18, "D": 60, "E": 40, "F": 22} {
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return err
		}
	}
	return nil
}

func fieldKindKey(f ImportField) string {
	switch {
	case f.Kind != FieldText:
		return string(f.Kind)
	case f.Type == ColumnTypeBoolean:
		return "boolean"
	case len(f.Enum) > 0:
		return "enum"
	default:
		return "text"
	}
}

// allowedValues describes the accepted values of a field.
func allowedValues(f ImportField, lookup []string, loc i18n.Locale, lists map[string]string) string {
	switch {
	case len(lookup) > 0:
		return i18n.Translate(loc, "imports.template.values_lookup")
	case len(f.Enum) > 0:
		return strings.Join(f.Enum, ", ")
	case f.Type == ColumnTypeBoolean:
		return "true, false"
	}
	if _, ok := lists[f.Key]; ok {
		return i18n.Translate(loc, "imports.template.values_lookup")
	}
	return ""
}

func headerStyle(color string) *excelize.Style {
	return &excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{color}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	}
}

func valueOr(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// sheetName makes a valid sheet name (31 characters, no []:*?/\).
func sheetName(name, fallback string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return -1
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" || strings.HasPrefix(name, "imports.") {
		return fallback
	}
	if utf8.RuneCountInString(name) > 31 {
		name = string([]rune(name)[:31])
	}
	return name
}

func quoteSheet(name string) string {
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}
