package ioengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// ExportTitle returns a localized document title for an export job.
func ExportTitle(locale string, resource string) string {
	loc := i18n.Normalize(locale)
	key := "export.title." + resource
	if title := i18n.Translate(loc, key); title != key {
		return title
	}
	resourceLabel := i18n.Translate(loc, "resources."+resource)
	if resourceLabel == "resources."+resource {
		resourceLabel = resource
	}
	return resourceLabel + " — " + i18n.Translate(loc, "export.document")
}

// ExportedAtLabel returns a localized export timestamp string.
func ExportedAtLabel(locale string) string {
	loc := i18n.Normalize(locale)
	prefix := i18n.Translate(loc, "export.generated_at")
	return prefix + ": " + formatDatetime(time.Now().UTC(), loc)
}

func formatDatetime(t time.Time, loc i18n.Locale) string {
	if loc == i18n.LocaleEN {
		return t.Format("2006-01-02 15:04 UTC")
	}
	return t.Format("02.01.2006 15:04 UTC")
}

func formatCell(v any, col Column, loc i18n.Locale) string {
	if v == nil {
		return ""
	}
	switch col.Type {
	case ColumnTypeBoolean:
		if b, ok := v.(bool); ok {
			if b {
				return i18n.Translate(loc, "common.yes")
			}
			return i18n.Translate(loc, "common.no")
		}
	case ColumnTypeEnum:
		s := fmt.Sprint(v)
		fieldKey := col.LabelKey + "." + s
		if translated := i18n.Translate(loc, fieldKey); translated != fieldKey {
			return translated
		}
		dot := strings.Index(col.LabelKey, ".")
		if dot >= 0 {
			prefix := col.LabelKey[:dot+1]
			enumKey := prefix + "status." + s
			if translated := i18n.Translate(loc, enumKey); translated != enumKey {
				return translated
			}
		}
	case ColumnTypeDatetime:
		switch t := v.(type) {
		case time.Time:
			return formatDatetime(t, loc)
		}
	}
	return fmt.Sprint(v)
}

func pdfColumnWeights(columns []Column) []float64 {
	weights := make([]float64, len(columns))
	for i, c := range columns {
		if c.Weight > 0 {
			weights[i] = c.Weight
			continue
		}
		switch c.Type {
		case ColumnTypeUUID:
			weights[i] = 1.05
		case ColumnTypeDatetime:
			weights[i] = 1.15
		case ColumnTypeBoolean, ColumnTypeEnum:
			weights[i] = 0.85
		default:
			switch c.Key {
			case "email":
				weights[i] = 1.35
			case "role_slugs", "permission_slugs", "title":
				weights[i] = 1.2
			case "name", "surname":
				weights[i] = 0.95
			default:
				weights[i] = 1.0
			}
		}
	}
	return weights
}

func pdfColumnWidths(columns []Column, totalWidth float64) []float64 {
	if len(columns) == 0 {
		return nil
	}
	weights := pdfColumnWeights(columns)
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	widths := make([]float64, len(columns))
	for i, w := range weights {
		widths[i] = totalWidth * w / sum
	}
	return widths
}
