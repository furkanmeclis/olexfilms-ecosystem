package guide

import (
	"context"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	library "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

var update = flag.Bool("update", false, "rewrite the HTML snapshots in testdata")

// guidesDir is docs/guides of the repository.
const guidesDir = "../../../../../docs/guides"

// fakeGotenberg is an httptest Gotenberg: it records the index.html of every
// conversion and answers with a small PDF.
type fakeGotenberg struct {
	mu    sync.Mutex
	html  []string
	calls int
}

func (f *fakeGotenberg) server(t *testing.T) *pdfrender.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/chromium/convert/html" {
			http.NotFound(w, r)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var index string
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if part.FileName() == "index.html" {
				b, _ := io.ReadAll(part)
				index = string(b)
			}
			_ = part.Close()
		}
		f.mu.Lock()
		f.html = append(f.html, index)
		f.calls++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.7\n% fake gotenberg\n"+strings.Repeat("x", len(index)%97+1)+"\n%%EOF\n")
	}))
	t.Cleanup(srv.Close)
	return pdfrender.NewWithOptions(srv.URL, pdfrender.Options{MaxRetries: 0, Timeout: 5 * time.Second})
}

type memStorage struct{ files map[string][]byte }

func (m *memStorage) Upload(_ context.Context, file platstorage.File, path string) error {
	if m.files == nil {
		m.files = map[string][]byte{}
	}
	b, err := io.ReadAll(file.Body)
	if err != nil {
		return err
	}
	m.files[path] = b
	return nil
}

func (m *memStorage) PresignGet(_ context.Context, path string, _ time.Duration) (string, error) {
	return "https://example.test/" + path, nil
}

type staticFeatures bool

func (s staticFeatures) Enabled(context.Context, int64, string) (bool, error) { return bool(s), nil }

type fixture struct {
	ctx     context.Context
	q       *db.Queries
	brandID int64
	store   *memStorage
	got     *fakeGotenberg
	pub     *Publisher
}

func newFixture(t *testing.T, libraryOn bool) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	f := &fixture{ctx: ctx, q: q, brandID: brand.ID, store: &memStorage{}, got: &fakeGotenberg{}}
	f.pub = New(q, library.New(q, f.store), f.got.server(t), staticFeatures(libraryOn), pdfrender.FontsSystem, nil)
	return f
}

func loadGuide(t *testing.T) []Source {
	t.Helper()
	sources, err := LoadSources(guidesDir, "eklentiler")
	if err != nil {
		t.Fatalf("LoadSources() error = %v", err)
	}
	return sources
}

func (f *fixture) versions(t *testing.T, res Result) []db.LibraryItemVersion {
	t.Helper()
	item, err := f.q.GetLibraryItemByUUID(f.ctx, res.ItemUUID)
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	rows, err := f.q.ListLibraryItemVersions(f.ctx, item.ID)
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	return rows
}

// Acceptance: running the publication twice on the test DB leaves one
// version per language; the PDF is not empty.
func TestPublish_TwiceKeepsOneVersionPerLanguage(t *testing.T) {
	f := newFixture(t, true)
	sources := loadGuide(t)
	g := Guides["eklentiler"]

	first, err := f.pub.Publish(f.ctx, f.brandID, g, sources)
	if err != nil {
		t.Fatalf("first Publish() error = %v", err)
	}
	second, err := f.pub.Publish(f.ctx, f.brandID, g, sources)
	if err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}
	if first.ItemUUID != second.ItemUUID {
		t.Fatalf("item changed between runs: %s → %s", first.ItemUUID, second.ItemUUID)
	}
	if f.got.calls != len(sources) {
		t.Fatalf("gotenberg calls = %d, want %d (second run must not render)", f.got.calls, len(sources))
	}
	for _, lr := range second.Languages {
		if lr.Published || lr.VersionNo != 1 {
			t.Fatalf("second run language %+v, want unchanged version 1", lr)
		}
	}
	rows := f.versions(t, first)
	if len(rows) != len(sources) {
		t.Fatalf("versions = %d, want one per language (%d)", len(rows), len(sources))
	}
	locales := map[string]bool{}
	for _, v := range rows {
		locales[v.Locale] = true
		if v.Mime != "application/pdf" || v.SizeBytes == 0 || len(f.store.files[v.StorageKey]) == 0 {
			t.Fatalf("version %+v is not a stored, non-empty PDF", v)
		}
	}
	if !locales["tr"] || !locales["en"] {
		t.Fatalf("locales = %v, want tr and en", locales)
	}

	item, err := f.q.GetLibraryItemByUUID(f.ctx, first.ItemUUID)
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item.AccessLevel != library.AccessAllNetwork || item.Name != g.Name {
		t.Fatalf("item = %q access %q, want %q all_network", item.Name, item.AccessLevel, g.Name)
	}
	center, _ := f.q.GetBrandCenter(f.ctx, f.brandID)
	dealer := library.Actor{OrganizationID: center.ID + 1_000_000, BrandID: f.brandID, OrgType: library.OrgDealer}
	items, _, err := library.New(f.q, f.store).ListItems(f.ctx, dealer, library.ListInput{Tags: []string{itemTag(g.Slug)}, Locale: "de"})
	if err != nil {
		t.Fatalf("dealer ListItems() error = %v", err)
	}
	if len(items) != 1 || items[0].LatestVersion == nil || items[0].LatestVersion.Locale != "en" {
		t.Fatalf("dealer (de) sees %+v, want the item with its en version", items)
	}
}

// A changed language opens a new version for that language only.
func TestPublish_ChangedLanguageGetsNewVersion(t *testing.T) {
	f := newFixture(t, true)
	sources := loadGuide(t)
	g := Guides["eklentiler"]
	if _, err := f.pub.Publish(f.ctx, f.brandID, g, sources); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	changed := make([]Source, len(sources))
	copy(changed, sources)
	for i := range changed {
		if changed[i].Lang == "en" {
			changed[i].Markdown = append(append([]byte{}, changed[i].Markdown...), "\nUpdated.\n"...)
		}
	}
	res, err := f.pub.Publish(f.ctx, f.brandID, g, changed)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	for _, lr := range res.Languages {
		wantPublished := lr.Lang == "en"
		if lr.Published != wantPublished {
			t.Fatalf("language %s published = %v, want %v", lr.Lang, lr.Published, wantPublished)
		}
		if wantPublished && lr.VersionNo != 2 {
			t.Fatalf("en version = %d, want 2", lr.VersionNo)
		}
	}
	if n := len(f.versions(t, res)); n != len(sources)+1 {
		t.Fatalf("versions = %d, want %d", n, len(sources)+1)
	}
}

// The library belongs to the announcements module; off → skipped.
func TestPublish_SkipsWhenLibraryModuleOff(t *testing.T) {
	f := newFixture(t, false)
	res, err := f.pub.Publish(f.ctx, f.brandID, Guides["eklentiler"], loadGuide(t))
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !res.Skipped || f.got.calls != 0 {
		t.Fatalf("result = %+v, calls = %d; want skipped without rendering", res, f.got.calls)
	}
}

// HTML snapshot of what the fake Gotenberg receives (go test -update).
func TestRenderHTML_Snapshot(t *testing.T) {
	for _, src := range loadGuide(t) {
		html, err := RenderHTML(src, pdfrender.FontsSystem)
		if err != nil {
			t.Fatalf("RenderHTML(%s) error = %v", src.Lang, err)
		}
		golden := filepath.Join("testdata", "eklentiler."+src.Lang+".html")
		if *update {
			if err := os.WriteFile(golden, []byte(html), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("read snapshot: %v (run go test -update)", err)
		}
		if html != string(want) {
			t.Fatalf("%s HTML differs from %s (run go test -update after a guide change)", src.Lang, golden)
		}
		if !strings.Contains(html, `<html lang="`+src.Lang+`"`) || !strings.Contains(html, `<article class="guide">`) {
			t.Fatalf("%s HTML lacks the document skeleton", src.Lang)
		}
		if strings.Contains(html, "<script") {
			t.Fatalf("%s HTML contains a script", src.Lang)
		}
	}
}

// Acceptance: the guide covers every F5 add-on and the chain rules, in every
// language with the same structure.
func TestGuide_CoversAddonsAndChainRules(t *testing.T) {
	required := []string{
		"dealer_showcase", "fleet", "certificates", "stock_forecast", "performance",
		"efficiency", "photo_standard", "e_invoice", "pricing.recommended.read",
		"modules.read", "modules.manage", "module_bundle",
	}
	var h2 = -1
	for _, src := range loadGuide(t) {
		text := string(src.Markdown)
		for _, key := range required {
			if !strings.Contains(text, "`"+key+"`") {
				t.Errorf("%s guide does not mention `%s`", src.Lang, key)
			}
		}
		n := strings.Count(text, "\n## ")
		if h2 >= 0 && n != h2 {
			t.Errorf("%s guide has %d sections, other language %d", src.Lang, n, h2)
		}
		h2 = n
		if Title(src.Markdown) == "" {
			t.Errorf("%s guide has no title", src.Lang)
		}
	}
}
