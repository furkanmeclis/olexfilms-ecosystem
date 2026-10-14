package handler

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/usecase"
)

func TestInspectRejectsPDF(t *testing.T) {
	if _, err := inspect([]byte("%PDF-1.7\n")); !errors.Is(err, usecase.ErrUnsupportedMedia) {
		t.Fatalf("inspect pdf err = %v, want unsupported media", err)
	}
}

func TestInspectReadsJPEGEXIF(t *testing.T) {
	body, err := os.ReadFile("testdata/exif.jpg")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := inspect(body)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Mime != "image/jpeg" || meta.Width == nil || meta.Height == nil {
		t.Fatalf("meta = %+v", meta)
	}
	if meta.ExifLat == nil || meta.ExifLng == nil {
		t.Fatalf("gps not read: %+v", meta)
	}
}

func TestSniffAcceptsHEIC(t *testing.T) {
	head := append([]byte{0, 0, 0, 24}, []byte("ftypheic0000")...)
	mime, ext := sniff(head)
	if mime != "image/heic" || ext != "heic" {
		t.Fatalf("heic sniff = %s %s", mime, ext)
	}
}

func TestInspectPNGDimensions(t *testing.T) {
	meta, err := inspect(png1x1)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Mime != "image/png" || meta.Width == nil || *meta.Width != 1 || meta.Height == nil || *meta.Height != 1 {
		t.Fatalf("png meta = %+v", meta)
	}
	if got := usecase.Digest(png1x1); len(got) != 64 {
		t.Fatalf("digest length = %d", len(got))
	}
}

var png1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x62, 0x60, 0x60, 0x60, 0x00,
	0x00, 0x00, 0x04, 0x00, 0x01, 0xf6, 0x17, 0x38, 0x55, 0x00, 0x00, 0x00,
	0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// TEC-513: the EXIF wall clock is read in the camera's offset, else in
// Europe/Istanbul, never in the server's zone.
func TestParseEXIFTime(t *testing.T) {
	cases := []struct {
		raw, offset string
		want        string // UTC
	}{
		{"2026:10:14 22:04:15", "+03:00", "2026-10-14T19:04:15Z"},
		{"2026:10:14 22:04:15", "+01:00", "2026-10-14T21:04:15Z"},
		{"2026:10:14 22:04:15", "", "2026-10-14T19:04:15Z"},
		{"2026:10:14 22:04:15", "garbage", "2026-10-14T19:04:15Z"},
	}
	for _, c := range cases {
		got, ok := parseEXIFTime(c.raw, c.offset)
		if !ok || got.UTC().Format(time.RFC3339) != c.want {
			t.Fatalf("parseEXIFTime(%q, %q) = %v %v, want %s", c.raw, c.offset, got.UTC(), ok, c.want)
		}
	}
	if _, ok := parseEXIFTime("", "+03:00"); ok {
		t.Fatal("empty date parsed")
	}
}

func TestInspectEXIFDeviceUnquoted(t *testing.T) {
	body, err := os.ReadFile("testdata/exif.jpg")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := inspect(body)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ExifDevice != nil && strings.Contains(*meta.ExifDevice, `"`) {
		t.Fatalf("device = %q", *meta.ExifDevice)
	}
}
