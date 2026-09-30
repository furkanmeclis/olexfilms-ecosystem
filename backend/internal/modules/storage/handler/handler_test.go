package handler

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/model"
)

func TestWriteFileForcesDownloadForActiveContent(t *testing.T) {
	cases := map[string]string{
		"text/html":              "attachment",
		"image/svg+xml":          "attachment",
		"application/xhtml+xml":  "attachment",
		"text/html; charset=utf": "attachment",
		"":                       "attachment",
		"image/png":              "inline",
		"application/pdf":        "inline",
		"video/mp4":              "inline",
	}
	for mime, want := range cases {
		rec := httptest.NewRecorder()
		writeFile(rec, model.OpenObject{
			Object: model.Object{Name: "a\"b\r\n.html", MimeType: mime},
			Body:   io.NopCloser(strings.NewReader("x")),
		})
		disp := rec.Header().Get("Content-Disposition")
		if !strings.HasPrefix(disp, want+";") {
			t.Fatalf("%q: disposition %q, want %s", mime, disp, want)
		}
		if strings.ContainsAny(disp, "\r\n") || strings.Count(disp, `"`) != 2 {
			t.Fatalf("%q: unsafe filename in %q", mime, disp)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%q: nosniff missing", mime)
		}
	}
}
