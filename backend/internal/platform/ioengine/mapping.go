package ioengine

import (
	"strings"
	"unicode"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// SuggestMapping fuzzy-matches source headers to import fields (header → field key).
func SuggestMapping(headers []string, fields []ImportField, locale i18n.Locale) map[string]string {
	out := make(map[string]string, len(headers))
	used := map[string]struct{}{}
	for _, h := range headers {
		hNorm := normalizeKey(h)
		if hNorm == "" {
			continue
		}
		bestKey := ""
		bestScore := 0
		for _, f := range fields {
			if _, taken := used[f.Key]; taken {
				continue
			}
			score := scoreMatch(hNorm, h, f.Key, f.LabelKey, locale)
			if score > bestScore {
				bestScore = score
				bestKey = f.Key
			}
		}
		if bestScore >= 60 && bestKey != "" {
			out[h] = bestKey
			used[bestKey] = struct{}{}
		}
	}
	return out
}

func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func scoreMatch(headerNorm, headerRaw, fieldKey, labelKey string, locale i18n.Locale) int {
	keyNorm := normalizeKey(fieldKey)
	if headerNorm == keyNorm {
		return 100
	}
	if strings.Contains(headerNorm, keyNorm) || strings.Contains(keyNorm, headerNorm) {
		return 80
	}
	label := i18n.Translate(locale, labelKey)
	labelNorm := normalizeKey(label)
	if labelNorm != "" {
		if headerNorm == labelNorm {
			return 95
		}
		if strings.Contains(headerNorm, labelNorm) || strings.Contains(labelNorm, headerNorm) {
			return 85
		}
	}
	if headerRaw != "" && strings.EqualFold(strings.TrimSpace(headerRaw), strings.TrimSpace(label)) {
		return 90
	}
	aliases := map[string][]string{
		"email":            {"eposta", "e-posta", "mail", "e-mail", "email address"},
		"name":             {"ad", "firstname", "first", "first name", "given name"},
		"surname":          {"soyad", "lastname", "last", "last name", "family name"},
		"status":           {"durum", "state"},
		"locale":           {"dil", "language", "lang"},
		"slug":             {"kod", "code", "identifier"},
		"description":      {"aciklama", "açıklama", "desc"},
		"role_slugs":       {"roles", "roller", "role", "rol"},
		"permission_slugs": {"permissions", "izinler", "permission", "izin"},
	}
	for _, a := range aliases[fieldKey] {
		aNorm := normalizeKey(a)
		if headerNorm == aNorm {
			return 75
		}
	}
	return 0
}

// ApplyMapping transforms raw rows using header→field mapping and defaults.
func ApplyMapping(
	rows []map[string]string,
	mapping map[string]string,
	defaults map[string]string,
) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, raw := range rows {
		row := map[string]any{}
		for src, dst := range mapping {
			if v, ok := raw[src]; ok {
				row[dst] = strings.TrimSpace(v)
			}
		}
		for k, v := range defaults {
			if _, set := row[k]; !set || row[k] == "" {
				row[k] = v
			}
		}
		out = append(out, row)
	}
	return out
}
