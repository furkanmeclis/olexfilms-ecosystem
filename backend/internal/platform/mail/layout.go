package mail

import (
	"bytes"
	"html/template"
	"strings"
)

// Layout is the branded HTML e-mail frame (logo, brand color, text
// direction). BodyHTML must already be safe HTML (msgtemplate.MarkdownToHTML).
type Layout struct {
	Lang      string
	Dir       string // "ltr" | "rtl" (i18n.Dir)
	Title     string
	BrandName string
	LogoURL   string
	Color     string // CSS color of the header bar and links
	BodyHTML  string
	Footer    string
}

var layoutTpl = template.Must(template.New("mail").Parse(`<!doctype html>
<html lang="{{.Lang}}" dir="{{.Dir}}">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title></head>
<body style="margin:0;padding:0;background:#f4f4f5;font-family:Arial,Helvetica,sans-serif;color:#18181b;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f5;padding:24px 0;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" dir="{{.Dir}}" style="max-width:600px;width:100%;background:#ffffff;border-radius:8px;overflow:hidden;text-align:{{if eq .Dir "rtl"}}right{{else}}left{{end}};">
<tr><td style="background:{{.Color}};padding:16px 24px;">{{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.BrandName}}" height="32" style="display:block;height:32px;">{{else}}<span style="color:#ffffff;font-size:18px;font-weight:bold;">{{.BrandName}}</span>{{end}}</td></tr>
<tr><td style="padding:24px;font-size:15px;line-height:1.6;">
<h1 style="font-size:20px;margin:0 0 16px;">{{.Title}}</h1>
{{.Body}}
</td></tr>
<tr><td style="padding:16px 24px;border-top:1px solid #e4e4e7;font-size:12px;color:#71717a;">{{.Footer}}</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`))

// RenderHTML renders the layout.
func (l Layout) RenderHTML() (string, error) {
	if l.Dir != "rtl" {
		l.Dir = "ltr"
	}
	if strings.TrimSpace(l.Color) == "" || strings.ContainsAny(l.Color, ";\"'<>") {
		l.Color = "#111827"
	}
	if l.Footer == "" {
		l.Footer = l.BrandName
	}
	var buf bytes.Buffer
	data := struct {
		Layout
		Body template.HTML
	}{Layout: l, Body: template.HTML(l.BodyHTML)} //nolint:gosec // BodyHTML is escaped by msgtemplate.MarkdownToHTML
	if err := layoutTpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
