package httpserver

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-139 acceptance: a list PDF export is queued on the exports queue
// (worker-docs) and the worker renders it through Gotenberg as an HTML
// table with the job's lang/dir and fonts (ar rtl, zh_CN, tr); without a
// Gotenberg client the PDF job fails instead of falling back; the CSV
// export of the same list is unchanged.
func TestIntegrationExportPDFGotenberg(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t,
		func(c *config.Config) { c.Gotenberg.URL = gotb.URL },
		func(d *Deps) { d.Storage = store; d.Queue = qc })
	ctx := context.Background()

	u, _ := it.user("t139pdf")
	if _, err := it.pool.Exec(ctx, "UPDATE users SET name = 'محمد', surname = '王小明' WHERE id = $1", u.ID); err != nil {
		t.Fatal(err)
	}
	tok := it.adminToken()
	q := "t139pdf-" + it.suffix
	want := map[string]string{} // job uuid → locale
	for _, req := range []struct{ format, locale string }{{"pdf", "ar"}, {"pdf", "zh_CN"}, {"pdf", "tr"}, {"csv", "tr"}} {
		code, env := it.do("POST", "/v1/platform/users/export?q="+q, hostOlex, tok,
			map[string]any{"format": req.format, "locale": req.locale})
		if code != http.StatusAccepted {
			t.Fatalf("export %s/%s = %d %s", req.format, req.locale, code, errCode(env))
		}
		job := decodeData[exportJob](t, env)
		if job.Status == "completed" {
			t.Fatalf("export ran in the request: %+v", job)
		}
		want[job.UUID] = req.format + "/" + req.locale
	}

	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 4 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
	reg := ioengine.NewRegistry(ioadapters.NewUsers(it.q))
	worker := exportusecase.New(it.q, store, reg, nil, nil, nil, nil)
	worker.SetDocumentPDF(pdfrender.New(gotb.URL))
	seen := map[string]bool{}
	for _, task := range tasks {
		p, err := queue.ParseExportProcessPayload(task.Payload)
		if err != nil {
			t.Fatal(err)
		}
		before := gotb.Calls.Load()
		if err := worker.ProcessExport(ctx, p.ExportJobID); err != nil {
			t.Fatalf("process export %d: %v", p.ExportJobID, err)
		}
		job, err := it.q.GetExportJobByID(ctx, p.ExportJobID)
		if err != nil || job.Status != "completed" {
			t.Fatalf("job %d = %s (%v)", p.ExportJobID, job.Status, err)
		}
		kind := want[job.Uuid.String()]
		seen[kind] = true
		body := readExport(t, store, job.Uuid.String(), job.Format)
		if job.Format == "csv" {
			if gotb.Calls.Load() != before {
				t.Fatal("csv export called gotenberg")
			}
			if !strings.Contains(body, "محمد") || !strings.Contains(body, "王小明") || !strings.Contains(body, u.Email.String) {
				t.Fatalf("csv = %q", body)
			}
			continue
		}
		if gotb.Calls.Load() != before+1 || body != documentstest.PDF {
			t.Fatalf("%s: gotenberg calls %d→%d, body %q", kind, before, gotb.Calls.Load(), body)
		}
		h := gotb.LastHTML()
		checks := map[string][]string{
			"pdf/ar":    {`<html lang="ar" dir="rtl">`, `font-family:"Noto Sans Arabic";font-style:normal`, "الاسم الأول"},
			"pdf/zh_CN": {`<html lang="zh-CN" dir="ltr">`, "名"},
			"pdf/tr":    {`<html lang="tr" dir="ltr">`, "<th>Ad</th>"},
		}[kind]
		checks = append(checks, `"Noto Sans CJK SC"`, `class="doc-table export-table"`, "محمد", "王小明", u.Email.String)
		for _, c := range checks {
			if !strings.Contains(h, c) {
				t.Errorf("%s html misses %q", kind, c)
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("processed = %v", seen)
	}

	// No Gotenberg client: the PDF job fails (no gofpdf fallback).
	code, env := it.do("POST", "/v1/platform/users/export?q="+q, hostOlex, tok, map[string]any{"format": "pdf", "locale": "tr"})
	if code != http.StatusAccepted {
		t.Fatalf("export = %d %s", code, errCode(env))
	}
	bareJob := decodeData[exportJob](t, env)
	tasks, _ = insp.ListPendingTasks(queue.QueueExports)
	bare := exportusecase.New(it.q, store, reg, nil, nil, nil, nil)
	processed := false
	for _, task := range tasks {
		p, _ := queue.ParseExportProcessPayload(task.Payload)
		row, err := it.q.GetExportJobByID(ctx, p.ExportJobID)
		if err != nil || row.Uuid.String() != bareJob.UUID {
			continue
		}
		processed = true
		if err := bare.ProcessExport(ctx, p.ExportJobID); err == nil {
			t.Fatal("pdf export without gotenberg succeeded")
		}
		if row, _ = it.q.GetExportJobByID(ctx, p.ExportJobID); row.Status != "failed" {
			t.Fatalf("job status = %s", row.Status)
		}
	}
	if !processed {
		t.Fatal("pdf job without gotenberg not queued")
	}
}

func readExport(t *testing.T, store storage.Driver, jobUUID, format string) string {
	t.Helper()
	rc, _, err := store.Download(context.Background(), storage.ExportObjectKey(jobUUID, ioengine.FileExtForExport(ioengine.ExportFormat(format))))
	if err != nil {
		t.Fatalf("download %s: %v", jobUUID, err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
