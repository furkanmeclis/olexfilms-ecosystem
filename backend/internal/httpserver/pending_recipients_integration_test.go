package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type pendingRecipientView struct {
	UUID        string  `json:"uuid"`
	Name        string  `json:"name"`
	Phone       string  `json:"phone"`
	Kind        string  `json:"kind"`
	State       string  `json:"state"`
	OrgStatus   string  `json:"organization_status"`
	TaxNo       *string `json:"tax_no"`
	OrdersCount int64   `json:"orders_count"`
}

// TEC-1062: the center opens a new recipient from the order form (no
// dealer record yet), opens and ships an order for it, finds it in the
// waiting pool and turns it into a dealer with its owner; a second
// recipient is kept as a retail point and still takes orders.
func TestIntegrationPendingRecipients(t *testing.T) {
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
	owner, pw := it.user("t-pool-owner")
	it.member(center, owner, "owner")
	tok := it.loginOrg(owner, pw, center)

	// A center staff member (orders.write, no organizations.write) may add a
	// recipient from the order form but cannot convert one.
	staff, spw := it.user("t-pool-staff")
	it.member(center, staff, "staff", "center_staff")
	staffTok := it.loginOrg(staff, spw, center)

	recipientPhone := itPhone()
	code, env := it.do("POST", "/v1/tenant/pending-recipients", hostOlex, staffTok, map[string]any{
		"name": "Perakende Müşteri " + it.suffix, "phone": recipientPhone, "city": "İzmir",
		"address": "Alsancak", "tax_no": "1234567890", "tax_office": "Konak", "kind": "dealer_candidate",
	})
	if code != http.StatusCreated {
		t.Fatalf("create recipient = %d %s %s", code, errCode(env), env.Data)
	}
	var rec pendingRecipientView
	if err := json.Unmarshal(env.Data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.State != "pending" || rec.Kind != "dealer_candidate" || rec.OrgStatus != "pending" || rec.Phone != recipientPhone ||
		rec.TaxNo == nil || *rec.TaxNo != "1234567890" {
		t.Fatalf("recipient = %+v", rec)
	}
	org, err := it.q.GetOrganizationByUUID(ctx, uuid.MustParse(rec.UUID))
	if err != nil {
		t.Fatal(err)
	}
	if org.Type != "dealer" || !org.ParentID.Valid || org.ParentID.Int64 != center.ID || org.AccessEndsAt.Valid {
		t.Fatalf("recipient org = %+v", org)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM organization_members WHERE organization_id = $1`, org.ID); n != 0 {
		t.Fatalf("recipient members = %d", n)
	}
	// Validation: phone required, tax number digits, kind.
	for _, body := range []map[string]any{
		{"name": "X"},
		{"name": "X", "phone": itPhone(), "tax_no": "12"},
		{"name": "X", "phone": itPhone(), "kind": "warehouse"},
	} {
		if c, env := it.do("POST", "/v1/tenant/pending-recipients", hostOlex, tok, body); c != http.StatusBadRequest {
			t.Fatalf("invalid recipient %v = %d %s", body, c, errCode(env))
		}
	}

	// The order for the recipient: approved, prepared and shipped as usual.
	p := it.product(center, "TPOOL")
	it.setListPrice(p, cur, "100")
	chain := it.stockChain()
	unit := chain.unit(center, p, 20262)
	chain.post(ledger.TypeEntry, unit, chain.nextRef(), ledger.Owner{Type: ledger.OwnerOrganization, ID: center.ID, OrgID: center.ID})
	o := it.orderCall("POST", "/v1/orders", tok, map[string]any{
		"buyer_org_uuid": rec.UUID,
		"items":          []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1, "unit_price": "250"}},
	}, http.StatusCreated)
	if o.Status != "approved" || o.Buyer.UUID != rec.UUID {
		t.Fatalf("order = %s buyer %s", o.Status, o.Buyer.UUID)
	}
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
	c, env := it.do("POST", "/v1/delivery-methods", hostOlex, tok, map[string]any{
		"code": fmt.Sprintf("PK%d", time.Now().UnixNano()%1_000_000), "name": "Kargo",
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
	if c, env := it.do("POST", "/v1/orders/"+o.UUID+"/transitions", hostOlex, tok, map[string]any{
		"status": "shipped", "delivery_method_uuid": method.UUID, "tracking_no": "TRP1",
	}); c != http.StatusOK {
		t.Fatalf("shipped = %d %s", c, errCode(env))
	}
	if st, err := it.q.GetUnitCurrentState(ctx, unit.ID); err != nil || st.HolderOrgID != org.ID {
		t.Fatalf("unit after shipment = %+v %v", st, err)
	}

	// The waiting pool lists it with its order.
	list := func(query string) []pendingRecipientView {
		t.Helper()
		c, env := it.do("GET", "/v1/tenant/pending-recipients"+query, hostOlex, tok, nil)
		if c != http.StatusOK {
			t.Fatalf("list %s = %d %s", query, c, errCode(env))
		}
		var page struct {
			Items []pendingRecipientView `json:"items"`
			Total int64                  `json:"total"`
		}
		if err := json.Unmarshal(env.Data, &page); err != nil {
			t.Fatal(err)
		}
		return page.Items
	}
	find := func(items []pendingRecipientView, id string) *pendingRecipientView {
		for i := range items {
			if items[i].UUID == id {
				return &items[i]
			}
		}
		return nil
	}
	pool := list("?state=pending&q=" + it.suffix)
	if got := find(pool, rec.UUID); got == nil || got.OrdersCount != 1 {
		t.Fatalf("pool = %+v", pool)
	}
	if c, env := it.do("GET", "/v1/tenant/pending-recipients?sort=-bogus", hostOlex, tok, nil); c != http.StatusBadRequest {
		t.Fatalf("bad sort = %d %s", c, errCode(env))
	}

	// Staff cannot convert; the owner converts with the dealer's owner.
	if c, env := it.do("POST", "/v1/tenant/pending-recipients/"+rec.UUID+"/convert", hostOlex, staffTok,
		map[string]any{}); c != http.StatusForbidden {
		t.Fatalf("staff convert = %d %s", c, errCode(env))
	}
	dealerPhone := itPhone()
	c, env = it.do("POST", "/v1/tenant/pending-recipients/"+rec.UUID+"/convert", hostOlex, tok, map[string]any{
		"owner": map[string]any{"name": "Yeni", "surname": "Bayi", "phone": dealerPhone},
	})
	if c != http.StatusOK {
		t.Fatalf("convert = %d %s %s", c, errCode(env), env.Data)
	}
	var conv struct {
		Recipient pendingRecipientView `json:"recipient"`
		Owner     *struct {
			Phone     string   `json:"phone"`
			RoleSlugs []string `json:"role_slugs"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(env.Data, &conv); err != nil {
		t.Fatal(err)
	}
	if conv.Recipient.State != "converted" || conv.Recipient.OrgStatus != "active" || conv.Owner == nil ||
		conv.Owner.Phone != dealerPhone || strings.Join(conv.Owner.RoleSlugs, ",") != "dealer_owner" {
		t.Fatalf("converted = %+v owner %+v", conv.Recipient, conv.Owner)
	}
	// The order stays on the same organization, now an active dealer.
	if row := it.orderRow(o.UUID, center.BrandID); row.BuyerOrgID != org.ID {
		t.Fatalf("order buyer = %d", row.BuyerOrgID)
	}
	if find(list("?state=pending"), rec.UUID) != nil {
		t.Fatal("converted recipient still in the pool")
	}
	if c, env := it.do("POST", "/v1/tenant/pending-recipients/"+rec.UUID+"/convert", hostOlex, tok,
		map[string]any{}); c != http.StatusConflict {
		t.Fatalf("convert twice = %d %s", c, errCode(env))
	}

	// A retail point: kept out of the pool, orders still open for it.
	c, env = it.do("POST", "/v1/tenant/pending-recipients", hostOlex, tok, map[string]any{
		"name": "Teslim Noktası " + it.suffix, "phone": itPhone(), "kind": "retail_point",
	})
	if c != http.StatusCreated {
		t.Fatalf("create retail = %d %s", c, errCode(env))
	}
	var retail pendingRecipientView
	if err := json.Unmarshal(env.Data, &retail); err != nil {
		t.Fatal(err)
	}
	c, env = it.do("POST", "/v1/tenant/pending-recipients/"+retail.UUID+"/keep-retail", hostOlex, tok, nil)
	if c != http.StatusOK || !strings.Contains(string(env.Data), `"state":"retail"`) {
		t.Fatalf("keep retail = %d %s %s", c, errCode(env), env.Data)
	}
	if got := find(list("?state=retail&kind=retail_point"), retail.UUID); got == nil || got.OrgStatus != "pending" {
		t.Fatalf("retail list = %+v", got)
	}
	it.orderCall("POST", "/v1/orders", tok, map[string]any{
		"buyer_org_uuid": retail.UUID,
		"items":          []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1}},
	}, http.StatusCreated)

	// A pending organization that is not in the pool is not a buyer.
	stray, err := it.q.SetRecipientOrganizationStatus(ctx, db.SetRecipientOrganizationStatusParams{ID: org.ID, Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if c, env := it.do("POST", "/v1/orders", hostOlex, tok, map[string]any{
		"buyer_org_uuid": stray.Uuid.String(),
		"items":          []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1}},
	}); c != http.StatusUnprocessableEntity && c != http.StatusBadRequest {
		t.Fatalf("converted recipient back in pending = %d %s", c, errCode(env))
	}
}
