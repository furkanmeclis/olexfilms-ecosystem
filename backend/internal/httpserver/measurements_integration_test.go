package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-233 acceptance (F2-05a, K28): a mobile upload answers 202 and stores a
// row; the same Idempotency-Key returns the same uuid without a second row;
// an upload without a VIN is vin_pending; another organization's service is
// 404; a panel (web) token is 403 REALM_FORBIDDEN.
func TestIntegrationMobileMeasurements(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	orgA := it.org("t233a", "dealer", center)
	orgB := it.org("t233b", "dealer", center)
	owner, pw := it.user("t233-owner")
	it.member(orgA, owner, "owner")
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_results WHERE organization_id IN ($1, $2)", orgA.ID, orgB.ID)
		_, _ = it.pool.Exec(bg, "DELETE FROM refresh_tokens WHERE user_id = $1", owner.ID)
	})

	service := func(org db.Organization, name, plate string, seq int) db.Service {
		t.Helper()
		cust, veh := it.svcCustomer(org, name, plate)
		svc, err := it.q.CreateService(ctx, db.CreateServiceParams{
			ServiceNo:      fmt.Sprintf("T233-%s-%d", it.suffix[len(it.suffix)-10:], seq),
			OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
			CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
		})
		if err != nil {
			t.Fatalf("service: %v", err)
		}
		return svc
	}
	own := service(orgA, "t233-cust-a", "34T233A", 1)
	foreign := service(orgB, "t233-cust-b", "34T233B", 2)

	tp := it.mobileLogin(owner.Email.String, pw, orgA.Slug, "t233-dev")
	type accepted struct {
		UUID   string `json:"uuid"`
		Status string `json:"status"`
	}
	upload := func(key string, body map[string]any, want int) (accepted, string) {
		t.Helper()
		code, env, _ := it.doMobileKey("POST", "/v1/mobile/measurements", tp.AccessToken, key, body)
		if code != want {
			t.Fatalf("upload = %d %s, want %d", code, errCode(env), want)
		}
		var out accepted
		if code == http.StatusAccepted {
			if err := json.Unmarshal(env.Data, &out); err != nil {
				t.Fatal(err)
			}
		}
		return out, errCode(env)
	}
	count := func() int {
		t.Helper()
		var n int
		if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM measurement_results WHERE organization_id = $1`, orgA.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	raw := map[string]any{"device": "NexPTG", "values": []int{98, 102}}
	key := "t233-key-" + it.suffix

	// 1. A valid upload: 202 and one row with the service, vehicle and VIN.
	first, _ := upload(key, map[string]any{
		"vin": "wvwzzz1jz3w386752", "service_uuid": own.Uuid.String(),
		"device": map[string]any{"serial": "NX-233"}, "raw": raw,
	}, http.StatusAccepted)
	if first.Status != "accepted" || first.UUID == "" || count() != 1 {
		t.Fatalf("first upload = %+v rows %d", first, count())
	}
	var row db.MeasurementResult
	if err := it.pool.QueryRow(ctx, `SELECT service_id, vehicle_id, vin, device_serial, source, created_by, brand_id
		FROM measurement_results WHERE uuid = $1`, first.UUID).Scan(&row.ServiceID, &row.VehicleID, &row.Vin,
		&row.DeviceSerial, &row.Source, &row.CreatedBy, &row.BrandID); err != nil {
		t.Fatal(err)
	}
	if row.ServiceID.Int64 != own.ID || row.VehicleID.Int64 != own.VehicleID || row.Vin.String != "WVWZZZ1JZ3W386752" ||
		row.DeviceSerial.String != "NX-233" || row.Source != "mobile" || row.CreatedBy.Int64 != owner.ID ||
		row.BrandID != orgA.BrandID {
		t.Fatalf("stored row = %+v", row)
	}

	// 2. Same Idempotency-Key: same uuid, still one row.
	again, _ := upload(key, map[string]any{"raw": raw}, http.StatusAccepted)
	if again.UUID != first.UUID || again.Status != "accepted" || count() != 1 {
		t.Fatalf("replay = %+v rows %d", again, count())
	}

	// 3. No VIN: vin_pending.
	pending, _ := upload("", map[string]any{"client_measurement_id": "t233-c-" + it.suffix, "raw": raw}, http.StatusAccepted)
	if pending.Status != "vin_pending" || count() != 2 {
		t.Fatalf("no vin = %+v rows %d", pending, count())
	}

	// 4. Another organization's service: 404, nothing written.
	if _, code := upload("", map[string]any{"vin": "WVWZZZ1JZ3W386752", "service_uuid": foreign.Uuid.String(), "raw": raw},
		http.StatusNotFound); code != "NOT_FOUND" || count() != 2 {
		t.Fatalf("foreign service = %s rows %d", code, count())
	}

	// 5. Validation: raw missing is 400.
	if _, code := upload("", map[string]any{"vin": "WVWZZZ1JZ3W386752"}, http.StatusBadRequest); code != "VALIDATION_ERROR" {
		t.Fatalf("missing raw = %s", code)
	}

	// 6. A panel (web) token is refused.
	panel := it.loginOrg(owner, pw, orgA)
	if code, env, _ := it.doMobile("POST", "/v1/mobile/measurements", panel, "1", map[string]any{"raw": raw}); code != http.StatusForbidden ||
		errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel token = %d %s", code, errCode(env))
	}
}

type measurementPage struct {
	Items []struct {
		UUID         string `json:"uuid"`
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
		Service *struct {
			UUID  string `json:"uuid"`
			Phase string `json:"phase"`
		} `json:"service"`
	} `json:"items"`
	Total int64 `json:"total"`
}

// TEC-295 acceptance: panel reads are scoped by measurements.read. A
// distributor sees its dealer subtree, another distributor does not, and a
// dealer cannot read another dealer's measurement.
func TestIntegrationPanelMeasurementScope(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	distA := it.org("t295-dist-a", "distributor", center)
	distB := it.org("t295-dist-b", "distributor", center)
	dealerA := it.org("t295-dealer-a", "dealer", distA)
	dealerB := it.org("t295-dealer-b", "dealer", distB)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM service_measurements WHERE organization_id IN ($1, $2)", dealerA.ID, dealerB.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM measurement_results WHERE organization_id IN ($1, $2)", dealerA.ID, dealerB.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM measurement_devices WHERE organization_id IN ($1, $2)", dealerA.ID, dealerB.ID)
	})

	distOwnerA, distPWA := it.user("t295-dist-a")
	distOwnerB, distPWB := it.user("t295-dist-b")
	dealerOwnerA, dealerPWA := it.user("t295-dealer-a")
	dealerOwnerB, dealerPWB := it.user("t295-dealer-b")
	it.member(distA, distOwnerA, "owner")
	it.member(distB, distOwnerB, "owner")
	it.member(dealerA, dealerOwnerA, "owner")
	it.member(dealerB, dealerOwnerB, "owner")

	service := func(org db.Organization, name, plate string, seq int) db.Service {
		t.Helper()
		cust, veh := it.svcCustomer(org, name, plate)
		svc, err := it.q.CreateService(ctx, db.CreateServiceParams{
			ServiceNo:      fmt.Sprintf("T295-%s-%d", it.suffix[len(it.suffix)-10:], seq),
			OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
			CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
			Vin: pgtype.Text{String: "WVWZZZ1JZ3W386752", Valid: true},
		})
		if err != nil {
			t.Fatalf("service: %v", err)
		}
		return svc
	}
	deviceA, err := it.q.CreateMeasurementDevice(ctx, db.CreateMeasurementDeviceParams{
		OrganizationID: dealerA.ID, BrandID: dealerA.BrandID, Serial: "NX-295-A", Label: pgtype.Text{String: "A", Valid: true}, IsActive: true,
	})
	if err != nil {
		t.Fatalf("device A: %v", err)
	}
	deviceB, err := it.q.CreateMeasurementDevice(ctx, db.CreateMeasurementDeviceParams{
		OrganizationID: dealerB.ID, BrandID: dealerB.BrandID, Serial: "NX-295-B", Label: pgtype.Text{String: "B", Valid: true}, IsActive: true,
	})
	if err != nil {
		t.Fatalf("device B: %v", err)
	}
	svcA := service(dealerA, "t295-cust-a", "34T295A", 1)
	svcB := service(dealerB, "t295-cust-b", "34T295B", 2)

	insertResult := func(org db.Organization, dev db.MeasurementDevice, svc db.Service, vin string) db.MeasurementResult {
		t.Helper()
		var row db.MeasurementResult
		if err := it.pool.QueryRow(ctx, `
			INSERT INTO measurement_results (
				organization_id, brand_id, service_id, vehicle_id, vin, status, raw,
				device_serial, source, created_by, measured_at, device_id, parsed_at
			) VALUES ($1,$2,$3,$4,$5,'accepted','{"raw":true}'::jsonb,$6,'mobile',$7,$8,$9,$8)
			RETURNING id, uuid, organization_id, brand_id, status
		`, org.ID, org.BrandID, svc.ID, svc.VehicleID, vin, dev.Serial, dealerOwnerA.ID, time.Now(), dev.ID).
			Scan(&row.ID, &row.Uuid, &row.OrganizationID, &row.BrandID, &row.Status); err != nil {
			t.Fatalf("measurement: %v", err)
		}
		return row
	}
	own := insertResult(dealerA, deviceA, svcA, "WVWZZZ1JZ3W386752")
	foreign := insertResult(dealerB, deviceB, svcB, "WVWZZZ1JZ3W386753")
	if _, err := it.q.LinkServiceMeasurement(ctx, db.LinkServiceMeasurementParams{
		OrganizationID: dealerA.ID, BrandID: dealerA.BrandID, ServiceID: svcA.ID,
		MeasurementResultID: own.ID, Phase: "before", LinkSource: "manual",
	}); err != nil {
		t.Fatalf("link: %v", err)
	}

	distTokA := it.loginOrg(distOwnerA, distPWA, distA)
	distTokB := it.loginOrg(distOwnerB, distPWB, distB)
	dealerTokA := it.loginOrg(dealerOwnerA, dealerPWA, dealerA)
	dealerTokB := it.loginOrg(dealerOwnerB, dealerPWB, dealerB)

	code, env := it.do("GET", "/v1/measurements?vin=WVWZZZ1JZ3W386752", hostOlex, distTokA, nil)
	if code != http.StatusOK {
		t.Fatalf("dist A list = %d %s", code, errCode(env))
	}
	var page measurementPage
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].UUID != own.Uuid.String() ||
		page.Items[0].Organization.UUID != dealerA.Uuid.String() {
		t.Fatalf("dist A page = %+v", page)
	}
	code, env = it.do("GET", "/v1/measurements/"+own.Uuid.String(), hostOlex, distTokA, nil)
	if code != http.StatusOK {
		t.Fatalf("dist A detail = %d %s", code, errCode(env))
	}
	var detail struct {
		UUID    string `json:"uuid"`
		Service *struct {
			UUID  string `json:"uuid"`
			Phase string `json:"phase"`
		} `json:"service"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.UUID != own.Uuid.String() || detail.Service == nil || detail.Service.UUID != svcA.Uuid.String() || detail.Service.Phase != "before" {
		t.Fatalf("detail = %+v", detail)
	}

	code, env = it.do("GET", "/v1/measurements/"+own.Uuid.String(), hostOlex, distTokB, nil)
	if code != http.StatusNotFound {
		t.Fatalf("other dist detail = %d %s, foreign=%s", code, errCode(env), foreign.Uuid)
	}
	code, env = it.do("GET", "/v1/measurements/"+own.Uuid.String(), hostOlex, dealerTokB, nil)
	if code != http.StatusNotFound {
		t.Fatalf("other dealer detail = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/measurements/"+own.Uuid.String(), hostOlex, dealerTokA, nil)
	if code != http.StatusOK {
		t.Fatalf("own dealer detail = %d %s", code, errCode(env))
	}
}

// doMobileKey is doMobile (version 1) with an optional Idempotency-Key.
func (it *itest) doMobileKey(method, path, bearer, key string, body any) (int, envelope, http.Header) {
	it.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OlexMobile/1.0")
	req.Header.Set("X-Mobile-Api-Version", "1")
	req.Header.Set("Authorization", "Bearer "+bearer)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env, rec.Header()
}
