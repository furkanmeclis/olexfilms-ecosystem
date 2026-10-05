package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	measurementsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-298 acceptance (F3-02f): GET /v1/measurements/{uuid}/pdf queues the
// worker-docs measurement:pdf task once (202 pending); after the task the
// stored PDF is streamed from pdf_key without another Gotenberg call.
// Another dealer is 404, a vin_pending measurement 422 and a portal
// session 403 (no portal route).
func TestIntegrationMeasurementPDF(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t,
		func(c *config.Config) { c.Gotenberg.URL = gotb.URL },
		func(d *Deps) { d.Storage = store; d.Queue = qc })
	ctx := context.Background()
	center := it.brandCenter("olex")
	orgA := it.org("t298a", "dealer", center)
	orgB := it.org("t298b", "dealer", center)
	owner, pw := it.user("t298-owner")
	other, otherPW := it.user("t298-other")
	it.member(orgA, owner, "owner")
	it.member(orgB, other, "owner")
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_results WHERE organization_id IN ($1, $2)", orgA.ID, orgB.ID)
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_devices WHERE organization_id IN ($1, $2)", orgA.ID, orgB.ID)
		_, _ = it.pool.Exec(bg, "DELETE FROM refresh_tokens WHERE user_id IN ($1, $2)", owner.ID, other.ID)
	})

	raw, err := os.ReadFile("../modules/measurements/usecase/testdata/nexptg_mobile.json")
	if err != nil {
		t.Fatal(err)
	}
	tp := it.mobileLogin(owner.Email.String, pw, orgA.Slug, "t298-dev")
	upload := func(id, vin string) string {
		t.Helper()
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		body["client_measurement_id"] = id + it.suffix
		if vin != "" {
			body["vin"] = vin
		}
		code, env, _ := it.doMobileKey("POST", "/v1/mobile/measurements", tp.AccessToken, "", body)
		if code != http.StatusAccepted {
			t.Fatalf("upload = %d %s", code, errCode(env))
		}
		var out struct {
			UUID string `json:"uuid"`
		}
		_ = json.Unmarshal(env.Data, &out)
		return out.UUID
	}
	accepted := upload("t298-a-", "WVWZZZ3CZKE298002")
	pending := upload("t298-b-", "")

	get := func(tok, id string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "/v1/measurements/"+id+"/pdf", nil)
		req.Host = "backend:8080"
		req.Header.Set("X-Forwarded-Host", hostOlex)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		it.handler.ServeHTTP(rec, req)
		return rec
	}
	code := func(rec *httptest.ResponseRecorder) string {
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return errCode(env)
	}
	tokA := it.loginOrg(owner, pw, orgA)
	tokB := it.loginOrg(other, otherPW, orgB)

	// 1. Rules: another dealer 404, vin_pending 422, portal session 403.
	if rec := get(tokB, accepted); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign = %d", rec.Code)
	}
	if rec := get(tokA, pending); rec.Code != http.StatusUnprocessableEntity || code(rec) != "MEASUREMENT_VIN_PENDING" {
		t.Fatalf("vin_pending = %d %s", rec.Code, code(rec))
	}
	portal, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: owner.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
	if err != nil {
		t.Fatal(err)
	}
	if rec := get(portal, accepted); rec.Code != http.StatusForbidden {
		t.Fatalf("portal = %d %s", rec.Code, code(rec))
	}

	// 2. The first requests queue one worker-docs task.
	for range 2 {
		rec := get(tokA, accepted)
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var out struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(env.Data, &out)
		if rec.Code != http.StatusAccepted || out.Status != "pending" {
			t.Fatalf("pending = %d %s", rec.Code, rec.Body.String())
		}
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueDocs)
	if err != nil || len(tasks) != 1 || tasks[0].Type != queue.TaskMeasurementPDF {
		t.Fatalf("pending docs tasks = %+v (%v)", tasks, err)
	}
	payload, err := queue.ParseMeasurementPDFPayload(tasks[0].Payload)
	if err != nil {
		t.Fatal(err)
	}

	// 3. worker-docs renders it; the request streams the stored PDF.
	worker := measurementsusecase.NewPDF(it.pool, it.q, store, pdfrender.New(gotb.URL), nil, pdfrender.FontsSystem, nil)
	if err := worker.GeneratePDF(ctx, payload.ResultID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		rec := get(tokA, accepted)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || rec.Body.String() != documentstest.PDF {
			t.Fatalf("ready = %d %s", rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	if n := gotb.Calls.Load(); n != 1 {
		t.Fatalf("gotenberg calls = %d, want 1", n)
	}
	var key string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(pdf_key, '') FROM measurement_results WHERE uuid = $1`, accepted).Scan(&key); err != nil || key == "" {
		t.Fatalf("pdf_key = %q %v", key, err)
	}
}
