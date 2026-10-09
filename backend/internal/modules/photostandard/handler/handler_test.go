package handler

import (
	"errors"
	"os"
	"testing"

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
