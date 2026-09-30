package ioengine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

type jsonExportDocument struct {
	Resource   string             `json:"resource"`
	Locale     string             `json:"locale"`
	ExportedAt string             `json:"exported_at"`
	Columns    []jsonExportColumn `json:"columns"`
	Rows       []map[string]any   `json:"rows"`
}

type jsonExportColumn struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

// EncodeJSON writes a UTF-8 JSON document with column metadata and row objects.
func EncodeJSON(ds Dataset, locale string) ([]byte, error) {
	loc := i18n.Normalize(locale)
	columns := make([]jsonExportColumn, 0, len(ds.Columns))
	for _, c := range ds.Columns {
		columns = append(columns, jsonExportColumn{
			Key:      c.Key,
			Label:    i18n.Translate(loc, c.LabelKey),
			Type:     string(c.Type),
			Required: c.Required,
		})
	}
	rows := make([]map[string]any, 0, len(ds.Rows))
	for _, row := range ds.Rows {
		item := make(map[string]any, len(ds.Columns))
		for _, c := range ds.Columns {
			item[c.Key] = formatJSONValue(row[c.Key], c)
		}
		rows = append(rows, item)
	}
	doc := jsonExportDocument{
		Resource:   ds.Resource,
		Locale:     string(loc),
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Columns:    columns,
		Rows:       rows,
	}
	return json.MarshalIndent(doc, "", "  ")
}

func formatJSONValue(v any, col Column) any {
	if v == nil {
		return nil
	}
	switch col.Type {
	case ColumnTypeBoolean:
		if b, ok := v.(bool); ok {
			return b
		}
	case ColumnTypeDatetime:
		switch t := v.(type) {
		case time.Time:
			return t.UTC().Format(time.RFC3339)
		}
	}
	return fmt.Sprint(v)
}
