package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-235 acceptance: distributor staff on the mobile API scan a barcode,
// add a count scan and create a warehouse transfer in their own warehouse;
// the center's warehouse id answers 404/403 (K4, K20); a panel (web) token
// is 403 REALM_FORBIDDEN; a missing version header is 426.
func TestIntegrationMobileWarehouse(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t235-dist", "distributor", center)

	whUser, whPw := it.user("t235-center-wh")
	it.member(center, whUser, "staff", rbac.RoleCenterWarehouse)
	owner, pw := it.user("t235-dist-owner")
	it.member(dist, owner, "owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE user_id = $1", owner.ID)
	})

	cTok := it.loginOrg(whUser, whPw, center)
	dPanel := it.loginOrg(owner, pw, dist)

	// Trees: the distributor's two warehouses (room + aisle each) and one
	// center warehouse.
	sfx := it.suffix[len(it.suffix)-8:]
	tree := func(tok, code string) (whItem, whItem) {
		t.Helper()
		var w, room, aisle whItem
		it.whDo(tok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": code + sfx, "name": "t235 " + code}, http.StatusCreated, &w)
		it.whDo(tok, "POST", "/v1/warehouse/warehouses/"+w.UUID+"/rooms", map[string]any{"code": "R1"}, http.StatusCreated, &room)
		it.whDo(tok, "POST", "/v1/warehouse/locations",
			map[string]any{"room_uuid": room.UUID, "type": "aisle", "code": "A"}, http.StatusCreated, &aisle)
		return w, aisle
	}
	w1, a1 := tree(dPanel, "MA")
	w2, _ := tree(dPanel, "MB")
	cw, _ := tree(cTok, "MC")

	// A center unit shipped onto the distributor's aisle in w1.
	var a1ID int64
	if err := it.pool.QueryRow(ctx, `SELECT id FROM warehouse_locations WHERE uuid = $1`, a1.UUID).Scan(&a1ID); err != nil {
		t.Fatalf("aisle id: %v", err)
	}
	p := it.product(center, "T235")
	c := it.stockChain()
	u := c.unit(center, p, 2351)
	c.post(ledger.TypeEntry, u, c.nextRef(), c.location(center, "C235"))
	c.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dist,
		ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: a1ID, OrgID: dist.ID})

	tp := it.mobileLogin(owner.Email.String, pw, dist.Slug, "t235-dev")
	m := func(method, path string, body any, want int) envelope {
		t.Helper()
		code, env, _ := it.doMobile(method, path, tp.AccessToken, "1", body)
		if code != want {
			t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
		}
		return env
	}
	var ref struct {
		UUID string `json:"uuid"`
	}

	// Lookup: own warehouses only.
	var list whList
	if err := json.Unmarshal(m("GET", "/v1/mobile/warehouse/warehouses", nil, http.StatusOK).Data, &list); err != nil {
		t.Fatal(err)
	}
	if !list.has(w1.UUID) || !list.has(w2.UUID) || list.has(cw.UUID) {
		t.Fatalf("mobile warehouse list = %+v", list.Items)
	}

	// 1. Barcode scan.
	var res scanResultView
	if err := json.Unmarshal(m("POST", "/v1/mobile/warehouse/scan", map[string]any{"code": u.Barcode}, http.StatusOK).Data, &res); err != nil {
		t.Fatal(err)
	}
	if res.Type != "unit" || res.Unit == nil || res.Unit.UUID != u.Uuid.String() ||
		res.Unit.Location == nil || res.Unit.Location.UUID != a1.UUID {
		t.Fatalf("mobile scan = %+v", res)
	}

	// 2. Count: create, start, location then unit scan.
	_ = json.Unmarshal(m("POST", "/v1/mobile/warehouse/stock-counts", map[string]any{
		"warehouse_uuid": w1.UUID, "method": "location_first", "visibility": "blind",
	}, http.StatusCreated).Data, &ref)
	countUUID := ref.UUID
	m("POST", "/v1/mobile/warehouse/stock-counts/"+countUUID+"/start", nil, http.StatusOK)
	m("POST", "/v1/mobile/warehouse/stock-counts/"+countUUID+"/scans", map[string]any{"code": "OFW:LOC:" + a1.FullCode}, http.StatusCreated)
	m("POST", "/v1/mobile/warehouse/stock-counts/"+countUUID+"/scans", map[string]any{"code": u.Barcode}, http.StatusCreated)
	var scans struct {
		Items []json.RawMessage `json:"items"`
	}
	_ = json.Unmarshal(m("GET", "/v1/mobile/warehouse/stock-counts/"+countUUID+"/scans", nil, http.StatusOK).Data, &scans)
	if len(scans.Items) == 0 {
		t.Fatal("mobile count has no scans")
	}

	// 3. Transfer w1 -> w2 with the unit.
	_ = json.Unmarshal(m("POST", "/v1/mobile/warehouse/transfers", map[string]any{
		"from_warehouse_uuid": w1.UUID, "to_warehouse_uuid": w2.UUID,
	}, http.StatusCreated).Data, &ref)
	trUUID := ref.UUID
	m("POST", "/v1/mobile/warehouse/transfers/"+trUUID+"/lines", map[string]any{"barcodes": []string{u.Barcode}}, http.StatusOK)
	m("GET", "/v1/mobile/warehouse/transfers/"+trUUID, nil, http.StatusOK)

	// 4. The center warehouse id is never reachable (K4, K20).
	notReached := func(method, path string, body any) {
		t.Helper()
		code, env, _ := it.doMobile(method, path, tp.AccessToken, "1", body)
		if code != http.StatusNotFound && code != http.StatusForbidden {
			t.Fatalf("%s %s with center warehouse = %d %s, want 404/403", method, path, code, errCode(env))
		}
	}
	notReached("POST", "/v1/mobile/warehouse/stock-counts", map[string]any{
		"warehouse_uuid": cw.UUID, "method": "location_first", "visibility": "blind",
	})
	notReached("POST", "/v1/mobile/warehouse/transfers", map[string]any{
		"from_warehouse_uuid": cw.UUID, "to_warehouse_uuid": w2.UUID,
	})
	notReached("POST", "/v1/mobile/warehouse/transfers", map[string]any{
		"from_warehouse_uuid": w1.UUID, "to_warehouse_uuid": cw.UUID,
	})

	// 5. A panel (cookie/BFF) token is refused on the mobile routes.
	for _, path := range []string{"/v1/mobile/warehouse/warehouses", "/v1/mobile/warehouse/transfers"} {
		if code, env, _ := it.doMobile("GET", path, dPanel, "1", nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
			t.Fatalf("panel token on %s = %d %s", path, code, errCode(env))
		}
	}
	// ...and the mobile token is refused on the panel route.
	if code, env := it.do("POST", "/v1/warehouse/scan", hostOlex, tp.AccessToken, map[string]any{"code": u.Barcode}); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("mobile token on panel scan = %d %s", code, errCode(env))
	}

	// 6. Missing version header: 426 with the current version.
	code, env, hdr := it.doMobile("POST", "/v1/mobile/warehouse/scan", tp.AccessToken, "", map[string]any{"code": u.Barcode})
	if code != http.StatusUpgradeRequired || errCode(env) != "MOBILE_API_VERSION_UNSUPPORTED" || hdr.Get("X-Mobile-Api-Version") != "1" {
		t.Fatalf("no version header = %d %s (%q)", code, errCode(env), hdr.Get("X-Mobile-Api-Version"))
	}
}
