package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type whItem struct {
	UUID       string  `json:"uuid"`
	Code       string  `json:"code"`
	FullCode   string  `json:"full_code"`
	Type       string  `json:"type"`
	ParentUUID *string `json:"parent_uuid"`
	SortOrder  int32   `json:"sort_order"`
}

type whList struct {
	Items []whItem `json:"items"`
}

type whGenerated struct {
	Created  int `json:"created"`
	Existing int `json:"existing"`
}

// whDo calls the API and decodes data into out when the status matches.
func (it *itest) whDo(token, method, path string, body any, want int, out any) {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, token, body)
	if code != want {
		errCode := ""
		if env.Error != nil {
			errCode = env.Error.Code
		}
		it.t.Fatalf("%s %s: status %d (%s), want %d", method, path, code, errCode, want)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			it.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
}

func (l whList) has(uuid string) bool {
	for _, i := range l.Items {
		if i.UUID == uuid {
			return true
		}
	}
	return false
}

// TEC-201 acceptance: a distributor builds warehouse -> room -> aisle ->
// shelf -> bin by hand and in bulk; full_code is derived and follows
// renames; siblings reorder; the dealer gets 403 (K12) and another
// distributor never reaches the tree (404, absent from its list).
func TestIntegrationWarehouseTree(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t201-dist", "distributor", center)
	dealer := it.org("t201-dealer", "dealer", dist)
	otherDist := it.org("t201-dist2", "distributor", center)

	distOwner, dPw := it.user("t201-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerOwner, rPw := it.user("t201-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	otherOwner, oPw := it.user("t201-dist2-owner")
	it.member(otherDist, otherOwner, "owner")
	wh, whPw := it.user("t201-center-wh")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)

	dTok := it.loginOrg(distOwner, dPw, dist)
	rTok := it.loginOrg(dealerOwner, rPw, dealer)
	oTok := it.loginOrg(otherOwner, oPw, otherDist)
	cTok := it.loginOrg(wh, whPw, center)

	sfx := it.suffix[len(it.suffix)-9:]
	wCode := "W" + sfx

	// Warehouses: a distributor may run several (K4).
	var w1, w2 whItem
	it.whDo(dTok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": wCode, "name": "Main"}, http.StatusCreated, &w1)
	it.whDo(dTok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": "x" + sfx}, http.StatusCreated, &w2)
	if w1.Code != wCode || w2.Code != "X"+sfx {
		t.Fatalf("codes %q %q", w1.Code, w2.Code)
	}
	it.whDo(dTok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": wCode}, http.StatusConflict, nil)
	it.whDo(dTok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": "BAD-CODE"}, http.StatusBadRequest, nil)
	var list whList
	it.whDo(dTok, "GET", "/v1/warehouse/warehouses", nil, http.StatusOK, &list)
	if len(list.Items) != 2 || list.Items[0].UUID != w1.UUID {
		t.Fatalf("distributor warehouses = %+v", list.Items)
	}
	it.whDo(dTok, "POST", "/v1/warehouse/warehouses/reorder", map[string]any{"uuids": []string{w2.UUID, w1.UUID}}, http.StatusNoContent, nil)
	it.whDo(dTok, "GET", "/v1/warehouse/warehouses", nil, http.StatusOK, &list)
	if list.Items[0].UUID != w2.UUID || list.Items[1].SortOrder != 1 {
		t.Fatalf("reordered warehouses = %+v", list.Items)
	}

	// Room and a hand-built branch.
	var room whItem
	it.whDo(dTok, "POST", "/v1/warehouse/warehouses/"+w1.UUID+"/rooms", map[string]any{"code": "R1"}, http.StatusCreated, &room)
	var aisle, shelf, bin whItem
	it.whDo(dTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "type": "aisle", "code": "a"}, http.StatusCreated, &aisle)
	it.whDo(dTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "parent_uuid": aisle.UUID, "type": "shelf", "code": "01"}, http.StatusCreated, &shelf)
	it.whDo(dTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "parent_uuid": shelf.UUID, "type": "bin", "code": "01"}, http.StatusCreated, &bin)
	if want := wCode + "-R1-A-01-01"; bin.FullCode != want {
		t.Fatalf("bin full_code = %q, want %q", bin.FullCode, want)
	}
	if bin.ParentUUID == nil || *bin.ParentUUID != shelf.UUID {
		t.Fatalf("bin parent = %v", bin.ParentUUID)
	}
	// Tree shape and sibling uniqueness.
	it.whDo(dTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "type": "bin", "code": "09"}, http.StatusBadRequest, nil)
	it.whDo(dTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "parent_uuid": aisle.UUID, "type": "bin", "code": "09"}, http.StatusBadRequest, nil)
	it.whDo(dTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "type": "aisle", "code": "A"}, http.StatusConflict, nil)

	// Bulk: aisle B x shelf 01-10 x bin 01-05, then an overlapping run.
	var gen whGenerated
	it.whDo(dTok, "POST", "/v1/warehouse/locations/generate", map[string]any{
		"room_uuid": room.UUID,
		"levels": []map[string]any{
			{"type": "aisle", "codes": []string{"B"}},
			{"type": "shelf", "from": 1, "to": 10, "pad": 2},
			{"type": "bin", "from": 1, "to": 5, "pad": 2},
		},
	}, http.StatusCreated, &gen)
	if gen.Created != 61 || gen.Existing != 0 {
		t.Fatalf("generate = %+v, want 61 created", gen)
	}
	it.whDo(dTok, "POST", "/v1/warehouse/locations/generate", map[string]any{
		"room_uuid": room.UUID,
		"levels": []map[string]any{
			{"type": "aisle", "codes": []string{"A", "B"}},
			{"type": "shelf", "from": 1, "to": 2, "pad": 2},
			{"type": "bin", "codes": []string{"01"}},
		},
	}, http.StatusCreated, &gen)
	if gen.Created != 2 || gen.Existing != 8 {
		t.Fatalf("overlapping generate = %+v, want 2 created / 8 existing", gen)
	}
	it.whDo(dTok, "POST", "/v1/warehouse/locations/generate", map[string]any{
		"room_uuid": room.UUID,
		"levels":    []map[string]any{{"type": "bin", "codes": []string{"01"}}},
	}, http.StatusBadRequest, nil)

	var locs whList
	it.whDo(dTok, "GET", "/v1/warehouse/rooms/"+room.UUID+"/locations", nil, http.StatusOK, &locs)
	if len(locs.Items) != 66 {
		t.Fatalf("room locations = %d, want 66", len(locs.Items))
	}
	codes := map[string]bool{}
	var aisleB string
	for _, l := range locs.Items {
		if codes[l.FullCode] {
			t.Fatalf("duplicate full_code %q", l.FullCode)
		}
		codes[l.FullCode] = true
		if l.Type == "aisle" && l.Code == "B" {
			aisleB = l.UUID
		}
	}
	if !codes[wCode+"-R1-B-10-05"] || !codes[wCode+"-R1-A-02-01"] {
		t.Fatal("generated full_codes missing")
	}

	// Renames re-derive the subtree.
	it.whDo(dTok, "PATCH", "/v1/warehouse/rooms/"+room.UUID, map[string]any{"code": "R9"}, http.StatusOK, nil)
	it.whDo(dTok, "PATCH", "/v1/warehouse/locations/"+aisle.UUID, map[string]any{"code": "C"}, http.StatusOK, nil)
	it.whDo(dTok, "GET", "/v1/warehouse/locations/"+bin.UUID, nil, http.StatusOK, &bin)
	if want := wCode + "-R9-C-01-01"; bin.FullCode != want {
		t.Fatalf("renamed bin full_code = %q, want %q", bin.FullCode, want)
	}
	it.whDo(dTok, "PATCH", "/v1/warehouse/locations/"+aisleB, map[string]any{"code": "C"}, http.StatusConflict, nil)

	// Reorder siblings; mixed levels are refused.
	it.whDo(dTok, "POST", "/v1/warehouse/locations/reorder", map[string]any{"uuids": []string{aisleB, aisle.UUID}}, http.StatusNoContent, nil)
	var a whItem
	it.whDo(dTok, "GET", "/v1/warehouse/locations/"+aisle.UUID, nil, http.StatusOK, &a)
	var b whItem
	it.whDo(dTok, "GET", "/v1/warehouse/locations/"+aisleB, nil, http.StatusOK, &b)
	if b.SortOrder != 0 || a.SortOrder != 1 {
		t.Fatalf("sort orders B=%d C=%d", b.SortOrder, a.SortOrder)
	}
	it.whDo(dTok, "POST", "/v1/warehouse/locations/reorder", map[string]any{"uuids": []string{aisleB, bin.UUID}}, http.StatusBadRequest, nil)

	// Delete: a parent is in use, a leaf goes.
	it.whDo(dTok, "DELETE", "/v1/warehouse/locations/"+aisle.UUID, nil, http.StatusConflict, nil)
	it.whDo(dTok, "DELETE", "/v1/warehouse/rooms/"+room.UUID, nil, http.StatusConflict, nil)
	it.whDo(dTok, "DELETE", "/v1/warehouse/locations/"+bin.UUID, nil, http.StatusNoContent, nil)
	it.whDo(dTok, "GET", "/v1/warehouse/locations/"+bin.UUID, nil, http.StatusNotFound, nil)
	it.whDo(dTok, "DELETE", "/v1/warehouse/warehouses/"+w2.UUID, nil, http.StatusNoContent, nil)

	// K12: the dealer has no warehouse module.
	for _, c := range []struct{ method, path string }{
		{"GET", "/v1/warehouse/warehouses"},
		{"POST", "/v1/warehouse/warehouses"},
		{"GET", "/v1/warehouse/rooms/" + room.UUID + "/locations"},
	} {
		code, _ := it.do(c.method, c.path, hostOlex, rTok, map[string]any{"code": "D" + sfx})
		if code != http.StatusForbidden {
			t.Fatalf("dealer %s %s = %d, want 403", c.method, c.path, code)
		}
	}

	// Another distributor sees only its own warehouses.
	it.whDo(oTok, "GET", "/v1/warehouse/warehouses", nil, http.StatusOK, &list)
	if len(list.Items) != 0 {
		t.Fatalf("other distributor sees %d warehouses", len(list.Items))
	}
	it.whDo(oTok, "GET", "/v1/warehouse/warehouses/"+w1.UUID, nil, http.StatusNotFound, nil)
	it.whDo(oTok, "GET", "/v1/warehouse/rooms/"+room.UUID+"/locations", nil, http.StatusNotFound, nil)
	it.whDo(oTok, "GET", "/v1/warehouse/locations/"+aisle.UUID, nil, http.StatusNotFound, nil)
	it.whDo(oTok, "PATCH", "/v1/warehouse/locations/"+aisle.UUID, map[string]any{"name": "x"}, http.StatusNotFound, nil)
	it.whDo(oTok, "POST", "/v1/warehouse/locations/generate", map[string]any{
		"room_uuid": room.UUID, "levels": []map[string]any{{"type": "aisle", "codes": []string{"Z"}}},
	}, http.StatusNotFound, nil)
	it.whDo(oTok, "POST", "/v1/warehouse/locations/reorder", map[string]any{"uuids": []string{aisle.UUID}}, http.StatusBadRequest, nil)

	// The center warehouse role runs its own tree and does not list the
	// distributor's.
	var cw whItem
	it.whDo(cTok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": "C" + sfx}, http.StatusCreated, &cw)
	it.whDo(cTok, "GET", "/v1/warehouse/warehouses", nil, http.StatusOK, &list)
	if !list.has(cw.UUID) || list.has(w1.UUID) {
		t.Fatalf("center list: has own=%v has distributor=%v", list.has(cw.UUID), list.has(w1.UUID))
	}
}
