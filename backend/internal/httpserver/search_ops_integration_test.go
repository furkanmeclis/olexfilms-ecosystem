package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/indexsync"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// stockEventsOfUnit decodes the stock.* outbox rows of one unit (the event
// entity is the movement; the unit uuid is in the payload).
func (it *itest) stockEventsOfUnit(unitUUID uuid.UUID) []events.Event {
	it.t.Helper()
	rows, err := it.pool.Query(context.Background(), `SELECT event_name, payload FROM outbox_events
		WHERE event_name LIKE 'stock.%' AND payload->'data'->>'unit_uuid' = $1 ORDER BY id`, unitUUID.String())
	if err != nil {
		it.t.Fatal(err)
	}
	defer rows.Close()
	var out []events.Event
	for rows.Next() {
		var (
			name string
			raw  []byte
		)
		if err := rows.Scan(&name, &raw); err != nil {
			it.t.Fatal(err)
		}
		var env struct {
			EventID    uuid.UUID      `json:"event_id"`
			EntityType string         `json:"entity_type"`
			EntityID   *int64         `json:"entity_id"`
			EntityUUID *uuid.UUID     `json:"entity_uuid"`
			Data       map[string]any `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			it.t.Fatal(err)
		}
		out = append(out, events.Event{
			Name: name, EventID: env.EventID, EntityType: env.EntityType, EntityID: env.EntityID,
			EntityUUID: env.EntityUUID, Payload: env.Data, OccurredAt: time.Now().UTC(),
		})
	}
	if len(out) == 0 {
		it.t.Fatalf("no stock outbox event for unit %s", unitUUID)
	}
	return out
}

// TEC-210 acceptance: the organizations, orders and stock units indexes.
//  1. Reindex (every adapter's ListAll) loads all records.
//  2. q goes to the index: the dealer code and the barcode find the right
//     record; an organization outside the caller's organizations.read reach
//     (center: brand, distributor: subtree, dealer: itself), another party's
//     order and another holder's unit never show, also when the index
//     ignores the filter (the Postgres reload keeps the scope).
//  3. Outbox -> index: organization.updated (tenant settings rename), an
//     order transition and the ledger movements of a shipment refresh the
//     documents.
func TestIntegrationSearchOpsIndexes(t *testing.T) {
	finder := &lateFinder{}
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.SearchFinder = finder })
	ctx := context.Background()
	mem := newMemIndex(t, searchengine.NewRegistry(
		orgusecase.NewSearchAdapter(it.q),
		ordersusecase.NewSearchAdapter(it.q),
		stockusecase.NewSearchAdapter(it.q),
	))
	finder.m = mem

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
	dist := it.org("t210-dist", "distributor", center)
	dealerA := it.org("t210-a", "dealer", dist)
	dealerB := it.org("t210-b", "dealer", dist)
	otherDist := it.org("t210-dist2", "distributor", center)
	otherDealer := it.org("t210-c", "dealer", otherDist)
	gDealer := it.org("t210-g", "dealer", it.brandCenter("glorian"))

	login := func(org db.Organization, name, role string) string {
		u, pw := it.user(name)
		it.member(org, u, role)
		return it.loginOrg(u, pw, org)
	}
	tokA := login(dealerA, "t210-owner-a", "owner")
	tokDist := login(dist, "t210-dist-owner", "owner")
	tokOther := login(otherDist, "t210-dist2-owner", "owner")
	tokCenter := login(center, "t210-center", "staff")

	// Orders: both distributors buy from the center.
	p := it.product(center, "T210")
	it.setListPrice(p, cur, "50")
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

	// Stock: u1 waits in the distributor's bin (shipped to dealer A in 3c),
	// u2 sits at the sibling distributor's dealer.
	c := it.stockChain()
	cLoc := c.location(center, "C")
	dLoc := c.location(dist, "D")
	oLoc := c.location(otherDist, "O")
	u1 := c.unit(center, p, 2101)
	c.post(ledger.TypeEntry, u1, c.nextRef(), cLoc)
	c.ship(u1, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	u2 := c.unit(center, p, 2102)
	c.post(ledger.TypeEntry, u2, c.nextRef(), cLoc)
	c.ship(u2, ledger.TypeTransferOut, ledger.TypeTransferIn, otherDist, oLoc)
	c.ship(u2, ledger.TypeOrderOut, ledger.TypeReceived, otherDealer,
		ledger.Owner{Type: ledger.OwnerOrganization, ID: otherDealer.ID, OrgID: otherDealer.ID})

	// 1. Reindex loads every record.
	counts := mem.reindex(ctx)
	for spec, ids := range map[string][]string{
		searchengine.SpecOrganizations: {dist.Uuid.String(), dealerA.Uuid.String(), otherDealer.Uuid.String(), gDealer.Uuid.String()},
		searchengine.SpecOrders:        {ordDist.UUID, ordOther.UUID},
		searchengine.SpecStockUnits:    {u1.Uuid.String(), u2.Uuid.String()},
	} {
		for _, id := range ids {
			if _, ok := mem.doc(spec, id); !ok {
				t.Fatalf("reindex %s (%d docs) misses %s", spec, counts[spec], id)
			}
		}
	}
	if d, _ := mem.doc(searchengine.SpecStockUnits, u1.Uuid.String()); d.Title != u1.Barcode ||
		!slices.Equal(d.OrganizationIDs, []int64{dist.ID}) || !slices.Contains(d.Keywords, "D-"+it.suffix) {
		t.Fatalf("unit doc = %+v", d)
	}

	// 2. Scope + brand, strict and leaky index.
	get := func(path, tok string) map[string]bool {
		t.Helper()
		return listUUIDs(t, it.custDo("GET", path, tok, nil, http.StatusOK))
	}
	orgs := func(tok, q string) map[string]bool {
		t.Helper()
		return get("/v1/tenant/organizations?q="+url.QueryEscape(q), tok)
	}
	units := func(tok string, org db.Organization, q string) map[string]bool {
		t.Helper()
		return get("/v1/stock/organizations/"+org.Uuid.String()+"/units?q="+url.QueryEscape(q), tok)
	}
	for _, leaky := range []bool{false, true} {
		mem.leaky = leaky
		label := fmt.Sprintf("leaky=%v", leaky)
		// Dealer code (slug) finds the organization inside the reach only.
		if got := orgs(tokA, dealerA.Slug); !got[dealerA.Uuid.String()] || len(got) != 1 {
			t.Fatalf("%s dealer A by own code = %v", label, got)
		}
		if got := orgs(tokA, dealerB.Slug); len(got) != 0 {
			t.Fatalf("%s dealer A found dealer B: %v", label, got)
		}
		got := orgs(tokDist, it.suffix)
		if !got[dist.Uuid.String()] || !got[dealerA.Uuid.String()] || !got[dealerB.Uuid.String()] ||
			got[otherDist.Uuid.String()] || got[otherDealer.Uuid.String()] || got[gDealer.Uuid.String()] {
			t.Fatalf("%s distributor subtree = %v", label, got)
		}
		got = orgs(tokCenter, it.suffix)
		if !got[otherDealer.Uuid.String()] || !got[dealerB.Uuid.String()] || got[gDealer.Uuid.String()] {
			t.Fatalf("%s center brand = %v", label, got)
		}
		// Order number: seller / buyer scope.
		if got := get("/v1/orders?q="+url.QueryEscape(noDist), tokDist); !got[ordDist.UUID] || len(got) != 1 {
			t.Fatalf("%s distributor own order = %v", label, got)
		}
		if got := get("/v1/orders?q="+url.QueryEscape(noDist), tokOther); len(got) != 0 {
			t.Fatalf("%s sibling found the order: %v", label, got)
		}
		if got := get("/v1/orders?side=seller&q="+url.QueryEscape(noOther), tokCenter); !got[ordOther.UUID] {
			t.Fatalf("%s center sales by number = %v", label, got)
		}
		// Barcode: the holder's list only (TEC-216 subtree reach).
		if got := units(tokDist, dist, u1.Barcode); !got[u1.Uuid.String()] || len(got) != 1 {
			t.Fatalf("%s distributor unit by barcode = %v", label, got)
		}
		if got := units(tokDist, dist, u2.Barcode); len(got) != 0 {
			t.Fatalf("%s distributor list shows another holder's unit: %v", label, got)
		}
	}
	mem.leaky = false
	if code, _ := it.orgUnits(tokDist, otherDealer, "?q="+u2.Barcode); code != http.StatusNotFound {
		t.Fatalf("distributor on a sibling's dealer units = %d", code)
	}
	for _, spec := range []string{searchengine.SpecOrganizations, searchengine.SpecOrders, searchengine.SpecStockUnits} {
		if mem.queries[spec] == 0 {
			t.Fatalf("%s list did not query the index", spec)
		}
	}

	// 3. Outbox -> index.
	sync := indexsync.New(it.q, mem, nil)

	// 3a. A tenant settings rename refreshes the organization document.
	newName := "Yeni Ad " + it.suffix
	it.custDo("PATCH", "/v1/tenant/settings", tokA, map[string]any{"company_name": newName}, http.StatusOK)
	if got := orgs(tokA, "Yeni Ad"); len(got) != 0 {
		t.Fatalf("index answered the new name before the outbox event: %v", got)
	}
	for _, ev := range it.outboxEventsOf(events.OrganizationUpdated, dealerA.Uuid.String()) {
		if err := sync.HandleOrganization(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if got := orgs(tokA, "Yeni Ad"); !got[dealerA.Uuid.String()] {
		t.Fatalf("organization not found by the new name after organization.updated: %v", got)
	}

	// 3b. An order transition refreshes the status filter.
	if code, ec := it.transition(tokDist, ordDist.UUID, "submitted"); code != http.StatusOK {
		t.Fatalf("submit = %d %s", code, ec)
	}
	statusQ := "/v1/orders?status=submitted&q=" + url.QueryEscape(noDist)
	if got := get(statusQ, tokDist); len(got) != 0 {
		t.Fatalf("index answered the new status before the outbox event: %v", got)
	}
	for _, ev := range it.outboxEventsOf(events.OrdersSubmitted, ordDist.UUID) {
		if err := sync.HandleOrder(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if got := get(statusQ, tokDist); !got[ordDist.UUID] {
		t.Fatalf("submitted order by status filter = %v", got)
	}

	// 3c. Ledger: shipping u1 to dealer A moves the unit document.
	c.ship(u1, ledger.TypeOrderOut, ledger.TypeReceived, dealerA,
		ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID})
	if got := units(tokA, dealerA, u1.Barcode); len(got) != 0 {
		t.Fatalf("index answered the new holder before the ledger events: %v", got)
	}
	for _, ev := range it.stockEventsOfUnit(u1.Uuid) {
		if err := sync.HandleStock(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if d, _ := mem.doc(searchengine.SpecStockUnits, u1.Uuid.String()); !slices.Equal(d.OrganizationIDs, []int64{dealerA.ID}) ||
		d.Status != "available" {
		t.Fatalf("unit doc after the shipment = %+v", d)
	}
	if got := units(tokA, dealerA, u1.Barcode); !got[u1.Uuid.String()] {
		t.Fatalf("dealer A unit by barcode after the ledger events = %v", got)
	}
	if got := units(tokDist, dist, u1.Barcode); len(got) != 0 {
		t.Fatalf("distributor still lists the shipped unit: %v", got)
	}
}
