package ioengine

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ParseUpload decodes an uploaded file into header-keyed raw rows.
func ParseUpload(format ImportFormat, r io.Reader) ([]string, []map[string]string, error) {
	switch format {
	case ImportJSON:
		return parseJSON(r)
	case ImportCSV:
		return parseCSV(r, ',')
	case ImportTSV:
		return parseCSV(r, '\t')
	case ImportXLSX:
		return parseXLSX(r)
	default:
		return nil, nil, fmt.Errorf("ioengine: unsupported import format %q", format)
	}
}

func parseJSON(r io.Reader) ([]string, []map[string]string, error) {
	var items []map[string]any
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return nil, nil, fmt.Errorf("ioengine: json decode: %w", err)
	}
	if len(items) == 0 {
		return nil, nil, nil
	}
	headers := collectKeys(items[0])
	rows := make([]map[string]string, 0, len(items))
	for _, item := range items {
		row := map[string]string{}
		for _, h := range headers {
			row[h] = fmt.Sprint(item[h])
		}
		rows = append(rows, row)
	}
	return headers, rows, nil
}

func collectKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func parseCSV(r io.Reader, comma rune) ([]string, []map[string]string, error) {
	cr := csv.NewReader(r)
	cr.Comma = comma
	cr.LazyQuotes = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("ioengine: csv read: %w", err)
	}
	if len(records) == 0 {
		return nil, nil, nil
	}
	headers := records[0]
	rows := make([]map[string]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		row := map[string]string{}
		for i, h := range headers {
			if i < len(rec) {
				row[strings.TrimSpace(h)] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return headers, rows, nil
}

func parseXLSX(r io.Reader) ([]string, []map[string]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("ioengine: xlsx open: %w", err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, nil
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil || len(rows) == 0 {
		return headersOrEmpty(rows), nil, err
	}
	headers := rows[0]
	out := make([]map[string]string, 0, len(rows)-1)
	for _, rec := range rows[1:] {
		row := map[string]string{}
		for i, h := range headers {
			if i < len(rec) {
				row[strings.TrimSpace(h)] = rec[i]
			}
		}
		out = append(out, row)
	}
	return headers, out, nil
}

func headersOrEmpty(rows [][]string) []string {
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

// EncodeSample builds a sample file for download.
func EncodeSample(format ImportFormat, locale string, fields []ImportField, rows []map[string]any) ([]byte, string, error) {
	ds := Dataset{
		Columns: fieldsToColumns(fields),
		Rows:    rows,
	}
	switch format {
	case ImportJSON:
		b, err := json.MarshalIndent(rows, "", "  ")
		return b, "application/json", err
	case ImportCSV:
		b, err := EncodeCSV(ds, locale)
		return b, "text/csv", err
	case ImportTSV:
		b, err := encodeDelimited(ds, locale, '\t')
		return b, "text/tab-separated-values", err
	case ImportXLSX:
		b, err := EncodeXLSX(ds, locale, nil)
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
