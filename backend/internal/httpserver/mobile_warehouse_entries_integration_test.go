package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-769 acceptance on the mobile API: center warehouse staff run a stock
// entry (create, lines, line delete, place, confirm, cancel, list, detail)
// and a bin -> bin move through /v1/mobile/warehouse/*, with the same use
// cases as the panel; a distributor user who is a member of one warehouse
// gets WAREHOUSE_NOT_MEMBER on writes elsewhere, the member default fills
// a missing warehouse_uuid, and removing the membership frees the user.
func TestIntegrationMobileWarehouseEntriesMoves(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t769-dist", "distributor", center)

	whUser, whPw := it.user("t769-center-wh")
	it.member(center, whUser, "staff", rbac.RoleCenterWarehouse)
	owner, pw := it.user("t769-dist-owner")
	it.member(dist, owner, "owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE user_id = ANY($1)", []int64{owner.ID, whUser.ID})
	})
	cPanel := it.loginOrg(whUser, whPw, center)
	dPanel := it.loginOrg(owner, pw, dist)

	sfx := it.suffix[len(it.suffix)-8:]
	type tree struct{ w, room, a, b whItem }
	newTree := func(tok, code string) tree {
		t.Helper()
		var tr tree
		it.whDo(tok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": code + sfx, "name": "t769 " + code}, http.StatusCreated, &tr.w)
		it.whDo(tok, "POST", "/v1/warehouse/warehouses/"+tr.w.UUID+"/rooms", map[string]any{"code": "R1"}, http.StatusCreated, &tr.room)
		it.whDo(tok, "POST", "/v1/warehouse/locations", map[string]any{"room_uuid": tr.room.UUID, "type": "aisle", "code": "A"}, http.StatusCreated, &tr.a)
		it.whDo(tok, "POST", "/v1/warehouse/locations", map[string]any{"room_uuid": tr.room.UUID, "type": "aisle", "code": "B"}, http.StatusCreated, &tr.b)
		return tr
	}
	c1 := newTree(cPanel, "EA")
	p := it.product(center, "T769")

	type entryView struct {
		UUID      string `json:"uuid"`
		Status    string `json:"status"`
		LineCount int64  `json:"line_count"`
		Warehouse *struct {
			UUID string `json:"uuid"`
		} `json:"warehouse"`
		Lines []struct {
			UUID    string `json:"uuid"`
			Barcode string `json:"barcode"`
		} `json:"lines"`
	}
	mobile := func(tok string) func(method, path string, body any, want int, out any) envelope {
		return func(method, path string, body any, want int, out any) envelope {
			t.Helper()
			code, env, _ := it.doMobile(method, path, tok, "1", body)
			if code != want {
				t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
			}
			if out != nil {
				if err := json.Unmarshal(env.Data, out); err != nil {
					t.Fatalf("%s %s: decode: %v", method, path, err)
				}
			}
			return env
		}
	}

	// 1. Center staff on mobile: a generate_new entry end to end.
	ct := it.mobileLogin(whUser.Email.String, whPw, center.Slug, "t769-c")
	cm := mobile(ct.AccessToken)
	const base = "/v1/mobile/warehouse/stock-entries"
	var entry entryView
	cm("POST", base, map[string]any{"warehouse_uuid": c1.w.UUID, "mode": "generate_new"}, http.StatusCreated, &entry)
	if entry.Status != "draft" || entry.Warehouse == nil || entry.Warehouse.UUID != c1.w.UUID {
		t.Fatalf("mobile entry = %+v", entry)
	}
	cm("POST", base+"/"+entry.UUID+"/lines", map[string]any{"product_uuid": p.Uuid.String(), "quantity": 3, "prefix": "M" + sfx[len(sfx)-6:]}, http.StatusOK, &entry)
	if entry.LineCount != 3 || len(entry.Lines) != 3 {
		t.Fatalf("mobile entry lines = %+v", entry)
	}
	cm("DELETE", base+"/"+entry.UUID+"/lines/"+entry.Lines[2].UUID, nil, http.StatusOK, &entry)
	if entry.LineCount != 2 {
		t.Fatalf("after line delete = %d", entry.LineCount)
	}
	cm("POST", base+"/"+entry.UUID+"/place", map[string]any{"location_code": "OFW:LOC:" + c1.a.FullCode}, http.StatusOK, &entry)
	cm("POST", base+"/"+entry.UUID+"/confirm", nil, http.StatusOK, &entry)
	if entry.Status != "confirmed" {
		t.Fatalf("confirmed = %+v", entry)
	}
	cm("GET", base+"/"+entry.UUID, nil, http.StatusOK, &entry)
	var page struct {
		Items []entryView `json:"items"`
		Total int64       `json:"total"`
	}
	cm("GET", base+"?warehouse_uuid="+c1.w.UUID, nil, http.StatusOK, &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].UUID != entry.UUID {
		t.Fatalf("mobile entry list = %+v", page)
	}
	var draft entryView
	cm("POST", base, map[string]any{"warehouse_uuid": c1.w.UUID, "mode": "with_existing"}, http.StatusCreated, &draft)
	cm("POST", base+"/"+draft.UUID+"/cancel", nil, http.StatusOK, &draft)
	if draft.Status != "cancelled" {
		t.Fatalf("cancelled = %+v", draft)
	}

	// 2. Bin -> bin move of a confirmed unit inside the warehouse.
	var moved struct {
		Location struct {
			UUID string `json:"uuid"`
		} `json:"location"`
		Units []struct {
			Barcode string `json:"barcode"`
		} `json:"units"`
	}
	cm("POST", "/v1/mobile/warehouse/moves", map[string]any{"barcodes": []string{entry.Lines[0].Barcode}, "location_uuid": c1.b.UUID}, http.StatusOK, &moved)
	if moved.Location.UUID != c1.b.UUID || len(moved.Units) != 1 || moved.Units[0].Barcode != entry.Lines[0].Barcode {
		t.Fatalf("mobile move = %+v", moved)
	}

	// Membership administration needs members.write.
	if code, env := it.do("POST", "/v1/warehouse/warehouses/"+c1.w.UUID+"/members", hostOlex, cPanel,
		map[string]any{"user_uuid": whUser.Uuid.String()}); code != http.StatusForbidden {
		t.Fatalf("center warehouse staff adds a member = %d %s", code, errCode(env))
	}

	// 3. Distributor: the owner (members.write) limits itself to d1.
	d1 := newTree(dPanel, "DA")
	d2 := newTree(dPanel, "DB")
	dt := it.mobileLogin(owner.Email.String, pw, dist.Slug, "t769-d")
	dm := mobile(dt.AccessToken)
	type mine struct {
		Restricted bool    `json:"restricted"`
		Default    *string `json:"default_warehouse_uuid"`
		Items      []struct {
			UUID   string `json:"uuid"`
			Member bool   `json:"member"`
		} `json:"items"`
	}
	var my mine
	dm("GET", "/v1/mobile/warehouse/my-warehouses", nil, http.StatusOK, &my)
	if my.Restricted || my.Default != nil {
		t.Fatalf("unrestricted mine = %+v", my)
	}
	it.whDo(dPanel, "POST", "/v1/warehouse/warehouses/"+d1.w.UUID+"/members",
		map[string]any{"user_uuid": owner.Uuid.String(), "is_default": true}, http.StatusOK, nil)
	var members struct {
		Items []struct {
			IsDefault bool `json:"is_default"`
			User      struct {
				UUID string `json:"uuid"`
			} `json:"user"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	it.whDo(dPanel, "GET", "/v1/warehouse/warehouses/"+d1.w.UUID+"/members?sort=-created_at", nil, http.StatusOK, &members)
	if members.Total != 1 || members.Items[0].User.UUID != owner.Uuid.String() || !members.Items[0].IsDefault {
		t.Fatalf("members = %+v", members)
	}
	dm("GET", "/v1/mobile/warehouse/my-warehouses", nil, http.StatusOK, &my)
	if !my.Restricted || my.Default == nil || *my.Default != d1.w.UUID {
		t.Fatalf("restricted mine = %+v", my)
	}
	var def entryView
	dm("POST", base, map[string]any{"mode": "with_existing"}, http.StatusCreated, &def)
	if def.Warehouse == nil || def.Warehouse.UUID != d1.w.UUID {
		t.Fatalf("default warehouse entry = %+v", def)
	}
	notMember := func(method, path string, body any) {
		t.Helper()
		env := dm(method, path, body, http.StatusForbidden, nil)
		if errCode(env) != "WAREHOUSE_NOT_MEMBER" {
			t.Fatalf("%s %s = %s, want WAREHOUSE_NOT_MEMBER", method, path, errCode(env))
		}
	}
	notMember("POST", base, map[string]any{"warehouse_uuid": d2.w.UUID, "mode": "with_existing"})
	notMember("POST", "/v1/mobile/warehouse/transfers", map[string]any{"from_warehouse_uuid": d2.w.UUID, "to_warehouse_uuid": d1.w.UUID})
	notMember("POST", "/v1/mobile/warehouse/moves", map[string]any{"barcodes": []string{"T769-NONE"}, "location_uuid": d2.a.UUID})
	notMember("POST", "/v1/mobile/warehouse/stock-counts", map[string]any{"warehouse_uuid": d2.w.UUID, "method": "location_first", "visibility": "blind"})
	// The panel follows the same rule.
	if code, env := it.do("POST", "/v1/warehouse/stock-entries", hostOlex, dPanel,
		map[string]any{"warehouse_uuid": d2.w.UUID, "mode": "with_existing"}); code != http.StatusForbidden || errCode(env) != "WAREHOUSE_NOT_MEMBER" {
		t.Fatalf("panel entry in d2 = %d %s", code, errCode(env))
	}
	// Reads stay organization-wide.
	dm("GET", "/v1/mobile/warehouse/warehouses", nil, http.StatusOK, nil)
	dm("GET", base+"?warehouse_uuid="+d2.w.UUID, nil, http.StatusOK, nil)
	// The member warehouse is writable.
	dm("POST", "/v1/mobile/warehouse/transfers", map[string]any{"from_warehouse_uuid": d1.w.UUID, "to_warehouse_uuid": d2.w.UUID}, http.StatusCreated, nil)
	// The default only moves to a member warehouse.
	notMember("PUT", "/v1/mobile/warehouse/my-warehouses/default", map[string]any{"warehouse_uuid": d2.w.UUID})

	// 4. Removing the membership lifts the restriction.
	it.whDo(dPanel, "DELETE", "/v1/warehouse/warehouses/"+d1.w.UUID+"/members/"+owner.Uuid.String(), nil, http.StatusNoContent, nil)
	if code, env := it.do("DELETE", "/v1/warehouse/warehouses/"+d1.w.UUID+"/members/"+owner.Uuid.String(), hostOlex, dPanel, nil); code != http.StatusNotFound || errCode(env) != "WAREHOUSE_MEMBER_NOT_FOUND" {
		t.Fatalf("remove twice = %d %s", code, errCode(env))
	}
	dm("POST", base, map[string]any{"warehouse_uuid": d2.w.UUID, "mode": "with_existing"}, http.StatusCreated, nil)
	// Without a default the warehouse_uuid is required again.
	if env := dm("POST", base, map[string]any{"mode": "with_existing"}, http.StatusBadRequest, nil); errCode(env) != "VALIDATION_ERROR" {
		t.Fatal("entry without warehouse and default accepted")
	}
}
