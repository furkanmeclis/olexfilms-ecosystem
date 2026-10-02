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
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-188 acceptance: the warranty certificate of a service is an export
// job on the exports queue (worker-docs); the HTML sent to (mock) Gotenberg
// carries every active warranty's code and QR code (verify URL
// PUBLIC_FRONTEND_URL/garanti/{public_code}) and is dir=rtl in Arabic.
// Another dealer's service is 404, a service without an active warranty
// 409; the portal customer gets the certificate of the warranties they
// hold, another customer 404.
func TestIntegrationWarrantyCertificate(t *testing.T) {
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
	dist := it.org("t188-dist", "distributor", center)
	dealerA := it.org("t188-dealer-a", "dealer", dist)
	dealerB := it.org("t188-dealer-b", "dealer", dist)
	ownerA, apw := it.user("t188-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, bpw := it.user("t188-owner-b")
	it.member(dealerB, ownerB, "owner")
	tokA := it.loginOrg(ownerA, apw, dealerA)
	tokB := it.loginOrg(ownerB, bpw, dealerB)

	plate := "34T188" + it.suffix[len(it.suffix)-4:]
	cust, veh := it.svcCustomer(dealerA, "t188-cust", plate)
	other, _ := it.user("t188-other", rbac.RoleCustomer)
	if err := it.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: cust.ID, Slug: rbac.RoleCustomer}); err != nil {
		t.Fatalf("customer role: %v", err)
	}

	p12 := it.product(center, "T188A")
	roll := it.product(center, "T188R")
	p0 := it.product(center, "T188Z")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	it.setWarrantyMonths(p12, 12)
	it.setWarrantyMonths(roll, 24)
	it.setWarrantyMonths(p0, 0)
	c := it.stockChain()
	u12 := c.unit(center, p12, 18801)
	uRoll := c.rollUnit(center, roll, 18802, "50.00")
	u0 := c.unit(center, p0, 18803)

	listener := warrantymodule.NewListener(it.pool, it.q, frontend, nil)
	s1 := it.directService(dealerA, cust, veh, 1,
		db.CreateServiceItemParams{ProductID: p12.ID, UnitID: u12.ID, Kind: "full"},
		db.CreateServiceItemParams{ProductID: roll.ID, UnitID: uRoll.ID, Kind: "partial", Meters: meters(t, "10")})
	if _, err := listener.CreateForService(ctx, s1.ID); err != nil {
		t.Fatalf("listener: %v", err)
	}
	ws := it.serviceWarranties(s1.ID, s1.BrandID)
	if len(ws) != 2 {
		t.Fatalf("warranties = %d", len(ws))
	}
	// S2 has no warranty (no period).
	s2 := it.directService(dealerA, cust, veh, 2, db.CreateServiceItemParams{ProductID: p0.ID, UnitID: u0.ID, Kind: "full"})
	if _, err := listener.CreateForService(ctx, s2.ID); err != nil {
		t.Fatalf("listener S2: %v", err)
	}

	path := func(s db.Service) string { return "/v1/services/" + s.Uuid.String() + "/warranty-certificate" }

	// 1. Scope: another dealer's service is 404; no active warranty is 409.
	it.accDo("POST", path(s1), tokB, map[string]any{"locale": "ar"}, http.StatusNotFound)
	if code, env := it.do("POST", path(s2), hostOlex, tokA, nil); code != http.StatusConflict || errCode(env) != "NO_ACTIVE_WARRANTY" {
		t.Fatalf("no warranty: %d %s", code, errCode(env))
	}

	// 2. The dealer queues the Arabic certificate; the customer the Turkish
	// one from the portal; another customer gets 404.
	job := decodeData[exportJob](t, it.accDo("POST", path(s1), tokA, map[string]any{"locale": "ar"}, http.StatusAccepted))
	if job.Resource != warrantyusecase.ResourceCertificate || job.Format != "pdf" || job.Status == "completed" {
		t.Fatalf("job = %+v", job)
	}
	portalTok := func(u db.User) string {
		tok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: u.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	portalPath := "/v1/portal/services/" + s1.Uuid.String() + "/warranty-certificate"
	it.accDo("POST", portalPath, portalTok(other), map[string]any{"locale": "tr"}, http.StatusNotFound)
	pjob := decodeData[exportJob](t, it.accDo("POST", portalPath, portalTok(cust), map[string]any{"locale": "tr"}, http.StatusAccepted))
	if pjob.Resource != warrantyusecase.ResourcePortalCertificate {
		t.Fatalf("portal job = %+v", pjob)
	}

	// 3. Both jobs wait on the exports queue (worker-docs).
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
	cert := warrantymodule.NewCertificate(it.q, store, frontend, nil)
	reg := ioengine.NewRegistry(warrantyusecase.NewCertificateAdapter(cert), warrantyusecase.NewPortalCertificateAdapter(cert))
	worker := exportusecase.New(it.q, store, reg, nil, nil, nil, nil)
	worker.SetDocumentPDF(pdfrender.New(gotb.URL))
	var arHTML, trHTML string
	for _, task := range tasks {
		if task.Type != queue.TaskExportProcess {
			t.Fatalf("task type %s", task.Type)
		}
		p, err := queue.ParseExportProcessPayload(task.Payload)
		if err != nil {
			t.Fatal(err)
		}
		before := gotb.Calls.Load()
		if err := worker.ProcessExport(ctx, p.ExportJobID); err != nil {
			t.Fatalf("process export %d: %v", p.ExportJobID, err)
		}
		if gotb.Calls.Load() != before+1 {
			t.Fatal("gotenberg was not called for the certificate")
		}
		if h := gotb.LastHTML(); strings.Contains(h, `dir="rtl"`) {
			arHTML = h
		} else {
			trHTML = h
		}
	}
	for name, h := range map[string]string{"ar": arHTML, "tr": trHTML} {
		if h == "" {
			t.Fatalf("%s certificate html missing", name)
		}
		if n := strings.Count(h, `<img src="data:image/png;base64,`); n != len(ws) {
			t.Fatalf("%s certificate: %d QR images, want %d", name, n, len(ws))
		}
		for _, w := range ws {
			if !strings.Contains(h, w.PublicCode) || !strings.Contains(h, frontend+"/garanti/"+w.PublicCode) {
				t.Fatalf("%s certificate misses warranty %s or its verify url", name, w.PublicCode)
			}
		}
		if !strings.Contains(h, plate) || !strings.Contains(h, dealerA.Name) {
			t.Fatalf("%s certificate misses the full plate or the dealer", name)
		}
	}
	if !strings.Contains(arHTML, "شروط الضمان") || !strings.Contains(trHTML, "Garanti koşulları") || !strings.Contains(trHTML, `dir="ltr"`) {
		t.Fatal("certificate terms not localized")
	}

	// 4. Download: the dealer through /v1/warranty-certificates, the
	// customer through /v1/portal/exports; dealer B never sees the job.
	done := decodeData[exportJob](t, it.accDo("GET", "/v1/warranty-certificates/"+job.UUID, tokA, nil, http.StatusOK))
	if done.Status != "completed" || done.DownloadURL == nil || *done.DownloadURL != "/v1/warranty-certificates/"+job.UUID+"/download" {
		t.Fatalf("job done = %+v", done)
	}
	if rec := it.raw("GET", *done.DownloadURL, tokA, "", nil, nil); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "application/pdf" || rec.Body.String() != documentstest.PDF {
		t.Fatalf("download = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	it.accDo("GET", "/v1/warranty-certificates/"+job.UUID, tokB, nil, http.StatusNotFound)
	if rec := it.raw("GET", "/v1/warranty-certificates/"+job.UUID+"/download", tokB, "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("dealer B download = %d", rec.Code)
	}
	pdone := decodeData[exportJob](t, it.accDo("GET", "/v1/portal/exports/"+pjob.UUID, portalTok(cust), nil, http.StatusOK))
	if pdone.Status != "completed" || pdone.DownloadURL == nil || *pdone.DownloadURL != "/v1/portal/exports/"+pjob.UUID+"/download" {
		t.Fatalf("portal job done = %+v", pdone)
	}
	if rec := it.raw("GET", *pdone.DownloadURL, portalTok(cust), "", nil, nil); rec.Code != http.StatusOK || rec.Body.String() != documentstest.PDF {
		t.Fatalf("portal download = %d", rec.Code)
	}
	it.accDo("GET", "/v1/portal/exports/"+pjob.UUID, portalTok(other), nil, http.StatusNotFound)
}
