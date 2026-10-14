// Package imagefit shrinks photos for documents (TEC-642, TEC-643): it
// applies the EXIF orientation, fits the image into a square of maxPx on
// the longer side and re-encodes it as a metadata-free JPEG. Gotenberg's
// Chromium embeds a data URI image decoded (uncompressed) at its full
// resolution, so a phone photo left as is grows a PDF by tens of MB.
package imagefit

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png" // decoder
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Quality is the JPEG quality of the output.
const Quality = 80

// ErrEmpty is an image without pixels.
var ErrEmpty = errors.New("imagefit: empty image")

// JPEG decodes a JPEG, PNG or WebP image, turns it upright by its EXIF
// orientation, fits it into maxPx on the longer side (never enlarging) and
// encodes it as JPEG without metadata.
func JPEG(raw []byte, mime string, maxPx int) ([]byte, error) {
	var (
		src image.Image
		err error
	)
	if strings.EqualFold(mime, "image/webp") {
		src, err = webp.Decode(bytes.NewReader(raw))
	} else {
		src, _, err = image.Decode(bytes.NewReader(raw))
	}
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, ErrEmpty
	}
	o := Orientation(raw)
	// Orientations 5-8 swap the axes: fit the upright size.
	w, h := b.Dx(), b.Dy()
	if o >= 5 {
		w, h = h, w
	}
	w, h = fit(w, h, maxPx)
	sw, sh := w, h
	if o >= 5 {
		sw, sh = h, w
	}
	scaled := image.NewRGBA(image.Rect(0, 0, sw, sh))
	xdraw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), src, b, xdraw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, orient(scaled, o), &jpeg.Options{Quality: Quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Orientation is the EXIF orientation (1-8) of raw; 1 when absent.
func Orientation(raw []byte) int {
	x, err := exif.Decode(bytes.NewReader(raw))
	if err != nil {
		return 1
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil || tag == nil {
		return 1
	}
	v, err := tag.Int(0)
	if err != nil || v < 1 || v > 8 {
		return 1
	}
	return v
}

func fit(w, h, maxPx int) (int, int) {
	if maxPx <= 0 || (w <= maxPx && h <= maxPx) {
		return w, h
	}
	if w >= h {
		h, w = h*maxPx/w, maxPx
	} else {
		w, h = w*maxPx/h, maxPx
	}
	return max(w, 1), max(h, 1)
}

// orient returns src turned upright for EXIF orientation o.
func orient(src *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2: // mirrored horizontally
				dx, dy = w-1-x, y
			case 3: // rotated 180°
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored vertically
				dx, dy = x, h-1-y
			case 5: // transposed
				dx, dy = y, x
			case 6: // rotate 90° clockwise to view
				dx, dy = h-1-y, x
			case 7: // transversed
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 90° counter-clockwise to view
				dx, dy = y, w-1-x
			}
			i := src.PixOffset(x, y)
			j := dst.PixOffset(dx, dy)
			copy(dst.Pix[j:j+4], src.Pix[i:i+4])
		}
	}
	return dst
}
