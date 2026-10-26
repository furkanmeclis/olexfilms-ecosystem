package i18n

import "strings"

// TEC-1009: Arabic text shows Arabic-Indic digits (٠١٢٣٤٥٦٧٨٩). Formatted
// values (dates, numbers, money, counts) are converted with Digits at the
// point they are written for a reader; identifiers keep Latin digits in
// every locale (LatinOnlyKey). Input arriving with Arabic-Indic or Persian
// digits is stored as Latin (LatinDigits).

// Digits writes the ASCII digits of a formatted value in the locale's
// digits: Arabic-Indic for ar, unchanged otherwise.
func Digits(locale Locale, s string) string {
	if locale != LocaleAR || !strings.ContainsAny(s, "0123456789") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune('٠' + (r - '0'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// LatinDigits rewrites Arabic-Indic (٠-٩) and Persian (۰-۹) digits as
// 0-9.
func LatinDigits(s string) string {
	if !strings.ContainsFunc(s, isEasternDigit) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isEasternDigit(r rune) bool {
	return (r >= '٠' && r <= '٩') || (r >= '۰' && r <= '۹')
}

// latinOnlyParts are the name parts of values that keep Latin digits:
// plate, VIN, barcode, service number, phone, codes, ids and links.
var latinOnlyParts = []string{
	"plate", "vin", "barcode", "service_no", "phone", "code", "uuid", "link", "url",
	"email", "serial", "sku", "number", "token", "iban", "tax",
}

// LatinOnlyKey reports whether a value named key (a template placeholder,
// an export column) is an identifier that keeps Latin digits.
func LatinOnlyKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "id" || strings.HasSuffix(k, "_id") || strings.HasSuffix(k, "_no") {
		return true
	}
	for _, p := range latinOnlyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}
