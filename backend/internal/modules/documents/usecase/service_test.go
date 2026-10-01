package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fakePDF = documentstest.PDF

type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *db.Queries
	store  *storage.Memory
	gotb   *documentstest.Gotenberg
	svc    *Service
	loader *documentstest.Loader
	org    db.Organization
	other  db.Organization
	brand  db.Brand
}

func newEnv(t *testing.T, enq Enqueuer, delay time.Duration) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping documents integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, pool: pool, q: q, store: storage.NewMemory(), gotb: documentstest.NewGotenberg(t, delay), brand: brand}
	e.org = e.mkOrg(center, "a")
	e.other = e.mkOrg(center, "b")
	pdf := pdfrender.NewWithOptions(e.gotb.URL, pdfrender.Options{Timeout: 5 * time.Second, Backoff: 5 * time.Millisecond})
	e.svc = New(pool, q, e.store, pdf, enq, pdfrender.FontsEmbedded, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.loader = documentstest.NewLoader(e.org.ID, brand.ID)
	if err := e.svc.RegisterLoader(model.KindService, e.loader); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, "DELETE FROM document_renders WHERE organization_id IN ($1, $2)", e.org.ID, e.other.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM document_templates WHERE brand_id = $1 AND name LIKE 'tec88-test%'", brand.ID)
	})
	return e
}

func (e *env) mkOrg(parent db.Organization, name string) db.Organization {
	e.t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	row, err := e.q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t88-" + name + "-" + suffix, Name: "Bayi " + name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: parent.ID, Valid: true},
		BrandID: parent.BrandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		e.t.Fatalf("create org: %v", err)
	}
	e.t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM document_renders WHERE organization_id = $1", row.ID)
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", row.ID)
	})
	return row
}

func (e *env) viewer() model.Viewer {
	return model.Viewer{UserID: 0, OrganizationID: e.org.ID, BrandID: e.brand.ID}
}

// publishBrandTemplate publishes an Olex brand override (the platform
// defaults stay untouched, so tests do not interfere).
func (e *env) publishBrandTemplate(lang, html string) model.TemplateView {
	e.t.Helper()
	ctx := context.Background()
	v, err := e.svc.SaveDraft(ctx, 0, SaveInput{Kind: model.KindService, BrandSlug: "olex", Language: lang, Name: "tec88-test " + lang, HTML: html})
	if err != nil {
		e.t.Fatal(err)
	}
	p, err := e.svc.Publish(ctx, v.UUID)
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

// Seeded templates only use variables their kind defines.
func TestSeedTemplatesUseKnownVariables(t *testing.T) {
	e := newEnv(t, nil, 0)
	rows, err := e.q.ListDocumentTemplates(context.Background(), db.ListDocumentTemplatesParams{CurrentOnly: true, LimitCount: 200})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.BrandID.Valid {
			continue
		}
		spec, ok := model.Spec(r.Kind)
		if !ok {
			t.Fatalf("unknown kind %s", r.Kind)
		}
		if u := spec.Unknown(r.Html); len(u) > 0 {
			t.Errorf("%s/%s uses unknown variables %v", r.Kind, r.Language, u)
		}
		seen[r.Kind+"/"+r.Language] = true
	}
	for _, k := range model.Kinds {
		for _, l := range []string{"tr", "en"} {
			if !seen[k+"/"+l] {
				t.Errorf("missing seed template %s/%s", k, l)
			}
		}
	}
}

// Acceptance: a template change invalidates the cache; no change keeps it.
func TestRenderCacheInvalidatedOnPublish(t *testing.T) {
	e := newEnv(t, nil, 0)
	ctx := context.Background()
	e.publishBrandTemplate("tr", `<h1>v1 {{plate}}</h1>`)
	in := RenderInput{Kind: model.KindService, SourceID: "1001", Locale: "tr"}

	first, ready, err := e.svc.RequestRender(ctx, e.viewer(), in)
	if err != nil || !ready {
		t.Fatalf("first render: ready=%v err=%v", ready, err)
	}
	second, ready, err := e.svc.RequestRender(ctx, e.viewer(), in)
	if err != nil || !ready {
		t.Fatalf("second render: ready=%v err=%v", ready, err)
	}
	if first.UUID != second.UUID || e.gotb.Calls.Load() != 1 {
		t.Fatalf("unchanged template must hit the cache: calls=%d", e.gotb.Calls.Load())
	}

	e.publishBrandTemplate("tr", `<h1>v2 {{plate}}</h1>`)
	third, ready, err := e.svc.RequestRender(ctx, e.viewer(), in)
	if err != nil || !ready {
		t.Fatalf("after publish: ready=%v err=%v", ready, err)
	}
	if third.UUID == first.UUID || e.gotb.Calls.Load() != 2 {
		t.Fatalf("publish must invalidate the cache: calls=%d", e.gotb.Calls.Load())
	}
	if !strings.Contains(e.gotb.LastHTML(), "v2 34 TEST 1001") {
		t.Fatal("new render must use the published version")
	}

	e.loader.Bump("1001")
	if _, _, err := e.svc.RequestRender(ctx, e.viewer(), in); err != nil {
		t.Fatal(err)
	}
	if e.gotb.Calls.Load() != 3 {
		t.Fatalf("source change must invalidate the cache: calls=%d", e.gotb.Calls.Load())
	}
}

// Acceptance: injected values are escaped and the CSP is present.
func TestRenderEscapesValuesAndSetsCSP(t *testing.T) {
	e := newEnv(t, nil, 0)
	e.publishBrandTemplate("tr", `<p>{{customer_name}}</p><script>alert(1)</script><img src="http://evil/x.png">`)
	e.loader.SetVar("customer_name", `<script>steal()</script><img src=http://evil/y>`)
	if _, _, err := e.svc.RequestRender(context.Background(), e.viewer(), RenderInput{Kind: model.KindService, SourceID: "2001", Locale: "tr"}); err != nil {
		t.Fatal(err)
	}
	html := e.gotb.LastHTML()
	// escaped text may still read "http://evil", but never as markup
	for _, bad := range []string{"<script", `src="http://evil`, "<img src=http", "alert(1)"} {
		if strings.Contains(html, bad) {
			t.Fatalf("rendered HTML contains %q", bad)
		}
	}
	if !strings.Contains(html, "&lt;script&gt;steal()&lt;/script&gt;") {
		t.Fatal("value must be HTML-escaped")
	}
	if !strings.Contains(html, `http-equiv="Content-Security-Policy" content="`+pdfrender.CSP+`"`) {
		t.Fatal("CSP meta missing")
	}
}

// Acceptance: Arabic documents are RTL with the Arabic font embedded.
func TestRenderArabicIsRTL(t *testing.T) {
	e := newEnv(t, nil, 0)
	e.publishBrandTemplate("ar", `<h1>نموذج الخدمة</h1><p>{{plate}}</p>`)
	if _, _, err := e.svc.RequestRender(context.Background(), e.viewer(), RenderInput{Kind: model.KindService, SourceID: "3001", Locale: "ar-SA"}); err != nil {
		t.Fatal(err)
	}
	html := e.gotb.LastHTML()
	if !strings.Contains(html, `<html lang="ar" dir="rtl">`) {
		t.Fatal("ar document must be rtl")
	}
	if !strings.Contains(html, `font-family:"Noto Sans Arabic"`) || !strings.Contains(html, "src:url(data:font/woff2;base64,") {
		t.Fatal("Noto Sans Arabic must be embedded as a data URI")
	}
}

// Missing languages fall back to the en platform template.
func TestRenderFallsBackToEnglish(t *testing.T) {
	e := newEnv(t, nil, 0)
	if _, _, err := e.svc.RequestRender(context.Background(), e.viewer(), RenderInput{Kind: model.KindService, SourceID: "3501", Locale: "de"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.gotb.LastHTML(), "Service Form") || !strings.Contains(e.gotb.LastHTML(), `lang="en"`) {
		t.Fatal("de must fall back to the en seed template")
	}
}

// Acceptance: the same cache key enqueues a single task.
func TestRequestRenderEnqueuesOnce(t *testing.T) {
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	e := newEnv(t, qc, 0)
	ctx := context.Background()
	in := RenderInput{Kind: model.KindService, SourceID: "4001", Locale: "tr"}
	a, ready, err := e.svc.RequestRender(ctx, e.viewer(), in)
	if err != nil || ready {
		t.Fatalf("first request: ready=%v err=%v", ready, err)
	}
	b, _, err := e.svc.RequestRender(ctx, e.viewer(), in)
	if err != nil {
		t.Fatal(err)
	}
	if a.UUID != b.UUID {
		t.Fatal("same cache key must return the same render")
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	defer func() { _ = insp.Close() }()
	tasks, err := insp.ListPendingTasks(queue.QueueDocs)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Type != queue.TaskDocsRender {
		t.Fatalf("want 1 pending docs task, got %d", len(tasks))
	}
	payload, _ := queue.ParseDocsRenderPayload(tasks[0].Payload)
	if err := e.svc.ProcessRender(ctx, payload.RenderID); err != nil {
		t.Fatal(err)
	}
	// worker retry of an already rendered row is a no-op
	if err := e.svc.ProcessRender(ctx, payload.RenderID); err != nil {
		t.Fatal(err)
	}
	if e.gotb.Calls.Load() != 1 {
		t.Fatalf("idempotent processing: calls=%d", e.gotb.Calls.Load())
	}
	c, ready, err := e.svc.RequestRender(ctx, e.viewer(), in)
	if err != nil || !ready || c.UUID != a.UUID {
		t.Fatalf("after processing: ready=%v err=%v", ready, err)
	}
}

// Acceptance: storage + download; other organizations get not found.
func TestDownloadScopedToOrganization(t *testing.T) {
	e := newEnv(t, nil, 0)
	ctx := context.Background()
	v, ready, err := e.svc.RequestRender(ctx, e.viewer(), RenderInput{Kind: model.KindService, SourceID: "5001", Locale: "tr"})
	if err != nil || !ready {
		t.Fatalf("render: ready=%v err=%v", ready, err)
	}
	rc, name, err := e.svc.Download(ctx, e.org.ID, v.UUID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	sum := sha256.Sum256(data)
	if string(data) != fakePDF || v.SHA256 == nil || *v.SHA256 != hex.EncodeToString(sum[:]) || !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("download mismatch: %q sha=%v", data, v.SHA256)
	}
	if ct, _, ok := e.store.GetMeta(storage.DocumentObjectKey(e.org.Uuid, model.KindService, v.UUID)); !ok || ct != "application/pdf" {
		t.Fatalf("stored object meta = %q, %v", ct, ok)
	}
	if _, _, err := e.svc.Download(ctx, e.other.ID, v.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other organization must get not found, got %v", err)
	}
	if _, err := e.svc.GetRender(ctx, e.other.ID, v.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other organization must not see the render, got %v", err)
	}
	// a viewer of another organization cannot request this source
	if _, _, err := e.svc.RequestRender(ctx, model.Viewer{OrganizationID: e.other.ID, BrandID: e.brand.ID}, RenderInput{Kind: model.KindService, SourceID: "5001"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign source must be not found, got %v", err)
	}
}

// Acceptance (fake Gotenberg, 300 ms each): 20 service PDFs processed
// concurrently finish in about one server latency.
func TestConcurrentRenders(t *testing.T) {
	e := newEnv(t, nil, 300*time.Millisecond)
	ctx := context.Background()
	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, ready, err := e.svc.RequestRender(ctx, e.viewer(), RenderInput{Kind: model.KindService, SourceID: fmt.Sprintf("6%03d", i), Locale: "tr"})
			if err == nil && !ready {
				err = errors.New("not ready")
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("20 concurrent renders took %v", elapsed)
	} else {
		t.Logf("20 concurrent renders: %v", elapsed)
	}
	if e.gotb.Calls.Load() != 20 {
		t.Fatalf("calls=%d", e.gotb.Calls.Load())
	}
}

func TestTemplateDraftLifecycle(t *testing.T) {
	e := newEnv(t, nil, 0)
	ctx := context.Background()
	_, err := e.svc.SaveDraft(ctx, 0, SaveInput{Kind: model.KindService, BrandSlug: "olex", Language: "tr", Name: "tec88-test x", HTML: "<p>{{nope}}</p>"})
	var unknown *UnknownVariablesError
	if !errors.As(err, &unknown) || unknown.Keys[0] != "nope" {
		t.Fatalf("unknown variable must be rejected, got %v", err)
	}
	d1, err := e.svc.SaveDraft(ctx, 0, SaveInput{Kind: model.KindService, BrandSlug: "olex", Language: "tr", Name: "tec88-test a", HTML: `<p onclick="x()">{{plate}}</p>`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(*d1.HTML, "onclick") || d1.Status != model.StatusDraft {
		t.Fatalf("draft must be sanitized: %+v", d1)
	}
	d2, err := e.svc.SaveDraft(ctx, 0, SaveInput{Kind: model.KindService, BrandSlug: "olex", Language: "tr", Name: "tec88-test b", HTML: `<p>{{plate}} 2</p>`})
	if err != nil || d2.UUID != d1.UUID {
		t.Fatalf("second save must update the open draft: %v", err)
	}
	pub, err := e.svc.Publish(ctx, d2.UUID)
	if err != nil || pub.Status != model.StatusActive {
		t.Fatalf("publish: %+v %v", pub, err)
	}
	if _, err := e.svc.UpdateDraft(ctx, pub.UUID, SaveInput{HTML: "<p>x</p>"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("published version must be immutable, got %v", err)
	}
	d3, err := e.svc.SaveDraft(ctx, 0, SaveInput{Kind: model.KindService, BrandSlug: "olex", Language: "tr", Name: "tec88-test c", HTML: `<p>{{plate}} 3</p>`})
	if err != nil || d3.Version != pub.Version+1 {
		t.Fatalf("new draft must open the next version: %+v %v", d3, err)
	}
	versions, err := e.svc.Versions(ctx, d3.UUID)
	if err != nil || len(versions) < 2 {
		t.Fatalf("versions: %v %v", len(versions), err)
	}
}

func TestPreviewUsesSampleData(t *testing.T) {
	e := newEnv(t, nil, 0)
	pdf, err := e.svc.Preview(context.Background(), PreviewInput{Kind: model.KindService, HTML: "<p>{{customer_name}}</p>{{items_table}}", Locale: "tr"})
	if err != nil || string(pdf) != fakePDF {
		t.Fatalf("preview: %v", err)
	}
	html := e.gotb.LastHTML()
	if !strings.Contains(html, "Ahmet Yılmaz") || !strings.Contains(html, `<table class="doc-table">`) {
		t.Fatal("preview must use tr sample data and html blocks")
	}
}
