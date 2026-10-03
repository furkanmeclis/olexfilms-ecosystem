package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	searchusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/usecase"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// globalGroups maps spec -> hit id -> hit of a GET /v1/search/global answer.
type globalGroups map[string]map[string]searchengine.Hit

func (it *itest) global(tok, q string) (searchusecase.GlobalResult, globalGroups) {
	it.t.Helper()
	res := decodeData[searchusecase.GlobalResult](it.t,
		it.custDo("GET", "/v1/search/global?limit=10&q="+url.QueryEscape(q), tok, nil, http.StatusOK))
	out := globalGroups{}
	for _, g := range res.Groups {
		if _, dup := out[g.Spec]; dup {
			it.t.Fatalf("group %s twice", g.Spec)
		}
		out[g.Spec] = map[string]searchengine.Hit{}
		for _, h := range g.Items {
			out[g.Spec][h.ID] = h
		}
	}
	return res, out
}

// TEC-213 acceptance: GET /v1/search/global searches every index through
// its module list.
//  1. Each type returns the right record (customer, vehicle, service,
//     warranty, order, organization, stock unit) with its detail link.
//  2. Records outside the caller's list scope never show, also when the
//     index ignores the filter (the list's Postgres reload keeps the scope).
//  3. A type the caller has no permission for is neither returned nor
//     queried (dealer accounting: orders only).
//  4. Meilisearch off: empty groups + info search_disabled.
func TestIntegrationGlobalSearch(t *testing.T) {
	finder := &lateFinder{}
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.SearchFinder = finder })
	ctx := context.Background()
	mem := newMemIndex(t, searchengine.NewRegistry(
		customersusecase.NewSearchAdapter(it.q),
		customersusecase.NewVehicleSearchAdapter(it.q),
		servicesusecase.NewSearchAdapter(it.q),
		warrantyusecase.NewSearchAdapter(it.q),
		orgusecase.NewSearchAdapter(it.q),
		ordersusecase.NewSearchAdapter(it.q),
		stockusecase.NewSearchAdapter(it.q),
	))
	finder.m = mem

	sfx := it.suffix[len(it.suffix)-6:]
	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	cur := strings.TrimSpace(brand.Currency)
	if cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	dist := it.org("t213-dist", "distributor", center)
	dealerA := it.org("t213-a", "dealer", dist)
	dealerB := it.org("t213-b", "dealer", dist)
	otherDist := it.org("t213-dist2", "distributor", center)

	p := it.product(center, "T213")
	it.setWarrantyMonths(p, 12)
	it.setListPrice(p, cur, "50")
	custA, vehA := it.svcCustomer(dealerA, "t213-cust-a", "34T213"+sfx)
	custB, vehB := it.svcCustomer(dealerB, "t213-cust-b", "06T213"+sfx)
	wA := it.warrantyFor(dealerA, center, p, custA, vehA, 213)
	wB := it.warrantyFor(dealerB, center, p, custB, vehB, 214)
	serviceUUID := func(id int64) uuid.UUID {
		var u uuid.UUID
		if err := it.pool.QueryRow(ctx, `SELECT uuid FROM services WHERE id = $1`, id).Scan(&u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	svcA, svcB := serviceUUID(wA.ServiceID), serviceUUID(wB.ServiceID)

	login := func(org db.Organization, name, role string, roles ...string) string {
		u, pw := it.user(name)
		it.member(org, u, role, roles...)
		return it.loginOrg(u, pw, org)
	}
	tokA := login(dealerA, "t213-owner-a", "owner")
	tokAcc := login(dealerA, "t213-acc-a", "staff", rbac.RoleDealerAccounting)
	tokDist := login(dist, "t213-dist-owner", "owner")
	tokOther := login(otherDist, "t213-dist2-owner", "owner")

	item := map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1}
	ordDist := it.orderCall("POST", "/v1/orders", tokDist, map[string]any{"items": []any{item}}, http.StatusCreated)
	ordOther := it.orderCall("POST", "/v1/orders", tokOther, map[string]any{"items": []any{item}}, http.StatusCreated)
	orderNo := func(id string) string {
		var no string
		if err := it.pool.QueryRow(ctx, `SELECT order_no FROM orders WHERE uuid = $1`, id).Scan(&no); err != nil {
			t.Fatal(err)
		}
		return no
	}
	noDist, noOther := orderNo(ordDist.UUID), orderNo(ordOther.UUID)

	c := it.stockChain()
	cLoc := c.location(center, "C")
	dLoc := c.location(dist, "D")
	oLoc := c.location(otherDist, "O")
	u1 := c.unit(center, p, 2131)
	c.post(ledger.TypeEntry, u1, c.nextRef(), cLoc)
	c.ship(u1, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	u2 := c.unit(center, p, 2132)
	c.post(ledger.TypeEntry, u2, c.nextRef(), cLoc)
	c.ship(u2, ledger.TypeTransferOut, ledger.TypeTransferIn, otherDist, oLoc)

	mem.reindex(ctx)

	has := func(label string, g globalGroups, spec, id, href string) {
		t.Helper()
		h, ok := g[spec][id]
		if !ok {
			t.Fatalf("%s: %s group misses %s (%v)", label, spec, id, g[spec])
		}
		if href != "" && h.Href != href {
			t.Fatalf("%s: %s href = %q, want %q", label, spec, h.Href, href)
		}
		if h.Title == "" {
			t.Fatalf("%s: %s hit without title: %+v", label, spec, h)
		}
	}
	none := func(label string, g globalGroups, spec string, ids ...string) {
		t.Helper()
		for _, id := range ids {
			if _, ok := g[spec][id]; ok {
				t.Fatalf("%s: %s group leaks %s", label, spec, id)
			}
		}
	}

	for _, leaky := range []bool{false, true} {
		mem.leaky = leaky
		label := fmt.Sprintf("leaky=%v", leaky)

		// 1 + 2. Dealer A owner: its customer, vehicle, service, warranty.
		res, g := it.global(tokA, "t213-cust-a")
		if !res.Enabled || res.Info != nil {
			t.Fatalf("%s: enabled=%v info=%v", label, res.Enabled, res.Info)
		}
		has(label, g, customersusecase.SearchSpec, custA.Uuid.String(), "/customers/"+custA.Uuid.String())
		has(label, g, searchengine.SpecVehicles, vehA.Uuid.String(), "/vehicles/"+vehA.Uuid.String())
		has(label, g, searchengine.SpecServices, svcA.String(), "/services/"+svcA.String())
		has(label, g, searchengine.SpecWarranties, wA.Uuid.String(), "/warranties/"+wA.Uuid.String())
		_, g = it.global(tokA, "t213-cust-b")
		none(label, g, customersusecase.SearchSpec, custB.Uuid.String())
		none(label, g, searchengine.SpecVehicles, vehB.Uuid.String())
		none(label, g, searchengine.SpecServices, svcB.String())
		none(label, g, searchengine.SpecWarranties, wB.Uuid.String())
		// Plate and warranty public code.
		_, g = it.global(tokA, "34T213"+sfx)
		has(label, g, searchengine.SpecVehicles, vehA.Uuid.String(), "")
		_, g = it.global(tokA, wB.PublicCode)
		none(label, g, searchengine.SpecWarranties, wB.Uuid.String())
		// Dealer code: own organization only.
		_, g = it.global(tokA, dealerA.Slug)
		has(label, g, searchengine.SpecOrganizations, dealerA.Uuid.String(), "/organizations/"+dealerA.Uuid.String())
		_, g = it.global(tokA, dealerB.Slug)
		none(label, g, searchengine.SpecOrganizations, dealerB.Uuid.String())

		// Distributor: its order and its own unit by barcode, nothing of
		// the sibling distributor.
		_, g = it.global(tokDist, noDist)
		has(label, g, searchengine.SpecOrders, ordDist.UUID, "/orders/"+ordDist.UUID)
		_, g = it.global(tokDist, noOther)
		none(label, g, searchengine.SpecOrders, ordOther.UUID)
		_, g = it.global(tokDist, u1.Barcode)
		has(label, g, searchengine.SpecStockUnits, u1.Uuid.String(), "")
		_, g = it.global(tokDist, u2.Barcode)
		none(label, g, searchengine.SpecStockUnits, u2.Uuid.String())
		_, g = it.global(tokDist, dealerB.Slug)
		has(label, g, searchengine.SpecOrganizations, dealerB.Uuid.String(), "")
	}
	mem.leaky = false

	// 3. Dealer accounting holds orders.read only (of these types): the
	// other groups are not in the answer and their indexes are not queried.
	before := map[string]int{}
	for k, v := range mem.queries {
		before[k] = v
	}
	_, g := it.global(tokAcc, "t213-cust-a")
	for _, spec := range []string{customersusecase.SearchSpec, searchengine.SpecVehicles, searchengine.SpecServices,
		searchengine.SpecWarranties, searchengine.SpecStockUnits, searchengine.SpecOrganizations} {
		if _, ok := g[spec]; ok {
			t.Fatalf("dealer accounting got the %s group", spec)
		}
		if mem.queries[spec] != before[spec] {
			t.Fatalf("dealer accounting queried the %s index", spec)
		}
	}
	if _, ok := g[searchengine.SpecOrders]; !ok {
		t.Fatalf("dealer accounting misses the orders group: %v", g)
	}

	// spec narrows to one group; q over 100 characters is 400.
	res := decodeData[searchusecase.GlobalResult](t, it.custDo("GET",
		"/v1/search/global?spec=customers&q=t213-cust-a", tokA, nil, http.StatusOK))
	if len(res.Groups) != 1 || res.Groups[0].Spec != customersusecase.SearchSpec {
		t.Fatalf("spec=customers groups = %+v", res.Groups)
	}
	it.custDo("GET", "/v1/search/global?q="+strings.Repeat("x", 101), tokA, nil, http.StatusBadRequest)

	// 4. Meilisearch off: the groups stay, empty, with the info code.
	finder.m = nil
	res, g = it.global(tokA, "t213-cust-a")
	if res.Enabled || res.Info == nil || *res.Info != searchusecase.InfoSearchDisabled {
		t.Fatalf("disabled: enabled=%v info=%v", res.Enabled, res.Info)
	}
	if _, ok := g[customersusecase.SearchSpec]; !ok {
		t.Fatalf("disabled: customers group missing: %v", g)
	}
	for spec, hits := range g {
		if len(hits) != 0 {
			t.Fatalf("disabled: %s has hits", spec)
		}
	}
}
