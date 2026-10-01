package msgtemplate

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"

// RoleGeneric is the template role written once for every recipient role.
const RoleGeneric = "generic"

// LocaleChain is the template language fallback chain (K10):
//
//	user -> organization -> brand center -> en -> tr
//
// Each stored value goes through i18n.Parse ("tr-TR" -> tr); unsupported or
// empty values are skipped and duplicates removed. The first element equals
// i18n.Resolve's locale whenever the user, organization or center sets one.
func LocaleChain(userLocale, orgLocale, centerLocale string) []i18n.Locale {
	out := make([]i18n.Locale, 0, 5)
	seen := map[i18n.Locale]bool{}
	add := func(l i18n.Locale) {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	for _, raw := range []string{userLocale, orgLocale, centerLocale} {
		if l, ok := i18n.Parse(raw); ok {
			add(l)
		}
	}
	add(i18n.FallbackLocale)
	add(i18n.DefaultLocale)
	return out
}

// Variant is a stored template variant (one notification_templates row).
type Variant struct {
	Role     string
	Language string
	// Branded is true for a brand specific override.
	Branded bool
}

// Pick returns the index of the best variant for role along chain: for each
// language in order, the role's own template before the generic one, a brand
// override before the global row. ok is false when nothing matches.
func Pick(variants []Variant, role string, chain []i18n.Locale) (int, bool) {
	roles := []string{role}
	if role != RoleGeneric {
		roles = append(roles, RoleGeneric)
	}
	for _, lang := range chain {
		for _, r := range roles {
			best := -1
			for i, v := range variants {
				if v.Role != r || v.Language != string(lang) {
					continue
				}
				if best < 0 || (v.Branded && !variants[best].Branded) {
					best = i
				}
			}
			if best >= 0 {
				return best, true
			}
		}
	}
	return -1, false
}
