package httpserver

import (
	"context"
	"encoding/json"
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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// portalForbiddenKeys are JSON keys the portal service detail must never
// carry: measurement fields, purchase / sale prices and stock details.
var portalForbiddenKeys = []string{
	"has_measurement", "measurement", "measurement_result_id", "measurements",
	"purchase_price", "sale_price", "sale_to_distributor_price", "recommended_sale_price", "price", "prices",
	"barcode", "meters", "quantity", "notes", "km", "customer", "phone",
}

// jsonKeys collects every object key of a decoded JSON value.
func jsonKeys(v any, into map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			into[k] = true
			jsonKeys(val, into)
		}
	case []any:
		for _, val := range x {
			jsonKeys(val, into)
		}
	}
}

type portalServiceDetail struct {
	UUID         string   `json:"uuid"`
	ServiceNo    string   `json:"service_no"`
	AppliedParts []string `json:"applied_parts"`
	Products     []struct {
		Name         string   `json:"name"`
		Category     string   `json:"category"`
		AppliedParts []string `json:"applied_parts"`
	} `json:"products"`
	Dealer struct {
		Name     string  `json:"name"`
		Address  string  `json:"address"`
		WhatsApp *string `json:"whatsapp"`
	} `json:"dealer"`
	Warranties []struct {
		PublicCode string `json:"public_code"`
	} `json:"warranties"`
}

// TEC-239 acceptance: the owner customer gets the portal service detail
// (parts and products filled, dealer card, warranties) with 200, another
// customer 404; the response carries no measurement or price field. The
// PDF answers 202 (queued) for the owner, 404 for another customer, and
// 200 with the portal download link once the render is completed.
func TestIntegrationPortalServiceDetail(t *testing.T) {
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
	dist := it.org("t239-dist", "distributor", center)
	dealer := it.org("t239-dealer", "dealer", dist)
	if _, err := it.pool.Exec(ctx, `UPDATE organizations SET phone = '+905321112239', address = 'T239 Cad. 1' WHERE id = $1`, dealer.ID); err != nil {
		t.Fatalf("dealer phone: %v", err)
	}

	plate := "34T239" + it.suffix[len(it.suffix)-4:]
	cust, veh := it.svcCustomer(dealer, "t239-cust", plate)
	if err := it.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: cust.ID, Slug: rbac.RoleCustomer}); err != nil {
		t.Fatalf("customer role: %v", err)
	}
	other, _ := it.user("t239-other", rbac.RoleCustomer)

	film := it.product(center, "T239A")
	roll := it.product(center, "T239R")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE product_categories SET available_parts = '["body_kaput","body_tavan"]' WHERE id = $1`, film.CategoryID); err != nil {
		t.Fatalf("category parts: %v", err)
	}
	it.setWarrantyMonths(film, 12)
	it.setWarrantyMonths(roll, 0)
	c := it.stockChain()
	uFilm := c.unit(center, film, 23901)
	uRoll := c.rollUnit(center, roll, 23902, "50.00")

	svc := it.directService(dealer, cust, veh, 239,
		db.CreateServiceItemParams{ProductID: film.ID, UnitID: uFilm.ID, Kind: "full",
			AppliedParts: []byte(`["body_tavan","body_kaput"]`), Notes: pgtype.Text{String: "staff-only-note", Valid: true}},
		db.CreateServiceItemParams{ProductID: roll.ID, UnitID: uRoll.ID, Kind: "partial", Meters: meters(t, "10")})
	if _, err := warrantymodule.NewListener(it.pool, it.q, frontend, nil).CreateForService(ctx, svc.ID); err != nil {
		t.Fatalf("listener: %v", err)
	}
	ws := it.serviceWarranties(svc.ID, svc.BrandID)
	if len(ws) != 1 {
		t.Fatalf("warranties = %d, want 1", len(ws))
	}

	portalTok := func(u db.User) string {
		tok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: u.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	path := "/v1/portal/services/" + svc.Uuid.String()

	// 1. Detail: the owner gets 200 with parts, products, dealer and
	// warranty; another customer 404 (existence does not leak).
	env := it.accDo("GET", path, portalTok(cust), nil, http.StatusOK)
	d := decodeData[portalServiceDetail](t, env)
	if d.UUID != svc.Uuid.String() || d.ServiceNo != svc.ServiceNo {
		t.Fatalf("detail = %+v", d)
	}
	if strings.Join(d.AppliedParts, ",") != "body_kaput,body_tavan" {
		t.Fatalf("applied parts = %v", d.AppliedParts)
	}
	if len(d.Products) != 2 || d.Products[0].Name != film.Name || d.Products[0].Category == "" ||
		len(d.Products[0].AppliedParts) != 2 || d.Products[1].Name != roll.Name {
		t.Fatalf("products = %+v", d.Products)
	}
	if d.Dealer.Name != dealer.Name || d.Dealer.Address != "T239 Cad. 1" || d.Dealer.WhatsApp == nil || *d.Dealer.WhatsApp != "+905321112239" {
		t.Fatalf("dealer = %+v", d.Dealer)
	}
	if len(d.Warranties) != 1 || d.Warranties[0].PublicCode != ws[0].PublicCode {
		t.Fatalf("warranties = %+v", d.Warranties)
	}
	it.accDo("GET", path, portalTok(other), nil, http.StatusNotFound)
	it.accDo("GET", "/v1/portal/services/00000000-0000-0000-0000-000000000000", portalTok(cust), nil, http.StatusNotFound)

	// 2. No measurement, price or stock field anywhere in the response.
	var raw any
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	jsonKeys(raw, keys)
	for _, k := range portalForbiddenKeys {
		if keys[k] {
			t.Fatalf("portal detail carries forbidden key %q: %s", k, env.Data)
		}
	}
	if strings.Contains(string(env.Data), "staff-only-note") || strings.Contains(string(env.Data), uFilm.Barcode) {
		t.Fatalf("portal detail leaks item notes or barcode: %s", env.Data)
	}

	// 3. PDF: another customer 404; the owner 202 (queued), a second call
	// returns the same running job instead of queueing another one.
	pdfPath := path + "/pdf?locale=tr"
	it.accDo("GET", pdfPath, portalTok(other), nil, http.StatusNotFound)
	job := decodeData[exportJob](t, it.accDo("GET", pdfPath, portalTok(cust), nil, http.StatusAccepted))
	if job.Resource != servicesusecase.ResourcePortalPDF || job.Format != "pdf" || job.Status == "completed" {
		t.Fatalf("portal pdf job = %+v", job)
	}
	again := decodeData[exportJob](t, it.accDo("GET", pdfPath, portalTok(cust), nil, http.StatusAccepted))
	if again.UUID != job.UUID {
		t.Fatalf("running job not reused: %s != %s", again.UUID, job.UUID)
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}

	// 4. The worker renders it through the service PDF (TEC-196) adapter.
	pdfSvc := servicesusecase.NewPDF(servicesusecase.New(it.pool, it.q, nil),
		warrantymodule.NewCertificate(it.q, store, frontend, nil), store, nil)
	worker := exportusecase.New(it.q, store, ioengine.NewRegistry(servicesusecase.NewPortalPDFAdapter(pdfSvc)), nil, nil, nil, nil)
	worker.SetDocumentPDF(pdfrender.New(gotb.URL))
	p, err := queue.ParseExportProcessPayload(tasks[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.ProcessExport(ctx, p.ExportJobID); err != nil {
		t.Fatalf("process export: %v", err)
	}
	h := gotb.LastHTML()
	if !strings.Contains(h, svc.ServiceNo) || !strings.Contains(h, ws[0].PublicCode) || !strings.Contains(h, plate) {
		t.Fatal("portal service pdf misses the service number, warranty or plate")
	}

	// 5. A completed render answers 200 with the portal download link; the
	// owner downloads it, another customer never sees the job.
	done := decodeData[exportJob](t, it.accDo("GET", pdfPath, portalTok(cust), nil, http.StatusOK))
	if done.UUID != job.UUID || done.Status != "completed" || done.DownloadURL == nil ||
		*done.DownloadURL != "/v1/portal/exports/"+job.UUID+"/download" {
		t.Fatalf("completed portal pdf = %+v", done)
	}
	if rec := it.raw("GET", *done.DownloadURL, portalTok(cust), "", nil, nil); rec.Code != http.StatusOK || rec.Body.String() != documentstest.PDF {
		t.Fatalf("portal pdf download = %d", rec.Code)
	}
	it.accDo("GET", "/v1/portal/exports/"+job.UUID, portalTok(other), nil, http.StatusNotFound)
	it.accDo("GET", pdfPath, portalTok(other), nil, http.StatusNotFound)
}
