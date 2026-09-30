package ioengine

import (
	"bytes"
	"encoding/csv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// EncodeCSV writes a dataset as UTF-8 CSV with BOM for Excel compatibility.
func EncodeCSV(ds Dataset, locale string) ([]byte, error) {
	return encodeDelimited(ds, locale, ',')
}

func encodeDelimited(ds Dataset, locale string, comma rune) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	w.Comma = comma
	loc := i18n.Normalize(locale)
	header := make([]string, len(ds.Columns))
	for i, c := range ds.Columns {
		header[i] = i18n.Translate(loc, c.LabelKey)
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, row := range ds.Rows {
		rec := make([]string, len(ds.Columns))
		for i, c := range ds.Columns {
			rec[i] = formatCell(row[c.Key], c, loc)
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
