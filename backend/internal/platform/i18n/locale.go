// Package i18n holds the supported locales (K10), locale/timezone parsing and
// resolution, and the backend label catalogs used by exports, imports and
// notifications.
package i18n

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Locale is a canonical BCP-47 code: a lower-case language subtag, plus an
// upper-case region only for zh-CN.
type Locale string

// Supported locales (K10: 12 languages + Arabic).
const (
	LocaleTR   Locale = "tr"
	LocaleEN   Locale = "en"
	LocaleBG   Locale = "bg"
	LocaleDE   Locale = "de"
	LocaleEL   Locale = "el"
	LocaleUK   Locale = "uk"
	LocaleRU   Locale = "ru"
	LocaleFR   Locale = "fr"
	LocaleES   Locale = "es"
	LocaleIT   Locale = "it"
	LocaleZhCN Locale = "zh-CN"
	LocaleAZ   Locale = "az"
	LocaleAR   Locale = "ar"
)

// Supported lists the canonical locales in display order. The users and
// organizations CHECK constraints (migration 000033) hold the same list.
var Supported = []Locale{
	LocaleTR, LocaleEN, LocaleBG, LocaleDE, LocaleEL, LocaleUK, LocaleRU,
	LocaleFR, LocaleES, LocaleIT, LocaleZhCN, LocaleAZ, LocaleAR,
}

// DefaultLocale ends the resolution chain and is what Normalize returns for
// an empty or unknown value.
const DefaultLocale = LocaleTR

// FallbackLocale is used per key when a catalog lacks a translation.
const FallbackLocale = LocaleEN

// DefaultTimezone ends the timezone resolution chain.
const DefaultTimezone = "Europe/Istanbul"

var (
	// ErrInvalidLocale is returned for an API value that is not a supported locale.
	ErrInvalidLocale = errors.New("unsupported locale")
	// ErrInvalidTimezone is returned for a value that is not an IANA timezone.
	ErrInvalidTimezone = errors.New("invalid timezone")
)

// byLower maps the lower-cased canonical code to the locale.
var byLower = func() map[string]Locale {
	out := make(map[string]Locale, len(Supported))
	for _, l := range Supported {
		out[strings.ToLower(string(l))] = l
	}
	return out
}()

// Parse maps a raw code to a supported locale. It trims, accepts "_" for "-"
// and any case; an exact match wins ("zh_CN" -> zh-CN), otherwise the primary
// language subtag is used ("tr-TR" -> tr, "ar-AE" -> ar, "zh" -> zh-CN).
// ok is false for an empty or unsupported value.
func Parse(raw string) (Locale, bool) {
	s := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "_", "-"))
	if s == "" {
		return "", false
	}
	if l, ok := byLower[s]; ok {
		return l, true
	}
	base := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		base = s[:i]
	}
	if base == "zh" {
		return LocaleZhCN, true
	}
	if l, ok := byLower[base]; ok {
		return l, true
	}
	return "", false
}

// Normalize returns the supported locale for raw, or DefaultLocale (tr) when
// raw is empty or unknown. Use Parse where an unknown value must be rejected.
func Normalize(raw string) Locale {
	if l, ok := Parse(raw); ok {
		return l
	}
	return DefaultLocale
}

// IsSupported reports whether code is exactly a canonical locale.
func IsSupported(code string) bool {
	l, ok := byLower[strings.ToLower(code)]
	return ok && string(l) == code
}

// SupportedList returns the canonical codes as "tr, en, ..." for messages.
func SupportedList() string {
	parts := make([]string, len(Supported))
	for i, l := range Supported {
		parts[i] = string(l)
	}
	return strings.Join(parts, ", ")
}

// Dir returns the text direction of the locale: "rtl" for Arabic, else "ltr".
func Dir(l Locale) string {
	if l == LocaleAR {
		return "rtl"
	}
	return "ltr"
}

// ParseAcceptLanguage returns the highest-weighted supported locale of an
// Accept-Language header ("de-DE,de;q=0.9,en;q=0.8"). Entries with q=0 and
// "*" are ignored; equal weights keep header order.
func ParseAcceptLanguage(header string) (Locale, bool) {
	type entry struct {
		tag string
		q   float64
	}
	var entries []entry
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		tag := strings.TrimSpace(fields[0])
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			f = strings.TrimSpace(f)
			if v, ok := strings.CutPrefix(f, "q="); ok {
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					q = parsed
				}
			}
		}
		if q <= 0 {
			continue
		}
		entries = append(entries, entry{tag: tag, q: q})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].q > entries[j].q })
	for _, e := range entries {
		if l, ok := Parse(e.tag); ok {
			return l, true
		}
	}
	return "", false
}

// ValidTimezone reports whether tz is a loadable IANA timezone name. Empty and
// "Local" (the server's zone) are rejected.
func ValidTimezone(tz string) bool {
	if tz == "" || tz == "Local" || strings.TrimSpace(tz) != tz {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}
