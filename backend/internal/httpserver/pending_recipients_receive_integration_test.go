package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1062 decision follow-up:
//   - a pool recipient has no member, so the center receives the shipped
//     order on its behalf (button on the detail, history marked) and the
//     K9 cari rows are booked; a recipient with an active member still
//     receives on its own (the center gets 403);
//   - GET /v1/tenant/organizations leaves pending organizations out unless
//     include_pending=true (the center's order form).
func TestIntegrationPendingRecipientReceiveOnBehalf(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
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
	owner, pw := it.user("t-pool-rcv-owner")
	it.member(center, owner, "owner")
	tok := it.loginOrg(owner, pw, center)

	newRecipient := func(name, kind string) (string, db.Organization) {
		t.Helper()
		c, env := it.do("POST", "/v1/tenant/pending-recipients", hostOlex, tok, map[string]any{
			"name": name + " " + it.suffix, "phone": itPhone(), "kind": kind,
		})
		if c != http.StatusCreated {
			t.Fatalf("create recipient = %d %s", c, errCode(env))
		}
		var rec pendingRecipientView
		if err := json.Unmarshal(env.Data, &rec); err != nil {
			t.Fatal(err)
		}
		org, err := it.q.GetOrganizationByUUID(ctx, uuid.MustParse(rec.UUID))
		if err != nil {
			t.Fatal(err)
		}
		return rec.UUID, org
	}

	p := it.product(center, "TPOOLRCV")
	it.setListPrice(p, cur, "100")
	chain := it.stockChain()
	c, env := it.do("POST", "/v1/delivery-methods", hostOlex, tok, map[string]any{
		"code": fmt.Sprintf("PR%d", time.Now().UnixNano()%1_000_000), "name": "Kargo",
	})
	if c != http.StatusCreated {
		t.Fatalf("delivery method = %d %s", c, errCode(env))
	}
	var method struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(env.Data, &method); err != nil {
		t.Fatal(err)
	}
	serial := 30620
	shipFor := func(recipient string) string {
		t.Helper()
		serial++
		unit := chain.unit(center, p, serial)
		chain.post(ledger.TypeEntry, unit, chain.nextRef(), ledger.Owner{Type: ledger.OwnerOrganization, ID: center.ID, OrgID: center.ID})
		o := it.orderCall("POST", "/v1/orders", tok, map[string]any{
			"buyer_org_uuid": recipient,
			"items":          []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1, "unit_price": "250"}},
		}, http.StatusCreated)
		if c, e := it.transition(tok, o.UUID, "preparing"); c != http.StatusOK {
			t.Fatalf("preparing = %d %s", c, e)
		}
		if c, env := it.do("POST", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID+"/units", hostOlex, tok,
			map[string]any{"barcode": unit.Barcode}); c != http.StatusOK {
			t.Fatalf("assign = %d %s", c, errCode(env))
		}
		if c, e := it.transition(tok, o.UUID, "ready"); c != http.StatusOK {
			t.Fatalf("ready = %d %s", c, e)
		}
		if c, env := it.do("POST", "/v1/orders/"+o.UUID+"/transitions", hostOlex, tok, map[string]any{
			"status": "shipped", "delivery_method_uuid": method.UUID,
		}); c != http.StatusOK {
			t.Fatalf("shipped = %d %s", c, errCode(env))
		}
		return o.UUID
	}
	type detail struct {
		Status               string   `json:"status"`
		AvailableTransitions []string `json:"available_transitions"`
		ReceiveOnBehalf      bool     `json:"receive_on_behalf"`
		History              []struct {
			ToStatus        string `json:"to_status"`
			OnBehalfOfBuyer bool   `json:"on_behalf_of_buyer"`
		} `json:"history"`
	}
	get := func(orderUUID string) detail {
		t.Helper()
		c, env := it.do("GET", "/v1/orders/"+orderUUID, hostOlex, tok, nil)
		if c != http.StatusOK {
			t.Fatalf("detail = %d %s", c, errCode(env))
		}
		var d detail
		if err := json.Unmarshal(env.Data, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}

	// 1. Memberless pool recipient: the center receives for it.
	poolUUID, poolOrg := newRecipient("Havuz Alıcı", "dealer_candidate")
	o1 := shipFor(poolUUID)
	d := get(o1)
	if !d.ReceiveOnBehalf || !slices.Contains(d.AvailableTransitions, "received") {
		t.Fatalf("shipped detail = %+v", d)
	}
	if c, e := it.transition(tok, o1, "received"); c != http.StatusOK {
		t.Fatalf("received on behalf = %d %s", c, e)
	}
	d = get(o1)
	if d.Status != "received" || d.ReceiveOnBehalf {
		t.Fatalf("received detail = %+v", d)
	}
	last := d.History[len(d.History)-1]
	if last.ToStatus != "received" || !last.OnBehalfOfBuyer {
		t.Fatalf("history = %+v", d.History)
	}
	rows := it.orderEntries(o1)
	var seller, buyer bool
	for _, r := range rows {
		seller = seller || (r.OrgID == center.ID && r.Counterparty == poolOrg.ID && r.Direction == "income")
		buyer = buyer || (r.OrgID == poolOrg.ID && r.Counterparty == center.ID && r.Direction == "expense")
	}
	if len(rows) != 2 || !seller || !buyer {
		t.Fatalf("cari rows = %+v", rows)
	}

	// 2. A retail point with an active member receives on its own.
	retailUUID, retailOrg := newRecipient("Teslim Noktası", "retail_point")
	if c, env := it.do("POST", "/v1/tenant/pending-recipients/"+retailUUID+"/keep-retail", hostOlex, tok, nil); c != http.StatusOK {
		t.Fatalf("keep retail = %d %s", c, errCode(env))
	}
	o2 := shipFor(retailUUID)
	if d := get(o2); !d.ReceiveOnBehalf {
		t.Fatalf("memberless retail detail = %+v", d)
	}
	member, _ := it.user("t-pool-rcv-member")
	it.member(retailOrg, member, "owner")
	if d := get(o2); d.ReceiveOnBehalf || slices.Contains(d.AvailableTransitions, "received") {
		t.Fatalf("retail with member detail = %+v", d)
	}
	if c, _ := it.transition(tok, o2, "received"); c != http.StatusForbidden {
		t.Fatalf("center receives for a buyer with a member = %d", c)
	}

	// 3. Pending organizations: only the order form asks for them.
	orgs := func(query string) map[string]bool {
		t.Helper()
		c, env := it.do("GET", "/v1/tenant/organizations?type=dealer&limit=200&q="+it.suffix+query, hostOlex, tok, nil)
		if c != http.StatusOK {
			t.Fatalf("tenant organizations %s = %d %s", query, c, errCode(env))
		}
		var page struct {
			Items []struct {
				UUID string `json:"uuid"`
			} `json:"items"`
		}
		if err := json.Unmarshal(env.Data, &page); err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, o := range page.Items {
			out[o.UUID] = true
		}
		return out
	}
	if got := orgs(""); got[poolUUID] || got[retailUUID] {
		t.Fatalf("default list shows pending recipients: %v", got)
	}
	if got := orgs("&include_pending=true"); !got[poolUUID] || !got[retailUUID] {
		t.Fatalf("include_pending misses recipients: %v", got)
	}
}
