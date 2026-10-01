package pdfrender

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Elements removed together with their content.
var dropSubtree = map[string]bool{
	"script": true, "noscript": true, "iframe": true, "frame": true, "frameset": true,
	"object": true, "embed": true, "applet": true, "template": true, "title": true,
	"textarea": true, "select": true, "audio": true, "video": true, "canvas": true,
	"portal": true, "foreignobject": true, "plaintext": true, "xmp": true, "noembed": true,
	"noframes": true,
}

// Tags removed while their children are kept (document wrappers, metadata
// and form controls). The renderer supplies its own html/head/body.
var dropTag = map[string]bool{
	"html": true, "head": true, "body": true, "meta": true, "base": true, "link": true,
	"form": true, "input": true, "button": true, "option": true, "source": true, "track": true,
	"param": true,
}

// URL attributes; only data:image/* (src) or safe link schemes (href) survive.
var urlAttrs = map[string]bool{
	"src": true, "href": true, "xlink:href": true, "srcset": true, "poster": true,
	"background": true, "action": true, "formaction": true, "ping": true, "srcdoc": true,
	"data": true, "codebase": true, "cite": true, "longdesc": true, "lowsrc": true, "dynsrc": true,
}

var (
	cssImportRE = regexp.MustCompile(`(?i)@import[^;]*;?`)
	cssExprRE   = regexp.MustCompile(`(?i)expression\s*\(|behavior\s*:|-moz-binding`)
	cssURLRE    = regexp.MustCompile(`(?i)url\(\s*(['"]?)\s*([^'")\s]*)`)
)

// SanitizeHTML strips active content from template HTML: scripts and
// embedded browsing contexts, meta/base/link, event handler attributes,
// javascript:/remote URLs and CSS imports. Styles (inline and <style>) stay,
// since templates need print CSS. The output is a body fragment. This is
// defence in depth: the document CSP and Gotenberg's disabled JavaScript
// already block execution and network access.
func SanitizeHTML(in string) string {
	z := xhtml.NewTokenizer(strings.NewReader(in))
	var out bytes.Buffer
	out.Grow(len(in))
	skip := 0 // depth inside a dropped subtree
	skipTag := ""
	inStyle := false
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break // io.EOF or a malformed tail: stop
		}
		switch tt {
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			tok := z.Token()
			name := strings.ToLower(tok.Data)
			if skip > 0 {
				if name == skipTag && tt == xhtml.StartTagToken {
					skip++
				}
				continue
			}
			if dropSubtree[name] {
				if tt == xhtml.StartTagToken && !isVoid(name) {
					skip, skipTag = 1, name
				}
				continue
			}
			if dropTag[name] {
				continue
			}
			if name == "style" && tt == xhtml.StartTagToken {
				inStyle = true
			}
			writeTag(&out, tok, tt == xhtml.SelfClosingTagToken)
		case xhtml.EndTagToken:
			tok := z.Token()
			name := strings.ToLower(tok.Data)
			if skip > 0 {
				if name == skipTag {
					skip--
				}
				continue
			}
			if dropSubtree[name] || dropTag[name] {
				continue
			}
			if name == "style" {
				inStyle = false
			}
			out.WriteString("</")
			out.WriteString(name)
			out.WriteString(">")
		case xhtml.TextToken:
			if skip > 0 {
				continue
			}
			raw := z.Raw()
			if inStyle {
				out.WriteString(sanitizeCSS(string(raw)))
				continue
			}
			out.Write(raw)
		case xhtml.CommentToken, xhtml.DoctypeToken:
			// dropped (conditional comments, doctype from pasted documents)
		}
	}
	return out.String()
}

func isVoid(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func writeTag(out *bytes.Buffer, tok xhtml.Token, selfClosing bool) {
	name := strings.ToLower(tok.Data)
	out.WriteString("<")
	out.WriteString(name)
	for _, a := range tok.Attr {
		key := strings.ToLower(a.Key)
		if a.Namespace != "" {
			key = strings.ToLower(a.Namespace) + ":" + key
		}
		val, ok := sanitizeAttr(name, key, a.Val)
		if !ok {
			continue
		}
		out.WriteString(" ")
		out.WriteString(key)
		out.WriteString(`="`)
		out.WriteString(html.EscapeString(val))
		out.WriteString(`"`)
	}
	if selfClosing {
		out.WriteString(" />")
		return
	}
	out.WriteString(">")
}

func sanitizeAttr(tag, key, val string) (string, bool) {
	if strings.HasPrefix(key, "on") {
		return "", false
	}
	if key == "style" {
		if cssExprRE.MatchString(val) || hasRemoteURL(val) {
			return "", false
		}
		return val, true
	}
	if !urlAttrs[key] {
		return val, true
	}
	v := strings.TrimSpace(val)
	lower := strings.ToLower(strings.Join(strings.Fields(v), ""))
	switch key {
	case "src":
		if strings.HasPrefix(lower, "data:image/") || isPlaceholder(v) {
			return v, true
		}
	case "href", "xlink:href":
		if tag == "a" && (strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") ||
			strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:") ||
			strings.HasPrefix(lower, "#") || isPlaceholder(v)) {
			return v, true
		}
		if strings.HasPrefix(lower, "#") {
			return v, true
		}
	}
	return "", false
}

// isPlaceholder allows an attribute that is exactly one {{key}} placeholder;
// the filled value is HTML-escaped.
func isPlaceholder(v string) bool {
	return strings.HasPrefix(v, "{{") && strings.HasSuffix(v, "}}") && !strings.ContainsAny(v[2:len(v)-2], "{}<>\"'")
}

func hasRemoteURL(css string) bool {
	for _, m := range cssURLRE.FindAllStringSubmatch(css, -1) {
		if !strings.HasPrefix(strings.ToLower(m[2]), "data:") {
			return true
		}
	}
	return false
}

func sanitizeCSS(css string) string {
	css = cssImportRE.ReplaceAllString(css, "")
	css = cssExprRE.ReplaceAllString(css, "")
	return cssURLRE.ReplaceAllStringFunc(css, func(m string) string {
		sub := cssURLRE.FindStringSubmatch(m)
		if strings.HasPrefix(strings.ToLower(sub[2]), "data:") {
			return m
		}
		return "url(" + sub[1]
	})
}
