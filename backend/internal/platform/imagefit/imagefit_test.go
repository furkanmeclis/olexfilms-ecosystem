package imagefit

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// marked is a w×h JPEG whose top-left quadrant is red, the rest blue.
func marked(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{0, 0, 255, 255}
			if x < w/2 && y < h/2 {
				c = color.RGBA{255, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withOrientation inserts an EXIF APP1 segment carrying orientation o.
func withOrientation(raw []byte, o uint16) []byte {
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0}
	entry := make([]byte, 12)
	binary.LittleEndian.PutUint16(entry[0:], 0x0112) // Orientation
	binary.LittleEndian.PutUint16(entry[2:], 3)      // SHORT
	binary.LittleEndian.PutUint32(entry[4:], 1)
	binary.LittleEndian.PutUint16(entry[8:], o)
	tiff = append(tiff, entry...)
	tiff = append(tiff, 0, 0, 0, 0) // no next IFD
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)
	out := append([]byte{}, raw[:2]...) // SOI
	out = append(out, seg...)
	return append(out, raw[2:]...)
}

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xc000 && g < 0x4000 && b < 0x4000
}

// TEC-643: an iPhone portrait photo (orientation 6) and the other rotations
// come out upright; the red quadrant lands where a viewer sees it.
func TestJPEGAppliesOrientation(t *testing.T) {
	src := marked(t, 80, 40) // landscape pixels, red top-left
	cases := []struct {
		o          uint16
		w, h       int
		redX, redY int // a point inside the red quadrant of the output
	}{
		{1, 80, 40, 20, 10},
		{3, 80, 40, 60, 30}, // 180°: bottom-right
		{6, 40, 80, 30, 20}, // 90° clockwise: top-right
		{8, 40, 80, 10, 60}, // 90° counter-clockwise: bottom-left
	}
	for _, c := range cases {
		raw := src
		if c.o != 1 {
			raw = withOrientation(src, c.o)
		}
		if got := Orientation(raw); got != int(c.o) {
			t.Fatalf("orientation %d read as %d", c.o, got)
		}
		out, err := JPEG(raw, "image/jpeg", 1600)
		if err != nil {
			t.Fatalf("o=%d: %v", c.o, err)
		}
		img := decode(t, out)
		if b := img.Bounds(); b.Dx() != c.w || b.Dy() != c.h {
			t.Fatalf("o=%d: size %dx%d, want %dx%d", c.o, b.Dx(), b.Dy(), c.w, c.h)
		}
		if !isRed(img.At(c.redX, c.redY)) {
			t.Fatalf("o=%d: (%d,%d) is not red", c.o, c.redX, c.redY)
		}
		if Orientation(out) != 1 {
			t.Fatalf("o=%d: output keeps EXIF", c.o)
		}
	}
}

// TEC-642: a 4032×3024 phone photo shrinks to 1600 on the longer side (the
// portrait one after turning it upright); a small image is not enlarged.
func TestJPEGFitsLongerSide(t *testing.T) {
	big := marked(t, 4032, 3024)
	out, err := JPEG(big, "image/jpeg", 1600)
	if err != nil {
		t.Fatal(err)
	}
	if b := decode(t, out).Bounds(); b.Dx() != 1600 || b.Dy() != 1200 {
		t.Fatalf("landscape = %dx%d, want 1600x1200", b.Dx(), b.Dy())
	}
	if len(out) >= len(big) {
		t.Fatalf("output %d bytes is not smaller than %d", len(out), len(big))
	}
	out, err = JPEG(withOrientation(big, 6), "image/jpeg", 1600)
	if err != nil {
		t.Fatal(err)
	}
	if b := decode(t, out).Bounds(); b.Dx() != 1200 || b.Dy() != 1600 {
		t.Fatalf("portrait = %dx%d, want 1200x1600", b.Dx(), b.Dy())
	}
	out, err = JPEG(marked(t, 300, 200), "image/jpeg", 1600)
	if err != nil {
		t.Fatal(err)
	}
	if b := decode(t, out).Bounds(); b.Dx() != 300 || b.Dy() != 200 {
		t.Fatalf("small = %dx%d, want unchanged 300x200", b.Dx(), b.Dy())
	}
}

func TestJPEGRejectsGarbage(t *testing.T) {
	if _, err := JPEG([]byte("not an image"), "image/jpeg", 1600); err == nil {
		t.Fatal("want a decode error")
	}
}
