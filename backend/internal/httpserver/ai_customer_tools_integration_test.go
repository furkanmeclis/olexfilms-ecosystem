package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
)

// callAITool runs one tool on the server's registry the way the portal
// chat (F4-01f) and WhatsApp (F4-02c) will: a realm principal with the
// conversation brand.
func (it *itest) callAITool(p aitools.Principal, name, input string) aitools.Result {
	it.t.Helper()
	res, err := it.srv.aiTools.Call(context.Background(), p, name, json.RawMessage(input))
	if err != nil && !errors.Is(err, aitools.ErrToolNotAllowed) {
		it.t.Fatalf("%s(%s): %v", name, input, err)
	}
	return res
}

func aiCustomer(u db.User, brandID int64) aitools.Principal {
	return aitools.Principal{
		Auth:  authctx.Principal{UserID: u.Uuid, UserInternal: u.ID, Realm: "portal"},
		Realm: aitools.RealmCustomer, Brand: &brandctx.Brand{ID: brandID, Slug: "olex", Name: "Olex", Status: "active"},
	}
}

// TEC-386 acceptance (F4-01d) on the real portal use cases: customer A
// asking for customer B's service by number (or uuid) gets NOT_FOUND from
// every service tool, sees only their own services and vehicles, and
// change_language writes users.locale.
func TestIntegrationAICustomerToolsOwnRecordsOnly(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	sfx := strings.ToUpper(it.suffix[len(it.suffix)-6:])

	center := it.brandCenter("olex")
	dealer := it.org("t386-d", "dealer", center)
	p := it.product(center, "T386")
	plateA, plateB := "34CA"+sfx, "34CB"+sfx
	service := func(name, plate string, seq int) (db.User, db.Service) {
		cust, veh := it.svcCustomer(dealer, name, plate)
		u := it.stockChain().unit(center, p, 38600+seq)
		svc := it.directService(dealer, cust, veh, 386000+seq,
			db.CreateServiceItemParams{ProductID: p.ID, UnitID: u.ID, Kind: "full"})
		if _, err := it.pool.Exec(ctx, `UPDATE services SET plate = $1, plate_country = 'TR' WHERE id = $2`, plate, svc.ID); err != nil {
			t.Fatal(err)
		}
		return cust, svc
	}
	custA, svcA := service("t386-cust-a", plateA, 1)
	custB, svcB := service("t386-cust-b", plateB, 2)
	a, b := aiCustomer(custA, center.BrandID), aiCustomer(custB, center.BrandID)

	// Customer A: own service yes.
	if r := it.callAITool(a, "my_service_detail", `{"service":"`+svcA.ServiceNo+`"}`); r.IsError || !strings.Contains(r.Content, plateA) {
		t.Fatalf("A own service: %+v", r)
	}
	// Customer B's service by number or uuid: not found, from every tool.
	for _, name := range []string{"my_service_detail", "my_service_pdf_link", "dealer_review_link"} {
		for _, ref := range []string{svcB.ServiceNo, svcB.Uuid.String()} {
			r := it.callAITool(a, name, `{"service":"`+ref+`"}`)
			if !r.IsError || r.Code != aitools.CodeNotFound || strings.Contains(r.Content, plateB) {
				t.Fatalf("A %s(%s) of B: %+v", name, ref, r)
			}
		}
	}
	// B finds it.
	if r := it.callAITool(b, "my_service_detail", `{"service":"`+svcB.ServiceNo+`"}`); r.IsError || !strings.Contains(r.Content, plateB) {
		t.Fatalf("B own service: %+v", r)
	}
	// Lists hold only the own records.
	for _, name := range []string{"my_services", "my_vehicles"} {
		r := it.callAITool(a, name, `{}`)
		if r.IsError || !strings.Contains(r.Content, plateA) || strings.Contains(r.Content, plateB) {
			t.Fatalf("A %s: %+v", name, r)
		}
	}
	if r := it.callAITool(a, "my_services", `{"query":"`+svcB.ServiceNo+`"}`); r.IsError || !strings.Contains(r.Content, `"total":0`) {
		t.Fatalf("A searched B's number: %+v", r)
	}
	// The PDF link is a short link to the own service page.
	r := it.callAITool(a, "my_service_pdf_link", `{"service":"`+svcA.ServiceNo+`"}`)
	if r.IsError || !strings.Contains(r.Content, "/s/") {
		t.Fatalf("A pdf link: %+v", r)
	}
	var target string
	if err := it.pool.QueryRow(ctx, `SELECT target_path FROM short_urls WHERE created_by = $1 ORDER BY id DESC LIMIT 1`,
		custA.ID).Scan(&target); err != nil || target != "/portal/services/"+svcA.Uuid.String() {
		t.Fatalf("short url target = %q, %v", target, err)
	}

	// A customer tool is not offered to a visitor or a panel principal.
	visitor := a
	visitor.Realm, visitor.Auth = aitools.RealmVisitor, authctx.Principal{}
	if r := it.callAITool(visitor, "my_services", `{}`); r.Code != aitools.CodeToolNotAllowed {
		t.Fatalf("visitor called a customer tool: %+v", r)
	}

	// change_language writes users.locale without a confirmation.
	if r := it.callAITool(a, "change_language", `{"locale":"de"}`); r.IsError || !strings.Contains(r.Content, `"locale":"de"`) {
		t.Fatalf("change_language: %+v", r)
	}
	var locale string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(locale, '') FROM users WHERE id = $1`, custA.ID).Scan(&locale); err != nil || locale != "de" {
		t.Fatalf("users.locale = %q, %v", locale, err)
	}
}

// TEC-386 acceptance: the nearest dealer tool lists only active dealers
// whose access window is open and whose contract has not expired, both by
// city / district and by location; the visitor result carries no personal
// field.
func TestIntegrationAIVisitorNearestDealers(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	city := "Tİ386 Şehir " + it.suffix

	dealer := func(name, extra string) db.Organization {
		o := it.org(name, "dealer", center)
		// A unique remote spot so that other tests' dealers are never near.
		if _, err := it.pool.Exec(ctx, `UPDATE organizations SET city = $1, district = 'Merkez',
			latitude = -47.1, longitude = -139.1, phone = '+905551112233' `+extra+` WHERE id = $2`, city, o.ID); err != nil {
			t.Fatalf("update %s: %v", name, err)
		}
		return o
	}
	ok := dealer("t386-ok", ", contract_valid_until = CURRENT_DATE + 30")
	dealer("t386-ro", ", status = 'read_only'")
	dealer("t386-susp", ", status = 'suspended'")
	dealer("t386-ended", ", access_ends_at = NOW() - interval '1 day'")
	dealer("t386-contract", ", contract_valid_until = CURRENT_DATE - 1")

	visitor := aitools.Principal{Realm: aitools.RealmVisitor,
		Brand: &brandctx.Brand{ID: center.BrandID, Slug: "olex", Name: "Olex", Status: "active"}}
	for _, input := range []string{
		// Case and Turkish letters are folded.
		`{"city":"` + strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(city, "İ", "i"), "Ş", "s")) + `"}`,
		`{"city":"` + city + `","district":"merkez"}`,
		`{"latitude":-47.1,"longitude":-139.1,"radius_km":5}`,
	} {
		r := it.callAITool(visitor, "find_nearest_dealers", input)
		if r.IsError || !strings.Contains(r.Content, ok.Name) || !strings.Contains(r.Content, `"total":1`) {
			t.Fatalf("find_nearest_dealers %s: %+v", input, r)
		}
		for _, bad := range []string{"t386-ro", "t386-susp", "t386-ended", "t386-contract"} {
			if strings.Contains(r.Content, bad) {
				t.Fatalf("%s listed: %s", bad, r.Content)
			}
		}
	}
	// The public nearby endpoint shares the query: the expired contract is
	// hidden there too.
	rows, err := orgusecase.New(it.pool, it.q).NearbyDealers(ctx, center.BrandID, orgusecase.NearbyInput{Lat: -47.1, Lng: -139.1, RadiusKm: 5})
	if err != nil || len(rows) != 1 || rows[0].UUID != ok.Uuid {
		t.Fatalf("public nearby: %+v %v", rows, err)
	}
}
