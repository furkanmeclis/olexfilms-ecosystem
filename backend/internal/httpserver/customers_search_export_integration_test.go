package httpserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-164 acceptance: a new customer writes customer.created (portal link,
// no phone in the payload) once; linking the same phone at another dealer
// writes none. The list keeps answering q without Meilisearch (SQL
// fallback). The list export runs through the I/O engine with the list's
// scope: dealer A's file holds only dealer A's customers, another
// organization cannot read the job, and the worker refuses a stored scope
// outside the job organization.
func TestIntegrationCustomerSearchExportNotify(t *testing.T) {
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t,
		func(c *config.Config) {
			c.Encryption.CustomerPIIKey = itCustomerPIIKey
			c.Auth.FrontendURL = "http://localhost:3000"
		},
		func(d *Deps) { d.Storage = store; d.Queue = qc })
	ctx := context.Background()

	center := it.brandCenter("olex")
	dist := it.org("t164-dist", "distributor", center)
	dealerA := it.org("t164-a", "dealer", dist)
	dealerB := it.org("t164-b", "dealer", dist)
	var phones []string
	it.cleanupCustomers(&phones, dealerA, dealerB, dist)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM export_jobs WHERE resource = $1 AND organization_id = ANY($2)`,
			customersusecase.ResourceListExport, []int64{dealerA.ID, dealerB.ID})
	})

	ownerA, pwA := it.user("t164-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, pwB := it.user("t164-owner-b")
	it.member(dealerB, ownerB, "owner")
	tokA := it.loginOrg(ownerA, pwA, dealerA)
	tokB := it.loginOrg(ownerB, pwB, dealerB)

	// 1. customer.created once per new customer, with the portal link.
	phShared, phB := itPhone(), itPhone()
	phones = append(phones, phShared, phB)
	custA := decodeData[custView](t, it.custDo("POST", "/v1/customers", tokA, map[string]any{
		"phone": phShared, "name": "Tecaltmis", "surname": "Dort", "locale": "tr",
	}, http.StatusCreated))
	it.custDo("POST", "/v1/customers", tokB, map[string]any{"phone": phShared, "name": "Tecaltmis", "surname": "Dort"}, http.StatusOK)
	custB := decodeData[custView](t, it.custDo("POST", "/v1/customers", tokB, map[string]any{
		"phone": phB, "name": "Yalnizb", "surname": "Bayi",
	}, http.StatusCreated))
	var n int
	var payload string
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*), MIN(payload::text) FROM outbox_events
		WHERE event_name = $1 AND payload->>'entity_uuid' = $2`, events.CustomerCreated, custA.UUID).Scan(&n, &payload); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("customer.created events = %d, want 1 (the second dealer only links)", n)
	}
	if !strings.Contains(payload, "http://localhost:3000/portal") || strings.Contains(payload, phShared) {
		t.Fatalf("event payload must carry the portal link and no phone: %s", payload)
	}

	// 2. q without Meilisearch: SQL fallback, scope kept.
	page := decodeData[custPage](t, it.custDo("GET", "/v1/customers?q=Tecaltmis", tokA, nil, http.StatusOK))
	if !hasUUID(page.Items, custA.UUID) || hasUUID(page.Items, custB.UUID) {
		t.Fatalf("q search of dealer A = %+v", page.Items)
	}
	page = decodeData[custPage](t, it.custDo("GET", "/v1/customers?q=Yalnizb", tokA, nil, http.StatusOK))
	if hasUUID(page.Items, custB.UUID) {
		t.Fatal("dealer A found dealer B's customer")
	}

	// 3. List export (CSV) of dealer A.
	if code, env := it.do("POST", "/v1/customers/export", hostOlex, tokA, map[string]any{"format": "docx"}); code != http.StatusBadRequest &&
		code != http.StatusUnprocessableEntity {
		t.Fatalf("bad format: %d %s", code, errCode(env))
	}
	job := decodeData[exportJob](t, it.custDo("POST", "/v1/customers/export", tokA,
		map[string]any{"format": "csv", "locale": "en"}, http.StatusAccepted))
	if job.Resource != customersusecase.ResourceListExport {
		t.Fatalf("job = %+v", job)
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
	custSvc := customersusecase.New(it.pool, it.q, nil, nil)
	adapter := customersusecase.NewListExportAdapter(custSvc)
	worker := exportusecase.New(it.q, store, ioengine.NewRegistry(adapter), nil, nil, nil, nil)
	p, err := queue.ParseExportProcessPayload(tasks[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.ProcessExport(ctx, p.ExportJobID); err != nil {
		t.Fatalf("process export: %v", err)
	}
	done := decodeData[exportJob](t, it.custDo("GET", "/v1/customer-list-exports/"+job.UUID, tokA, nil, http.StatusOK))
	if done.Status != "completed" || done.DownloadURL == nil || *done.DownloadURL != "/v1/customer-list-exports/"+job.UUID+"/download" {
		t.Fatalf("job done = %+v", done)
	}
	it.custDo("GET", "/v1/customer-list-exports/"+job.UUID, tokB, nil, http.StatusNotFound)
	rec := it.raw("GET", *done.DownloadURL, tokA, "", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Tecaltmis Dort") || strings.Contains(body, "Yalnizb") {
		t.Fatalf("dealer A export must hold only its customers:\n%s", body)
	}

	// 4. The worker re-authorizes the stored scope: dealer B's organization
	// is outside a dealer A job, the distributor job covers both.
	q := ioengine.ExportQuery{
		ioengine.QueryOrganizationID: strconv.FormatInt(dealerA.ID, 10),
		customersusecase.QueryScope:  strconv.FormatInt(dealerB.ID, 10),
	}
	if _, err := adapter.Export(ctx, q, i18n.Locale("en")); err == nil {
		t.Fatal("dealer A job with dealer B scope must fail")
	}
	q = ioengine.ExportQuery{
		ioengine.QueryOrganizationID: strconv.FormatInt(dist.ID, 10),
		customersusecase.QueryScope:  customersusecase.ScopeBrand, // not a center: falls back to the subtree
	}
	ds, err := adapter.Export(ctx, q, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range ds.Rows {
		names[r["name"].(string)] = true
	}
	if !names["Tecaltmis Dort"] || !names["Yalnizb Bayi"] {
		t.Fatalf("distributor export rows = %v", ds.Rows)
	}
}
