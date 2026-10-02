package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-196 acceptance: the service PDF is an export job on the exports
// queue (worker-docs); the HTML sent to (mock) Gotenberg carries the service
// number, the items (product, barcode, applied part) and the plate, the
// warranty codes with their QR codes, and is dir=rtl in Arabic. Another
// dealer's service is 404 and never sees the job.
func TestIntegrationServicePDF(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	const frontend = "https://olexfilms.test"
	it := newIntegrationWithDeps(t,
		func(c *config.Config) { c.Gotenberg.URL = gotb.URL; c.Auth.FrontendURL = frontend },
		func(d *Deps) { d.Storage = store; d.Queue = qc })
	ctx := context.Background()

	center := it.brandCenter("olex")
	dist := it.org("t196-dist", "distributor", center)
	dealerA := it.org("t196-dealer-a", "dealer", dist)
	dealerB := it.org("t196-dealer-b", "dealer", dist)
	ownerA, apw := it.user("t196-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, bpw := it.user("t196-owner-b")
	it.member(dealerB, ownerB, "owner")
	tokA := it.loginOrg(ownerA, apw, dealerA)
	tokB := it.loginOrg(ownerB, bpw, dealerB)

	plate := "34T196" + it.suffix[len(it.suffix)-4:]
	cust, veh := it.svcCustomer(dealerA, "t196-cust", plate)

	film := it.product(center, "T196A")
	roll := it.product(center, "T196R")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE product_categories SET available_parts = '["body_kaput"]' WHERE id = $1`, film.CategoryID); err != nil {
		t.Fatalf("category parts: %v", err)
	}
	it.setWarrantyMonths(film, 12)
	it.setWarrantyMonths(roll, 0)
	c := it.stockChain()
	uFilm := c.unit(center, film, 19601)
	uRoll := c.rollUnit(center, roll, 19602, "50.00")

	svc := it.directService(dealerA, cust, veh, 1,
		db.CreateServiceItemParams{ProductID: film.ID, UnitID: uFilm.ID, Kind: "full", AppliedParts: []byte(`["body_kaput"]`)},
		db.CreateServiceItemParams{ProductID: roll.ID, UnitID: uRoll.ID, Kind: "partial", Meters: meters(t, "10")})
	if _, err := warrantymodule.NewListener(it.pool, it.q, frontend, nil).CreateForService(ctx, svc.ID); err != nil {
		t.Fatalf("listener: %v", err)
	}
	ws := it.serviceWarranties(svc.ID, svc.BrandID)
	if len(ws) != 1 {
		t.Fatalf("warranties = %d, want 1", len(ws))
	}
	path := "/v1/services/" + svc.Uuid.String() + "/pdf"

	// 1. Scope: another dealer's service is 404.
	it.accDo("POST", path, tokB, map[string]any{"locale": "ar"}, http.StatusNotFound)

	// 2. The dealer queues the Arabic and the Turkish PDF.
	job := decodeData[exportJob](t, it.accDo("POST", path, tokA, map[string]any{"locale": "ar"}, http.StatusAccepted))
	if job.Resource != servicesusecase.ResourcePDF || job.Format != "pdf" || job.Status == "completed" {
		t.Fatalf("job = %+v", job)
	}
	it.accDo("POST", path, tokA, map[string]any{"locale": "tr"}, http.StatusAccepted)

	// 3. Both jobs wait on the exports queue (worker-docs).
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
	cert := warrantymodule.NewCertificate(it.q, store, frontend, nil)
	pdfSvc := servicesusecase.NewPDF(servicesusecase.New(it.pool, it.q, nil), cert, store, nil)
	worker := exportusecase.New(it.q, store, ioengine.NewRegistry(servicesusecase.NewPDFAdapter(pdfSvc)), nil, nil, nil, nil)
	worker.SetDocumentPDF(pdfrender.New(gotb.URL))
	var arHTML, trHTML string
	for _, task := range tasks {
		p, err := queue.ParseExportProcessPayload(task.Payload)
		if err != nil {
			t.Fatal(err)
		}
		before := gotb.Calls.Load()
		if err := worker.ProcessExport(ctx, p.ExportJobID); err != nil {
			t.Fatalf("process export %d: %v", p.ExportJobID, err)
		}
		if gotb.Calls.Load() != before+1 {
			t.Fatal("gotenberg was not called for the service pdf")
		}
		if h := gotb.LastHTML(); strings.Contains(h, `dir="rtl"`) {
			arHTML = h
		} else {
			trHTML = h
		}
	}
	for name, h := range map[string]string{"ar": arHTML, "tr": trHTML} {
		if h == "" {
			t.Fatalf("%s service pdf html missing", name)
		}
		for _, want := range []string{svc.ServiceNo, plate, film.Name, roll.Name, uFilm.Barcode, uRoll.Barcode,
			dealerA.Name, ws[0].PublicCode, frontend + "/garanti/" + ws[0].PublicCode} {
			if !strings.Contains(h, want) {
				t.Errorf("%s service pdf misses %q", name, want)
			}
		}
		if n := strings.Count(h, `<img src="data:image/png;base64,`); n != len(ws) {
			t.Errorf("%s service pdf: %d QR images, want %d", name, n, len(ws))
		}
	}
	if !strings.Contains(trHTML, `dir="ltr"`) || !strings.Contains(trHTML, "Kaput") || !strings.Contains(trHTML, "Garanti özeti") {
		t.Fatal("tr service pdf not localized")
	}
	if !strings.Contains(arHTML, "غطاء المحرك") || !strings.Contains(arHTML, "ملخص الضمان") {
		t.Fatal("ar service pdf not localized")
	}

	// 4. Poll and download through /v1/service-pdfs; dealer B never sees the job.
	done := decodeData[exportJob](t, it.accDo("GET", "/v1/service-pdfs/"+job.UUID, tokA, nil, http.StatusOK))
	if done.Status != "completed" || done.DownloadURL == nil || *done.DownloadURL != "/v1/service-pdfs/"+job.UUID+"/download" {
		t.Fatalf("job done = %+v", done)
	}
	if rec := it.raw("GET", *done.DownloadURL, tokA, "", nil, nil); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "application/pdf" || rec.Body.String() != documentstest.PDF {
		t.Fatalf("download = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	it.accDo("GET", "/v1/service-pdfs/"+job.UUID, tokB, nil, http.StatusNotFound)
	if rec := it.raw("GET", "/v1/service-pdfs/"+job.UUID+"/download", tokB, "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("dealer B download = %d", rec.Code)
	}
}
