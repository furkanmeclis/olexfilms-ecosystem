package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

func TestStripHeaderBreaks(t *testing.T) {
	got := stripHeaderBreaks("Hi Ali\r\nBcc: attacker@example.com\nX")
	if got != "Hi Ali Bcc: attacker@example.com X" {
		t.Fatalf("got %q", got)
	}
}

// TEC-476: a message with an attachment is multipart/mixed: the
// alternative body part, then the file base64 encoded.
func TestWriteBodyWithAttachment(t *testing.T) {
	pdf := bytes.Repeat([]byte("%PDF-1.7 fleet report "), 20)
	var b strings.Builder
	writeBody(&b, Message{
		Body: "plain", HTMLBody: "<p>html</p>",
		Attachments: []Attachment{{Filename: "fleet-report-2026-09.pdf", ContentType: "application/pdf", Data: pdf}},
	})
	head, body, ok := strings.Cut(b.String(), "\r\n\r\n")
	if !ok {
		t.Fatal("no header separator")
	}
	mt, params, err := mime.ParseMediaType(strings.TrimPrefix(head, "Content-Type: "))
	if err != nil || mt != "multipart/mixed" {
		t.Fatalf("content type = %q, %v", mt, err)
	}
	r := multipart.NewReader(strings.NewReader(body), params["boundary"])
	first, err := r.NextPart()
	if err != nil || !strings.HasPrefix(first.Header.Get("Content-Type"), "multipart/alternative") {
		t.Fatalf("first part = %v, %v", first.Header, err)
	}
	second, err := r.NextRawPart()
	if err != nil {
		t.Fatal(err)
	}
	h := textproto.MIMEHeader(second.Header)
	if h.Get("Content-Type") != `application/pdf; name="fleet-report-2026-09.pdf"` ||
		h.Get("Content-Disposition") != `attachment; filename="fleet-report-2026-09.pdf"` {
		t.Fatalf("attachment headers = %v", h)
	}
	raw, err := io.ReadAll(second)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\r\n") {
		if len(line) > 76 {
			t.Fatalf("base64 line longer than 76: %d", len(line))
		}
	}
	got, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(raw), "\r\n", ""))
	if err != nil || !bytes.Equal(got, pdf) {
		t.Fatalf("attachment round trip failed: %v", err)
	}
	if _, err := r.NextPart(); err != io.EOF {
		t.Fatalf("want two parts, got more: %v", err)
	}
}

// Without attachments the body is unchanged (plain text).
func TestWriteBodyPlain(t *testing.T) {
	var b strings.Builder
	writeBody(&b, Message{Body: "hello"})
	if b.String() != "Content-Type: text/plain; charset=UTF-8\r\n\r\nhello" {
		t.Fatalf("got %q", b.String())
	}
}
