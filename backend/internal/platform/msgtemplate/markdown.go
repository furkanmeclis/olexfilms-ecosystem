package msgtemplate

import (
	"html"
	"regexp"
	"strings"
)

// Markdown subset used by notification templates (e-mail): paragraphs, line
// breaks, "# " headings, "- " lists, **bold**, *italic*, `code` and
// [text](https://...) links. Everything is HTML-escaped first, so rendered
// placeholder values can never inject markup; links accept http(s) and
// mailto only.

var (
	mdBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdItalic = regexp.MustCompile(`\*([^*]+)\*`)
	mdCode   = regexp.MustCompile("`([^`]+)`")
	mdLink   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

// MarkdownToHTML converts the template markdown subset to safe HTML.
func MarkdownToHTML(md string) string {
	md = strings.ReplaceAll(strings.ReplaceAll(md, "\r\n", "\n"), "\r", "\n")
	var b strings.Builder
	for _, block := range strings.Split(md, "\n\n") {
		block = strings.Trim(block, "\n")
		if strings.TrimSpace(block) == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		switch {
		case isList(lines):
			b.WriteString("<ul>")
			for _, l := range lines {
				b.WriteString("<li>")
				b.WriteString(inline(strings.TrimSpace(strings.TrimSpace(l)[2:])))
				b.WriteString("</li>")
			}
			b.WriteString("</ul>")
		case len(lines) == 1 && strings.HasPrefix(lines[0], "# "):
			b.WriteString("<h2>" + inline(strings.TrimSpace(lines[0][2:])) + "</h2>")
		case len(lines) == 1 && strings.HasPrefix(lines[0], "## "):
			b.WriteString("<h3>" + inline(strings.TrimSpace(lines[0][3:])) + "</h3>")
		default:
			parts := make([]string, len(lines))
			for i, l := range lines {
				parts[i] = inline(l)
			}
			b.WriteString("<p>" + strings.Join(parts, "<br>") + "</p>")
		}
	}
	return b.String()
}

// MarkdownToText strips the markdown subset for plain-text channels.
func MarkdownToText(md string) string {
	out := mdLink.ReplaceAllString(md, "$1 ($2)")
	out = mdBold.ReplaceAllString(out, "$1")
	out = mdItalic.ReplaceAllString(out, "$1")
	out = mdCode.ReplaceAllString(out, "$1")
	return strings.TrimSpace(out)
}

func isList(lines []string) bool {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "- ") && !strings.HasPrefix(t, "* ") {
			return false
		}
	}
	return len(lines) > 0
}

func inline(s string) string {
	s = html.EscapeString(s)
	s = mdLink.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdLink.FindStringSubmatch(m)
		href := html.UnescapeString(sub[2])
		lower := strings.ToLower(href)
		if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "mailto:") {
			return sub[1]
		}
		return `<a href="` + html.EscapeString(href) + `">` + sub[1] + `</a>`
	})
	s = mdBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = mdItalic.ReplaceAllString(s, "<em>$1</em>")
	s = mdCode.ReplaceAllString(s, "<code>$1</code>")
	return s
}
