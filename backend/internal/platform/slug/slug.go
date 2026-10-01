package slug

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// turkish maps Turkish letters before lowercasing: strings.ToLower turns
// "İ" into "i̇" (i + combining dot) and leaves "ı" outside a-z (TEC-142).
var turkish = strings.NewReplacer(
	"Ç", "c", "ç", "c",
	"Ğ", "g", "ğ", "g",
	"I", "i", "ı", "i",
	"İ", "i", "i̇", "i",
	"Ö", "o", "ö", "o",
	"Ş", "s", "ş", "s",
	"Ü", "u", "ü", "u",
)

// preDecompose maps lowercase letters that NFKD would break (Cyrillic й, ё,
// ї decompose to и/е/і + mark) or does not decompose at all (ß, ø, ə ...).
var preDecompose = strings.NewReplacer(
	// Latin letters without a canonical decomposition (de, da/no, fr, pl, az, is).
	"ß", "ss", "æ", "ae", "ø", "o", "œ", "oe", "đ", "d", "ł", "l",
	"ə", "e", "þ", "th", "ð", "d", "ħ", "h", "ŀ", "l",
	// Cyrillic (ru, uk, bg; basic scientific-style table).
	"а", "a", "б", "b", "в", "v", "г", "g", "ґ", "g", "д", "d",
	"е", "e", "ё", "yo", "є", "ye", "ж", "zh", "з", "z", "и", "i",
	"і", "i", "ї", "yi", "й", "y", "к", "k", "л", "l", "м", "m",
	"н", "n", "о", "o", "п", "p", "р", "r", "с", "s", "т", "t",
	"у", "u", "ф", "f", "х", "h", "ц", "ts", "ч", "ch", "ш", "sh",
	"щ", "shch", "ъ", "", "ы", "y", "ь", "", "э", "e", "ю", "yu",
	"я", "ya",
)

// greek runs after NFKD so tonos/dialytika are already stripped.
var greek = strings.NewReplacer(
	"θ", "th", "χ", "ch", "ψ", "ps",
	"α", "a", "β", "v", "γ", "g", "δ", "d", "ε", "e", "ζ", "z",
	"η", "i", "ι", "i", "κ", "k", "λ", "l", "μ", "m", "ν", "n",
	"ξ", "x", "ο", "o", "π", "p", "ρ", "r", "σ", "s", "ς", "s",
	"τ", "t", "υ", "y", "φ", "f", "ω", "o",
)

// transliterate folds a display name to lowercase ASCII where it can:
// Turkish letters first, then a Cyrillic/special-Latin table, then NFKD with
// combining marks removed (de/fr/es/az accents), then Greek.
func transliterate(s string) string {
	s = strings.ToLower(turkish.Replace(s))
	s = preDecompose.Replace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFKD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return greek.Replace(strings.ToLower(b.String()))
}

// FromName turns a display name into a URL-safe ASCII slug. It is applied
// when a record is created; stored slugs are never recomputed.
func FromName(name string) string {
	name = strings.TrimSpace(transliterate(name))
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
