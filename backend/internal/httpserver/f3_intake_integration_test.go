package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	contractsrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	contractsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-303: F3 gate scenario, end to end through the public HTTP handlers:
// a dealer intake requires an executed contract, before/after NexPTG
// measurements auto-link by VIN, the deviation table is computed, completion
// consumes stock and creates warranties, and the portal exposes the contract
// PDF without exposing measurement fields on the service detail.
func TestIntegrationF3IntakeContractMeasurement(t *testing.T) {
	ctx := context.Background()
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	qredis := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: qredis.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	fw := &fakeWuzapi{}
	wa := httptestServer(t, fw)
	defer wa.Close()

	it := newIntegrationWithDeps(t,
		func(c *config.Config) {
			c.Gotenberg.URL = gotb.URL
			c.Wuzapi = config.WuzapiConfig{URL: wa.URL, AdminToken: itWuzapiAdmin, WebhookSecret: itWebhookKey, WebhookURL: itWebhookURL}
		},
		func(d *Deps) { d.Storage = store; d.Queue = qc },
	)
	it.lockWhatsAppSettings()
	reset := func() { _, _ = it.srv.sysconfig.Reset(ctx, sysconfig.KeyContractsIntakeRequired) }
	reset()
	t.Cleanup(reset)

	center := it.brandCenter("olex")
	dist := it.org("t303-dist", "distributor", center)
	dealer := it.org("t303-dealer", "dealer", dist)
	centerUser, centerPW := it.user("t303-center")
	it.member(center, centerUser, "staff")
	owner, ownerPW := it.user("t303-owner")
	it.member(dealer, owner, "owner")
	centerTok := it.loginOrg(centerUser, centerPW, center)
	dealerTok := it.loginOrg(owner, ownerPW, dealer)

	admin, _ := it.user("t303-admin", rbac.RoleSuperAdmin)
	if _, err := it.srv.sysconfig.Set(ctx, sysconfig.KeyContractsIntakeRequired, json.RawMessage("true"), admin.ID); err != nil {
		t.Fatal(err)
	}

	customer, vehicle := it.svcCustomer(dealer, "t303-cust", "34T303"+it.suffix[len(it.suffix)-4:])
	phone := itPhone()
	if _, err := it.pool.Exec(ctx, `UPDATE users SET phone_e164 = $2, phone_verified_at = NOW() WHERE id = $1`, customer.ID, phone); err != nil {
		t.Fatalf("customer phone: %v", err)
	}
	const vin = "WVWZZZ1JZ3W303001"
	it.custDo("PATCH", "/v1/vehicles/"+vehicle.Uuid.String(), dealerTok, map[string]any{"vin": vin}, http.StatusOK)

	product := it.product(center, "T303P")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET micron_thickness = '190.00'::numeric WHERE id = $1`, product.ID); err != nil {
		t.Fatalf("product micron: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE product_categories SET available_parts = '["body_kaput"]'::jsonb WHERE id = $1`, product.CategoryID); err != nil {
		t.Fatalf("product parts: %v", err)
	}
	it.setWarrantyMonths(product, 24)
	stock := it.stockChain()
	cLoc := stock.location(center, "C303")
	dealerOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	unit := stock.unit(center, product, 30301)
	stock.post(ledger.TypeEntry, unit, stock.nextRef(), cLoc)
	stock.ship(unit, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOwner)
	if got := it.stockOf(dealerTok, dealer, product.Uuid.String()); got != 1 {
		t.Fatalf("dealer stock before service = %d, want 1", got)
	}

	mobile := it.mobileLogin(owner.Email.String, ownerPW, dealer.Slug, "t303-dev").AccessToken
	before := it.uploadMeasurement(mobile, "t303-before-"+it.suffix, vin, time.Now().Add(-2*time.Hour), "100")

	body := map[string]any{"customer_uuid": customer.Uuid.String(), "vehicle_uuid": vehicle.Uuid.String(), "has_measurement": true}
	svc, _ := it.svcCall("POST", "/v1/services", dealerTok, body, http.StatusCreated)
	for _, ev := range it.outboxEventsOf(events.ServiceCreated, svc.UUID) {
		if err := it.srv.events.Publish(ctx, ev); err != nil {
			t.Fatalf("publish service.created: %v", err)
		}
	}
	links := decodeData[serviceMeasurementsBody](t, it.accDo("GET", "/v1/services/"+svc.UUID+"/measurements", dealerTok, nil, http.StatusOK))
	if m, src, confirmed, found := links.link("before"); !found || m != before || src != "auto" || confirmed {
		t.Fatalf("before auto link = %s %s confirmed=%v found=%v", m, src, confirmed, found)
	}
	links = decodeData[serviceMeasurementsBody](t, it.accDo("POST", "/v1/services/"+svc.UUID+"/measurements", dealerTok,
		map[string]any{"measurement_uuid": before, "phase": "before"}, http.StatusOK))
	if _, _, confirmed, _ := links.link("before"); !confirmed {
		t.Fatalf("before not confirmed: %+v", links)
	}
	it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", dealerTok, map[string]string{"status": "pending"}, http.StatusOK)
	if _, ec := it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", dealerTok, map[string]string{"status": "processing"}, http.StatusUnprocessableEntity); ec != "CONTRACT_REQUIRED" {
		t.Fatalf("processing without contract = %s, want CONTRACT_REQUIRED", ec)
	}

	templateUUID := it.contractTemplate(centerTok)
	contract := decodeData[struct {
		UUID    string `json:"uuid"`
		Status  string `json:"status"`
		Signers []struct {
			Role string `json:"role"`
		} `json:"signers"`
	}](t, it.accDo("POST", "/v1/services/"+svc.UUID+"/contract", dealerTok, map[string]any{"template_uuid": templateUUID, "locale": "tr"}, http.StatusCreated))
	if contract.Status != "pending" || contract.UUID == "" || len(contract.Signers) != 2 {
		t.Fatalf("contract = %+v", contract)
	}
	it.accDo("POST", "/v1/contracts/"+contract.UUID+"/signers/customer/otp", dealerTok, nil, http.StatusAccepted)
	code, _ := sentCode(t, fw, phone)
	it.accDo("POST", "/v1/contracts/"+contract.UUID+"/signers/customer/sign", dealerTok,
		map[string]any{"code": code, "signature_png": f3PNGBase64()}, http.StatusOK)
	executed := decodeData[struct {
		UUID       string  `json:"uuid"`
		Status     string  `json:"status"`
		ExecutedAt *string `json:"executed_at"`
	}](t, it.accDo("POST", "/v1/contracts/"+contract.UUID+"/signers/staff/sign", dealerTok,
		map[string]any{"signature_png": f3PNGBase64()}, http.StatusOK))
	if executed.Status != "executed" || executed.ExecutedAt == nil {
		t.Fatalf("executed contract = %+v", executed)
	}
	for _, ev := range it.outboxEventsOf(events.ContractExecuted, contract.UUID) {
		if err := it.srv.events.Publish(ctx, ev); err != nil {
			t.Fatalf("publish contract.executed: %v", err)
		}
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: qredis.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueDocs)
	if err != nil || len(tasks) != 1 || tasks[0].Type != queue.TaskContractPDF {
		t.Fatalf("contract pdf tasks = %d (%v)", len(tasks), err)
	}
	payload, err := queue.ParseContractPDFPayload(tasks[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	pdfSvc := contractsusecase.New(contractsrepo.New(it.pool, it.q),
		contractsusecase.WithStorage(store), contractsusecase.WithPDFRenderer(pdfrender.New(gotb.URL)))
	if err := pdfSvc.GenerateExecutedPDF(ctx, payload.InstanceID); err != nil {
		t.Fatalf("contract pdf: %v", err)
	}
	var pdfKey string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(pdf_key, '') FROM contract_instances WHERE uuid = $1`, contract.UUID).Scan(&pdfKey); err != nil {
		t.Fatal(err)
	}
	if pdfKey == "" || gotb.Calls.Load() != 1 {
		t.Fatalf("pdf_key=%q gotenberg_calls=%d", pdfKey, gotb.Calls.Load())
	}

	processing, _ := it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", dealerTok, map[string]string{"status": "processing"}, http.StatusOK)
	if processing.Status != "processing" {
		t.Fatalf("processing = %+v", processing)
	}
	it.svcCall("POST", "/v1/services/"+svc.UUID+"/items", dealerTok,
		map[string]any{"barcode": unit.Barcode, "kind": "full", "applied_parts": []string{"body_kaput"}}, http.StatusCreated)
	after := it.uploadMeasurement(mobile, "t303-after-"+it.suffix, vin, time.Now().Add(time.Minute), "220")
	links = decodeData[serviceMeasurementsBody](t, it.accDo("GET", "/v1/services/"+svc.UUID+"/measurements", dealerTok, nil, http.StatusOK))
	if m, src, _, found := links.link("after"); !found || m != after || src != "auto" {
		t.Fatalf("after auto link = %s %s found=%v", m, src, found)
	}
	diff := decodeData[struct {
		CheckRequired bool `json:"check_required"`
		Parts         []struct {
			PartType  string  `json:"part_type"`
			DiffUM    *string `json:"diff_um"`
			Deviation bool    `json:"deviation"`
		} `json:"parts"`
	}](t, it.accDo("GET", "/v1/services/"+svc.UUID+"/measurements/diff", dealerTok, nil, http.StatusOK))
	if !diff.CheckRequired || len(diff.Parts) != 1 || diff.Parts[0].PartType != "HOOD" || !diff.Parts[0].Deviation {
		t.Fatalf("diff = %+v", diff)
	}

	done, _ := it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", dealerTok, map[string]string{"status": "completed"}, http.StatusOK)
	if done.Status != "completed" {
		t.Fatalf("completed = %+v", done)
	}
	var svcID int64
	if err := it.pool.QueryRow(ctx, `SELECT id FROM services WHERE uuid = $1`, svc.UUID).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	if got := it.stockOf(dealerTok, dealer, product.Uuid.String()); got != 0 {
		t.Fatalf("dealer stock after service = %d, want 0", got)
	}
	if err := it.srv.events.Publish(ctx, it.outboxEvent(events.ServiceCompleted, svc.UUID)); err != nil {
		t.Fatalf("publish service.completed: %v", err)
	}
	if ws := it.serviceWarranties(svcID, dealer.BrandID); len(ws) != 1 || ws[0].Status != "active" {
		t.Fatalf("warranties = %+v", ws)
	}

	if err := it.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: customer.ID, Slug: rbac.RoleCustomer}); err != nil {
		t.Fatalf("customer role: %v", err)
	}
	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: customer.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
	if err != nil {
		t.Fatal(err)
	}
	contracts := decodeData[portalCountPage](t, it.custDo("GET", "/v1/portal/contracts", portalTok, nil, http.StatusOK))
	if contracts.Total != 1 || len(contracts.Items) != 1 || contracts.Items[0]["contract_uuid"] != contract.UUID || contracts.Items[0]["pdf_ready"] != true {
		t.Fatalf("portal contracts = %+v", contracts)
	}
	if rec := it.raw("GET", "/v1/portal/contracts/"+contract.UUID+"/pdf", portalTok, "", nil, nil); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("portal contract pdf = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	env := it.accDo("GET", "/v1/portal/services/"+svc.UUID, portalTok, nil, http.StatusOK)
	var raw any
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	jsonKeys(raw, keys)
	for _, k := range append(portalForbiddenKeys, "links", "suggestions", "candidates", "phase", "link_source") {
		if keys[k] {
			t.Fatalf("portal service detail carries %q: %s", k, env.Data)
		}
	}
}

func (it *itest) stockOf(token string, org db.Organization, productUUID string) int32 {
	it.t.Helper()
	code, page := it.productStock(token, "/v1/stock/organizations/"+org.Uuid.String()+"/products?product_uuid="+productUUID)
	if code != http.StatusOK {
		it.t.Fatalf("stock of %s = %d", org.Slug, code)
	}
	q, _ := page.qty(productUUID)
	return q
}

func (it *itest) uploadMeasurement(token, key, vin string, at time.Time, value string) string {
	it.t.Helper()
	body := map[string]any{
		"vin": vin, "measured_at": at.UTC().Format(time.RFC3339),
		"device": map[string]any{"serial": "NX-303"},
		"raw": map[string]any{"measurements": []map[string]any{{
			"place_id": "top", "part_type": "HOOD", "value": value, "timestamp": at.UTC().Format(time.RFC3339), "position": 1,
		}}},
	}
	code, env, _ := it.doMobileKey("POST", "/v1/mobile/measurements", token, key, body)
	if code != http.StatusAccepted {
		it.t.Fatalf("measurement upload = %d %s", code, errCode(env))
	}
	var out struct {
		UUID   string `json:"uuid"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		it.t.Fatal(err)
	}
	if out.Status != "accepted" || out.UUID == "" {
		it.t.Fatalf("measurement upload = %+v", out)
	}
	return out.UUID
}

func (it *itest) contractTemplate(token string) string {
	it.t.Helper()
	truth := true
	tpl := decodeData[struct {
		UUID string `json:"uuid"`
	}](it.t, it.accDo("POST", "/v1/platform/contract-templates", token, map[string]any{
		"name": "TEC303 " + it.suffix, "kind": "vehicle_intake", "is_default": truth,
		"otp_required": truth, "signature_required": truth,
	}, http.StatusCreated))
	it.accDo("PUT", "/v1/platform/contract-templates/"+tpl.UUID+"/locales/tr", token, map[string]any{
		"html": `<p>{{service_no}} {{customer_name}} {{staff_name}} {{plate}}</p>`,
	}, http.StatusOK)
	return tpl.UUID
}

func f3PNGBase64() string {
	raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	return base64.StdEncoding.EncodeToString(raw)
}

func httptestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(h)
}
