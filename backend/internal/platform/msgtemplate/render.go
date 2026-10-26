// Package msgtemplate is the placeholder engine and template-type registry
// shared by message templates (Ayarlar > Mesajlar) and the notification center.
//
// Rendering is plain-text substitution only: `{{key}}` (whitespace and a
// leading dot are tolerated: `{{ key }}`, `{{.key}}`). There is no expression
// evaluation, no loops and no nested expansion — a value containing `{{x}}` is
// inserted literally. Placeholders without a value render as the empty string;
// unknown placeholders are rejected when a template is saved (Validate), so a
// typo cannot reach a customer.
package msgtemplate

import (
	"regexp"
	"sort"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

var placeholderRE = regexp.MustCompile(`\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}`)

// Render substitutes placeholders in one pass. Missing values render empty.
func Render(tpl string, vars map[string]string) string {
	if tpl == "" || !strings.Contains(tpl, "{{") {
		return tpl
	}
	return placeholderRE.ReplaceAllStringFunc(tpl, func(m string) string {
		sub := placeholderRE.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return vars[strings.ToLower(sub[1])]
	})
}

// RenderLocale is Render for a message in language lang (TEC-1009): in
// Arabic the formatted values (dates, amounts, counts) are written with
// Arabic-Indic digits; identifiers (plate, VIN, codes, links, numbers, ids;
// i18n.LatinOnlyKey) and the template text itself are left as they are.
func RenderLocale(tpl string, vars map[string]string, lang string) string {
	loc, ok := i18n.Parse(lang)
	if !ok || loc != i18n.LocaleAR || len(vars) == 0 {
		return Render(tpl, vars)
	}
	local := make(map[string]string, len(vars))
	for k, v := range vars {
		if i18n.LatinOnlyKey(k) {
			local[k] = v
			continue
		}
		local[k] = i18n.Digits(loc, v)
	}
	return Render(tpl, local)
}

// Placeholders returns the distinct placeholder keys used in tpl (sorted).
func Placeholders(tpl string) []string {
	seen := map[string]struct{}{}
	for _, m := range placeholderRE.FindAllStringSubmatch(tpl, -1) {
		seen[strings.ToLower(m[1])] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Unknown returns placeholder keys in the texts that the type does not allow.
func Unknown(spec TypeSpec, texts ...string) []string {
	allowed := map[string]struct{}{}
	for _, p := range spec.Placeholders {
		allowed[p.Key] = struct{}{}
	}
	var out []string
	seen := map[string]struct{}{}
	for _, t := range texts {
		for _, k := range Placeholders(t) {
			if _, ok := allowed[k]; ok {
				continue
			}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
