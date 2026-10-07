package llm_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

func TestModelsResolve(t *testing.T) {
	m := llm.Models{
		Default: "claude-sonnet-5-5",
		Fast:    "claude-haiku-4-5",
		Allowed: []string{"claude-sonnet-5-5", "claude-haiku-4-5", "claude-opus-5-5"},
	}
	cases := []struct{ configured, def, fast string }{
		{"", "claude-sonnet-5-5", "claude-haiku-4-5"},
		{"  ", "claude-sonnet-5-5", "claude-haiku-4-5"},
		{"claude-opus-5-5", "claude-opus-5-5", "claude-opus-5-5"},
		{"gpt-9", "claude-sonnet-5-5", "claude-haiku-4-5"},
	}
	for _, c := range cases {
		if got := m.ResolveDefault(c.configured); got != c.def {
			t.Errorf("ResolveDefault(%q) = %q, want %q", c.configured, got, c.def)
		}
		if got := m.ResolveFast(c.configured); got != c.fast {
			t.Errorf("ResolveFast(%q) = %q, want %q", c.configured, got, c.fast)
		}
	}
	noList := llm.Models{Default: "claude-sonnet-5-5", Fast: "claude-haiku-4-5"}
	if !noList.IsAllowed("claude-haiku-4-5") || noList.IsAllowed("claude-opus-5-5") {
		t.Fatal("empty allow list must allow only the env models")
	}
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decoded(t *testing.T, b llm.Block) image.Config {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b.Data)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestImageBlockKeepsSmallImage(t *testing.T) {
	src := encodePNG(t, image.NewRGBA(image.Rect(0, 0, 40, 30)))
	b, err := llm.ImageBlock(src)
	if err != nil {
		t.Fatal(err)
	}
	if b.Type != llm.BlockImage || b.MediaType != "image/png" || b.Data != base64.StdEncoding.EncodeToString(src) {
		t.Fatalf("block = %+v", b)
	}
}

func TestImageBlockDownscalesLongEdge(t *testing.T) {
	src := encodePNG(t, image.NewRGBA(image.Rect(0, 0, 3136, 1000)))
	b, err := llm.ImageBlock(src)
	if err != nil {
		t.Fatal(err)
	}
	cfg := decoded(t, b)
	if b.MediaType != "image/png" || cfg.Width != llm.MaxImageEdge || cfg.Height != 500 {
		t.Fatalf("got %s %dx%d", b.MediaType, cfg.Width, cfg.Height)
	}
}

func TestImageBlockReencodesOversizedPayload(t *testing.T) {
	// Noise PNG within the edge limit but above 5 MB once base64 encoded.
	img := image.NewRGBA(image.Rect(0, 0, 1500, 1500))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.IntN(256))
		if i%4 == 3 {
			img.Pix[i] = 255 // opaque
		}
	}
	src := encodePNG(t, img)
	if base64.StdEncoding.EncodedLen(len(src)) <= llm.MaxImageBytes {
		t.Fatalf("fixture too small: %d bytes", len(src))
	}
	b, err := llm.ImageBlock(src)
	if err != nil {
		t.Fatal(err)
	}
	if b.MediaType != "image/jpeg" || len(b.Data) > llm.MaxImageBytes {
		t.Fatalf("got %s with %d base64 bytes", b.MediaType, len(b.Data))
	}
	if cfg := decoded(t, b); cfg.Width != 1500 {
		t.Fatalf("width = %d", cfg.Width)
	}
}

func TestImageBlockRejectsNonImage(t *testing.T) {
	if _, err := llm.ImageBlock([]byte("%PDF-1.7 not an image")); !errors.Is(err, llm.ErrUnsupportedImage) {
		t.Fatalf("err = %v", err)
	}
}

func TestImageFromStorage(t *testing.T) {
	store := storage.NewMemory()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2000, 4000)), nil); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Upload(ctx, storage.File{Body: &buf, ContentType: "image/jpeg"}, "warranty/claim.jpg"); err != nil {
		t.Fatal(err)
	}
	b, err := llm.ImageFromStorage(ctx, store, "warranty/claim.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if cfg := decoded(t, b); b.MediaType != "image/jpeg" || cfg.Width != 784 || cfg.Height != llm.MaxImageEdge {
		t.Fatalf("got %s %dx%d", b.MediaType, cfg.Width, cfg.Height)
	}
	if _, err := llm.ImageFromStorage(ctx, store, "missing.png"); err == nil {
		t.Fatal("missing object must fail")
	}
}

func TestCollectStreamWithoutStop(t *testing.T) {
	ch := make(chan llm.Event, 1)
	ch <- llm.Event{Type: llm.EventTextDelta, Text: "x"}
	close(ch)
	if _, err := llm.Collect(ch); err == nil {
		t.Fatal("expected error for stream without message_stop")
	}
}
