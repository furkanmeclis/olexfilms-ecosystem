package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// The fake store applies the same ownership / brand / Glorian / draft rules
// as the SQL (internal/database/queries/portal_vehicles.sql), so the tests
// pin that the handler scopes every read to the session user and brand.

const (
	brandOlex    int64 = 1
	brandGlorian int64 = 2
	userA        int64 = 10
	userB        int64 = 20
)

type fVehicle struct {
	id, user, brand int64
	uuid            uuid.UUID
	plate           string
}

type fService struct {
	id, user, brand, vehicle int64
	org                      string
	status                   string
	created                  time.Time
	contract                 bool // TEC-245: services.contract_id set
}

type fWarranty struct {
	user, brand, vehicle int64
	status               string
	start, end           time.Time
}

type fakeStore struct {
	vehicles   []fVehicle
	services   []fService
	warranties []fWarranty
}

func glorian(brand int64) bool { return brand == brandGlorian }

func (f *fakeStore) ownVehicles(user, brand int64) []fVehicle {
	var out []fVehicle
	for _, v := range f.vehicles {
		if v.user == user && v.brand == brand && !glorian(v.brand) {
			out = append(out, v)
		}
	}
	return out
}

func (f *fakeStore) ownServices(user, brand int64, vehicle pgtype.Int8) []fService {
	var out []fService
	for _, s := range f.services {
		if s.user == user && s.brand == brand && s.status != "draft" && !glorian(s.brand) &&
			(!vehicle.Valid || s.vehicle == vehicle.Int64) {
			out = append(out, s)
		}
	}
	return out
}

func (f *fakeStore) ListPortalVehicles(_ context.Context, a db.ListPortalVehiclesParams) ([]db.ListPortalVehiclesRow, error) {
	rows := []db.ListPortalVehiclesRow{}
	for _, v := range f.ownVehicles(a.UserID, a.BrandID) {
		rows = append(rows, db.ListPortalVehiclesRow{
			Uuid: v.uuid, Plate: pgtype.Text{String: v.plate, Valid: true},
			ServiceCount: int64(len(f.ownServices(a.UserID, a.BrandID, pgtype.Int8{Int64: v.id, Valid: true}))),
		})
	}
	return rows, nil
}

func (f *fakeStore) CountPortalVehicles(_ context.Context, a db.CountPortalVehiclesParams) (int64, error) {
	return int64(len(f.ownVehicles(a.UserID, a.BrandID))), nil
}

func (f *fakeStore) GetPortalVehicle(_ context.Context, a db.GetPortalVehicleParams) (db.GetPortalVehicleRow, error) {
	for _, v := range f.ownVehicles(a.UserID, a.BrandID) {
		if v.uuid == a.Uuid {
			return db.GetPortalVehicleRow{ID: v.id, Uuid: v.uuid, Plate: pgtype.Text{String: v.plate, Valid: true}}, nil
		}
	}
	return db.GetPortalVehicleRow{}, pgx.ErrNoRows
}

func (f *fakeStore) GetPortalVehicleServiceSummary(_ context.Context, a db.GetPortalVehicleServiceSummaryParams) (db.GetPortalVehicleServiceSummaryRow, error) {
	rows := f.ownServices(a.UserID, a.BrandID, pgtype.Int8{Int64: a.VehicleID, Valid: true})
	orgs := map[string]bool{}
	var done int64
	for _, s := range rows {
		orgs[s.org] = true
		if s.status == "completed" {
			done++
		}
	}
	return db.GetPortalVehicleServiceSummaryRow{Total: int64(len(rows)), Completed: done, OrganizationCount: int64(len(orgs))}, nil
}

func (f *fakeStore) ListPortalVehicleActiveWarranties(_ context.Context, a db.ListPortalVehicleActiveWarrantiesParams) ([]db.ListPortalVehicleActiveWarrantiesRow, error) {
	rows := []db.ListPortalVehicleActiveWarrantiesRow{}
	for _, w := range f.warranties {
		if w.user == a.UserID && w.brand == a.BrandID && w.vehicle == a.VehicleID && w.status == "active" &&
			w.end.After(a.Now.Time) && !glorian(w.brand) {
			rows = append(rows, db.ListPortalVehicleActiveWarrantiesRow{
				Uuid: uuid.New(), StartAt: pgtype.Timestamptz{Time: w.start, Valid: true},
				EndAt: pgtype.Timestamptz{Time: w.end, Valid: true}, ProductName: "PPF",
			})
		}
	}
	return rows, nil
}

func (f *fakeStore) ListPortalServices(_ context.Context, a db.ListPortalServicesParams) ([]db.ListPortalServicesRow, error) {
	rows := []db.ListPortalServicesRow{}
	for _, s := range f.ownServices(a.UserID, a.BrandID, a.VehicleID) {
		rows = append(rows, db.ListPortalServicesRow{
			Uuid: uuid.New(), ServiceNo: s.org + "-svc", Status: s.status, OrganizationName: s.org,
			CreatedAt: pgtype.Timestamptz{Time: s.created, Valid: true},
		})
	}
	return rows, nil
}

func (f *fakeStore) CountPortalServices(_ context.Context, a db.CountPortalServicesParams) (int64, error) {
	return int64(len(f.ownServices(a.UserID, a.BrandID, a.VehicleID))), nil
}

func (f *fakeStore) ownContracts(user, brand int64) []fService {
	var out []fService
	for _, s := range f.ownServices(user, brand, pgtype.Int8{}) {
		if s.contract {
			out = append(out, s)
		}
	}
	return out
}

func (f *fakeStore) ListPortalContracts(_ context.Context, a db.ListPortalContractsParams) ([]db.ListPortalContractsRow, error) {
	rows := []db.ListPortalContractsRow{}
	for _, s := range f.ownContracts(a.UserID, a.BrandID) {
		rows = append(rows, db.ListPortalContractsRow{
			Uuid: uuid.New(), ServiceNo: s.org + "-svc", Status: s.status, OrganizationName: s.org,
			CreatedAt: pgtype.Timestamptz{Time: s.created, Valid: true},
		})
	}
	return rows, nil
}

func (f *fakeStore) CountPortalContracts(_ context.Context, a db.CountPortalContractsParams) (int64, error) {
	return int64(len(f.ownContracts(a.UserID, a.BrandID))), nil
}

var (
	vehA  = uuid.New()
	vehB  = uuid.New()
	vehAG = uuid.New() // user A's Glorian vehicle
	now   = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
)

func fixture() *fakeStore {
	return &fakeStore{
		vehicles: []fVehicle{
			{id: 1, user: userA, brand: brandOlex, uuid: vehA, plate: "34ABC1"},
			{id: 2, user: userB, brand: brandOlex, uuid: vehB, plate: "06XYZ2"},
			{id: 3, user: userA, brand: brandGlorian, uuid: vehAG, plate: "35GLR3"},
		},
		services: []fService{
			{id: 1, user: userA, brand: brandOlex, vehicle: 1, org: "dealer-a", status: "completed", created: now.AddDate(0, -2, 0)},
			{id: 2, user: userA, brand: brandOlex, vehicle: 1, org: "dealer-b", status: "processing", created: now.AddDate(0, -1, 0)},
			{id: 3, user: userA, brand: brandOlex, vehicle: 1, org: "dealer-b", status: "draft", created: now},
			{id: 4, user: userB, brand: brandOlex, vehicle: 2, org: "dealer-a", status: "completed", created: now, contract: true},
			{id: 5, user: userA, brand: brandGlorian, vehicle: 3, org: "glorian-dealer", status: "completed", created: now},
		},
		warranties: []fWarranty{
			{user: userA, brand: brandOlex, vehicle: 1, status: "active", start: now.AddDate(0, -3, 0), end: now.AddDate(0, 9, 0)},
			{user: userA, brand: brandOlex, vehicle: 1, status: "expired", start: now.AddDate(-2, 0, 0), end: now.AddDate(-1, 0, 0)},
			{user: userA, brand: brandGlorian, vehicle: 3, status: "active", start: now, end: now.AddDate(1, 0, 0)},
		},
	}
}

func mux(f *fakeStore) http.Handler {
	h := handler.New(usecase.New(f).WithClock(func() time.Time { return now }))
	m := http.NewServeMux()
	m.HandleFunc("GET /v1/portal/vehicles", h.ListVehicles)
	m.HandleFunc("GET /v1/portal/vehicles/{uuid}", h.GetVehicle)
	m.HandleFunc("GET /v1/portal/services", h.ListServices)
	m.HandleFunc("GET /v1/portal/contracts", h.ListContracts)
	return m
}

func get(t *testing.T, f *fakeStore, path string, user, brand int64) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: user})
	ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: brand, Slug: map[int64]string{brandOlex: "olex", brandGlorian: "glorian"}[brand]})
	rec := httptest.NewRecorder()
	mux(f).ServeHTTP(rec, req.WithContext(ctx))
	return rec.Code, rec.Body.Bytes()
}

type page struct {
	Data struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	} `json:"data"`
}

func decodePage(t *testing.T, body []byte) page {
	t.Helper()
	var p page
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return p
}

func noMeasurement(t *testing.T, body []byte) {
	t.Helper()
	if strings.Contains(strings.ToLower(string(body)), "measurement") {
		t.Fatalf("response carries measurement data: %s", body)
	}
}

// Acceptance: a customer with services at two dealers sees both in one list
// (drafts and Glorian rows stay out); no measurement field.
func TestPortalServicesAcrossDealers(t *testing.T) {
	code, body := get(t, fixture(), "/v1/portal/services", userA, brandOlex)
	if code != http.StatusOK {
		t.Fatalf("code = %d %s", code, body)
	}
	noMeasurement(t, body)
	p := decodePage(t, body)
	if p.Data.Total != 2 || len(p.Data.Items) != 2 {
		t.Fatalf("services = %d/%d, want 2", len(p.Data.Items), p.Data.Total)
	}
	orgs := map[string]bool{}
	for _, it := range p.Data.Items {
		orgs[it["organization"].(map[string]any)["name"].(string)] = true
	}
	if !orgs["dealer-a"] || !orgs["dealer-b"] || orgs["glorian-dealer"] {
		t.Fatalf("organizations = %v", orgs)
	}
}

// Acceptance: another customer's vehicle answers 404; a bad uuid too.
func TestPortalVehicleIsolation(t *testing.T) {
	f := fixture()
	for _, path := range []string{"/v1/portal/vehicles/" + vehB.String(), "/v1/portal/vehicles/not-a-uuid"} {
		if code, body := get(t, f, path, userA, brandOlex); code != http.StatusNotFound {
			t.Fatalf("%s = %d %s, want 404", path, code, body)
		}
	}
	code, body := get(t, f, "/v1/portal/vehicles", userA, brandOlex)
	if code != http.StatusOK {
		t.Fatalf("list = %d", code)
	}
	p := decodePage(t, body)
	if p.Data.Total != 1 || p.Data.Items[0]["uuid"] != vehA.String() {
		t.Fatalf("vehicles = %+v", p.Data)
	}
	// User B sees only its own vehicle and service.
	p = decodePage(t, mustOK(t, f, "/v1/portal/vehicles", userB))
	if p.Data.Total != 1 || p.Data.Items[0]["uuid"] != vehB.String() {
		t.Fatalf("user B vehicles = %+v", p.Data)
	}
	p = decodePage(t, mustOK(t, f, "/v1/portal/services", userB))
	if p.Data.Total != 1 {
		t.Fatalf("user B services = %d", p.Data.Total)
	}
}

func mustOK(t *testing.T, f *fakeStore, path string, user int64) []byte {
	t.Helper()
	code, body := get(t, f, path, user, brandOlex)
	if code != http.StatusOK {
		t.Fatalf("%s = %d %s", path, code, body)
	}
	return body
}

// Acceptance: brand=glorian records never come back: not on the Olex domain
// and not on a Glorian domain either.
func TestPortalGlorianClosed(t *testing.T) {
	f := fixture()
	if code, _ := get(t, f, "/v1/portal/vehicles/"+vehAG.String(), userA, brandOlex); code != http.StatusNotFound {
		t.Fatalf("glorian vehicle on olex = %d", code)
	}
	if code, _ := get(t, f, "/v1/portal/vehicles/"+vehAG.String(), userA, brandGlorian); code != http.StatusNotFound {
		t.Fatalf("glorian vehicle on glorian = %d", code)
	}
	for _, path := range []string{"/v1/portal/vehicles", "/v1/portal/services"} {
		code, body := get(t, f, path, userA, brandGlorian)
		if code != http.StatusOK || decodePage(t, body).Data.Total != 0 {
			t.Fatalf("%s on glorian = %d %s", path, code, body)
		}
	}
	// No brand on the request: nothing.
	code, body := get(t, f, "/v1/portal/vehicles", userA, 0)
	if code != http.StatusOK || decodePage(t, body).Data.Total != 0 {
		t.Fatalf("no brand = %d %s", code, body)
	}
}

// The vehicle detail: summary across dealers, embedded services, only the
// active warranty with days / percent left, no measurement field.
func TestPortalVehicleDetail(t *testing.T) {
	body := mustOK(t, fixture(), "/v1/portal/vehicles/"+vehA.String(), userA)
	noMeasurement(t, body)
	var env struct {
		Data struct {
			UUID           string `json:"uuid"`
			ServiceSummary struct {
				Total             int64 `json:"total"`
				Completed         int64 `json:"completed"`
				OrganizationCount int64 `json:"organization_count"`
			} `json:"service_summary"`
			Services         []map[string]any `json:"services"`
			ActiveWarranties []struct {
				DaysLeft    int `json:"days_left"`
				PercentLeft int `json:"percent_left"`
			} `json:"active_warranties"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	d := env.Data
	if d.UUID != vehA.String() || d.ServiceSummary.Total != 2 || d.ServiceSummary.Completed != 1 ||
		d.ServiceSummary.OrganizationCount != 2 || len(d.Services) != 2 {
		t.Fatalf("detail = %+v", d)
	}
	if len(d.ActiveWarranties) != 1 {
		t.Fatalf("active warranties = %+v", d.ActiveWarranties)
	}
	w := d.ActiveWarranties[0]
	wantDays, wantPct := usecase.TimeLeft(now.AddDate(0, -3, 0), now.AddDate(0, 9, 0), now)
	if w.DaysLeft != wantDays || w.PercentLeft != wantPct || w.PercentLeft < 70 || w.PercentLeft > 80 {
		t.Fatalf("time left = %+v, want %d days %d%%", w, wantDays, wantPct)
	}
}

// TEC-245 acceptance: a customer without a contract gets 200 with an empty
// list (items is [], not null); a customer with one sees only their own.
func TestPortalContracts(t *testing.T) {
	f := fixture()
	body := mustOK(t, f, "/v1/portal/contracts", userA)
	if !strings.Contains(string(body), `"items":[]`) {
		t.Fatalf("empty contracts must be an empty list: %s", body)
	}
	if p := decodePage(t, body); p.Data.Total != 0 {
		t.Fatalf("user A contracts = %d", p.Data.Total)
	}
	p := decodePage(t, mustOK(t, f, "/v1/portal/contracts", userB))
	if p.Data.Total != 1 || len(p.Data.Items) != 1 {
		t.Fatalf("user B contracts = %+v", p.Data)
	}
	if svc := p.Data.Items[0]["service"].(map[string]any); svc["service_no"] != "dealer-a-svc" {
		t.Fatalf("contract service = %v", svc)
	}
	// Glorian host: nothing.
	code, body := get(t, f, "/v1/portal/contracts", userB, brandGlorian)
	if code != http.StatusOK || decodePage(t, body).Data.Total != 0 {
		t.Fatalf("glorian contracts = %d %s", code, body)
	}
}
