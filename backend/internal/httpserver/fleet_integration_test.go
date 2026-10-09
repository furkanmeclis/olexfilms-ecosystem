package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	fleetusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-473 (F5-02b) fixtures.

type fleetNet struct {
	it               *itest
	center, dist     db.Organization
	dealerA, dealerB db.Organization
	tokA, tokB       string
	carBrand         db.CarBrand
	carModel         db.CarModel
}

type fleetView struct {
	UUID string `json:"uuid"`
	Link *struct {
		UUID   string `json:"uuid"`
		Status string `json:"status"`
	} `json:"link"`
}

type fleetCard struct {
	UUID           string `json:"uuid"`
	VehicleCount   int64  `json:"vehicle_count"`
	ServiceCount   int64  `json:"service_count"`
	RecentServices []struct {
		UUID string `json:"uuid"`
	} `json:"recent_services"`
	Cari *struct {
		UUID    string `json:"uuid"`
		Balance string `json:"balance"`
	} `json:"cari"`
}

type fleetPage struct {
	Items []struct {
		UUID         string `json:"uuid"`
		Name         string `json:"name"`
		Plate        string `json:"plate"`
		VehicleCount int64  `json:"vehicle_count"`
		Link         struct {
			Status string `json:"status"`
		} `json:"link"`
	} `json:"items"`
	Total int64 `json:"total"`
}

// tecVKN derives a checksum-valid VKN unique to the run.
func tecVKN(t *testing.T, seed string) string {
	t.Helper()
	base := seed[len(seed)-9:]
	for d := 0; d <= 9; d++ {
		s := fmt.Sprintf("%s%d", base, d)
		sum := 0
		for i := 0; i < 9; i++ {
			v := (int(s[i]-'0') + 9 - i) % 10
			if v == 9 {
				sum += 9
				continue
			}
			sum += (v << (9 - i)) % 9
		}
		if (10-sum%10)%10 == int(s[9]-'0') {
			return s
		}
	}
	t.Fatalf("no VKN for %s", base)
	return ""
}

// fleetPlate is a TR plate unique to the run ("34 ABC 1231").
func (it *itest) fleetPlate(n int) string {
	tail := it.suffix[len(it.suffix)-6:]
	letters := ""
	for _, c := range tail[3:] {
		letters += string(rune('A' + (c - '0')))
	}
	return fmt.Sprintf("34 %s %s%d", letters, tail[:3], n)
}

func (it *itest) fleetNet(prefix string) fleetNet {
	it.t.Helper()
	ctx := context.Background()
	n := fleetNet{it: it, center: it.brandCenter("olex")}
	n.dist = it.org(prefix+"-dist", "distributor", n.center)
	n.dealerA = it.org(prefix+"-a", "dealer", n.dist)
	n.dealerB = it.org(prefix+"-b", "dealer", n.dist)
	ownerA, pwA := it.user(prefix + "-owner-a")
	it.member(n.dealerA, ownerA, "owner")
	ownerB, pwB := it.user(prefix + "-owner-b")
	it.member(n.dealerB, ownerB, "owner")
	n.tokA, n.tokB = it.loginOrg(ownerA, pwA, n.dealerA), it.loginOrg(ownerB, pwB, n.dealerB)
	var err error
	if n.carBrand, err = it.q.CreateCarBrand(ctx, db.CreateCarBrandParams{Name: "T473 " + prefix + " " + it.suffix, ShowName: true, Active: true}); err != nil {
		it.t.Fatal(err)
	}
	if n.carModel, err = it.q.CreateCarModel(ctx, db.CreateCarModelParams{CarBrandID: n.carBrand.ID, Name: "Golf " + it.suffix, Active: true}); err != nil {
		it.t.Fatal(err)
	}
	return n
}

func (it *itest) enableFleet(orgs ...db.Organization) {
	it.t.Helper()
	admin, _ := it.user("t473-admin")
	for _, o := range orgs {
		if _, err := it.srv.features.SetByAdmin(context.Background(), admin.ID, o.ID, features.ModuleFleet, true); err != nil {
			it.t.Fatalf("enable fleet: %v", err)
		}
		org := o
		it.t.Cleanup(func() { _, _ = it.srv.features.ClearByAdmin(context.Background(), org.ID, features.ModuleFleet) })
	}
}

func (it *itest) fleetDo(method, path, token string, body any, want int) envelope {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, token, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s %s, want %d", method, path, code, errCode(env), env.Data, want)
	}
	return env
}

// completedFleetService inserts a completed service of org on the vehicle.
func (it *itest) completedFleetService(org db.Organization, vehicleUUID string, n fleetNet) string {
	it.t.Helper()
	ctx := context.Background()
	v, err := it.q.GetVehicleByUUID(ctx, uuid.MustParse(vehicleUUID))
	if err != nil {
		it.t.Fatal(err)
	}
	var out string
	if err := it.pool.QueryRow(ctx, `INSERT INTO services (service_no, organization_id, brand_id, customer_user_id,
		vehicle_id, car_brand_id, car_model_id, plate, plate_country, status, completed_at)
		VALUES ('T473' || right(gen_random_uuid()::text, 8), $1, $2, $3, $4, $5, $6, $7, 'TR', 'completed', NOW())
		RETURNING uuid::text`, org.ID, org.BrandID, v.UserID, v.ID, n.carBrand.ID, n.carModel.ID, v.Plate.String).Scan(&out); err != nil {
		it.t.Fatalf("service: %v", err)
	}
	return out
}

// TEC-473 acceptance (HTTP): the fleet module gate (403 when off), the VKN
// lookup and the pending link of a second dealer, the portal accept, the
// fleet vehicle service income booked on the fleet cari (not the
// customer's), the second dealer seeing only its own services and cari,
// another customer's vehicle refused with 409, and the list contracts.
func TestIntegrationFleetManagement(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	n := it.fleetNet("t473")

	// 1. Module off: 403 FEATURE_DISABLED on every panel route.
	for _, p := range []string{"/v1/fleets", "/v1/fleets/" + uuid.NewString()} {
		if code, env := it.do("GET", p, hostOlex, n.tokA, nil); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
			t.Fatalf("module off GET %s = %d %s", p, code, errCode(env))
		}
	}
	if code, env := it.do("POST", "/v1/fleets", hostOlex, n.tokA, map[string]any{"legal_name": "x", "tax_number": "1"}); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("module off POST = %d %s", code, errCode(env))
	}
	it.enableFleet(n.dealerA, n.dealerB)

	// 2. Dealer A opens the fleet; dealer B finds it by VKN and asks for a link.
	tax := tecVKN(t, it.suffix)
	it.fleetDo("POST", "/v1/fleets", n.tokA, map[string]any{"legal_name": "T473 Filo", "tax_number": "1089325651"}, http.StatusUnprocessableEntity)
	opened := decodeData[fleetView](t, it.fleetDo("POST", "/v1/fleets", n.tokA, map[string]any{
		"legal_name": "T473 Filo A.Ş.", "name": "T473 Filo " + it.suffix, "tax_number": tax, "billing_email": "fatura-" + it.suffix + "@example.test",
	}, http.StatusCreated))
	if opened.Link == nil || opened.Link.Status != "active" {
		t.Fatalf("opened = %+v", opened)
	}
	exists := it.fleetDo("POST", "/v1/fleets", n.tokB, map[string]any{"legal_name": "Kopya", "tax_number": tax}, http.StatusConflict)
	if errCode(exists) != "FLEET_ALREADY_EXISTS" || decodeData[struct {
		FleetUUID string `json:"fleet_uuid"`
	}](t, exists).FleetUUID != opened.UUID {
		t.Fatalf("second open = %s %s", errCode(exists), exists.Data)
	}
	if got := it.countRows(`SELECT COUNT(*) FROM fleet_profiles WHERE tax_number = $1`, tax); got != 1 {
		t.Fatalf("fleets with the VKN = %d", got)
	}
	base := "/v1/fleets/" + opened.UUID
	req := decodeData[fleetView](t, it.fleetDo("POST", base+"/links", n.tokB, nil, http.StatusCreated))
	if req.Link == nil || req.Link.Status != "pending" {
		t.Fatalf("link request = %+v", req)
	}
	it.fleetDo("GET", base, n.tokB, nil, http.StatusNotFound)

	// 3. The first invited user is the primary user; the fleet user accepts
	// the pending link in the portal.
	invited := decodeData[struct {
		UserUUID  string `json:"user_uuid"`
		IsPrimary bool   `json:"is_primary"`
	}](t, it.fleetDo("POST", base+"/users", n.tokA, map[string]any{
		"email": "t473-fleet-" + it.suffix + "@example.test", "name": "Filo", "surname": "Yönetici",
	}, http.StatusCreated))
	if !invited.IsPrimary {
		t.Fatalf("first fleet user is not primary: %+v", invited)
	}
	if got := it.countRows(`SELECT COUNT(*) FROM otp_codes WHERE email = $1 AND type = 'password_reset'`,
		"t473-fleet-"+it.suffix+"@example.test"); got != 1 {
		t.Fatalf("password set code rows = %d", got)
	}
	fleetUser, err := it.q.GetUserByUUID(ctx, uuid.MustParse(invited.UserUUID))
	if err != nil {
		t.Fatal(err)
	}
	pw := "Fleet-Passw0rd!x"
	hash, _ := password.Hash(pw)
	if _, err := it.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, fleetUser.ID, hash); err != nil {
		t.Fatal(err)
	}
	portal := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": fleetUser.Email.String, "password": pw, "realm": "portal",
	})).AccessToken
	links := decodeData[fleetPage](t, it.fleetDo("GET", "/v1/portal/fleet/links", portal, nil, http.StatusOK))
	if links.Total != 2 {
		t.Fatalf("portal links = %+v", links)
	}
	it.fleetDo("POST", "/v1/portal/fleet/links/"+req.Link.UUID+"/accept", portal, nil, http.StatusOK)
	it.fleetDo("POST", "/v1/portal/fleet/links/"+req.Link.UUID+"/reject", portal, nil, http.StatusConflict)
	// Dealer B's panel token never reaches the portal decision.
	if code, _ := it.do("POST", "/v1/portal/fleet/links/"+req.Link.UUID+"/accept", hostOlex, n.tokB, nil); code < 400 {
		t.Fatalf("panel token on the portal decision = %d", code)
	}

	// 4. Vehicles: a new fleet vehicle; another customer's plate is 409.
	veh := decodeData[struct {
		UUID string `json:"uuid"`
	}](t, it.fleetDo("POST", base+"/vehicles", n.tokA, map[string]any{
		"plate": it.fleetPlate(1), "car_brand_uuid": n.carBrand.Uuid.String(), "car_model_uuid": n.carModel.Uuid.String(), "model_year": 2022,
	}, http.StatusCreated))
	_, foreign := it.svcCustomer(n.dealerA, "t473-other", strings.ReplaceAll(it.fleetPlate(2), " ", ""))
	if _, err := it.pool.Exec(ctx, `UPDATE vehicles SET plate = $2 WHERE id = $1`, foreign.ID, it.fleetPlate(2)); err != nil {
		t.Fatal(err)
	}
	if env := it.fleetDo("POST", base+"/vehicles", n.tokA, map[string]any{"plate": it.fleetPlate(2)}, http.StatusConflict); errCode(env) != "FLEET_VEHICLE_OTHER_OWNER" {
		t.Fatalf("other customer's plate = %s", errCode(env))
	}
	if env := it.fleetDo("POST", base+"/vehicles", n.tokA, map[string]any{"vehicle_uuid": foreign.Uuid.String()}, http.StatusConflict); errCode(env) != "FLEET_VEHICLE_OTHER_OWNER" {
		t.Fatalf("other customer's vehicle = %s", errCode(env))
	}
	if env := it.fleetDo("POST", base+"/vehicles", n.tokA, map[string]any{"plate": it.fleetPlate(1)}, http.StatusConflict); errCode(env) != "FLEET_VEHICLE_EXISTS" {
		t.Fatalf("fleet's own plate = %s", errCode(env))
	}
	it.fleetDo("PATCH", base+"/vehicles/"+veh.UUID, n.tokA, map[string]any{"model_year": 2023}, http.StatusOK)

	// 5. Income of a fleet vehicle's service (payment on account) lands on
	// the fleet cari in each dealer's own ledger, never on the customer cari.
	svcA := it.completedFleetService(n.dealerA, veh.UUID, n)
	svcB := it.completedFleetService(n.dealerB, veh.UUID, n)
	it.fleetDo("POST", "/v1/services/"+svcA+"/income", n.tokA, map[string]any{"amount": "1000", "payment_method": "cari"}, http.StatusCreated)
	it.fleetDo("POST", "/v1/services/"+svcB+"/income", n.tokB, map[string]any{"amount": "250", "payment_method": "cari"}, http.StatusCreated)
	fleetOrg, err := it.q.GetFleetByUUID(ctx, uuid.MustParse(opened.UUID))
	if err != nil {
		t.Fatal(err)
	}
	if got := it.countRows(`SELECT COUNT(*) FROM finance_entries fe JOIN cari_accounts c ON c.id = fe.cari_id
		WHERE fe.source_type = 'service_income' AND c.counterparty_org_id = $1`, fleetOrg.Organization.ID); got != 2 {
		t.Fatalf("service income rows on the fleet cari = %d", got)
	}
	if got := it.countRows(`SELECT COUNT(*) FROM cari_accounts WHERE counterparty_user_id = $1`, fleetUser.ID); got != 0 {
		t.Fatalf("customer cari rows of the fleet owner = %d", got)
	}
	cardA := decodeData[fleetCard](t, it.fleetDo("GET", base, n.tokA, nil, http.StatusOK))
	cardB := decodeData[fleetCard](t, it.fleetDo("GET", base, n.tokB, nil, http.StatusOK))
	if cardA.Cari == nil || cardA.Cari.Balance != "1000.00" || cardB.Cari == nil || cardB.Cari.Balance != "250.00" {
		t.Fatalf("cari balances A=%+v B=%+v", cardA.Cari, cardB.Cari)
	}
	if cardB.ServiceCount != 1 || len(cardB.RecentServices) != 1 || cardB.RecentServices[0].UUID != svcB || cardB.VehicleCount != 1 {
		t.Fatalf("dealer B card = %+v", cardB)
	}
	stB := decodeData[struct {
		Lines []struct {
			Kind string `json:"kind"`
		} `json:"lines"`
		ServiceIncomeTotal string `json:"service_income_total"`
	}](t, it.fleetDo("GET", base+"/statement?period_from=2026-01-01&period_to=2026-12-31", n.tokB, nil, http.StatusOK))
	if len(stB.Lines) != 1 || stB.Lines[0].Kind != "service_income" || stB.ServiceIncomeTotal != "250.00" {
		t.Fatalf("dealer B statement = %+v", stB)
	}
	it.fleetDo("GET", base+"/statement?period_from=2026-02-01&period_to=2026-01-01", n.tokB, nil, http.StatusBadRequest)

	// 6. List contracts: sort whitelist (400 otherwise), status CSV filter,
	// vehicle_count range; vehicles and users.
	list := decodeData[fleetPage](t, it.fleetDo("GET", "/v1/fleets?status=active,pending&sort=-vehicle_count&q="+url.QueryEscape(tax[:6]), n.tokB, nil, http.StatusOK))
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].UUID != opened.UUID || list.Items[0].VehicleCount != 1 {
		t.Fatalf("dealer B fleets = %+v", list)
	}
	if env := it.fleetDo("GET", "/v1/fleets?sort=bogus", n.tokB, nil, http.StatusBadRequest); errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("unknown sort = %s", errCode(env))
	}
	it.fleetDo("GET", "/v1/fleets?status=bogus", n.tokB, nil, http.StatusBadRequest)
	if l := decodeData[fleetPage](t, it.fleetDo("GET", "/v1/fleets?vehicle_count_min=2", n.tokB, nil, http.StatusOK)); l.Total != 0 {
		t.Fatalf("vehicle_count_min = %+v", l)
	}
	vl := decodeData[fleetPage](t, it.fleetDo("GET", base+"/vehicles?sort=-plate&limit=5", n.tokB, nil, http.StatusOK))
	if vl.Total != 1 || vl.Items[0].UUID != veh.UUID {
		t.Fatalf("fleet vehicles = %+v", vl)
	}
	it.fleetDo("GET", base+"/vehicles?sort=vin", n.tokB, nil, http.StatusBadRequest)
	ul := decodeData[fleetPage](t, it.fleetDo("GET", base+"/users?status=active&sort=-created_at", n.tokA, nil, http.StatusOK))
	if ul.Total != 1 {
		t.Fatalf("fleet users = %+v", ul)
	}
	it.fleetDo("GET", base+"/users?sort=bogus", n.tokA, nil, http.StatusBadRequest)

	// 7. Removing a vehicle keeps it.
	it.fleetDo("DELETE", base+"/vehicles/"+veh.UUID, n.tokA, nil, http.StatusNoContent)
	if v, err := it.q.GetVehicleByUUID(ctx, uuid.MustParse(veh.UUID)); err != nil || v.FleetOrgID.Valid {
		t.Fatalf("removed vehicle = %+v, %v", v.FleetOrgID, err)
	}
}

func TestIntegrationFleetServicePlansRequireAppointmentsFeature(t *testing.T) {
	it := newIntegration(t)
	n := it.fleetNet("t475-gate")
	it.enableFleet(n.dealerA)
	if _, err := it.q.UpsertOrgModuleFlag(context.Background(), db.UpsertOrgModuleFlagParams{
		Scope: "org", OrganizationID: pgtype.Int8{Int64: n.dealerA.ID, Valid: true},
		ModuleKey: features.ModuleAppointments, Enabled: false, Source: "admin",
	}); err != nil {
		t.Fatalf("disable appointments: %v", err)
	}
	code, env := it.do("POST", "/v1/fleets/"+uuid.NewString()+"/service-plans/preview", hostOlex, n.tokA, map[string]any{
		"vehicle_uuids": []string{uuid.NewString()}, "service_type": "PPF", "start_date": "2026-10-05",
	})
	if code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("appointments module off = %d %s", code, errCode(env))
	}
}

type fleetImportJob struct {
	UUID           string            `json:"uuid"`
	Status         string            `json:"status"`
	Defaults       map[string]string `json:"defaults"`
	PreviewSummary struct {
		Counts map[string]int `json:"counts"`
		Rows   []struct {
			Index  int            `json:"index"`
			Status string         `json:"status"`
			Target map[string]any `json:"target"`
		} `json:"rows"`
		Errors []struct {
			Index int    `json:"index"`
			Code  string `json:"code"`
		} `json:"errors"`
	} `json:"preview_summary"`
}

func (j fleetImportJob) row(i int) (string, map[string]any) {
	for _, r := range j.PreviewSummary.Rows {
		if r.Index == i {
			return r.Status, r.Target
		}
	}
	return "", nil
}

func (it *itest) uploadFleetImport(path, token, csv string) (int, fleetImportJob) {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "fleet.csv")
	if err != nil {
		it.t.Fatal(err)
	}
	_, _ = part.Write([]byte(csv))
	_ = mw.WriteField("format", "csv")
	_ = mw.WriteField("locale", "en")
	_ = mw.Close()
	rec := it.raw("POST", path, token, mw.FormDataContentType(), buf.Bytes(), nil)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	var job fleetImportJob
	if rec.Code < 300 {
		if err := json.Unmarshal(env.Data, &job); err != nil {
			it.t.Fatal(err)
		}
	} else {
		it.t.Logf("upload %s = %d %s", path, rec.Code, rec.Body.String())
	}
	return rec.Code, job
}

// TEC-473 acceptance: the fleet vehicle import is a staged import: the
// preview (dry run) classifies the rows and writes nothing; confirm creates
// the new vehicle and links the fleet owner's vehicle; undo deletes the
// created vehicle and unlinks the linked one.
func TestIntegrationFleetVehicleImportDryRunUndo(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	ctx := context.Background()
	n := it.fleetNet("t473i")
	it.enableFleet(n.dealerA, n.dealerB)
	opened := decodeData[fleetView](t, it.fleetDo("POST", "/v1/fleets", n.tokA, map[string]any{
		"legal_name": "T473 Import Filo", "tax_number": tecVKN(t, it.suffix),
	}, http.StatusCreated))
	base := "/v1/fleets/" + opened.UUID
	path := base + "/vehicles/import"
	csv := "plate,vin,car_brand,car_model,model_year\n" + it.fleetPlate(1) + ",,x,,2020\n"

	// No primary user yet: the preview is refused.
	code, job := it.uploadFleetImport(path, n.tokA, csv)
	if code != http.StatusCreated || job.Defaults["fleet_uuid"] != opened.UUID {
		t.Fatalf("upload = %d %+v", code, job)
	}
	it.fleetDo("POST", "/v1/tenant/imports/"+job.UUID+"/preview", n.tokA, nil, http.StatusBadRequest)
	// A dealer without a link cannot upload for the fleet.
	if code, _ := it.uploadFleetImport(path, n.tokB, csv); code != http.StatusNotFound {
		t.Fatalf("unlinked dealer upload = %d", code)
	}

	invited := decodeData[struct {
		UserUUID string `json:"user_uuid"`
	}](t, it.fleetDo("POST", base+"/users", n.tokA, map[string]any{
		"email": "t473i-fleet-" + it.suffix + "@example.test", "name": "Filo",
	}, http.StatusCreated))
	owner, err := it.q.GetUserByUUID(ctx, uuid.MustParse(invited.UserUUID))
	if err != nil {
		t.Fatal(err)
	}
	// The owner's vehicle outside the fleet (link row) and another
	// customer's vehicle (conflict row).
	ownPlate, otherPlate := it.fleetPlate(3), it.fleetPlate(4)
	ownV, err := it.q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: owner.ID, BrandID: n.dealerA.BrandID, Plate: pgtype.Text{String: ownPlate, Valid: true},
		PlateNormalized: pgtype.Text{String: strings.ReplaceAll(ownPlate, " ", ""), Valid: true},
		PlateCountry:    pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, other := it.svcCustomer(n.dealerA, "t473i-other", strings.ReplaceAll(otherPlate, " ", ""))
	_ = other

	csv = "plate,vin,car_brand,car_model,model_year\n" +
		it.fleetPlate(1) + ",," + n.carBrand.Name + "," + n.carModel.Name + ",2021\n" + // 1 new
		it.fleetPlate(1) + ",,,,\n" + // 2 duplicate in file
		ownPlate + ",,,,\n" + // 3 link
		otherPlate + ",,,,\n" + // 4 conflict
		it.fleetPlate(5) + ",BADVIN,,,\n" + // 5 invalid
		it.fleetPlate(6) + ",,Nope " + it.suffix + ",,\n" // 6 invalid brand
	code, job = it.uploadFleetImport(path, n.tokA, csv)
	if code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}
	jobBase := "/v1/tenant/imports/" + job.UUID
	vehiclesBefore := it.countRows(`SELECT COUNT(*) FROM vehicles WHERE fleet_org_id IS NOT NULL AND user_id = $1`, owner.ID)
	pre := decodeData[fleetImportJob](t, it.fleetDo("POST", jobBase+"/preview", n.tokA, nil, http.StatusOK))
	want := map[int]string{1: "new", 2: "duplicate", 3: "link", 4: "conflict", 5: "invalid", 6: "invalid"}
	for i, w := range want {
		if got, _ := pre.row(i); got != w {
			t.Fatalf("preview row %d = %q, want %q (%+v)", i, got, w, pre.PreviewSummary)
		}
	}
	if _, target := pre.row(3); target["vehicle_uuid"] != ownV.Uuid.String() {
		t.Fatalf("link row target = %v", target)
	}
	if pre.PreviewSummary.Counts["new"] != 1 || pre.PreviewSummary.Counts["link"] != 1 {
		t.Fatalf("preview counts = %v", pre.PreviewSummary.Counts)
	}
	if got := it.countRows(`SELECT COUNT(*) FROM vehicles WHERE fleet_org_id IS NOT NULL AND user_id = $1`, owner.ID); got != vehiclesBefore {
		t.Fatalf("dry run wrote vehicles: %d -> %d", vehiclesBefore, got)
	}

	applied := decodeData[fleetImportJob](t, it.fleetDo("POST", jobBase+"/confirm", n.tokA, nil, http.StatusAccepted))
	if applied.Status != "applied" || applied.PreviewSummary.Counts["applied"] != 2 {
		t.Fatalf("confirm = %s %v", applied.Status, applied.PreviewSummary.Counts)
	}
	_, newTarget := applied.row(1)
	newUUID, _ := newTarget["vehicle_uuid"].(string)
	if got := it.countRows(`SELECT COUNT(*) FROM vehicles v JOIN organizations o ON o.id = v.fleet_org_id
		WHERE o.uuid = $1 AND v.deleted_at IS NULL`, opened.UUID); got != 2 {
		t.Fatalf("fleet vehicles after confirm = %d", got)
	}
	again := decodeData[fleetImportJob](t, it.fleetDo("POST", jobBase+"/confirm", n.tokA, nil, http.StatusAccepted))
	if again.Status != "applied" {
		t.Fatalf("second confirm = %s", again.Status)
	}

	undone := decodeData[fleetImportJob](t, it.fleetDo("POST", jobBase+"/rollback", n.tokA, nil, http.StatusOK))
	if undone.Status != "rolled_back" || undone.PreviewSummary.Counts["undone"] != 2 {
		t.Fatalf("undo = %s %v", undone.Status, undone.PreviewSummary.Counts)
	}
	if got := it.countRows(`SELECT COUNT(*) FROM vehicles WHERE uuid = $1 AND deleted_at IS NULL`, newUUID); got != 0 {
		t.Fatalf("created vehicle survived the undo")
	}
	if v, err := it.q.GetVehicleByUUID(ctx, ownV.Uuid); err != nil || v.FleetOrgID.Valid {
		t.Fatalf("linked vehicle after undo = %+v, %v", v.FleetOrgID, err)
	}
	it.fleetDo("POST", jobBase+"/rollback", n.tokA, nil, http.StatusOK)

	// The sample file.
	if rec := it.raw("GET", "/v1/fleets/vehicle-import/sample?format=csv", n.tokA, "", nil, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "34 ABC 123") {
		t.Fatalf("sample = %d %s", rec.Code, rec.Body.String())
	}
}

// TEC-473: the statement export (I/O engine) is queued only for the
// caller's own active link; the stored query names the fleet and the
// period, and the worker adapter rebuilds the statement for the job
// organization (XLSX / PDF encodable).
func TestIntegrationFleetStatementExport(t *testing.T) {
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory(); d.Queue = qc })
	ctx := context.Background()
	n := it.fleetNet("t473e")
	it.enableFleet(n.dealerA, n.dealerB)
	opened := decodeData[fleetView](t, it.fleetDo("POST", "/v1/fleets", n.tokA, map[string]any{
		"legal_name": "T473 Export Filo", "tax_number": tecVKN(t, it.suffix),
	}, http.StatusCreated))
	base := "/v1/fleets/" + opened.UUID
	body := map[string]any{"format": "xlsx", "period_from": "2026-01-01", "period_to": "2026-12-31", "locale": "en"}
	job := decodeData[struct {
		UUID string `json:"uuid"`
	}](t, it.fleetDo("POST", base+"/statement/export", n.tokA, body, http.StatusAccepted))
	q := it.exportJobQuery(job.UUID)
	if q[fleetusecase.QueryFleetUUID] != opened.UUID || q[fleetusecase.QueryPeriodFrom] != "2026-01-01" {
		t.Fatalf("export query = %v", q)
	}
	// Dealer B has no link: 404, nothing queued.
	it.fleetDo("POST", base+"/statement/export", n.tokB, body, http.StatusNotFound)
	body["format"] = "docx"
	it.fleetDo("POST", base+"/statement/export", n.tokA, body, http.StatusBadRequest)

	// The worker side: the adapter re-checks the job organization's link.
	adapter := fleetusecase.NewStatementAdapter(fleetusecase.New(it.pool))
	q[ioengine.QueryOrganizationID] = fmt.Sprint(n.dealerA.ID)
	ds, err := adapter.Export(ctx, q, "en")
	if err != nil || len(ds.Rows) != 1 || ds.Totals["balance"] != "0.00" {
		t.Fatalf("dataset = %+v, %v", ds, err)
	}
	for _, f := range []ioengine.ExportFormat{ioengine.ExportXLSX, ioengine.ExportCSV} {
		if out, err := ioengine.EncodeExport(f, ds, "en", nil, "Fleet statement"); err != nil || len(out) == 0 {
			t.Fatalf("encode %s: %v", f, err)
		}
	}
	// TEC-139: the PDF is the Gotenberg HTML table.
	if h := ioengine.ExportTableHTML(ds, "en", nil, "Fleet statement"); !strings.Contains(h, "Fleet statement") || !strings.Contains(h, "0.00") {
		t.Fatal("fleet statement pdf html")
	}
	q[ioengine.QueryOrganizationID] = fmt.Sprint(n.dealerB.ID)
	if _, err := adapter.Export(ctx, q, "en"); err == nil {
		t.Fatal("unlinked job organization exported the statement")
	}
}
