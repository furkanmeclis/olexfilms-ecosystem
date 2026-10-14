package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand/v2"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-642: a full resolution phone photo attached to a contract is embedded
// in the PDF HTML as a JPEG of at most PDFMediaMaxPx on the longer side.
func TestContractMediaIsShrunkForThePDF(t *testing.T) {
	ctx := context.Background()
	src := image.NewRGBA(image.Rect(0, 0, 4032, 3024))
	for y := range 3024 {
		for x := range 4032 {
			src.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	var raw bytes.Buffer
	if err := jpeg.Encode(&raw, src, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemory()
	if err := store.Upload(ctx, storage.File{Body: bytes.NewReader(raw.Bytes()), Size: int64(raw.Len()), ContentType: "image/jpeg"}, "contracts/m1.jpg"); err != nil {
		t.Fatal(err)
	}
	s := &Service{storage: store}
	out := s.mediaHTML(ctx, i18n.Locale("tr"), []db.ContractMedium{{
		StorageKey: "contracts/m1.jpg", MimeType: "image/jpeg", Title: pgtype.Text{String: "Ön tampon", Valid: true},
	}})
	const prefix = `src="data:image/jpeg;base64,`
	i := strings.Index(out, prefix)
	if i < 0 {
		t.Fatalf("no JPEG data URI in %.200q", out)
	}
	rest := out[i+len(prefix):]
	data, err := base64.StdEncoding.DecodeString(rest[:strings.IndexByte(rest, '"')])
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != PDFMediaMaxPx || cfg.Height != 1200 {
		t.Fatalf("embedded image %dx%d, want %dx1200", cfg.Width, cfg.Height, PDFMediaMaxPx)
	}
	if len(data) >= raw.Len() {
		t.Fatalf("embedded %d bytes, original %d", len(data), raw.Len())
	}
	// The stored original is untouched.
	rc, _, err := store.Download(ctx, "contracts/m1.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	var kept bytes.Buffer
	_, _ = kept.ReadFrom(rc)
	if !bytes.Equal(kept.Bytes(), raw.Bytes()) {
		t.Fatal("the original changed")
	}
}

// photo is a noisy w×h JPEG, close to a phone photo in entropy.
func photo(t *testing.T, w, h int, seed uint64) []byte {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, seed+1))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			n := uint8(r.IntN(48))
			img.Set(x, y, color.RGBA{uint8(x/16) + n, uint8(y/16) + n, 128 + n, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TEC-642 acceptance on a real Gotenberg: a contract with two 4032×3024
// photos renders under 10 MB.
func TestContractMediaPDFSizeOnGotenberg(t *testing.T) {
	url := os.Getenv("TEST_GOTENBERG_URL")
	if url == "" {
		t.Skip("TEST_GOTENBERG_URL not set")
	}
	ctx := context.Background()
	store := storage.NewMemory()
	var media []db.ContractMedium
	for i, key := range []string{"contracts/a.jpg", "contracts/b.jpg"} {
		data := photo(t, 4032, 3024, uint64(i+1))
		if err := store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "image/jpeg"}, key); err != nil {
			t.Fatal(err)
		}
		media = append(media, db.ContractMedium{StorageKey: key, MimeType: "image/jpeg"})
	}
	s := &Service{storage: store}
	doc := pdfrender.Document{Lang: "tr", Title: "Sözleşme", Body: s.mediaHTML(ctx, i18n.Locale("tr"), media),
		Fonts: pdfrender.FontsEmbedded}.HTML()
	pdf, err := pdfrender.New(url).Convert(ctx, pdfrender.Request{HTML: doc})
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("TEST_GOTENBERG_OUT"); dir != "" {
		_ = os.WriteFile(dir+"/tec642-contract-media.pdf", pdf, 0o600)
	}
	t.Logf("PDF with two 4032x3024 photos: %d bytes", len(pdf))
	if len(pdf) >= 10<<20 {
		t.Fatalf("PDF is %d bytes, want < 10 MB", len(pdf))
	}
}
