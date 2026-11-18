package mail

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DefaultBrandName is the brand shown in the e-mail frame when the brand
// of a message is unknown (no brand id or the lookup failed).
const DefaultBrandName = "Olex Films"

// BrandName returns name, or DefaultBrandName when it is blank.
func BrandName(name string) string {
	if strings.TrimSpace(name) == "" {
		return DefaultBrandName
	}
	return name
}

// DefaultColor is the header and button color when the brand has none (or
// an unsafe value).
const DefaultColor = "#111827"

// Layout is the shared branded e-mail frame: header (logo or brand name),
// title, hidden preheader, body, optional primary button, optional muted
// secondary block and the footer. Table based with inline CSS, so Gmail,
// Outlook (VML button) and Apple Mail render it alike; a small <style>
// carries only the dark mode and mobile width overrides.
//
// BodyHTML must already be safe HTML (msgtemplate.MarkdownToHTML); every
// other field is plain text and escaped here. The zero value of each
// optional field leaves its part out.
type Layout struct {
	Lang      string
	Dir       string // "ltr" | "rtl" (i18n.Dir)
	Title     string
	BrandName string
	LogoURL   string
	Color     string // CSS color of the header bar, links and the button
	BodyHTML  string
	// BodyText is the plain body (msgtemplate.MarkdownToText) used by
	// RenderText and the default preheader. Empty: derived from BodyHTML.
	BodyText string
	// Footer replaces the brand name line of the footer (legacy).
	Footer string

	// Preheader is the inbox preview text (hidden in the body). Empty:
	// the first ~110 characters of the plain body.
	Preheader string
	// ButtonLabel + ButtonURL (absolute http/https) draw the primary
	// button; either empty (or an unsafe URL) leaves it out.
	ButtonLabel string
	ButtonURL   string
	// Secondary is muted small text under the button (for example "If the
	// button does not work, open this link:" or a code expiry).
	// SecondaryURL (http/https) is printed after it as a visible link.
	Secondary    string
	SecondaryURL string
	// Reason is the footer line telling why the recipient got the e-mail.
	Reason string
	// UnsubscribeURL (http/https) adds "UnsubscribeText UnsubscribeLabel"
	// to the footer, the label linked.
	UnsubscribeText  string
	UnsubscribeLabel string
	UnsubscribeURL   string
}

// PreheaderMax is the length of a derived preheader in characters.
const PreheaderMax = 110

var colorRe = regexp.MustCompile(`^(#[0-9A-Fa-f]{3}|#[0-9A-Fa-f]{6}|[A-Za-z]{3,20}|rgba?\([0-9., %]+\))$`)

// safeColor keeps a brand color only when it is a plain CSS color literal.
func safeColor(c string) string {
	c = strings.TrimSpace(c)
	if c == "" || strings.ContainsAny(c, ";\"'<>{}\\") || !colorRe.MatchString(c) {
		return DefaultColor
	}
	return c
}

// onColor picks a legible text color (white or near black) for a
// background; non hex colors keep white.
func onColor(bg string) string {
	if !strings.HasPrefix(bg, "#") {
		return "#ffffff"
	}
	h := bg[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return "#ffffff"
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return "#ffffff"
	}
	lin := func(c uint64) float64 {
		x := float64(c) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		// gamma 2.4 approximated by x^2.2 is enough to pick a side
		return x * x * (0.5 + 0.5*x)
	}
	l := 0.2126*lin(v>>16&0xff) + 0.7152*lin(v>>8&0xff) + 0.0722*lin(v&0xff)
	if l > 0.45 {
		return "#111827"
	}
	return "#ffffff"
}

// SafeURL reports whether u is an absolute http(s) URL fit for an href.
func SafeURL(u string) bool {
	u = strings.TrimSpace(u)
	lower := strings.ToLower(u)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return false
	}
	return !strings.ContainsAny(u, " \t\r\n\"'<>`") && len(u) > len("https://")
}

var (
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	blockRe = regexp.MustCompile(`(?i)</(p|h[1-6]|li|ul|ol|div)>|<br\s*/?>`)
	spaceRe = regexp.MustCompile(`[ \t]+`)
	nlRe    = regexp.MustCompile(`\n{3,}`)
)

// htmlToText is a small fallback for callers that set no BodyText.
func htmlToText(s string) string {
	s = blockRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(nlRe.ReplaceAllString(s, "\n\n"))
}

func (l Layout) bodyText() string {
	if t := strings.TrimSpace(l.BodyText); t != "" {
		return t
	}
	return htmlToText(l.BodyHTML)
}

// PreheaderText is the preview text: Preheader, else the plain body
// collapsed to one line and cut at PreheaderMax characters on a word.
func (l Layout) PreheaderText() string {
	if p := strings.TrimSpace(l.Preheader); p != "" {
		return strings.Join(strings.Fields(p), " ")
	}
	return Excerpt(l.bodyText(), PreheaderMax)
}

// Excerpt collapses whitespace and cuts s at max characters, on a word
// boundary when one is close, adding an ellipsis.
func Excerpt(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)[:max]
	cut := string(r)
	if i := strings.LastIndexByte(cut, ' '); i > len(cut)*2/3 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-") + "…"
}

func (l Layout) normalized() Layout {
	if l.Dir != "rtl" {
		l.Dir = "ltr"
	}
	if strings.TrimSpace(l.Lang) == "" {
		l.Lang = "tr"
	}
	l.Color = safeColor(l.Color)
	l.BrandName = strings.TrimSpace(l.BrandName)
	if l.Footer == "" {
		l.Footer = l.BrandName
	}
	if !SafeURL(l.ButtonURL) || strings.TrimSpace(l.ButtonLabel) == "" {
		l.ButtonURL, l.ButtonLabel = "", ""
	}
	if !SafeURL(l.SecondaryURL) {
		l.SecondaryURL = ""
	}
	if !SafeURL(l.UnsubscribeURL) || strings.TrimSpace(l.UnsubscribeLabel) == "" {
		l.UnsubscribeURL = ""
	}
	if !SafeURL(l.LogoURL) {
		l.LogoURL = ""
	}
	return l
}

const (
	fontStack = `Arial,Helvetica,sans-serif`
	colBg     = "#f4f4f5"
	colCard   = "#ffffff"
	colText   = "#18181b"
	colMuted  = "#52525b"
	colBorder = "#e4e4e7"
)

// headStyle holds only what inline CSS cannot do: dark mode and the
// mobile width. Clients without <style> support get the light layout.
const headStyle = `<style>
:root{color-scheme:light dark;supported-color-schemes:light dark;}
body{margin:0;padding:0;-webkit-text-size-adjust:100%;-ms-text-size-adjust:100%;}
table{border-collapse:collapse;}
img{border:0;outline:none;text-decoration:none;}
.em-body a{word-break:break-word;}
@media only screen and (max-width:620px){
.em-container{width:100%!important;max-width:100%!important;}
.em-pad{padding-left:20px!important;padding-right:20px!important;}
.em-btn a{display:block!important;}
}
@media (prefers-color-scheme:dark){
.em-bg{background-color:#111113!important;}
.em-card{background-color:#1f1f23!important;}
.em-text,.em-text p,.em-text li,.em-text h1,.em-text h2,.em-text h3{color:#f4f4f5!important;}
.em-muted,.em-muted a{color:#a1a1aa!important;}
.em-body a{color:#93c5fd!important;}
.em-border{border-color:#3f3f46!important;}
.em-pre{color:#111113!important;}
}
[data-ogsc] .em-bg{background-color:#111113!important;}
[data-ogsc] .em-card{background-color:#1f1f23!important;}
[data-ogsc] .em-text,[data-ogsc] .em-text p,[data-ogsc] .em-text li,[data-ogsc] .em-text h1{color:#f4f4f5!important;}
[data-ogsc] .em-muted{color:#a1a1aa!important;}
</style>`

const msoHead = `<!--[if mso]><noscript><xml><o:OfficeDocumentSettings><o:AllowPNG/><o:PixelsPerInch>96</o:PixelsPerInch></o:OfficeDocumentSettings></xml></noscript><style>table,td,div,h1,p,a{font-family:Arial,Helvetica,sans-serif!important;}</style><![endif]-->`

// styleBody inlines spacing on the bare tags msgtemplate.MarkdownToHTML
// writes (clients drop <style> rules for them); other markup is untouched.
func styleBody(body, color string) string {
	link := color
	if onColor(color) != "#ffffff" {
		link = "#1d4ed8" // a light brand color is not legible as link text
	}
	return strings.NewReplacer(
		"<p>", `<p style="margin:0 0 16px;">`,
		"<ul>", `<ul style="margin:0 0 16px;padding:0 24px;">`,
		"<li>", `<li style="margin:0 0 6px;">`,
		"<h2>", `<h2 class="em-text" style="margin:24px 0 12px;font-size:18px;line-height:1.35;color:`+colText+`;">`,
		"<h3>", `<h3 class="em-text" style="margin:20px 0 8px;font-size:16px;line-height:1.4;color:`+colText+`;">`,
		`<a href=`, `<a style="color:`+link+`;text-decoration:underline;" href=`,
	).Replace(body)
}

// RenderHTML renders the layout.
func (l Layout) RenderHTML() (string, error) {
	l = l.normalized()
	esc := html.EscapeString
	align, start := "left", "left"
	if l.Dir == "rtl" {
		align, start = "right", "right"
	}
	var b strings.Builder
	w := func(s string) { b.WriteString(s) }

	w(`<!doctype html>` + "\n")
	w(`<html lang="` + esc(l.Lang) + `" dir="` + l.Dir + `" xmlns="http://www.w3.org/1999/xhtml" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office">` + "\n")
	w(`<head>` + "\n")
	w(`<meta charset="utf-8">` + "\n")
	w(`<meta name="viewport" content="width=device-width,initial-scale=1">` + "\n")
	w(`<meta http-equiv="X-UA-Compatible" content="IE=edge">` + "\n")
	w(`<meta name="x-apple-disable-message-reformatting">` + "\n")
	w(`<meta name="format-detection" content="telephone=no,address=no,email=no,date=no,url=no">` + "\n")
	w(`<meta name="color-scheme" content="light dark">` + "\n")
	w(`<meta name="supported-color-schemes" content="light dark">` + "\n")
	w(`<title>` + esc(l.Title) + `</title>` + "\n")
	w(msoHead + "\n")
	w(headStyle + "\n")
	w(`</head>` + "\n")
	w(`<body class="em-bg" style="margin:0;padding:0;width:100%;background-color:` + colBg + `;">` + "\n")

	// Hidden preview text; the spacer keeps body text out of the preview.
	if pre := l.PreheaderText(); pre != "" {
		w(`<div class="em-pre" style="display:none;font-size:1px;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;mso-hide:all;color:` + colBg + `;">` + esc(pre))
		w(strings.Repeat("&#847;&zwnj;&nbsp;", 40))
		w(`</div>` + "\n")
	}

	w(`<table role="presentation" class="em-bg" width="100%" cellpadding="0" cellspacing="0" border="0" dir="` + l.Dir + `" style="width:100%;background-color:` + colBg + `;">` + "\n")
	w(`<tr><td align="center" style="padding:24px 8px;">` + "\n")
	w(`<!--[if mso]><table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" align="center"><tr><td><![endif]-->` + "\n")
	w(`<table role="presentation" class="em-container em-card" width="600" cellpadding="0" cellspacing="0" border="0" align="center" dir="` + l.Dir + `" style="width:600px;max-width:600px;background-color:` + colCard + `;border-radius:8px;">` + "\n")

	// Header: logo or brand name on the brand color.
	fg := onColor(l.Color)
	w(`<tr><td class="em-pad" align="` + start + `" dir="` + l.Dir + `" style="background-color:` + l.Color + `;padding:18px 32px;border-radius:8px 8px 0 0;text-align:` + start + `;">`)
	if l.LogoURL != "" {
		w(`<img src="` + esc(l.LogoURL) + `" alt="` + esc(l.BrandName) + `" height="32" style="display:block;height:32px;width:auto;border:0;color:` + fg + `;font-family:` + fontStack + `;font-size:18px;font-weight:bold;">`)
	} else {
		w(`<span style="color:` + fg + `;font-family:` + fontStack + `;font-size:18px;line-height:24px;font-weight:bold;">` + esc(l.BrandName) + `</span>`)
	}
	w(`</td></tr>` + "\n")

	// Title + body.
	w(`<tr><td class="em-pad em-text em-body" align="` + align + `" dir="` + l.Dir + `" style="padding:28px 32px 8px;font-family:` + fontStack + `;font-size:16px;line-height:1.6;color:` + colText + `;text-align:` + align + `;">` + "\n")
	if strings.TrimSpace(l.Title) != "" {
		w(`<h1 class="em-text" style="margin:0 0 16px;font-family:` + fontStack + `;font-size:22px;line-height:1.3;font-weight:bold;color:` + colText + `;">` + esc(l.Title) + `</h1>` + "\n")
	}
	w(styleBody(l.BodyHTML, l.Color) + "\n") // safe HTML from msgtemplate.MarkdownToHTML
	w(`</td></tr>` + "\n")

	// Primary button: VML for Outlook, a padded link everywhere else.
	if l.ButtonURL != "" {
		u, label := esc(l.ButtonURL), esc(l.ButtonLabel)
		width := utf8.RuneCountInString(l.ButtonLabel)*9 + 56
		if width < 200 {
			width = 200
		}
		w(`<tr><td class="em-pad em-btn" align="` + align + `" dir="` + l.Dir + `" style="padding:16px 32px 8px;text-align:` + align + `;">` + "\n")
		w(`<!--[if mso]><v:roundrect xmlns:v="urn:schemas-microsoft-com:vml" xmlns:w="urn:schemas-microsoft-com:office:word" href="` + u + `" style="height:46px;v-text-anchor:middle;width:` + strconv.Itoa(width) + `px;" arcsize="14%" stroke="f" fillcolor="` + l.Color + `"><w:anchorlock/><center style="color:` + fg + `;font-family:Arial,Helvetica,sans-serif;font-size:16px;font-weight:bold;">` + label + `</center></v:roundrect><![endif]-->` + "\n")
		w(`<!--[if !mso]><!-- --><a href="` + u + `" target="_blank" rel="noopener" style="display:inline-block;background-color:` + l.Color + `;color:` + fg + `;font-family:` + fontStack + `;font-size:16px;font-weight:bold;line-height:46px;min-height:46px;min-width:160px;padding:0 28px;border-radius:6px;text-align:center;text-decoration:none;-webkit-text-size-adjust:none;mso-hide:all;">` + label + `</a><!--<![endif]-->` + "\n")
		w(`</td></tr>` + "\n")
	}

	// Secondary info: muted small text (fallback link, code expiry).
	if strings.TrimSpace(l.Secondary) != "" || l.SecondaryURL != "" {
		w(`<tr><td class="em-pad em-muted" align="` + align + `" dir="` + l.Dir + `" style="padding:12px 32px 4px;font-family:` + fontStack + `;font-size:13px;line-height:1.5;color:` + colMuted + `;text-align:` + align + `;">`)
		if s := strings.TrimSpace(l.Secondary); s != "" {
			w(esc(s))
		}
		if l.SecondaryURL != "" {
			if strings.TrimSpace(l.Secondary) != "" {
				w(`<br>`)
			}
			u := esc(l.SecondaryURL)
			w(`<a href="` + u + `" target="_blank" rel="noopener" dir="ltr" style="color:` + colMuted + `;text-decoration:underline;word-break:break-all;">` + u + `</a>`)
		}
		w(`</td></tr>` + "\n")
	}

	w(`<tr><td style="height:24px;line-height:24px;font-size:0;">&nbsp;</td></tr>` + "\n")

	// Footer.
	w(`<tr><td class="em-pad em-muted em-border" align="` + align + `" dir="` + l.Dir + `" style="padding:16px 32px 24px;border-top:1px solid ` + colBorder + `;font-family:` + fontStack + `;font-size:12px;line-height:1.5;color:` + colMuted + `;text-align:` + align + `;">`)
	parts := []string{}
	if f := strings.TrimSpace(l.Footer); f != "" {
		parts = append(parts, `<strong style="color:`+colMuted+`;">`+esc(f)+`</strong>`)
	}
	if r := strings.TrimSpace(l.Reason); r != "" {
		parts = append(parts, esc(r))
	}
	if l.UnsubscribeURL != "" {
		line := ""
		if t := strings.TrimSpace(l.UnsubscribeText); t != "" {
			line = esc(t) + " "
		}
		line += `<a href="` + esc(l.UnsubscribeURL) + `" target="_blank" rel="noopener" style="color:` + colMuted + `;text-decoration:underline;">` + esc(l.UnsubscribeLabel) + `</a>`
		parts = append(parts, line)
	}
	w(strings.Join(parts, "<br>"))
	w(`</td></tr>` + "\n")

	w(`</table>` + "\n")
	w(`<!--[if mso]></td></tr></table><![endif]-->` + "\n")
	w(`</td></tr>` + "\n")
	w(`</table>` + "\n")
	w(`</body>` + "\n")
	w(`</html>` + "\n")
	return b.String(), nil
}

// RenderText renders the plain-text alternative: title, body, the button
// as "label: url", the secondary info and the footer.
func (l Layout) RenderText() string {
	l = l.normalized()
	var blocks []string
	if t := strings.TrimSpace(l.Title); t != "" {
		blocks = append(blocks, t)
	}
	if body := l.bodyText(); body != "" {
		blocks = append(blocks, body)
	}
	if l.ButtonURL != "" {
		blocks = append(blocks, strings.TrimSpace(l.ButtonLabel)+": "+l.ButtonURL)
	}
	// The fallback link repeats the button URL; the text part has it.
	if s := strings.TrimSpace(l.Secondary); s != "" && (l.SecondaryURL == "" || l.SecondaryURL != l.ButtonURL) {
		if l.SecondaryURL != "" {
			s += " " + l.SecondaryURL
		}
		blocks = append(blocks, s)
	} else if s == "" && l.SecondaryURL != "" && l.SecondaryURL != l.ButtonURL {
		blocks = append(blocks, l.SecondaryURL)
	}
	var foot []string
	if f := strings.TrimSpace(l.Footer); f != "" {
		foot = append(foot, f)
	}
	if r := strings.TrimSpace(l.Reason); r != "" {
		foot = append(foot, r)
	}
	if l.UnsubscribeURL != "" {
		line := strings.TrimSpace(l.UnsubscribeText)
		if line != "" {
			line += " "
		}
		foot = append(foot, line+strings.TrimSpace(l.UnsubscribeLabel)+": "+l.UnsubscribeURL)
	}
	if len(foot) > 0 {
		blocks = append(blocks, "--\n"+strings.Join(foot, "\n"))
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
