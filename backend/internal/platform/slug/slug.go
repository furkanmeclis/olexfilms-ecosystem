package slug

import (
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// FromName turns a display name into a URL-safe slug (ASCII-oriented).
func FromName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "item"
	}
	slug := nonSlug.ReplaceAllString(name, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "item"
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}
