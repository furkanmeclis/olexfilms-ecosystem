package ioengine

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/xuri/excelize/v2"
)

// EncodeXLSX writes a dataset with optional letterhead band.
func EncodeXLSX(ds Dataset, locale string, lh *Letterhead) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	loc := i18n.Normalize(locale)
	rowStart := 1
	if lh != nil {
		rowStart = writeXLSXLetterhead(f, sheet, lh) + 1
	}
	if len(ds.Info) > 0 {
		for _, line := range ds.Info {
			if strings.TrimSpace(line.Value) == "" {
				continue
			}
			_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rowStart), i18n.Translate(loc, line.LabelKey))
			_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rowStart), line.Value)
			rowStart++
		}
		rowStart++
	}
	// header
	for i, c := range ds.Columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, rowStart)
		_ = f.SetCellValue(sheet, cell, i18n.Translate(loc, c.LabelKey))
		if lh != nil && lh.PrimaryColor != "" {
			style, _ := f.NewStyle(&excelize.Style{
				Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
				Fill: excelize.Fill{Type: "pattern", Color: []string{strings.TrimPrefix(lh.PrimaryColor, "#")}, Pattern: 1},
			})
			_ = f.SetCellStyle(sheet, cell, cell, style)
		}
	}
	// rows
	for ri, row := range ds.Rows {
		for ci, c := range ds.Columns {
			cell, _ := excelize.CoordinatesToCellName(ci+1, rowStart+1+ri)
			_ = f.SetCellValue(sheet, cell, formatCell(row[c.Key], c, loc))
		}
	}
	if len(ds.Totals) > 0 {
		totalsRow := rowStart + 1 + len(ds.Rows)
		bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
		for ci, c := range ds.Columns {
			cell, _ := excelize.CoordinatesToCellName(ci+1, totalsRow)
			_ = f.SetCellValue(sheet, cell, formatCell(ds.Totals[c.Key], c, loc))
			_ = f.SetCellStyle(sheet, cell, cell, bold)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeXLSXLetterhead(f *excelize.File, sheet string, lh *Letterhead) int {
	row := 1
	if lh.CompanyName != "" {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), lh.CompanyName)
		row++
	}
	if lh.Tagline != "" {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), lh.Tagline)
		row++
	}
	meta := strings.Join(filterNonEmpty([]string{lh.Address, lh.Phone, lh.Email, lh.Website}), " · ")
	if meta != "" {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), meta)
		row++
	}
	row++ // blank
	return row
}

func filterNonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ContentTypeForExport returns MIME for export format.
func ContentTypeForExport(format ExportFormat) string {
	switch format {
	case ExportPDF:
		return "application/pdf"
	case ExportXLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ExportJSON:
		return "application/json; charset=utf-8"
	default:
		return "text/csv"
	}
}

// FileExtForExport returns file extension.
func FileExtForExport(format ExportFormat) string {
	switch format {
	case ExportPDF:
		return "pdf"
	case ExportXLSX:
		return "xlsx"
	case ExportJSON:
		return "json"
	default:
		return "csv"
	}
}
