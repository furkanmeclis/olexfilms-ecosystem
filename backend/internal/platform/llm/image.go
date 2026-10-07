package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Image input limits (Anthropic vision): the base64 payload stays within
// MaxImageBytes and the long edge within MaxImageEdge; larger images are
// downscaled and re-encoded. Sources above MaxImageSourceBytes are refused
// without decoding.
const (
	MaxImageBytes       = 5 << 20
	MaxImageEdge        = 1568
	MaxImageSourceBytes = 25 << 20
)

// Image errors.
var (
	ErrUnsupportedImage = errors.New("llm: unsupported image type")
	ErrImageTooLarge    = errors.New("llm: image too large")
)

// ObjectReader reads an object from storage (storage.Driver satisfies it).
type ObjectReader interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// ImageFromStorage loads an S3 object and returns it as a base64 image block
// (jpeg/png/webp/gif), downscaling when it exceeds the limits.
func ImageFromStorage(ctx context.Context, store ObjectReader, key string) (Block, error) {
	rc, _, err := store.Download(ctx, key)
	if err != nil {
		return Block{}, fmt.Errorf("llm: read image %q: %w", key, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, MaxImageSourceBytes+1))
	if err != nil {
		return Block{}, fmt.Errorf("llm: read image %q: %w", key, err)
	}
	if len(data) > MaxImageSourceBytes {
		return Block{}, ErrImageTooLarge
	}
	return ImageBlock(data)
}

// ImageBlock builds a base64 image block from raw bytes, downscaling to the
// long edge MaxImageEdge and re-encoding when the payload would exceed
// MaxImageBytes.
func ImageBlock(data []byte) (Block, error) {
	mediaType := http.DetectContentType(data)
	switch mediaType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return Block{}, ErrUnsupportedImage
	}
	cfg, err := decodeConfig(mediaType, data)
	if err != nil {
		return Block{}, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	if max(cfg.Width, cfg.Height) <= MaxImageEdge && base64.StdEncoding.EncodedLen(len(data)) <= MaxImageBytes {
		return imageBlock(mediaType, data), nil
	}
	img, err := decodeImage(mediaType, data)
	if err != nil {
		return Block{}, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	img = fitEdge(img, MaxImageEdge)
	if mediaType == "image/png" {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil && base64.StdEncoding.EncodedLen(buf.Len()) <= MaxImageBytes {
			return imageBlock("image/png", buf.Bytes()), nil
		}
	}
	flat := flatten(img)
	for _, q := range []int{85, 70, 55, 40} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: q}); err != nil {
			return Block{}, fmt.Errorf("llm: encode image: %w", err)
		}
		if base64.StdEncoding.EncodedLen(buf.Len()) <= MaxImageBytes {
			return imageBlock("image/jpeg", buf.Bytes()), nil
		}
	}
	return Block{}, ErrImageTooLarge
}

func imageBlock(mediaType string, data []byte) Block {
	return Block{Type: BlockImage, MediaType: mediaType, Data: base64.StdEncoding.EncodeToString(data)}
}

func decodeConfig(mediaType string, data []byte) (image.Config, error) {
	r := bytes.NewReader(data)
	switch mediaType {
	case "image/jpeg":
		return jpeg.DecodeConfig(r)
	case "image/png":
		return png.DecodeConfig(r)
	case "image/gif":
		return gif.DecodeConfig(r)
	default:
		return webp.DecodeConfig(r)
	}
}

// decodeImage decodes the image (first frame of an animated GIF).
func decodeImage(mediaType string, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch mediaType {
	case "image/jpeg":
		return jpeg.Decode(r)
	case "image/png":
		return png.Decode(r)
	case "image/gif":
		return gif.Decode(r)
	default:
		return webp.Decode(r)
	}
}

// fitEdge scales img down so its long edge is at most edge.
func fitEdge(img image.Image, edge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) <= edge {
		return img
	}
	if w >= h {
		h = max(1, h*edge/w)
		w = edge
	} else {
		w = max(1, w*edge/h)
		h = edge
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// flatten composites img on white for JPEG (no alpha channel).
func flatten(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
	return dst
}
