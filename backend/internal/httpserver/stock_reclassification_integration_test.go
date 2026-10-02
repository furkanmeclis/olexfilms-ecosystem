package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockrebuild "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type reclassView struct {
	UUID   string `json:"uuid"`
	Status string `json:"status"`
	Unit   struct {
		Barcode string `json:"barcode"`
	} `json:"unit"`
	FromProduct struct {
		UUID string `json:"uuid"`
	} `json:"from_product"`
	ToProduct struct {
		UUID string `json:"uuid"`
	} `json:"to_product"`
	RequestedBy *struct {
		UUID string `json:"uuid"`
	} `json:"requested_by"`
	DecidedBy *struct {
		UUID string `json:"uuid"`
	} `json:"decided_by"`
	MovementUUID *string `json:"movement_uuid"`
}

// reclass calls a reclassification endpoint and checks the status (and
// the error code when code is set).
func (it *itest) reclass(method, path, token string, body any, want int, code string) reclassView {
	it.t.Helper()
	got, env := it.do(method, path, hostOlex, token, body)
	if got != want || (code != "" && errCode(env) != code) {
		it.t.Fatalf("%s %s = %d %s, want %d %s", method, path, got, errCode(env), want, code)
	}
	var v reclassView
	if got < 300 {
		if err := json.Unmarshal(env.Data, &v); err != nil {
			it.t.Fatal(err)
		}
	}
	return v
}

func (it *itest) countRows(query string, args ...any) int {
	it.t.Helper()
	var n int
	if err := it.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		it.t.Fatalf("count: %v", err)
	}
	return n
}

// TEC-157 acceptance: a distributor requests a reclassification of a unit
// it holds; nothing moves until a center warehouse user approves (the
// requester, a user without stock.reclassify, a user without step-up and
// the requester itself cannot apply it); the approval keeps the barcode,
// moves the unit to the target product in the ledger and the projections
// and writes the audit rows; a second approval writes nothing; wrong
// routings answer their own codes; the rebuild check finds no drift.
func TestIntegrationStockReclassification(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	glorian := it.brandCenter("glorian")
	dist := it.org("t157-dist", "distributor", center)

	distOwner, dpw := it.user("t157-dist-owner")
	it.member(dist, distOwner, "owner")
	wh, wpw := it.user("t157-wh")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	wh2, w2pw := it.user("t157-wh2")
	it.member(center, wh2, "staff", rbac.RoleCenterWarehouse)
	distTok := it.loginOrg(distOwner, dpw, dist)
	whTok := it.loginOrg(wh, wpw, center)
	wh2Tok := it.loginOrg(wh2, w2pw, center)

	pieceA := it.product(center, "T157A")
	pieceB := it.product(center, "T157B")
	roll := it.product(center, "T157R")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	inactive := it.product(center, "T157X")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET active = FALSE WHERE id = $1`, inactive.ID); err != nil {
		t.Fatalf("inactive product: %v", err)
	}
	foreign := it.product(glorian, "T157G")

	c := it.stockChain()
	cLoc := c.location(center, "C157")
	dLoc := c.location(dist, "D157")
	toDist := func(u db.Unit) {
		c.post(ledger.TypeEntry, u, c.nextRef(), cLoc)
		c.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	}
	u1 := c.unit(center, pieceA, 1570)
	u2 := c.unit(center, pieceA, 1571)
	u3 := c.rollUnit(center, roll, 1572, "30.00")
	u4 := c.unit(center, pieceA, 1573)
	u5 := c.unit(center, pieceA, 1574)
	u6 := c.unit(center, pieceA, 1575)
	for _, u := range []db.Unit{u1, u2, u3, u5} {
		toDist(u)
	}
	c.post(ledger.TypeEntry, u4, c.nextRef(), cLoc)
	c.post(ledger.TypeEntry, u6, c.nextRef(), cLoc)
	c.post(ledger.TypeVoid, u5, c.nextRef(), ledger.Owner{Type: ledger.OwnerTrash, ID: dist.ID, OrgID: dist.ID})

	const base = "/v1/stock/reclassifications"
	body := func(u db.Unit, to db.Product) map[string]any {
		return map[string]any{"barcode": u.Barcode, "to_product_uuid": to.Uuid.String(), "reason": "wrong label"}
	}

	// 1. Request: pending, nothing moves yet.
	req := it.reclass("POST", base, distTok, body(u1, pieceB), http.StatusCreated, "")
	if req.Status != "pending" || req.Unit.Barcode != u1.Barcode || req.FromProduct.UUID != pieceA.Uuid.String() ||
		req.ToProduct.UUID != pieceB.Uuid.String() || req.RequestedBy == nil || req.RequestedBy.UUID != distOwner.Uuid.String() {
		t.Fatalf("request = %+v", req)
	}
	movesBefore := it.countRows(`SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1`, u1.ID)
	it.reclass("POST", base, distTok, body(u1, pieceB), http.StatusConflict, "STOCK_RECLASSIFICATION_PENDING_EXISTS")

	// 2. Unapproved application is refused: no stock.reclassify, no
	// step-up, or the requester itself.
	approve := base + "/" + req.UUID + "/approve"
	it.reclass("POST", approve, distTok, nil, http.StatusForbidden, "")
	it.reclass("POST", approve, whTok, nil, http.StatusForbidden, "STEP_UP_REQUIRED")
	if n := it.countRows(`SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1`, u1.ID); n != movesBefore {
		t.Fatalf("movements before approval = %d, want %d", n, movesBefore)
	}
	if got, err := it.q.GetUnit(ctx, u1.ID); err != nil || got.ProductID != pieceA.ID {
		t.Fatalf("unit before approval: %+v %v", got, err)
	}

	// 3. Approval applies it: same barcode, new product, one movement,
	// audit rows, stock moved between products.
	it.stepUp(wh.Uuid)
	it.stepUp(wh2.Uuid)
	done := it.reclass("POST", approve, whTok, map[string]any{"note": "checked"}, http.StatusOK, "")
	if done.Status != "approved" || done.MovementUUID == nil || done.DecidedBy == nil || done.DecidedBy.UUID != wh.Uuid.String() {
		t.Fatalf("approved = %+v", done)
	}
	unit, err := it.q.GetUnit(ctx, u1.ID)
	if err != nil || unit.ProductID != pieceB.ID || unit.Barcode != u1.Barcode {
		t.Fatalf("unit after approval: %+v %v", unit, err)
	}
	reqRow, err := it.q.GetStockReclassificationByUUID(ctx, uuid.MustParse(done.UUID))
	if err != nil || reqRow.Status != "approved" || !reqRow.DecidedByUserID.Valid || reqRow.DecidedByUserID.Int64 != wh.ID ||
		!reqRow.RequestedByUserID.Valid || reqRow.RequestedByUserID.Int64 != distOwner.ID || reqRow.Reason != "wrong label" {
		t.Fatalf("request row: %+v %v", reqRow, err)
	}
	key := fmt.Sprintf("reclassification:stock_reclassification:%d:reclassification:%s", reqRow.ID, u1.Barcode)
	mv, err := it.q.GetStockMovementByIdempotencyKey(ctx, key)
	if err != nil {
		t.Fatalf("reclassification movement: %v", err)
	}
	if mv.Type != "reclassification" || mv.ProductID != pieceB.ID || mv.Uuid.String() != *done.MovementUUID ||
		mv.FromOwnerID != mv.ToOwnerID || mv.ToOwnerID.Int64 != dLoc.ID || mv.OrganizationID != dist.ID {
		t.Fatalf("movement = %+v", mv)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1`, u1.ID); n != movesBefore+1 {
		t.Fatalf("movements after approval = %d, want %d", n, movesBefore+1)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM activity_events WHERE resource = 'stock.reclassification'
		AND resource_uuid = $1 AND action IN ('stock.reclassification.requested', 'stock.reclassification.approved')`, reqRow.Uuid); n != 2 {
		t.Fatalf("audit rows = %d, want requested + approved", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = 'stock.reclassification'
		AND payload::text LIKE '%' || $1 || '%'`, key); n != 1 {
		t.Fatalf("outbox rows = %d", n)
	}
	code, hist := it.unitHistory(distTok, u1.Barcode, "")
	if code != http.StatusOK || hist.Current == nil || hist.Current.Status != "available" {
		t.Fatalf("history = %d %+v", code, hist)
	}
	code, stock := it.productStock(distTok, "/v1/stock/organizations/"+dist.Uuid.String()+"/products")
	if code != http.StatusOK {
		t.Fatalf("dist stock = %d", code)
	}
	// u1 moved to B; u2 stays on A (u3 is a roll, u5 is void).
	if q, _ := stock.qty(pieceB.Uuid.String()); q != 1 {
		t.Fatalf("dist stock of B = %d, want 1", q)
	}
	if q, _ := stock.qty(pieceA.Uuid.String()); q != 1 {
		t.Fatalf("dist stock of A = %d, want 1", q)
	}

	// 4. A second approval is idempotent.
	again := it.reclass("POST", approve, wh2Tok, nil, http.StatusOK, "")
	if again.Status != "approved" || again.MovementUUID == nil || *again.MovementUUID != *done.MovementUUID {
		t.Fatalf("second approval = %+v", again)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1`, u1.ID); n != movesBefore+1 {
		t.Fatalf("movements after a second approval = %d", n)
	}

	// 5. Wrong routings answer their own codes.
	it.reclass("POST", base, distTok, body(u2, foreign), http.StatusUnprocessableEntity, "STOCK_RECLASSIFICATION_BRAND_MISMATCH")
	it.reclass("POST", base, distTok, body(u3, pieceB), http.StatusUnprocessableEntity, "STOCK_RECLASSIFICATION_UNIT_TYPE_MISMATCH")
	it.reclass("POST", base, distTok, body(u2, pieceA), http.StatusUnprocessableEntity, "STOCK_RECLASSIFICATION_SAME_PRODUCT")
	it.reclass("POST", base, distTok, body(u2, inactive), http.StatusUnprocessableEntity, "STOCK_RECLASSIFICATION_PRODUCT_INACTIVE")
	it.reclass("POST", base, distTok, body(u5, pieceB), http.StatusConflict, "STOCK_RECLASSIFICATION_UNIT_CONSUMED")
	it.reclass("POST", base, distTok, body(u4, pieceB), http.StatusConflict, "STOCK_RECLASSIFICATION_UNIT_ELSEWHERE")

	// 6. Reject: nothing moves; a rejected request cannot be applied.
	r2 := it.reclass("POST", base, distTok, body(u2, pieceB), http.StatusCreated, "")
	it.reclass("POST", base+"/"+r2.UUID+"/reject", whTok, map[string]any{"note": "label is right"}, http.StatusOK, "")
	it.reclass("POST", base+"/"+r2.UUID+"/approve", whTok, nil, http.StatusConflict, "STOCK_RECLASSIFICATION_NOT_PENDING")
	if got, err := it.q.GetUnit(ctx, u2.ID); err != nil || got.ProductID != pieceA.ID {
		t.Fatalf("rejected unit: %+v %v", got, err)
	}

	// 7. The requester cannot approve its own request; it can withdraw it.
	r6 := it.reclass("POST", base, whTok, body(u6, pieceB), http.StatusCreated, "")
	it.reclass("POST", base+"/"+r6.UUID+"/approve", whTok, nil, http.StatusForbidden, "STOCK_RECLASSIFICATION_SELF_APPROVAL")
	if v := it.reclass("POST", base+"/"+r6.UUID+"/cancel", whTok, nil, http.StatusOK, ""); v.Status != "cancelled" {
		t.Fatalf("cancel = %+v", v)
	}

	// 8. Reads: the distributor sees its requests.
	code, env := it.do("GET", base+"?status=approved", hostOlex, distTok, nil)
	var page struct {
		Items []reclassView `json:"items"`
		Total int64         `json:"total"`
	}
	if code != http.StatusOK || json.Unmarshal(env.Data, &page) != nil || page.Total != 1 || page.Items[0].UUID != done.UUID {
		t.Fatalf("list = %d %+v", code, page)
	}
	it.reclass("GET", base+"/"+done.UUID, distTok, nil, http.StatusOK, "")

	// 9. Rebuild/replay after the reclassification: no drift, no anomaly.
	for _, org := range []int64{dist.ID, center.ID} {
		rep, err := stockrebuild.New(it.pool, it.q).Check(ctx, org)
		if err != nil {
			t.Fatalf("rebuild check: %v", err)
		}
		for _, a := range rep.Anomalies {
			for _, u := range []db.Unit{u1, u2, u3, u4, u5, u6} {
				if strings.Contains(a, "("+u.Barcode+")") {
					t.Fatalf("anomaly on a test unit: %s", a)
				}
			}
		}
		for _, d := range rep.Diffs {
			for _, u := range []db.Unit{u1, u2, u3, u4, u5, u6} {
				if d.UnitID == u.ID {
					t.Fatalf("drift on a test unit: %+v", d)
				}
			}
			if d.Key == fmt.Sprintf("organization:%d/product:%d", dist.ID, pieceB.ID) ||
				d.Key == fmt.Sprintf("organization:%d/product:%d", dist.ID, pieceA.ID) ||
				d.Key == fmt.Sprintf("location:%d/product:%d", dLoc.ID, pieceB.ID) ||
				d.Key == fmt.Sprintf("location:%d/product:%d", dLoc.ID, pieceA.ID) {
				t.Fatalf("product stock drift: %+v", d)
			}
		}
	}
}
