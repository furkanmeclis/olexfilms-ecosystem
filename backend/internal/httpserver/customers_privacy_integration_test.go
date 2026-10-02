package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type dataExportDoc struct {
	Version int `json:"version"`
	Profile struct {
		UUID            string  `json:"uuid"`
		Name            string  `json:"name"`
		Surname         string  `json:"surname"`
		Email           *string `json:"email"`
		Phone           *string `json:"phone"`
		Status          string  `json:"status"`
		Anonymized      bool    `json:"anonymized"`
		NationalIDLast4 *string `json:"national_id_last4"`
	} `json:"profile"`
	Vehicles []struct {
		UUID  string  `json:"uuid"`
		Plate *string `json:"plate"`
		VIN   *string `json:"vin"`
	} `json:"vehicles"`
	Services   []json.RawMessage `json:"services"`
	Warranties []json.RawMessage `json:"warranties"`
}

type anonymizeResult struct {
	UUID    string `json:"uuid"`
	Status  string `json:"status"`
	Changed bool   `json:"changed"`
}

// TEC-161 acceptance: the center exports a customer's data (JSON) and the
// customer exports their own (PDF, mock Gotenberg, RTL); anonymization keeps
// the vehicle and plate queryable with the personal data masked, a second
// anonymization changes nothing, the anonymized user cannot sign in, and
// every operation is audited.
func TestIntegrationCustomerPrivacy(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t,
		func(c *config.Config) {
			c.Encryption.CustomerPIIKey = itCustomerPIIKey
			c.Gotenberg.URL = gotb.URL
		},
		func(d *Deps) { d.Storage = store; d.Queue = qc })
	ctx := context.Background()

	center := it.brandCenter("olex")
	dist := it.org("t161-dist", "distributor", center)
	dealer := it.org("t161-dealer", "dealer", dist)
	var phones []string
	it.cleanupCustomers(&phones, dealer, dist)
	var custUUID string
	t.Cleanup(func() {
		// The anonymized user has no phone any more: clean up by uuid.
		if custUUID == "" {
			return
		}
		_, _ = it.pool.Exec(ctx, `DELETE FROM export_jobs WHERE actor_id IN (SELECT id FROM users WHERE uuid = $1)
			OR resource IN ($2, $3) AND query_json->>'customer_uuid' = $1`, custUUID,
			customersusecase.ResourceDataExport, customersusecase.ResourcePortalDataExport)
		_, _ = it.pool.Exec(ctx, `DELETE FROM vehicles WHERE user_id IN (SELECT id FROM users WHERE uuid = $1)`, custUUID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM customer_organizations WHERE user_id IN (SELECT id FROM users WHERE uuid = $1)`, custUUID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id IN (SELECT id FROM users WHERE uuid = $1)`, custUUID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM users WHERE uuid = $1`, custUUID)
	})

	owner, pwO := it.user("t161-owner")
	it.member(dealer, owner, "owner")
	staff, pwC := it.user("t161-center")
	it.member(center, staff, "staff")
	tokO := it.loginOrg(owner, pwO, dealer)
	tokC := it.loginOrg(staff, pwC, center)

	// A dealer customer with identity number, address and a vehicle.
	ph := itPhone()
	phones = append(phones, ph)
	email := fmt.Sprintf("t161-%s@example.test", it.suffix)
	cust := decodeData[custView](t, it.custDo("POST", "/v1/customers", tokO, map[string]any{
		"phone": ph, "name": "Ayşe", "surname": "Kaya", "email": email,
		"national_id": "12345678901", "type": "individual",
		"address": map[string]any{"line1": "Gizli Sokak 7", "city": "İstanbul"},
	}, http.StatusCreated))
	custUUID = cust.UUID
	veh := decodeData[vehView](t, it.custDo("POST", "/v1/vehicles", tokO, map[string]any{
		"customer_uuid": cust.UUID, "plate": "34 TEC 161", "vin": "WVWZZZ1JZXW000161",
	}, http.StatusCreated))

	anonPath := "/v1/customers/" + cust.UUID + "/anonymize"
	exportPath := "/v1/customers/" + cust.UUID + "/data-export"

	// 1. Permissions: a dealer cannot anonymize or export; the center needs
	// a fresh step-up.
	it.custDo("POST", anonPath, tokO, nil, http.StatusForbidden)
	it.custDo("POST", exportPath, tokO, map[string]any{"format": "json"}, http.StatusForbidden)
	if code, env := it.do("POST", anonPath, hostOlex, tokC, nil); code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("anonymize without step-up: %d %s", code, errCode(env))
	}
	it.stepUp(staff.Uuid)

	// 2. Export requests: center JSON, portal PDF (the customer's own data).
	it.custDo("POST", exportPath, tokC, map[string]any{"format": "csv"}, http.StatusBadRequest)
	jsonJob := decodeData[exportJob](t, it.custDo("POST", exportPath, tokC, map[string]any{"format": "json"}, http.StatusAccepted))
	if jsonJob.Resource != customersusecase.ResourceDataExport || jsonJob.Status == "completed" {
		t.Fatalf("json job = %+v", jsonJob)
	}
	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: uuid.MustParse(cust.UUID), Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, env := it.do("POST", "/v1/portal/me/data-export", hostOlex, tokC, map[string]any{"format": "pdf"}); code != http.StatusForbidden ||
		errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel token on portal export: %d %s", code, errCode(env))
	}
	pdfJob := decodeData[exportJob](t, it.custDo("POST", "/v1/portal/me/data-export", portalTok,
		map[string]any{"format": "pdf", "locale": "ar"}, http.StatusAccepted))
	if pdfJob.Resource != customersusecase.ResourcePortalDataExport {
		t.Fatalf("pdf job = %+v", pdfJob)
	}

	// 3. Both jobs are on the exports queue; a worker with a mock Gotenberg
	// renders them.
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
	custSvc := customersusecase.New(it.pool, it.q, nil, nil)
	reg := ioengine.NewRegistry(customersusecase.NewDataExportAdapter(custSvc), customersusecase.NewPortalDataExportAdapter(custSvc))
	worker := exportusecase.New(it.q, store, reg, nil, nil, nil, nil)
	worker.SetDocumentPDF(pdfrender.New(gotb.URL))
	for _, task := range tasks {
		p, err := queue.ParseExportProcessPayload(task.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessExport(ctx, p.ExportJobID); err != nil {
			t.Fatalf("process export %d: %v", p.ExportJobID, err)
		}
	}
	if gotb.Calls.Load() < 1 {
		t.Fatal("gotenberg was not called for the pdf export")
	}
	if h := gotb.LastHTML(); !strings.Contains(h, `dir="rtl"`) || !strings.Contains(h, "34 TEC 161") || !strings.Contains(h, "8901") ||
		strings.Contains(h, "12345678901") {
		t.Fatal("portal pdf document missing rtl, plate or masked id (or leaks the full id)")
	}

	// 4. JSON content (center download).
	done := decodeData[exportJob](t, it.custDo("GET", "/v1/customer-data-exports/"+jsonJob.UUID, tokC, nil, http.StatusOK))
	if done.Status != "completed" || done.DownloadURL == nil || *done.DownloadURL != "/v1/customer-data-exports/"+jsonJob.UUID+"/download" {
		t.Fatalf("json job done = %+v", done)
	}
	it.custDo("GET", "/v1/customer-data-exports/"+jsonJob.UUID, tokO, nil, http.StatusForbidden)
	rec := it.raw("GET", *done.DownloadURL, tokC, "", nil, nil)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("json download = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("12345678901")) {
		t.Fatal("full national id leaked in the export")
	}
	var doc dataExportDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("json export: %v", err)
	}
	for _, key := range []string{`"profile"`, `"vehicles"`, `"services"`, `"warranties"`, `"exported_at"`} {
		if !bytes.Contains(rec.Body.Bytes(), []byte(key)) {
			t.Fatalf("json export misses %s", key)
		}
	}
	if doc.Version != 1 || doc.Profile.UUID != cust.UUID || doc.Profile.Name != "Ayşe" || doc.Profile.Phone == nil || *doc.Profile.Phone != ph ||
		doc.Profile.Email == nil || *doc.Profile.Email != email || doc.Profile.NationalIDLast4 == nil || *doc.Profile.NationalIDLast4 != "8901" ||
		len(doc.Vehicles) != 1 || doc.Vehicles[0].UUID != veh.UUID || doc.Vehicles[0].VIN == nil || *doc.Vehicles[0].VIN != "WVWZZZ1JZXW000161" ||
		doc.Services == nil || doc.Warranties == nil {
		t.Fatalf("json export content = %+v", doc)
	}

	// 5. Portal download: own job only.
	pdone := decodeData[exportJob](t, it.custDo("GET", "/v1/portal/exports/"+pdfJob.UUID, portalTok, nil, http.StatusOK))
	if pdone.Status != "completed" || pdone.DownloadURL == nil || *pdone.DownloadURL != "/v1/portal/exports/"+pdfJob.UUID+"/download" {
		t.Fatalf("pdf job done = %+v", pdone)
	}
	if rec := it.raw("GET", *pdone.DownloadURL, portalTok, "", nil, nil); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "application/pdf" || rec.Body.String() != documentstest.PDF {
		t.Fatalf("pdf download = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	it.custDo("GET", "/v1/portal/exports/"+jsonJob.UUID, portalTok, nil, http.StatusNotFound)

	// 6. Anonymize. A live refresh token of the customer must be revoked.
	rawRefresh := "t161-refresh-" + it.suffix
	sum := sha256.Sum256([]byte(rawRefresh))
	if _, err := it.pool.Exec(ctx, `INSERT INTO refresh_tokens (user_id, token_hash, expires_at, realm)
		SELECT id, $2, NOW() + interval '1 hour', 'portal' FROM users WHERE uuid = $1`, cust.UUID, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	res := decodeData[anonymizeResult](t, it.custDo("POST", anonPath, tokC, nil, http.StatusOK))
	if !res.Changed || res.Status != "anonymized" || res.UUID != cust.UUID {
		t.Fatalf("anonymize = %+v", res)
	}
	var (
		name, surname, status, mail string
		phone                       *string
		updatedAt                   time.Time
	)
	if err := it.pool.QueryRow(ctx, `SELECT name, surname, status, email, phone_e164, updated_at FROM users WHERE uuid = $1`, cust.UUID).
		Scan(&name, &surname, &status, &mail, &phone, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if name != "Anonim" || surname != "" || status != "anonymized" || phone != nil ||
		mail != "anonymized+"+cust.UUID+"@anonymized.invalid" {
		t.Fatalf("users row not anonymized: %s %s %s %s %v", name, surname, status, mail, phone)
	}
	var encNil, last4Nil bool
	var address string
	var anonAt time.Time
	if err := it.pool.QueryRow(ctx, `SELECT p.national_id_enc IS NULL, p.national_id_last4 IS NULL, p.address::text, p.anonymized_at
		FROM customer_profiles p JOIN users u ON u.id = p.user_id WHERE u.uuid = $1`, cust.UUID).Scan(&encNil, &last4Nil, &address, &anonAt); err != nil {
		t.Fatal(err)
	}
	if !encNil || !last4Nil || address != "{}" {
		t.Fatalf("profile not cleared: enc_nil=%v last4_nil=%v address=%s", encNil, last4Nil, address)
	}

	// Vehicle and plate are still queryable; personal data is masked.
	v := decodeData[vehView](t, it.custDo("GET", "/v1/vehicles/"+veh.UUID, tokO, nil, http.StatusOK))
	if v.Plate == nil || *v.Plate == "" || v.VIN == nil || *v.VIN != "WVWZZZ1JZXW000161" || v.CustomerUUID != cust.UUID {
		t.Fatalf("vehicle after anonymization = %+v", v)
	}
	byPlate := decodeData[vehPage](t, it.custDo("GET", "/v1/vehicles?plate=34TEC161", tokO, nil, http.StatusOK))
	if byPlate.Total != 1 || len(byPlate.Items) != 1 || byPlate.Items[0].UUID != veh.UUID {
		t.Fatalf("plate lookup after anonymization = %+v", byPlate)
	}
	c := decodeData[custView](t, it.custDo("GET", "/v1/customers/"+cust.UUID, tokO, nil, http.StatusOK))
	if !c.Anonymized || c.Name == "Ayşe" || c.Surname != "" || c.Phone != nil || c.Email != nil || c.NationalIDLast4 != nil {
		t.Fatalf("customer after anonymization not masked: %+v", c)
	}

	// 7. Second anonymization: 200, changed=false, no write.
	res2 := decodeData[anonymizeResult](t, it.custDo("POST", anonPath, tokC, nil, http.StatusOK))
	if res2.Changed || res2.Status != "anonymized" {
		t.Fatalf("second anonymize = %+v", res2)
	}
	var updatedAt2, anonAt2 time.Time
	if err := it.pool.QueryRow(ctx, `SELECT u.updated_at, p.anonymized_at FROM users u JOIN customer_profiles p ON p.user_id = u.id
		WHERE u.uuid = $1`, cust.UUID).Scan(&updatedAt2, &anonAt2); err != nil {
		t.Fatal(err)
	}
	if !updatedAt2.Equal(updatedAt) || !anonAt2.Equal(anonAt) {
		t.Fatal("second anonymization changed the rows")
	}

	// 8. The anonymized user cannot sign in: live access token, refresh
	// token and OTP sign-in are all refused (the phone is gone).
	if code, _ := it.do("GET", "/v1/auth/me", hostOlex, portalTok, nil); code != http.StatusUnauthorized {
		t.Fatalf("access token after anonymization: %d", code)
	}
	if code, _ := it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": rawRefresh}); code == http.StatusOK {
		t.Fatal("refresh token still works after anonymization")
	}
	var liveTokens int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM refresh_tokens r JOIN users u ON u.id = r.user_id
		WHERE u.uuid = $1 AND r.revoked_at IS NULL`, cust.UUID).Scan(&liveTokens); err != nil || liveTokens != 0 {
		t.Fatalf("live refresh tokens after anonymization = %d (%v)", liveTokens, err)
	}
	var phoneOwner int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE phone_e164 = $1`, ph).Scan(&phoneOwner); err != nil || phoneOwner != 0 {
		t.Fatalf("phone still attached to a user: %d (%v)", phoneOwner, err)
	}
	// Anonymized customers are read-only.
	if code, env := it.do("PATCH", "/v1/customers/"+cust.UUID, hostOlex, tokO, map[string]any{"name": "X"}); code != http.StatusConflict ||
		errCode(env) != "CUSTOMER_ANONYMIZED" {
		t.Fatalf("patch anonymized: %d %s", code, errCode(env))
	}

	// 9. Audit rows: two anonymizations (changed true/false), two export
	// requests, one download per channel. No personal data in payloads.
	rows, err := it.pool.Query(ctx, `SELECT action, payload::text FROM activity_events
		WHERE resource = 'customers' AND resource_uuid = $1 ORDER BY id`, cust.UUID)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	var payloads []string
	for rows.Next() {
		var action, payload string
		if err := rows.Scan(&action, &payload); err != nil {
			t.Fatal(err)
		}
		counts[action]++
		payloads = append(payloads, payload)
	}
	rows.Close()
	if counts[customersusecase.ActionAnonymized] != 2 || counts[customersusecase.ActionDataExportRequested] != 2 {
		t.Fatalf("audit rows = %v", counts)
	}
	for _, p := range payloads {
		if strings.Contains(p, "Ayşe") || strings.Contains(p, ph) || strings.Contains(p, email) {
			t.Fatalf("audit payload carries personal data: %s", p)
		}
	}
	var downloads int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events WHERE action = 'customers.data_export_downloaded'
		AND resource_uuid IN ($1::uuid, $2::uuid)`, jsonJob.UUID, pdfJob.UUID).Scan(&downloads); err != nil || downloads != 2 {
		t.Fatalf("download audit rows = %d (%v)", downloads, err)
	}
}
