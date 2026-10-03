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
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type scanPathView struct {
	Level string `json:"level"`
	UUID  string `json:"uuid"`
	Code  string `json:"code"`
}

type scanLocationView struct {
	UUID     string         `json:"uuid"`
	FullCode string         `json:"full_code"`
	Path     []scanPathView `json:"path"`
}

type scanResultView struct {
	Type      string            `json:"type"`
	MatchedBy string            `json:"matched_by"`
	Code      string            `json:"code"`
	Location  *scanLocationView `json:"location"`
	Unit      *struct {
		UUID            string                 `json:"uuid"`
		Barcode         string                 `json:"barcode"`
		Status          string                 `json:"status"`
		RemainingMeters *string                `json:"remaining_meters"`
		Holder          *struct{ UUID string } `json:"holder"`
		Location        *scanLocationView      `json:"location"`
	} `json:"unit"`
	Product *struct {
		UUID string `json:"uuid"`
		SKU  string `json:"sku"`
	} `json:"product"`
}

// scan posts one code and returns the status, the error code and the result.
func (it *itest) scan(token, code string) (int, string, scanResultView) {
	it.t.Helper()
	status, env := it.do("POST", "/v1/warehouse/scan", hostOlex, token, map[string]any{"code": code})
	var out scanResultView
	errCode := ""
	if env.Error != nil {
		errCode = env.Error.Code
	}
	if status == http.StatusOK {
		if err := json.Unmarshal(env.Data, &out); err != nil {
			it.t.Fatalf("scan %q: decode: %v", code, err)
		}
	}
	return status, errCode, out
}

func (it *itest) mustScan(token, code, wantType, wantMatch string) scanResultView {
	it.t.Helper()
	status, errCode, res := it.scan(token, code)
	if status != http.StatusOK || res.Type != wantType || res.MatchedBy != wantMatch {
		it.t.Fatalf("scan %q = %d %s %+v, want %s/%s", code, status, errCode, res, wantType, wantMatch)
	}
	return res
}

func (it *itest) scanFails(token, code string, wantStatus int, wantCode string) {
	it.t.Helper()
	status, errCode, res := it.scan(token, code)
	if status != wantStatus || (wantCode != "" && errCode != wantCode) {
		it.t.Fatalf("scan %q = %d %s %+v, want %d %s", code, status, errCode, res, wantStatus, wantCode)
	}
}

// TEC-203 acceptance: every input type resolves to the right entity
// (location QR with its tree path, unit barcode, unit QR, roll split
// barcode, SKU, short code); an unknown code is 404 SCAN_NO_MATCH; a unit
// or location outside the active organization's reach never resolves,
// while the distributor still reads its dealer's unit (TEC-216 subtree).
func TestIntegrationWarehouseScan(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t203-dist", "distributor", center)
	dealer := it.org("t203-dealer", "dealer", dist)
	otherDist := it.org("t203-dist2", "distributor", center)

	whUser, whPw := it.user("t203-center-wh")
	it.member(center, whUser, "staff", rbac.RoleCenterWarehouse)
	distOwner, dPw := it.user("t203-dist-owner")
	it.member(dist, distOwner, "owner")
	otherOwner, oPw := it.user("t203-dist2-owner")
	it.member(otherDist, otherOwner, "owner")
	dealerOwner, rPw := it.user("t203-dealer-owner")
	it.member(dealer, dealerOwner, "owner")

	cTok := it.loginOrg(whUser, whPw, center)
	dTok := it.loginOrg(distOwner, dPw, dist)
	oTok := it.loginOrg(otherOwner, oPw, otherDist)
	rTok := it.loginOrg(dealerOwner, rPw, dealer)

	// Center tree: warehouse -> room -> aisle -> shelf -> bin.
	sfx := it.suffix[len(it.suffix)-9:]
	var w, room, aisle, shelf, bin whItem
	it.whDo(cTok, "POST", "/v1/warehouse/warehouses", map[string]any{"code": "S" + sfx, "name": "Scan"}, http.StatusCreated, &w)
	it.whDo(cTok, "POST", "/v1/warehouse/warehouses/"+w.UUID+"/rooms", map[string]any{"code": "R1"}, http.StatusCreated, &room)
	it.whDo(cTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "type": "aisle", "code": "A"}, http.StatusCreated, &aisle)
	it.whDo(cTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "parent_uuid": aisle.UUID, "type": "shelf", "code": "01"}, http.StatusCreated, &shelf)
	it.whDo(cTok, "POST", "/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "parent_uuid": shelf.UUID, "type": "bin", "code": "02"}, http.StatusCreated, &bin)
	var binID int64
	if err := it.pool.QueryRow(ctx, `SELECT id FROM warehouse_locations WHERE uuid = $1`, bin.UUID).Scan(&binID); err != nil {
		t.Fatalf("bin id: %v", err)
	}
	binOwner := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: binID, OrgID: center.ID}

	p := it.product(center, "T203")
	roll := it.product(center, "T203R")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}

	c := it.stockChain()
	newUnit := func(prod db.Product, barcode, meters string) db.Unit {
		t.Helper()
		var m pgtype.Numeric
		if meters != "" {
			if err := m.Scan(meters); err != nil {
				t.Fatal(err)
			}
		}
		u, err := it.q.CreateUnit(ctx, db.CreateUnitParams{
			OrganizationID: center.ID, BrandID: center.BrandID, ProductID: prod.ID, Barcode: barcode,
			UnitKind: ledger.KindSerial, Source: "import", Status: string(ledger.StatusPrinted),
			InitialMeters: m, RemainingMeters: m,
		})
		if err != nil {
			t.Fatalf("unit %s: %v", barcode, err)
		}
		return u
	}

	// u1: legacy/import barcode in the center bin.
	u1 := c.unit(center, p, 2031)
	c.post(ledger.TypeEntry, u1, c.nextRef(), binOwner)
	// Roll split barcode (TEC-184 format) with remaining meters.
	split := newUnit(roll, fmt.Sprintf("T203R-%s-S1", it.suffix), "12.50")
	c.post(ledger.TypeEntry, split, c.nextRef(), binOwner)
	// Generated barcode reachable by its short code.
	seq := 90_000_000 + time.Now().UnixNano()%9_000_000
	gen := newUnit(p, stockusecase.Barcode(stockusecase.DefaultPrefix("olex"), seq), "")
	c.post(ledger.TypeEntry, gen, c.nextRef(), binOwner)
	// u4: center -> distributor -> dealer.
	u4 := c.unit(center, p, 2034)
	c.post(ledger.TypeEntry, u4, c.nextRef(), c.location(center, "C203"))
	c.ship(u4, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, c.location(dist, "D203"))
	c.ship(u4, ledger.TypeOrderOut, ledger.TypeReceived, dealer, ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID})

	// 1. Location QR -> location with its tree path (case-insensitive).
	res := it.mustScan(cTok, "OFW:LOC:"+bin.FullCode, "location", "location_qr")
	if res.Location == nil || res.Location.UUID != bin.UUID || res.Code != bin.FullCode || len(res.Location.Path) != 5 {
		t.Fatalf("location result = %+v", res.Location)
	}
	levels := []string{"warehouse", "room", "aisle", "shelf", "bin"}
	for i, n := range res.Location.Path {
		if n.Level != levels[i] {
			t.Fatalf("path[%d] = %+v, want level %s", i, n, levels[i])
		}
	}
	if res.Location.Path[0].UUID != w.UUID || res.Location.Path[4].UUID != bin.UUID {
		t.Fatalf("path = %+v", res.Location.Path)
	}
	it.mustScan(cTok, "  ofw:loc:"+strings.ToLower(bin.FullCode)+" ", "location", "location_qr")
	it.scanFails(cTok, "OFW:LOC:NOPE-"+sfx, http.StatusNotFound, "SCAN_LOCATION_NOT_FOUND")
	// A bare full_code is off by default (scan.bare_location_code_enabled).
	it.scanFails(cTok, bin.FullCode, http.StatusNotFound, "SCAN_NO_MATCH")

	// 2. Unit barcode -> unit with product, status and location.
	res = it.mustScan(cTok, u1.Barcode, "unit", "barcode")
	cur, err := it.q.GetUnit(ctx, u1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Unit == nil || res.Unit.UUID != u1.Uuid.String() || res.Unit.Status != cur.Status ||
		res.Product == nil || res.Product.SKU != p.Sku ||
		res.Unit.Location == nil || res.Unit.Location.FullCode != bin.FullCode || len(res.Unit.Location.Path) != 5 ||
		res.Unit.Holder == nil || res.Unit.Holder.UUID != center.Uuid.String() {
		t.Fatalf("unit result = %+v", res)
	}
	it.mustScan(cTok, "OFW:UNIT:"+u1.Barcode, "unit", "unit_qr")

	// 3. Roll split barcode -> remaining meters.
	res = it.mustScan(cTok, split.Barcode, "unit", "barcode")
	if res.Unit == nil || res.Unit.RemainingMeters == nil || *res.Unit.RemainingMeters != "12.50" {
		t.Fatalf("split result = %+v", res.Unit)
	}

	// 4. SKU -> product.
	res = it.mustScan(cTok, p.Sku, "product", "sku")
	if res.Product == nil || res.Product.UUID != p.Uuid.String() || res.Unit != nil || res.Location != nil {
		t.Fatalf("product result = %+v", res)
	}

	// 5. Short code -> generated barcode.
	res = it.mustScan(cTok, fmt.Sprintf("%d", seq), "unit", "short_code")
	if res.Code != gen.Barcode || res.Unit == nil || res.Unit.UUID != gen.Uuid.String() {
		t.Fatalf("short code result = %+v", res)
	}

	// 6. Unknown and empty codes.
	it.scanFails(cTok, "ZZ-NOPE-"+it.suffix, http.StatusNotFound, "SCAN_NO_MATCH")
	it.scanFails(cTok, "   ", http.StatusBadRequest, "VALIDATION_ERROR")

	// 7. Org scope: the distributor neither reaches the center-held unit
	// nor the center's location, but reads its dealer's unit (subtree)
	// and the brand catalog.
	it.scanFails(dTok, u1.Barcode, http.StatusNotFound, "SCAN_NO_MATCH")
	it.scanFails(dTok, "OFW:LOC:"+bin.FullCode, http.StatusNotFound, "SCAN_LOCATION_NOT_FOUND")
	res = it.mustScan(dTok, u4.Barcode, "unit", "barcode")
	if res.Unit == nil || res.Unit.Holder == nil || res.Unit.Holder.UUID != dealer.Uuid.String() || res.Unit.Location != nil {
		t.Fatalf("dealer unit via distributor = %+v", res.Unit)
	}
	it.mustScan(dTok, p.Sku, "product", "sku")
	// A sibling distributor never reaches that unit; the dealer has no
	// warehouse module (K12).
	it.scanFails(oTok, u4.Barcode, http.StatusNotFound, "SCAN_NO_MATCH")
	it.scanFails(rTok, u4.Barcode, http.StatusForbidden, "")
}
