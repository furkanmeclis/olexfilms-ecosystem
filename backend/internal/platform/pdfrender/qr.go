package pdfrender

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
)

// QR codes for printed documents (TEC-188): the code is a PNG embedded as a
// data: URI, the only image source the document CSP allows.

// DefaultQRSize is the pixel size of a document QR code (printed at about
// 80pt, so 256px keeps every module sharp on A4).
const DefaultQRSize = 256

// ErrEmptyQR is returned for an empty QR payload.
var ErrEmptyQR = errors.New("pdfrender: empty qr content")

// QRCodePNG encodes content as a QR code PNG of size x size pixels (quiet
// zone included) with error correction level M (readable from a phone on paper, small enough
// for a URL).
func QRCodePNG(content string, size int) ([]byte, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrEmptyQR
	}
	if size <= 0 {
		size = DefaultQRSize
	}
	code, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return nil, err
	}
	img := renderQR(code, size)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// QRCodeImageTag returns an <img> with the QR code of content as a data:
// URI (ErrEmptyQR for empty content). alt is escaped.
func QRCodeImageTag(content, alt string) (string, error) {
	data, err := QRCodePNG(content, DefaultQRSize)
	if err != nil {
		return "", err
	}
	return ImageTag("image/png", data, alt), nil
}

// qrQuietZone is the white border in modules the QR specification requires
// around the symbol (boombuler codes carry none).
const qrQuietZone = 4

// renderQR draws the code with a quiet zone on a white size x size canvas
// using whole pixels per module (sharp edges); a canvas too small for one
// pixel per module grows to fit.
func renderQR(code barcode.Barcode, size int) *image.Gray {
	n := code.Bounds().Dx()
	total := n + 2*qrQuietZone
	if size < total {
		size = total
	}
	px := size / total
	offset := (size-total*px)/2 + qrQuietZone*px
	img := image.NewGray(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	b := code.Bounds()
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if r, _, _, _ := code.At(b.Min.X+x, b.Min.Y+y).RGBA(); r >= 0x8000 {
				continue // light module
			}
			for dy := 0; dy < px; dy++ {
				for dx := 0; dx < px; dx++ {
					img.SetGray(offset+x*px+dx, offset+y*px+dy, color.Gray{Y: 0})
				}
			}
		}
	}
	return img
}
