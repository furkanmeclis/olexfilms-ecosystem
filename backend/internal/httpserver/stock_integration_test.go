package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// stockChain posts ledger movements in committed transactions (as the
// order/transfer flows will). stock_movements is append-only, so fixture
// rows stay in the test database; names carry the test suffix.
type stockChain struct {
	it  *itest
	l   *ledger.Ledger
	ref int64
}

func (it *itest) stockChain() *stockChain {
	return &stockChain{it: it, l: ledger.New(it.q, outbox.NewStore(it.pool, it.q)), ref: time.Now().UnixNano() % 1_000_000_000_000}
}

func (c *stockChain) nextRef() int64 { c.ref++; return c.ref }

func (c *stockChain) location(org db.Organization, code string) ledger.Owner {
	c.it.t.Helper()
	loc, err := c.it.q.CreateWarehouseLocation(context.Background(), db.CreateWarehouseLocationParams{
		OrganizationID: org.ID, Code: code + "-" + c.it.suffix, Name: "Bin " + code, Active: true,
	})
	if err != nil {
		c.it.t.Fatalf("location: %v", err)
	}
	return ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: org.ID}
}

func (c *stockChain) unit(center db.Organization, p db.Product, n int) db.Unit {
	c.it.t.Helper()
	u, err := c.it.q.CreateUnit(context.Background(), db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: center.BrandID, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T155-%s-%d", c.it.suffix, n),
		UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
	})
	if err != nil {
		c.it.t.Fatalf("unit: %v", err)
	}
	return u
}

func (c *stockChain) post(typ ledger.MovementType, u db.Unit, ref int64, to ledger.Owner) {
	c.it.t.Helper()
	ctx := context.Background()
	tx, err := c.it.pool.Begin(ctx)
	if err != nil {
		c.it.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := c.l.Post(ctx, tx, ledger.Movement{
		Type: typ, UnitID: u.ID, To: &to, Source: "test", RefType: "t155", RefID: ref,
	}); err != nil {
		c.it.t.Fatalf("post %s: %v", typ, err)
	}
	if err := tx.Commit(ctx); err != nil {
		c.it.t.Fatal(err)
	}
}

// ship moves a unit from its holder to org (out + in) and returns.
func (c *stockChain) ship(u db.Unit, out, in ledger.MovementType, org db.Organization, at ledger.Owner) {
	ref := c.nextRef()
	c.post(out, u, ref, ledger.Owner{Type: ledger.OwnerOrganization, ID: org.ID, OrgID: org.ID})
	c.post(in, u, ref, at)
}

type stockOrgRef struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type stockOwnerView struct {
	Type         string       `json:"type"`
	Organization *stockOrgRef `json:"organization"`
	Masked       bool         `json:"masked"`
}

type unitHistoryView struct {
	Unit struct {
		Barcode string `json:"barcode"`
		Status  string `json:"status"`
	} `json:"unit"`
	Current *struct {
		Holder stockOrgRef `json:"holder"`
		Status string      `json:"status"`
	} `json:"current"`
	Movements struct {
		Total int64 `json:"total"`
		Items []struct {
			Type               string          `json:"type"`
			Organization       *stockOrgRef    `json:"organization"`
			OrganizationMasked bool            `json:"organization_masked"`
			ToOwner            *stockOwnerView `json:"to_owner"`
		} `json:"items"`
	} `json:"movements"`
}

type productStockPage struct {
	Total int64 `json:"total"`
	Items []struct {
		Product struct {
			UUID string `json:"uuid"`
		} `json:"product"`
		Quantity int32  `json:"quantity"`
		Meters   string `json:"meters"`
	} `json:"items"`
}

func (it *itest) unitHistory(token, barcode, query string) (int, unitHistoryView) {
	it.t.Helper()
	code, env := it.do("GET", "/v1/stock/units/by-barcode/"+barcode+query, hostOlex, token, nil)
	var v unitHistoryView
	if code == http.StatusOK {
		if err := json.Unmarshal(env.Data, &v); err != nil {
			it.t.Fatal(err)
		}
	}
	return code, v
}

func (it *itest) productStock(token, path string) (int, productStockPage) {
	it.t.Helper()
	code, env := it.do("GET", path, hostOlex, token, nil)
	var p productStockPage
	if code == http.StatusOK {
		if err := json.Unmarshal(env.Data, &p); err != nil {
			it.t.Fatal(err)
		}
	}
	return code, p
}

func (p productStockPage) qty(productUUID string) (int32, bool) {
	for _, i := range p.Items {
		if i.Product.UUID == productUUID {
			return i.Quantity, true
		}
	}
	return 0, false
}

// TEC-155 acceptance: a chain center -> distributor -> dealer built with
// ledger.Post. The dealer reads the barcode history (its suppliers named,
// other organizations masked); a dealer of another distributor gets 404;
// the organization stock projections are right after the ledger.
func TestIntegrationStockReadAPI(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t155-dist", "distributor", center)
	dealer := it.org("t155-dealer", "dealer", dist)
	otherDist := it.org("t155-dist2", "distributor", center)
	otherDealer := it.org("t155-dealer2", "dealer", otherDist)

	wh, whPw := it.user("t155-wh")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	distOwner, dPw := it.user("t155-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerOwner, rPw := it.user("t155-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	otherOwner, oPw := it.user("t155-dealer2-owner")
	it.member(otherDealer, otherOwner, "owner")

	p := it.product(center, "T155")
	c := it.stockChain()
	cLoc := c.location(center, "C")
	dLoc := c.location(dist, "D")
	oLoc := c.location(otherDist, "O")
	dealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}

	// u1: center -> distributor -> dealer.
	u1 := c.unit(center, p, 1)
	c.post(ledger.TypeEntry, u1, c.nextRef(), cLoc)
	c.ship(u1, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	c.ship(u1, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOrg)
	// u2: stays at the distributor.
	u2 := c.unit(center, p, 2)
	c.post(ledger.TypeEntry, u2, c.nextRef(), cLoc)
	c.ship(u2, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	// u3: passes through the other distributor before reaching the dealer.
	u3 := c.unit(center, p, 3)
	c.post(ledger.TypeEntry, u3, c.nextRef(), cLoc)
	c.ship(u3, ledger.TypeTransferOut, ledger.TypeTransferIn, otherDist, oLoc)
	c.ship(u3, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	c.ship(u3, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOrg)

	whTok := it.loginOrg(wh, whPw, center)
	distTok := it.loginOrg(distOwner, dPw, dist)
	dealerTok := it.loginOrg(dealerOwner, rPw, dealer)
	otherTok := it.loginOrg(otherOwner, oPw, otherDealer)

	// 1. The dealer reads the history of its unit in ledger order.
	code, h := it.unitHistory(dealerTok, u1.Barcode, "")
	if code != http.StatusOK {
		t.Fatalf("dealer history = %d", code)
	}
	wantTypes := []string{"entry", "transfer_out", "transfer_in", "order_out", "received"}
	if h.Movements.Total != int64(len(wantTypes)) || len(h.Movements.Items) != len(wantTypes) {
		t.Fatalf("dealer history total = %d items = %d", h.Movements.Total, len(h.Movements.Items))
	}
	for i, w := range wantTypes {
		m := h.Movements.Items[i]
		if m.Type != w || m.OrganizationMasked || m.Organization == nil {
			t.Fatalf("movement %d = %+v, want %s with its supplier named", i, m, w)
		}
	}
	if h.Movements.Items[0].Organization.UUID != center.Uuid.String() ||
		h.Movements.Items[3].Organization.UUID != dist.Uuid.String() {
		t.Fatalf("supplier rows = %+v", h.Movements.Items)
	}
	if h.Current == nil || h.Current.Holder.UUID != dealer.Uuid.String() || h.Current.Status != "available" {
		t.Fatalf("current = %+v", h.Current)
	}
	// Paging keeps the order.
	if code, h = it.unitHistory(dealerTok, u1.Barcode, "?limit=2&offset=3"); code != http.StatusOK ||
		len(h.Movements.Items) != 2 || h.Movements.Items[0].Type != "order_out" || h.Movements.Total != 5 {
		t.Fatalf("paged history = %d %+v", code, h.Movements)
	}

	// 2. A dealer of another distributor gets 404 for the same barcode; the
	// dealer gets 404 for a unit still at its distributor.
	if code, _ = it.unitHistory(otherTok, u1.Barcode, ""); code != http.StatusNotFound {
		t.Fatalf("other dealer history = %d, want 404", code)
	}
	if code, _ = it.unitHistory(dealerTok, u2.Barcode, ""); code != http.StatusNotFound {
		t.Fatalf("dealer history of a distributor unit = %d, want 404", code)
	}
	if code, _ = it.unitHistory(distTok, u2.Barcode, ""); code != http.StatusOK {
		t.Fatalf("distributor history of its unit = %d", code)
	}

	// 3. Rows of an organization outside the dealer's chain are masked.
	if code, h = it.unitHistory(dealerTok, u3.Barcode, ""); code != http.StatusOK || h.Movements.Total != 7 {
		t.Fatalf("u3 history = %d total %d", code, h.Movements.Total)
	}
	masked := 0
	for _, m := range h.Movements.Items {
		if m.Organization != nil && m.Organization.UUID == otherDist.Uuid.String() {
			t.Fatalf("other distributor named in the dealer's history: %+v", m)
		}
		if m.ToOwner != nil && m.ToOwner.Organization != nil && m.ToOwner.Organization.UUID == otherDist.Uuid.String() {
			t.Fatalf("other distributor owner named: %+v", m.ToOwner)
		}
		if m.OrganizationMasked {
			masked++
		}
	}
	if masked != 2 { // transfer_in at the other distributor + its transfer_out
		t.Fatalf("masked rows = %d, want 2", masked)
	}
	// The center warehouse (scope all, K20) sees every name.
	if code, h = it.unitHistory(whTok, u3.Barcode, ""); code != http.StatusOK {
		t.Fatalf("warehouse history = %d", code)
	}
	for _, m := range h.Movements.Items {
		if m.OrganizationMasked || m.Organization == nil {
			t.Fatalf("warehouse sees a masked row: %+v", m)
		}
	}

	// 4. Organization stock after the ledger.
	pid := p.Uuid.String()
	for _, tc := range []struct {
		name  string
		token string
		org   db.Organization
		want  int32
	}{
		{"dealer own", dealerTok, dealer, 2},
		{"distributor own", distTok, dist, 1},
		{"warehouse dealer", whTok, dealer, 2},
		{"warehouse center", whTok, center, 0},
		{"warehouse other distributor", whTok, otherDist, 0},
	} {
		code, page := it.productStock(tc.token, "/v1/stock/organizations/"+tc.org.Uuid.String()+"/products?product_uuid="+pid)
		got, ok := page.qty(pid)
		if code != http.StatusOK || !ok || got != tc.want {
			t.Fatalf("%s stock = %d (%v, found %v), want %d", tc.name, code, got, ok, tc.want)
		}
	}
	if code, _ := it.productStock(otherTok, "/v1/stock/organizations/"+dealer.Uuid.String()+"/products"); code != http.StatusNotFound {
		t.Fatalf("other dealer reading the dealer stock = %d, want 404", code)
	}
	// Status filter: the center row is out of stock.
	if _, page := it.productStock(whTok, "/v1/stock/organizations/"+center.Uuid.String()+"/products?status=in_stock&product_uuid="+pid); page.Total != 0 {
		t.Fatalf("center in_stock = %d rows", page.Total)
	}
	if code, _ := it.productStock(whTok, "/v1/stock/organizations/"+center.Uuid.String()+"/products?status=bogus"); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d", code)
	}

	// 5. Bin stock: u2 sits in the distributor's location.
	locs, err := it.q.ListWarehouseLocationsByIDs(context.Background(), []int64{dLoc.ID})
	if err != nil || len(locs) != 1 {
		t.Fatalf("location: %v", err)
	}
	code, page := it.productStock(distTok, "/v1/stock/locations/"+locs[0].Uuid.String()+"/products")
	if got, ok := page.qty(pid); code != http.StatusOK || !ok || got != 1 {
		t.Fatalf("distributor bin stock = %d %d", code, got)
	}
	if code, _ := it.productStock(dealerTok, "/v1/stock/locations/"+locs[0].Uuid.String()+"/products"); code != http.StatusNotFound {
		t.Fatalf("dealer reading the distributor bin = %d, want 404", code)
	}
}
