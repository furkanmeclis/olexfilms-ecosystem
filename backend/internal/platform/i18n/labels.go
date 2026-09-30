package i18n

import "strings"

// ResourceLabel returns a localized display name for an IO resource slug.
func ResourceLabel(locale Locale, resource string) string {
	key := "resources." + strings.TrimSpace(resource)
	if label := Translate(locale, key); label != key {
		return label
	}
	return resource
}

// ExportFormatLabel returns a localized export format name for notifications.
func ExportFormatLabel(locale Locale, format string) string {
	key := "export.format." + strings.ToLower(strings.TrimSpace(format))
	if label := Translate(locale, key); label != key {
		return label
	}
	return strings.ToUpper(format)
}
