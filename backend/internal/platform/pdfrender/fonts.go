package pdfrender

import (
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// Noto Sans (latin + latin-ext: Turkish, Azerbaijani, Western/Central
// European) and Noto Sans Arabic (arabic subset), weights 400 and 700,
// woff2 from @fontsource 5.3.0 (SIL OFL 1.1, see fonts/OFL.txt). The CSP of
// the document only allows data: fonts, so they are inlined as base64.
// Cyrillic, Greek and CJK fall back to the fonts installed in the Gotenberg
// image (Noto Sans / Noto Sans CJK), which need no network either.
// TEC-1233: Persian documents use Vazirmatn (arabic subset, @fontsource
// 5.2.5, SIL OFL 1.1) with Noto Sans Arabic as the fallback.
//
//go:embed fonts/*.woff2
var fontFS embed.FS

// FontMode selects how document fonts are provided.
type FontMode string

const (
	// FontsEmbedded inlines the Noto subsets as data: URIs (default).
	FontsEmbedded FontMode = "embedded"
	// FontsSystem relies on the fonts installed in the Gotenberg image.
	FontsSystem FontMode = "system"
)

// ParseFontMode maps PDF_FONTS to a mode (unknown → embedded).
func ParseFontMode(raw string) FontMode {
	if strings.EqualFold(strings.TrimSpace(raw), string(FontsSystem)) {
		return FontsSystem
	}
	return FontsEmbedded
}

type fontFace struct {
	family string
	file   string
	weight int
	ranges string
}

const (
	rangeLatin    = "U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD"
	rangeLatinExt = "U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF"
	rangeArabic   = "U+0600-06FF,U+0750-077F,U+0870-088E,U+0890-0891,U+0897-08E1,U+08E3-08FF,U+200C-200E,U+2010-2011,U+204F,U+2E41,U+FB50-FDFF,U+FE70-FE74,U+FE76-FEFC"
)

var latinFaces = []fontFace{
	{"Noto Sans", "noto-sans-latin-400-normal.woff2", 400, rangeLatin},
	{"Noto Sans", "noto-sans-latin-700-normal.woff2", 700, rangeLatin},
	{"Noto Sans", "noto-sans-latin-ext-400-normal.woff2", 400, rangeLatinExt},
	{"Noto Sans", "noto-sans-latin-ext-700-normal.woff2", 700, rangeLatinExt},
}

var arabicFaces = []fontFace{
	{"Noto Sans Arabic", "noto-sans-arabic-arabic-400-normal.woff2", 400, rangeArabic},
	{"Noto Sans Arabic", "noto-sans-arabic-arabic-700-normal.woff2", 700, rangeArabic},
}

var persianFaces = []fontFace{
	{"Vazirmatn", "vazirmatn-arabic-400-normal.woff2", 400, rangeArabic},
	{"Vazirmatn", "vazirmatn-arabic-700-normal.woff2", 700, rangeArabic},
}

var (
	fontOnce       sync.Once
	latinFontCSS   string
	arabicFontCSS  string
	persianFontCSS string
)

func loadFontCSS() {
	fontOnce.Do(func() {
		latinFontCSS = facesCSS(latinFaces)
		arabicFontCSS = facesCSS(arabicFaces)
		persianFontCSS = facesCSS(persianFaces)
	})
}

func facesCSS(faces []fontFace) string {
	var b strings.Builder
	for _, f := range faces {
		data, err := fontFS.ReadFile("fonts/" + f.file)
		if err != nil {
			panic(fmt.Sprintf("pdfrender: embedded font %s: %v", f.file, err))
		}
		fmt.Fprintf(&b,
			"@font-face{font-family:%q;font-style:normal;font-weight:%d;font-display:block;"+
				"src:url(data:font/woff2;base64,%s) format(\"woff2\");unicode-range:%s}\n",
			f.family, f.weight, base64.StdEncoding.EncodeToString(data), f.ranges)
	}
	return b.String()
}

// fontCSS returns the @font-face rules for a document. Arabic faces are only
// inlined when the document needs them (RTL language or Arabic text), which
// keeps the common Latin request ~190 KB smaller; Vazirmatn only for a
// Persian document (lang fa).
func fontCSS(mode FontMode, lang string, body string) string {
	if mode == FontsSystem {
		return ""
	}
	loadFontCSS()
	if isPersian(lang) {
		return latinFontCSS + persianFontCSS + arabicFontCSS
	}
	if IsRTL(lang) || containsArabic(body) {
		return latinFontCSS + arabicFontCSS
	}
	return latinFontCSS
}

// isPersian reports whether a document language is Persian (fa, fa-IR).
func isPersian(lang string) bool {
	l := strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	return l == "fa"
}

func containsArabic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}
