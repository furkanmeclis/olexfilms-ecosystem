package pdfrender

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	gozxingqr "github.com/makiuchi-d/gozxing/qrcode"
)

func decodeQR(t *testing.T, data []byte) string {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png: %v", err)
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatalf("bitmap: %v", err)
	}
	res, err := gozxingqr.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		t.Fatalf("qr decode: %v", err)
	}
	return res.GetText()
}

// TEC-188 acceptance: the QR code decodes to the verify URL.
func TestQRCodePNGDecodesToURL(t *testing.T) {
	url := "https://olexfilms.app/garanti/AbCdEfGhIjKlMnOpQrStUv"
	data, err := QRCodePNG(url, DefaultQRSize)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != DefaultQRSize || b.Dy() != DefaultQRSize {
		t.Fatalf("size = %v", b)
	}
	// Quiet zone: the border is white; the symbol has dark modules.
	if r, _, _, _ := img.At(2, 2).RGBA(); r < 0x8000 {
		t.Fatal("quiet zone corner is not white")
	}
	dark := 0
	for y := 0; y < DefaultQRSize; y++ {
		for x := 0; x < DefaultQRSize; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r < 0x8000 {
				dark++
			}
		}
	}
	if dark == 0 || dark > DefaultQRSize*DefaultQRSize*3/4 {
		t.Fatalf("dark pixels = %d", dark)
	}
	if got := decodeQR(t, data); got != url {
		t.Fatalf("decoded %q, want %q", got, url)
	}
}

func TestQRCodeImageTag(t *testing.T) {
	url := "http://localhost:3000/garanti/x-_y123456789012"
	tag, err := QRCodeImageTag(url, `QR "a"`)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = `<img src="data:image/png;base64,`
	if !strings.HasPrefix(tag, prefix) || !strings.Contains(tag, `alt="QR &#34;a&#34;"`) {
		t.Fatalf("tag = %.80s", tag)
	}
	b64 := tag[len(prefix):strings.Index(tag, `" alt=`)]
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeQR(t, data); got != url {
		t.Fatalf("decoded %q", got)
	}
	if _, err := QRCodeImageTag("  ", "x"); !errors.Is(err, ErrEmptyQR) {
		t.Fatalf("empty content err = %v", err)
	}
	// A size below the module count grows to fit instead of failing.
	if _, err := QRCodePNG(url, 5); err != nil {
		t.Fatalf("small size: %v", err)
	}
}
