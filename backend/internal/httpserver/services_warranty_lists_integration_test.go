package httpserver

// TEC-377 (DT-BE-7): list contract of the service, portal service,
// warranty, portal warranty and warranty claim lists (sort, multi-value
// filters, organization filter, date windows) and the service / warranty
// list exports.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// dt7List calls a list endpoint and returns the status code, the values of
// key in row order and the total.
func (it *itest) dt7List(tok, path, key string) (int, []string, int64) {
	it.t.Helper()
	code, env := it.do("GET", path, hostOlex, tok, nil)
	if code != http.StatusOK {
		return code, nil, 0
	}
	page := decodeData[struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
		Limit int32            `json:"limit"`
	}](it.t, env)
	out := make([]string, 0, len(page.Items))
	for _, row := range page.Items {
		out = append(out, fmt.Sprint(row[key]))
	}
	return code, out, page.Total
}

// dt7Want asserts the list order.
func (it *itest) dt7Want(tok, path, key string, want ...string) {
	it.t.Helper()
	code, got, total := it.dt7List(tok, path, key)
	if code != http.StatusOK {
		it.t.Fatalf("%s = %d", path, code)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") || total != int64(len(want)) {
		it.t.Fatalf("%s = %v (total %d), want %v", path, got, total, want)
	}
}

// dt7Service writes a service of org in the given (not completed) status.
func (it *itest) dt7Service(org db.Organization, cust db.User, veh db.Vehicle, no, status string) db.Service {
	it.t.Helper()
	svc, err := it.q.CreateService(context.Background(), db.CreateServiceParams{
		ServiceNo: no, OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
		CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: status,
		Plate: veh.Plate, PlateCountry: veh.PlateCountry,
	})
	if err != nil {
		it.t.Fatalf("service: %v", err)
	}
	return svc
}

func (it *itest) dt7Exec(sql string, args ...any) {
	it.t.Helper()
	if _, err := it.pool.Exec(context.Background(), sql, args...); err != nil {
		it.t.Fatalf("%s: %v", sql, err)
	}
}

func dt7Tag(it *itest) string {
	s := it.suffix
	if len(s) > 8 {
		s = s[len(s)-8:]
	}
	return s
}

func TestIntegrationServiceWarrantyListsServices(t *testing.T) {
	it := dt5Integration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t377s-dist", "distributor", center)
	dist2 := it.org("t377s-dist2", "distributor", center)
	dealerA := it.org("t377s-a", "dealer", dist)
	dealerB := it.org("t377s-b", "dealer", dist)
	tag := dt7Tag(it)
	custA, vehA := it.svcCustomer(dealerA, "t377s-cust-a", "34S"+tag[4:])
	custB, vehB := it.svcCustomer(dealerB, "t377s-cust-b", "06S"+tag[4:])
	u, pw := it.user("t377s-dist-owner")
	it.member(dist, u, "owner")
	tok := it.loginOrg(u, pw, dist)

	sA1 := it.dt7Service(dealerA, custA, vehA, "T377-"+tag+"-1", "pending")
	sA2 := it.directService(dealerA, custA, vehA, 37702)
	sB1 := it.dt7Service(dealerB, custB, vehB, "T377-"+tag+"-3", "processing")
	for i, s := range []db.Service{sA1, sA2, sB1} {
		it.dt7Exec(`UPDATE services SET created_at = $2 WHERE id = $1`, s.ID,
			time.Date(2026, 1, 10+i, 10, 0, 0, 0, time.UTC))
	}
	a1, a2, b1 := sA1.ServiceNo, sA2.ServiceNo, sB1.ServiceNo
	base := "/v1/services?limit=50&"

	// Sort: default -created_at, every field both ways, empty values last.
	it.dt7Want(tok, base, "service_no", b1, a2, a1)
	it.dt7Want(tok, base+"sort=created_at", "service_no", a1, a2, b1)
	it.dt7Want(tok, base+"sort=service_no", "service_no", a2, a1, b1)
	it.dt7Want(tok, base+"sort=-service_no", "service_no", b1, a1, a2)
	it.dt7Want(tok, base+"sort=status", "service_no", a1, b1, a2)
	it.dt7Want(tok, base+"sort=-status", "service_no", a2, b1, a1)
	it.dt7Want(tok, base+"sort=organization", "service_no", a1, a2, b1)
	it.dt7Want(tok, base+"sort=-organization", "service_no", b1, a2, a1)
	it.dt7Want(tok, base+"sort=completed_at", "service_no", a2, a1, b1)
	it.dt7Want(tok, base+"sort=-completed_at", "service_no", a2, b1, a1)
	it.dt7Want(tok, base+"sort=-updated_at", "service_no", b1, a2, a1)

	// Filters: CSV status, organization, created / completed windows, q.
	it.dt7Want(tok, base+"sort=created_at&status=pending,processing", "service_no", a1, b1)
	it.dt7Want(tok, base+"sort=created_at&status=completed", "service_no", a2)
	it.dt7Want(tok, base+"sort=created_at&organization_uuid="+dealerB.Uuid.String(), "service_no", b1)
	it.dt7Want(tok, base+"sort=created_at&organization_uuid="+dealerA.Uuid.String()+","+dealerB.Uuid.String(), "service_no", a1, a2, b1)
	it.dt7Want(tok, base+"sort=created_at&organization_uuid="+dist2.Uuid.String(), "service_no")
	it.dt7Want(tok, base+"created_from=2026-01-11&created_to=2026-01-11", "service_no", a2)
	today := time.Now().UTC().Format(time.DateOnly)
	it.dt7Want(tok, base+"completed_from="+today+"&completed_to="+today, "service_no", a2)
	it.dt7Want(tok, base+"sort=created_at&q="+url.QueryEscape("T377-"+tag), "service_no", a1, b1)
	for _, bad := range []string{"sort=customer", "sort=created_at,bogus", "status=open", "status=pending,bogus",
		"organization_uuid=nope", "created_from=2026-13-01", "created_from=2026-02-01&created_to=2026-01-01",
		"completed_to=x"} {
		if code, _, _ := it.dt7List(tok, base+bad, "service_no"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}

	// Export: same filters and sort; a bad parameter is 400 at request time.
	if code, env := it.do("POST", "/v1/services/export", hostOlex, tok, map[string]any{
		"format": "csv", "query": map[string]string{"sort": "customer"},
	}); code != http.StatusBadRequest {
		t.Fatalf("export bad sort = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/services/export", hostOlex, tok, map[string]any{"format": "docx"}); code != http.StatusBadRequest {
		t.Fatalf("export bad format = %d", code)
	}
	job := decodeData[exportJob](t, it.custDo("POST", "/v1/services/export", tok, map[string]any{
		"format": "csv", "locale": "en",
		"query": map[string]string{"status": "pending,completed", "sort": "service_no", "limit": "1"},
	}, http.StatusAccepted))
	if job.Resource != servicesusecase.ResourceListExport {
		t.Fatalf("job = %+v", job)
	}
	stored := it.dt5JobQuery(job.UUID)
	if stored[ioengine.QueryScopeFilter] == "" || stored["sort"] != "service_no" || stored["limit"] != "" ||
		stored[servicesusecase.QueryScopeUser] != "" {
		t.Fatalf("stored query = %v", stored)
	}
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(dist.ID, 10) // set by the worker
	adapter := servicesusecase.NewListExportAdapter(servicesusecase.New(it.pool, it.q, nil))
	ds, err := adapter.Export(ctx, stored, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 2 || ds.Rows[0]["service_no"] != a2 || ds.Rows[1]["service_no"] != a1 ||
		ds.Rows[0]["status"] != "Completed" || ds.Rows[1]["organization"] != dealerA.Name {
		t.Fatalf("export rows = %v", ds.Rows)
	}
	// The worker re-authorizes the stored scope: dist2 is outside a dist job.
	stored[ioengine.QueryScopeFilter] = strconv.FormatInt(dist2.ID, 10)
	if _, err := adapter.Export(ctx, stored, i18n.Locale("en")); err == nil {
		t.Fatal("a job scope outside the job organization must fail")
	}
}

func TestIntegrationServiceWarrantyListsPortalServices(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dealerA := it.org("t377p-a", "dealer", center)
	dealerB := it.org("t377p-b", "dealer", center)
	tag := dt7Tag(it)
	cust, veh := it.svcCustomer(dealerA, "t377p-cust", "34P"+tag[4:])
	other, otherVeh := it.svcCustomer(dealerB, "t377p-other", "06P"+tag[4:])
	s1 := it.dt7Service(dealerA, cust, veh, "T377P-"+tag+"-1", "pending")
	s2 := it.directService(dealerA, cust, veh, 37712)
	s3 := it.dt7Service(dealerB, cust, veh, "T377P-"+tag+"-3", "processing")
	it.dt7Service(dealerB, cust, veh, "T377P-"+tag+"-4", "draft")            // drafts stay out
	it.dt7Service(dealerB, other, otherVeh, "T377P-"+tag+"-5", "processing") // not the user's
	for i, s := range []db.Service{s1, s2, s3} {
		it.dt7Exec(`UPDATE services SET created_at = $2 WHERE id = $1`, s.ID,
			time.Date(2026, 2, 10+i, 10, 0, 0, 0, time.UTC))
	}
	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: cust.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}
	n1, n2, n3 := s1.ServiceNo, s2.ServiceNo, s3.ServiceNo
	base := "/v1/portal/services?limit=50&"
	it.dt7Want(portalTok, base, "service_no", n3, n2, n1)
	it.dt7Want(portalTok, base+"sort=created_at", "service_no", n1, n2, n3)
	it.dt7Want(portalTok, base+"sort=service_no", "service_no", n2, n1, n3)
	it.dt7Want(portalTok, base+"sort=-status", "service_no", n2, n3, n1)
	it.dt7Want(portalTok, base+"sort=-organization", "service_no", n3, n2, n1)
	it.dt7Want(portalTok, base+"sort=-completed_at", "service_no", n2, n3, n1)
	it.dt7Want(portalTok, base+"sort=created_at&status=pending,processing", "service_no", n1, n3)
	it.dt7Want(portalTok, base+"organization_uuid="+dealerB.Uuid.String(), "service_no", n3)
	it.dt7Want(portalTok, base+"created_from=2026-02-11&created_to=2026-02-12", "service_no", n3, n2)
	it.dt7Want(portalTok, base+"q="+url.QueryEscape("T377P-"+tag+"-3"), "service_no", n3)
	it.dt7Want(portalTok, base+"q=%25", "service_no")
	for _, bad := range []string{"sort=customer", "status=draft", "organization_uuid=x", "created_to=2026-02-31"} {
		if code, _, _ := it.dt7List(portalTok, base+bad, "service_no"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}
}

func TestIntegrationServiceWarrantyListsWarranties(t *testing.T) {
	it := dt5Integration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t377w-dist", "distributor", center)
	dist2 := it.org("t377w-dist2", "distributor", center)
	dealerA := it.org("t377w-a", "dealer", dist)
	dealerB := it.org("t377w-b", "dealer", dist)
	pA := it.product(center, "T377WA")
	pB := it.product(center, "T377WB")
	it.setWarrantyMonths(pA, 12)
	it.setWarrantyMonths(pB, 12)
	tag := dt7Tag(it)
	custA, vehA := it.svcCustomer(dealerA, "t377w-cust-a", "34W"+tag[4:])
	custB, vehB := it.svcCustomer(dealerB, "t377w-cust-b", "06W"+tag[4:])
	wA1 := it.warrantyFor(dealerA, center, pA, custA, vehA, 37721)
	wA2 := it.warrantyFor(dealerA, center, pB, custA, vehA, 37722)
	wB1 := it.warrantyFor(dealerB, center, pA, custB, vehB, 37723)
	// start_at is immutable (the service completion); the ends differ.
	it.dt7Exec(`UPDATE warranties SET end_at = start_at + INTERVAL '60 days' WHERE id = $1`, wA1.ID)
	it.dt7Exec(`UPDATE warranties SET end_at = start_at + INTERVAL '30 days' WHERE id = $1`, wA2.ID)
	it.dt7Exec(`UPDATE warranties SET status = 'expired', expired_at = NOW(), end_at = start_at + INTERVAL '1 day' WHERE id = $1`, wB1.ID)
	day := func(d int) string { return time.Now().UTC().AddDate(0, 0, d).Format(time.DateOnly) }
	u, pw := it.user("t377w-dist-owner")
	it.member(dist, u, "owner")
	tok := it.loginOrg(u, pw, dist)
	a1, a2, b1 := wA1.PublicCode, wA2.PublicCode, wB1.PublicCode
	base := "/v1/warranties?limit=50&"

	// Sort: expiry (default: active soonest end first, then the rest),
	// every field both ways.
	it.dt7Want(tok, base, "public_code", a2, a1, b1)
	it.dt7Want(tok, base+"sort=-expiry", "public_code", b1, a1, a2)
	it.dt7Want(tok, base+"sort=end_at", "public_code", b1, a2, a1)
	it.dt7Want(tok, base+"sort=-end_at", "public_code", a1, a2, b1)
	it.dt7Want(tok, base+"sort=start_at", "public_code", a1, a2, b1)
	it.dt7Want(tok, base+"sort=-start_at", "public_code", b1, a2, a1)
	it.dt7Want(tok, base+"sort=status", "public_code", a1, a2, b1)
	it.dt7Want(tok, base+"sort=-status", "public_code", b1, a2, a1)
	it.dt7Want(tok, base+"sort=product", "public_code", a1, b1, a2)
	it.dt7Want(tok, base+"sort=-organization", "public_code", b1, a2, a1)
	it.dt7Want(tok, base+"sort=created_at", "public_code", a1, a2, b1)
	codes := []string{a1, a2, b1}
	sort.Strings(codes)
	it.dt7Want(tok, base+"sort=public_code", "public_code", codes...)
	it.dt7Want(tok, base+"sort=-service_no", "public_code", b1, a2, a1)

	// Filters.
	it.dt7Want(tok, base+"status=active,expired", "public_code", a2, a1, b1)
	it.dt7Want(tok, base+"status=void,expired", "public_code", b1)
	it.dt7Want(tok, base+"organization_uuid="+dealerB.Uuid.String(), "public_code", b1)
	it.dt7Want(tok, base+"organization_uuid="+dist2.Uuid.String(), "public_code")
	it.dt7Want(tok, base+"start_from="+day(-1)+"&start_to="+day(1), "public_code", a2, a1, b1)
	it.dt7Want(tok, base+"start_to="+day(-2), "public_code")
	it.dt7Want(tok, base+"end_from="+day(20)+"&end_to="+day(40), "public_code", a2)
	it.dt7Want(tok, base+"sort=end_at&product_uuid="+pA.Uuid.String(), "public_code", b1, a1)
	for _, bad := range []string{"sort=holder", "status=open", "status=active,open", "organization_uuid=x",
		"end_from=bad", "start_from=2026-02-01&start_to=2026-01-01"} {
		if code, _, _ := it.dt7List(tok, base+bad, "public_code"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}

	// Portal: the holder's warranties with the same parameters.
	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: custA.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}
	it.dt7Want(portalTok, "/v1/portal/warranties?sort=-end_at", "public_code", a1, a2)
	it.dt7Want(portalTok, "/v1/portal/warranties?status=expired", "public_code")
	it.dt7Want(portalTok, "/v1/portal/warranties?sort=start_at&end_from="+day(20), "public_code", a1, a2)
	if code, _, _ := it.dt7List(portalTok, "/v1/portal/warranties?sort=bogus", "public_code"); code != http.StatusBadRequest {
		t.Fatalf("portal bad sort = %d", code)
	}

	// Export.
	if code, env := it.do("POST", "/v1/warranties/export", hostOlex, tok, map[string]any{
		"format": "csv", "query": map[string]string{"status": "open"},
	}); code != http.StatusBadRequest {
		t.Fatalf("export bad status = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/warranties/export", hostOlex, portalTok, map[string]any{"format": "csv"}); code == http.StatusAccepted {
		t.Fatal("a portal token must not export")
	}
	job := decodeData[exportJob](t, it.custDo("POST", "/v1/warranties/export", tok, map[string]any{
		"format": "xlsx", "locale": "en", "query": map[string]string{"sort": "end_at", "status": "active,expired"},
	}, http.StatusAccepted))
	if job.Resource != warrantyusecase.ResourceListExport {
		t.Fatalf("job = %+v", job)
	}
	stored := it.dt5JobQuery(job.UUID)
	if stored[ioengine.QueryScopeFilter] == "" || stored["sort"] != "end_at" {
		t.Fatalf("stored query = %v", stored)
	}
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(dist.ID, 10)
	adapter := warrantyusecase.NewListExportAdapter(warrantyusecase.NewReader(it.pool, it.q, nil, ""))
	ds, err := adapter.Export(ctx, stored, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 3 || ds.Rows[0]["public_code"] != b1 || ds.Rows[2]["public_code"] != a1 ||
		ds.Rows[0]["status"] != "Expired" || ds.Rows[1]["product"] != pB.Name || ds.Rows[2]["holder"] == "" {
		t.Fatalf("export rows = %v", ds.Rows)
	}
	stored[ioengine.QueryScopeFilter] = strconv.FormatInt(dist2.ID, 10)
	if _, err := adapter.Export(ctx, stored, i18n.Locale("en")); err == nil {
		t.Fatal("a job scope outside the job organization must fail")
	}
}

func TestIntegrationServiceWarrantyListsClaims(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t377c-dist", "distributor", center)
	dealerA := it.org("t377c-a", "dealer", dist)
	dealerB := it.org("t377c-b", "dealer", dist)
	p := it.product(center, "T377C")
	it.setWarrantyMonths(p, 12)
	tag := dt7Tag(it)
	custA, vehA := it.svcCustomer(dealerA, "t377c-cust-a", "34C"+tag[4:])
	custB, vehB := it.svcCustomer(dealerB, "t377c-cust-b", "06C"+tag[4:])
	wA1 := it.warrantyFor(dealerA, center, p, custA, vehA, 37731)
	wA2 := it.warrantyFor(dealerA, center, p, custA, vehA, 37732)
	wB1 := it.warrantyFor(dealerB, center, p, custB, vehB, 37733)
	claim := func(w db.Warranty, desc, status string, day int) db.WarrantyClaim {
		t.Helper()
		c, err := it.q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
			OrganizationID: w.OrganizationID, BrandID: w.BrandID, WarrantyID: w.ID, ServiceID: w.ServiceID,
			VehicleID: w.VehicleID, CustomerUserID: w.HolderUserID, Description: desc, Status: "open",
			CoverageCheck: []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if status == "approved" {
			it.dt7Exec(`UPDATE warranty_claims SET status = 'approved', decided_at = NOW() WHERE id = $1`, c.ID)
		} else if status != "open" {
			it.dt7Exec(`UPDATE warranty_claims SET status = $2 WHERE id = $1`, c.ID, status)
		}
		it.dt7Exec(`UPDATE warranty_claims SET created_at = $2 WHERE id = $1`, c.ID,
			time.Date(2026, 3, day, 10, 0, 0, 0, time.UTC))
		return c
	}
	cA1 := claim(wA1, "Kaput cizik "+tag, "open", 1)
	cA2 := claim(wA2, "Tampon kabarma", "approved", 2)
	cB1 := claim(wB1, "Cam filmi soyuldu", "dealer_review", 3)
	if cA1.ClaimNo != 1 || cA2.ClaimNo != 2 || cB1.ClaimNo != 1 {
		t.Fatalf("claim numbers = %d %d %d", cA1.ClaimNo, cA2.ClaimNo, cB1.ClaimNo)
	}
	u, pw := it.user("t377c-dist-owner")
	it.member(dist, u, "owner")
	tok := it.loginOrg(u, pw, dist)
	a1, a2, b1 := cA1.Uuid.String(), cA2.Uuid.String(), cB1.Uuid.String()
	base := "/v1/warranty-claims?"

	it.dt7Want(tok, base, "uuid", b1, a2, a1)
	it.dt7Want(tok, base+"sort=created_at", "uuid", a1, a2, b1)
	it.dt7Want(tok, base+"sort=claim_no", "uuid", a1, b1, a2)
	it.dt7Want(tok, base+"sort=-claim_no", "uuid", a2, b1, a1)
	it.dt7Want(tok, base+"sort=status", "uuid", a1, b1, a2)
	it.dt7Want(tok, base+"sort=-status", "uuid", a2, b1, a1)
	it.dt7Want(tok, base+"sort=decided_at", "uuid", a2, a1, b1)
	it.dt7Want(tok, base+"sort=-decided_at", "uuid", a2, b1, a1)
	it.dt7Want(tok, base+"sort=created_at&status=open,dealer_review", "uuid", a1, b1)
	it.dt7Want(tok, base+"organization_uuid="+dealerB.Uuid.String(), "uuid", b1)
	var svcA2 string
	if err := it.pool.QueryRow(ctx, `SELECT uuid::text FROM services WHERE id = $1`, wA2.ServiceID).Scan(&svcA2); err != nil {
		t.Fatal(err)
	}
	it.dt7Want(tok, base+"service_uuid="+svcA2, "uuid", a2)
	it.dt7Want(tok, base+"q="+url.QueryEscape("cizik "+tag), "uuid", a1)
	it.dt7Want(tok, base+"q=%25", "uuid")
	it.dt7Want(tok, base+"created_from=2026-03-02&created_to=2026-03-02", "uuid", a2)
	it.dt7Want(tok, base+"created_from="+url.QueryEscape("2026-03-02T00:00:00Z"), "uuid", b1, a2)
	for _, bad := range []string{"sort=description", "status=bogus", "service_uuid=nope", "organization_uuid=x",
		"created_from=03/02/2026"} {
		if code, _, _ := it.dt7List(tok, base+bad, "uuid"); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", bad, code)
		}
	}
	// limit: default 20, at most 100 (a larger value falls back to the default).
	for raw, want := range map[string]float64{"": 20, "limit=100": 100, "limit=500": 20} {
		code, env := it.do("GET", base+raw, hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("%s = %d", raw, code)
		}
		page := decodeData[struct {
			Limit float64 `json:"limit"`
		}](t, env)
		if page.Limit != want {
			t.Fatalf("%s: limit = %v, want %v", raw, page.Limit, want)
		}
	}
}
