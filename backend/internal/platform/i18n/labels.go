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

// TranslateParams resolves key like Translate and fills its {{name}}
// placeholders from params in one pass (a value is never re-expanded).
// Unknown placeholders are left as they are.
func TranslateParams(locale Locale, key string, params map[string]string) string {
	tpl := Translate(locale, key)
	if len(params) == 0 {
		return tpl
	}
	pairs := make([]string, 0, len(params)*2)
	for k, v := range params {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...).Replace(tpl)
}
