package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// Every seeded template renders through a real Gotenberg with sample data
// (TEST_GOTENBERG_URL, e.g. compose.local.yml's http://127.0.0.1:3001).
func TestSeedTemplatesRenderWithGotenberg(t *testing.T) {
	url := os.Getenv("TEST_GOTENBERG_URL")
	if url == "" {
		t.Skip("TEST_GOTENBERG_URL not set; skipping real Gotenberg test")
	}
	e := newEnv(t, nil, 0)
	e.svc.pdf = pdfrender.New(url)
	ctx := context.Background()
	rows, err := e.q.ListDocumentTemplates(ctx, db.ListDocumentTemplatesParams{CurrentOnly: true, LimitCount: 200})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range rows {
		if r.BrandID.Valid || !r.IsActive {
			continue
		}
		id := r.Uuid
		pdf, err := e.svc.Preview(ctx, PreviewInput{TemplateUUID: &id})
		if err != nil {
			t.Fatalf("%s/%s: %v", r.Kind, r.Language, err)
		}
		if !strings.HasPrefix(string(pdf), "%PDF-") {
			t.Fatalf("%s/%s: not a PDF", r.Kind, r.Language)
		}
		if dir := os.Getenv("TEST_GOTENBERG_OUT"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, "seed-"+r.Kind+"-"+r.Language+".pdf"), pdf, 0o600)
		}
		n++
	}
	if n < 12 {
		t.Fatalf("rendered %d seed templates, want 12", n)
	}
}
