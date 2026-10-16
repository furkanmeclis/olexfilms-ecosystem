package ioengine

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ResourceSlug strips the platform prefix from a resource slug.
func ResourceSlug(resource string) string {
	s := strings.TrimSpace(resource)
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// ExportDownloadFilename builds a client-facing filename with extension.
// Example: platform.users + pdf → users_20260823.pdf
func ExportDownloadFilename(resource string, format ExportFormat, at time.Time) string {
	slug := ResourceSlug(resource)
	if slug == "" {
		slug = "export"
	}
	date := at.UTC().Format("20060102")
	return fmt.Sprintf("%s_%s.%s", slug, date, FileExtForExport(format))
}

// AttachmentDisposition is a Content-Disposition attachment header: an
// ASCII filename (other runes, quotes and backslashes become _) plus the
// UTF-8 filename* form.
func AttachmentDisposition(filename string) string {
	safe := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(filename))
}
