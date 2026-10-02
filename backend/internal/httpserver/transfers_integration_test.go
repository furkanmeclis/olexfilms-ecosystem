package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
)

type stockTransferView struct {
	UUID       string `json:"uuid"`
	TransferNo string `json:"transfer_no"`
	Status     string `json:"status"`
	Role       string `json:"role"`
	Sender     struct {
		UUID string `json:"uuid"`
	} `json:"sender"`
	Receiver struct {
		UUID string `json:"uuid"`
	} `json:"receiver"`
	AvailableTransitions []string `json:"available_transitions"`
	ItemCount            int64    `json:"item_count"`
	Items                []struct {
		Barcode  string `json:"barcode"`
		Shipped  bool   `json:"shipped"`
		Received bool   `json:"received"`
		Restored bool   `json:"restored"`
	} `json:"items"`
}

func (it *itest) transferCall(method, path, access string, body any, want int) (stockTransferView, string) {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, access, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
	}
	var v stockTransferView
	if code < 300 {
		if err := json.Unmarshal(env.Data, &v); err != nil {
			it.t.Fatal(err)
		}
	}
	return v, errCode(env)
}

func (it *itest) transferMove(access, id, status string) (int, string) {
	it.t.Helper()
	code, env := it.do("POST", "/v1/stock-transfers/"+id+"/transitions", hostOlex, access, map[string]string{"status": status})
	return code, errCode(env)
}

// TEC-197 acceptance: dealer A hands a unit to its sibling dealer B. The
// receiver approves, A ships (transfer_out), B receives (transfer_in): the
// unit is available at B. A rejected request writes no stock movement; a
// request to a dealer under another distributor is refused (422); outsiders
// get 404; a cancel after shipping restores the unit to A.
func TestIntegrationStockTransferRequests(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t197-dist", "distributor", center)
	dealerA := it.org("t197-dealer-a", "dealer", dist)
	dealerB := it.org("t197-dealer-b", "dealer", dist)
	dist2 := it.org("t197-dist2", "distributor", center)
	dealerC := it.org("t197-dealer-c", "dealer", dist2)

	ua, apw := it.user("t197-a-owner")
	it.member(dealerA, ua, "owner")
	ub, bpw := it.user("t197-b-owner")
	it.member(dealerB, ub, "owner")
	uc, cpw := it.user("t197-c-owner")
	it.member(dealerC, uc, "owner")
	ud, dpw := it.user("t197-dist-owner")
	it.member(dist, ud, "owner")
	aTok := it.loginOrg(ua, apw, dealerA)
	bTok := it.loginOrg(ub, bpw, dealerB)
	cTok := it.loginOrg(uc, cpw, dealerC)
	dTok := it.loginOrg(ud, dpw, dist)

	piece := it.product(center, "T197P")
	chain := it.stockChain()
	loc := chain.location(center, "T197")
	atA := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID}
	u1 := chain.unit(center, piece, 1971)
	u2 := chain.unit(center, piece, 1972)
	for _, u := range []db.Unit{u1, u2} {
		chain.post(ledger.TypeEntry, u, chain.nextRef(), loc)
		chain.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dealerA, atA)
	}

	state := func(u db.Unit) db.UnitCurrentState {
		t.Helper()
		st, err := it.q.GetUnitCurrentState(ctx, u.ID)
		if err != nil {
			t.Fatalf("state %s: %v", u.Barcode, err)
		}
		return st
	}
	history := func(u db.Unit) string {
		t.Helper()
		rows, err := it.pool.Query(ctx, `SELECT type FROM stock_movements WHERE unit_id = $1 ORDER BY id`, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var types []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			types = append(types, s)
		}
		return strings.Join(types, ",")
	}
	create := func(tok string, to db.Organization, barcode string, want int) (stockTransferView, string) {
		t.Helper()
		return it.transferCall("POST", "/v1/stock-transfers", tok, map[string]any{
			"to_org_uuid": to.Uuid.String(),
			"note":        "t197",
			"items":       []map[string]any{{"barcode": barcode}},
		}, want)
	}
	const base = "entry,transfer_out,transfer_in"

	// Targets: only the sibling under the same distributor.
	code, env := it.do("GET", "/v1/stock-transfers/targets", hostOlex, aTok, nil)
	if code != http.StatusOK {
		t.Fatalf("targets = %d %s", code, errCode(env))
	}
	if s := string(env.Data); !strings.Contains(s, dealerB.Uuid.String()) || strings.Contains(s, dealerC.Uuid.String()) {
		t.Fatalf("targets = %s", s)
	}

	// Another parent: refused, nothing written.
	if _, ec := create(aTok, dealerC, u1.Barcode, http.StatusUnprocessableEntity); ec != "TRANSFER_NOT_SIBLING" {
		t.Fatalf("other parent = %s", ec)
	}
	if _, ec := create(aTok, dist, u1.Barcode, http.StatusUnprocessableEntity); ec != "TRANSFER_NOT_SIBLING" {
		t.Fatalf("parent as target = %s", ec)
	}
	// A unit the sender does not hold.
	other := chain.unit(center, piece, 1973)
	chain.post(ledger.TypeEntry, other, chain.nextRef(), loc)
	if _, ec := create(aTok, dealerB, other.Barcode, http.StatusUnprocessableEntity); ec != "VALIDATION_ERROR" {
		t.Fatalf("foreign unit = %s", ec)
	}

	// 1. Approved, shipped and received: the unit moves to B.
	r1, _ := create(aTok, dealerB, u1.Barcode, http.StatusCreated)
	if r1.Status != "requested" || r1.Role != "sender" || strings.Join(r1.AvailableTransitions, ",") != "cancelled" {
		t.Fatalf("created = %+v", r1)
	}
	if v, _ := it.transferCall("GET", "/v1/stock-transfers/"+r1.UUID, bTok, nil, http.StatusOK); v.Role != "receiver" ||
		strings.Join(v.AvailableTransitions, ",") != "approved,rejected" {
		t.Fatalf("receiver view = %+v", v)
	}
	it.transferCall("GET", "/v1/stock-transfers/"+r1.UUID, cTok, nil, http.StatusNotFound)
	// The same unit cannot be on two open requests.
	if _, ec := create(aTok, dealerB, u1.Barcode, http.StatusUnprocessableEntity); ec != "VALIDATION_ERROR" {
		t.Fatalf("double request = %s", ec)
	}
	if code, _ := it.transferMove(aTok, r1.UUID, "approved"); code != http.StatusForbidden {
		t.Fatalf("sender approve = %d", code)
	}
	if code, ec := it.transferMove(aTok, r1.UUID, "shipped"); code != http.StatusConflict || ec != "TRANSFER_INVALID_TRANSITION" {
		t.Fatalf("ship before approval = %d %s", code, ec)
	}
	if code, ec := it.transferMove(bTok, r1.UUID, "approved"); code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, ec)
	}
	if h := history(u1); h != base {
		t.Fatalf("u1 after approval = %s", h)
	}
	if code, ec := it.transferMove(aTok, r1.UUID, "shipped"); code != http.StatusOK {
		t.Fatalf("ship = %d %s", code, ec)
	}
	if st := state(u1); st.Status != "in_transit" || st.HolderOrgID != dealerB.ID {
		t.Fatalf("u1 shipped = %+v", st)
	}
	// A second ship is a no-op.
	if code, ec := it.transferMove(aTok, r1.UUID, "shipped"); code != http.StatusOK {
		t.Fatalf("second ship = %d %s", code, ec)
	}
	if code, _ := it.transferMove(aTok, r1.UUID, "received"); code != http.StatusForbidden {
		t.Fatalf("sender receive = %d", code)
	}
	if code, ec := it.transferMove(bTok, r1.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive = %d %s", code, ec)
	}
	if st := state(u1); st.Status != "available" || st.HolderOrgID != dealerB.ID || st.OwnerType != "organization" || st.OwnerID != dealerB.ID {
		t.Fatalf("u1 received = %+v", st)
	}
	if h := history(u1); h != base+",transfer_out,transfer_in" {
		t.Fatalf("u1 history = %s", h)
	}
	var keyed int
	if err := it.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM stock_movements m JOIN stock_transfer_request_items i ON i.id = m.reference_id
		WHERE m.unit_id = $1 AND m.reference_type = 'transfer_item'
		  AND m.idempotency_key IN ('transfer:transfer_item:' || i.id || ':transfer_out:' || $2,
		                            'transfer:transfer_item:' || i.id || ':transfer_in:' || $2)`, u1.ID, u1.Barcode).Scan(&keyed); err != nil {
		t.Fatal(err)
	}
	if keyed != 2 {
		t.Fatalf("keyed transfer movements = %d", keyed)
	}
	if v, _ := it.transferCall("GET", "/v1/stock-transfers/"+r1.UUID, bTok, nil, http.StatusOK); v.Status != "received" ||
		len(v.Items) != 1 || !v.Items[0].Shipped || !v.Items[0].Received || len(v.AvailableTransitions) != 0 {
		t.Fatalf("received view = %+v", v)
	}
	var events int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name LIKE 'transfers.%' AND payload->'data'->>'transfer_uuid' = $1`,
		r1.UUID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 4 {
		t.Fatalf("transfer events = %d", events)
	}

	// 2. Rejected: no stock movement at all.
	r2, _ := create(aTok, dealerB, u2.Barcode, http.StatusCreated)
	if code, ec := it.transferMove(bTok, r2.UUID, "rejected"); code != http.StatusOK {
		t.Fatalf("reject = %d %s", code, ec)
	}
	if h := history(u2); h != base {
		t.Fatalf("u2 after reject = %s", h)
	}
	var moved int
	if err := it.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM stock_movements m JOIN stock_transfer_request_items i ON i.id = m.reference_id
		JOIN stock_transfer_requests r ON r.id = i.request_id
		WHERE m.reference_type = 'transfer_item' AND r.uuid = $1`, r2.UUID).Scan(&moved); err != nil {
		t.Fatal(err)
	}
	if moved != 0 {
		t.Fatalf("rejected request movements = %d", moved)
	}
	if code, ec := it.transferMove(aTok, r2.UUID, "shipped"); code != http.StatusConflict || ec != "TRANSFER_INVALID_TRANSITION" {
		t.Fatalf("ship rejected = %d %s", code, ec)
	}
	if st := state(u2); st.Status != "available" || st.HolderOrgID != dealerA.ID {
		t.Fatalf("u2 after reject = %+v", st)
	}

	// 3. Approved by the common parent, shipped, cancelled: back at A.
	r3, _ := create(aTok, dealerB, u2.Barcode, http.StatusCreated)
	if v, _ := it.transferCall("GET", "/v1/stock-transfers/"+r3.UUID, dTok, nil, http.StatusOK); v.Role != "parent" ||
		strings.Join(v.AvailableTransitions, ",") != "approved,rejected,cancelled" {
		t.Fatalf("parent view = %+v", v)
	}
	for _, step := range []struct{ tok, status string }{{dTok, "approved"}, {aTok, "shipped"}, {aTok, "cancelled"}} {
		if code, ec := it.transferMove(step.tok, r3.UUID, step.status); code != http.StatusOK {
			t.Fatalf("r3 %s = %d %s", step.status, code, ec)
		}
	}
	if st := state(u2); st.Status != "available" || st.HolderOrgID != dealerA.ID || st.OwnerID != dealerA.ID {
		t.Fatalf("u2 after cancel = %+v", st)
	}
	if h := history(u2); h != base+",transfer_out,transfer_cancel_restore" {
		t.Fatalf("u2 history = %s", h)
	}
}
