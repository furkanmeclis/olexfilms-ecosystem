package whatsapp

import (
	"bytes"
	"errors"
	"testing"
)

func TestSniffMedia(t *testing.T) {
	cases := map[string]string{
		"\xFF\xD8\xFF\xE0rest":             MimeJPEG,
		"\x89PNG\r\n\x1a\nrest":            MimePNG,
		"RIFF\x10\x00\x00\x00WEBPVP8 rest": MimeWebP,
		"%PDF-1.4 rest":                    MimePDF,
		"OggS\x00rest":                     MimeOGG,
		"RIFF\x10\x00\x00\x00WAVEfmt ":     "",
		"<html>":                           "",
		"":                                 "",
	}
	for in, want := range cases {
		if got := SniffMedia([]byte(in)); got != want {
			t.Errorf("SniffMedia(%q) = %q, want %q", in, got, want)
		}
	}
}

// The file name never decides the type; a known extension must match the
// content (a ".pdf" that is not a PDF is rejected).
func TestValidateMedia(t *testing.T) {
	pdf := []byte("%PDF-1.7\n")
	png := []byte("\x89PNG\r\n\x1a\n....")
	if mime, err := ValidateMedia(pdf, "fatura.PDF"); err != nil || mime != MimePDF {
		t.Fatalf("pdf: %q %v", mime, err)
	}
	if mime, err := ValidateMedia(png, ""); err != nil || mime != MimePNG {
		t.Fatalf("png without name: %q %v", mime, err)
	}
	if mime, err := ValidateMedia(pdf, "scan.bin"); err != nil || mime != MimePDF {
		t.Fatalf("unknown extension is sniffed: %q %v", mime, err)
	}
	if _, err := ValidateMedia(png, "fatura.pdf"); !errors.Is(err, ErrMediaMismatch) {
		t.Fatalf("png named .pdf: %v", err)
	}
	if _, err := ValidateMedia([]byte("hello"), "fatura.pdf"); !errors.Is(err, ErrMediaMismatch) {
		t.Fatalf("text named .pdf: %v", err)
	}
	if _, err := ValidateMedia([]byte("hello"), "a.txt"); !errors.Is(err, ErrMediaType) {
		t.Fatalf("text: %v", err)
	}
	big := append([]byte("%PDF-"), bytes.Repeat([]byte{0}, MaxMediaBytes)...)
	if _, err := ValidateMedia(big, "big.pdf"); !errors.Is(err, ErrMediaTooLarge) {
		t.Fatalf("over 16 MB: %v", err)
	}
}
