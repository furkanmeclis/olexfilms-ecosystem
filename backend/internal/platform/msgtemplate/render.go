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
