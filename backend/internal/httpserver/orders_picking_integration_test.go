package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgtype"
)

type suggestionsResp struct {
	Missing     string `json:"missing"`
	Covered     string `json:"covered"`
	Complete    bool   `json:"complete"`
	Suggestions []struct {
		UnitUUID     string  `json:"unit_uuid"`
		Barcode      string  `json:"barcode"`
		Quantity     *int64  `json:"quantity"`
		Meters       *string `json:"meters"`
		Located      bool    `json:"located"`
		LocationCode *string `json:"location_code"`
	} `json:"suggestions"`
}

type shippingOrderResp struct {
	Status         string `json:"status"`
	DeliveryMethod *struct {
		Code     string `json:"code"`
		IsSystem bool   `json:"is_system"`
	} `json:"delivery_method"`
	TrackingNo          *string `json:"tracking_no"`
	TrackingURL         *string `json:"tracking_url"`
	HasShippingDocument bool    `json:"has_shipping_document"`
	HasReceiptDocument  bool    `json:"has_receipt_document"`
	Splits              []struct {
		NewBarcode string `json:"new_barcode"`
	} `json:"splits"`
}

type deliveryMethodResp struct {
	UUID     string `json:"uuid"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	IsSystem bool   `json:"is_system"`
	Active   bool   `json:"active"`
}

func pickData[T any](t *testing.T, env envelope) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("payload: %v %s", err, string(env.Data))
	}
	return out
}

func (it *itest) suggest(tok, orderUUID, itemUUID string, unlocated bool) suggestionsResp {
	it.t.Helper()
	path := "/v1/orders/" + orderUUID + "/items/" + itemUUID + "/suggestions"
	if unlocated {
		path += "?include_unlocated=true"
	}
	code, env := it.do("GET", path, hostOlex, tok, nil)
	if code != http.StatusOK {
		it.t.Fatalf("suggest = %d %s", code, errCode(env))
	}
	return pickData[suggestionsResp](it.t, env)
}

func (it *itest) apply(tok, orderUUID, itemUUID string, s suggestionsResp) shippingOrderResp {
	it.t.Helper()
	units := make([]map[string]any, 0, len(s.Suggestions))
	for _, x := range s.Suggestions {
		u := map[string]any{"unit_uuid": x.UnitUUID}
		if x.Quantity != nil {
			u["quantity"] = *x.Quantity
		}
		if x.Meters != nil {
			u["meters"] = json.Number(*x.Meters)
		}
		units = append(units, u)
	}
	code, env := it.do("POST", "/v1/orders/"+orderUUID+"/items/"+itemUUID+"/suggestions/apply", hostOlex, tok,
		map[string]any{"units": units})
	if code != http.StatusOK {
		it.t.Fatalf("apply = %d %s %s", code, errCode(env), string(env.Data))
	}
	return pickData[shippingOrderResp](it.t, env)
}

func barcodes(s suggestionsResp) []string {
	out := make([]string, 0, len(s.Suggestions))
	for _, x := range s.Suggestions {
		out = append(out, x.Barcode)
	}
	return out
}

// TEC-768 acceptance: barcode suggestions put placed units first and the
// oldest entry first (FIFO); units without a location only join with
// include_unlocated; a roll line takes the first roll covering the meters
// (else whole rolls, the last one cut); fixed barcodes offer the located
// quantity. Applying assigns through the scan rules. The pick locations and
// the pick list follow the location codes. The system delivery method is
// read-only; shipping stores the method and the tracking number, renders
// the shipping note PDF and the buyer attaches a delivery receipt.
func TestIntegrationOrdersPickingAndShipping(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t,
		func(c *config.Config) { c.Gotenberg.URL = gotb.URL },
		func(d *Deps) { d.Storage = store })
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
	dist := it.org("t768-dist", "distributor", center)
	dist2 := it.org("t768-dist2", "distributor", center)
	staff, spw := it.user("t768-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t768-dist-owner")
	it.member(dist, distOwner, "owner")
	dist2Owner, d2pw := it.user("t768-dist2-owner")
	it.member(dist2, dist2Owner, "owner")

	piece := it.product(center, "T768P")
	roll := it.product(center, "T768R")
	fixed := it.product(center, "T768F")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, fixed.ID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []db.Product{piece, roll, fixed} {
		it.setListPrice(p, cur, "10")
	}

	chain := it.stockChain()
	locA := chain.location(center, "T768A")
	locB := chain.location(center, "T768B")
	unlocated := ledger.Owner{Type: ledger.OwnerOrganization, ID: center.ID, OrgID: center.ID}
	entered := func(u db.Unit, day int) {
		t.Helper()
		if _, err := it.pool.Exec(ctx, `UPDATE units SET created_at = $2 WHERE id = $1`,
			u.ID, time.Date(2020, 1, day, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	// Pieces: u2 is the oldest but has no location.
	var pu [4]db.Unit
	for i, at := range []ledger.Owner{locB, locA, unlocated, locA} {
		pu[i] = chain.unit(center, piece, 7680+i)
		chain.post(ledger.TypeEntry, pu[i], chain.nextRef(), at)
	}
	entered(pu[0], 3)
	entered(pu[1], 2)
	entered(pu[2], 1)
	entered(pu[3], 4)
	// Rolls: 5 m (oldest), 20 m, 30 m.
	var ru [3]db.Unit
	for i, m := range []string{"5", "20", "30"} {
		ru[i] = chain.rollUnit(center, roll, 7690+i, m)
		chain.post(ledger.TypeEntry, ru[i], chain.nextRef(), []ledger.Owner{locA, locA, locB}[i])
		entered(ru[i], i+1)
	}
	// Fixed: 4 at a location, 6 without one.
	fu := chain.fixedUnit(center, fixed, 768, locA, 4)
	chain.postFixed(ledger.TypeEntry, fu, chain.nextRef(), nil, &unlocated, 6)

	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	dist2Tok := it.loginOrg(dist2Owner, d2pw, dist2)

	a := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 4},
		map[string]any{"product_uuid": roll.Uuid.String(), "meters": "12"},
		map[string]any{"product_uuid": fixed.Uuid.String(), "quantity": 5},
	})
	aPiece, aRoll, aFixed := a.Items[0].UUID, a.Items[1].UUID, a.Items[2].UUID

	// 1. Pieces: placed first, FIFO; the unlocated unit only on request.
	s := it.suggest(staffTok, a.UUID, aPiece, false)
	if got := barcodes(s); len(got) != 3 || got[0] != pu[1].Barcode || got[1] != pu[0].Barcode || got[2] != pu[3].Barcode ||
		s.Complete || s.Missing != "4" || s.Covered != "3" {
		t.Fatalf("piece suggestions = %v %+v", got, s)
	}
	if s.Suggestions[0].LocationCode == nil || !s.Suggestions[0].Located {
		t.Fatalf("placed suggestion without location: %+v", s.Suggestions[0])
	}
	sAll := it.suggest(staffTok, a.UUID, aPiece, true)
	if got := barcodes(sAll); len(got) != 4 || got[3] != pu[2].Barcode || !sAll.Complete || sAll.Suggestions[3].Located {
		t.Fatalf("piece suggestions with unlocated = %v %+v", got, sAll)
	}
	// The buyer gets no suggestions.
	if code, _ := it.do("GET", "/v1/orders/"+a.UUID+"/items/"+aPiece+"/suggestions", hostOlex, distTok, nil); code != http.StatusForbidden {
		t.Fatalf("buyer suggest = %d", code)
	}

	// 2. Rolls: the first roll covering 12 m (20 m), not the oldest 5 m.
	sr := it.suggest(staffTok, a.UUID, aRoll, false)
	if len(sr.Suggestions) != 1 || sr.Suggestions[0].Barcode != ru[1].Barcode || *sr.Suggestions[0].Meters != "12.00" || !sr.Complete {
		t.Fatalf("roll suggestion = %+v", sr)
	}
	// No roll covers 40 m: whole rolls in order, the last one cut.
	b := it.openPreparing(dist2Tok, staffTok, []any{map[string]any{"product_uuid": roll.Uuid.String(), "meters": "40"}})
	sb := it.suggest(staffTok, b.UUID, b.Items[0].UUID, false)
	if got := barcodes(sb); len(got) != 3 || got[0] != ru[0].Barcode || *sb.Suggestions[0].Meters != "5.00" ||
		*sb.Suggestions[1].Meters != "20.00" || *sb.Suggestions[2].Meters != "15.00" || !sb.Complete {
		t.Fatalf("roll greedy = %v %+v", got, sb)
	}

	// 3. Fixed: the located 4 only; 5 from the unlocated holding of 6 (one
	// assignment ships from one owner).
	sf := it.suggest(staffTok, a.UUID, aFixed, false)
	if len(sf.Suggestions) != 1 || *sf.Suggestions[0].Quantity != 4 || sf.Complete {
		t.Fatalf("fixed suggestion = %+v", sf)
	}
	sfAll := it.suggest(staffTok, a.UUID, aFixed, true)
	if len(sfAll.Suggestions) != 1 || *sfAll.Suggestions[0].Quantity != 5 || !sfAll.Complete {
		t.Fatalf("fixed suggestion with unlocated = %+v", sfAll)
	}

	// 4. Pick locations: by location code, the stock without a location last.
	code, env := it.do("GET", "/v1/orders/"+a.UUID+"/items/"+aPiece+"/pick-locations", hostOlex, staffTok, nil)
	if code != http.StatusOK {
		t.Fatalf("pick locations = %d %s", code, errCode(env))
	}
	locs := pickData[struct {
		Items []struct {
			LocationCode *string `json:"location_code"`
			Quantity     int64   `json:"quantity"`
		} `json:"items"`
	}](t, env)
	if len(locs.Items) != 3 || *locs.Items[0].LocationCode != "T768A-"+it.suffix || locs.Items[0].Quantity != 2 ||
		*locs.Items[1].LocationCode != "T768B-"+it.suffix || locs.Items[1].Quantity != 1 ||
		locs.Items[2].LocationCode != nil || locs.Items[2].Quantity != 1 {
		t.Fatalf("pick locations = %+v", locs.Items)
	}

	// 5. Apply: the suggestions become assignments (the roll is split).
	it.apply(staffTok, a.UUID, aPiece, sAll)
	applied := it.apply(staffTok, a.UUID, aRoll, sr)
	if len(applied.Splits) != 1 {
		t.Fatalf("roll split = %+v", applied.Splits)
	}
	it.apply(staffTok, a.UUID, aFixed, sfAll)
	// A reserved unit is refused on another order; nothing is assigned.
	code, env = it.do("POST", "/v1/orders/"+b.UUID+"/items/"+b.Items[0].UUID+"/suggestions/apply", hostOlex, staffTok,
		map[string]any{"units": []any{
			map[string]any{"unit_uuid": ru[0].Uuid.String(), "meters": json.Number("5")},
			map[string]any{"unit_uuid": applied.Splits[0].NewBarcode},
		}})
	if code != http.StatusBadRequest {
		t.Fatalf("apply with a bad unit = %d %s", code, errCode(env))
	}
	if v := it.orderCall("GET", "/v1/orders/"+b.UUID, staffTok, nil, http.StatusOK); v.Items[0].Assigned != "0.00" {
		t.Fatalf("partial apply kept = %s", v.Items[0].Assigned)
	}

	// 6. Pick list: assigned units in location order, no location last.
	code, env = it.do("GET", "/v1/orders/"+a.UUID+"/pick-list", hostOlex, staffTok, nil)
	if code != http.StatusOK {
		t.Fatalf("pick list = %d %s", code, errCode(env))
	}
	pl := pickData[struct {
		Rows []struct {
			Kind         string  `json:"kind"`
			LocationCode *string `json:"location_code"`
			Barcode      *string `json:"barcode"`
		} `json:"rows"`
	}](t, env)
	if len(pl.Rows) != 6 {
		t.Fatalf("pick list rows = %+v", pl.Rows)
	}
	for i, r := range pl.Rows {
		if r.Kind != "assigned" {
			t.Fatalf("row %d kind = %s", i, r.Kind)
		}
		if i > 0 && pl.Rows[i-1].LocationCode == nil && r.LocationCode != nil {
			t.Fatalf("row without location before a located row: %+v", pl.Rows)
		}
		if i > 0 && pl.Rows[i-1].LocationCode != nil && r.LocationCode != nil && *pl.Rows[i-1].LocationCode > *r.LocationCode {
			t.Fatalf("rows out of location order: %+v", pl.Rows)
		}
	}

	// 7. Delivery methods: the system method is listed and read-only.
	code, env = it.do("GET", "/v1/delivery-methods?is_system=true", hostOlex, staffTok, nil)
	if code != http.StatusOK {
		t.Fatalf("delivery methods = %d %s", code, errCode(env))
	}
	page := pickData[struct {
		Items []deliveryMethodResp `json:"items"`
		Total int                  `json:"total"`
	}](t, env)
	if page.Total != 1 || !page.Items[0].IsSystem || page.Items[0].Code != "warehouse_pickup" {
		t.Fatalf("system method = %+v", page)
	}
	system := page.Items[0]
	if code, env := it.do("PUT", "/v1/delivery-methods/"+system.UUID, hostOlex, staffTok,
		map[string]any{"code": "x", "name": "X"}); code != http.StatusConflict || errCode(env) != "DELIVERY_METHOD_SYSTEM" {
		t.Fatalf("edit system = %d %s", code, errCode(env))
	}
	if code, env := it.do("DELETE", "/v1/delivery-methods/"+system.UUID, hostOlex, staffTok, nil); code != http.StatusConflict || errCode(env) != "DELIVERY_METHOD_SYSTEM" {
		t.Fatalf("delete system = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/delivery-methods", hostOlex, staffTok, map[string]any{
		"code": "aras", "name": "Aras", "tracking_url_template": "not a url {tracking_no}",
	}); code != http.StatusBadRequest {
		t.Fatalf("bad template = %d %s", code, errCode(env))
	}
	arasCode := "aras_" + it.suffix[len(it.suffix)-10:]
	code, env = it.do("POST", "/v1/delivery-methods", hostOlex, staffTok, map[string]any{
		"code": arasCode, "name": "Aras Kargo", "tracking_url_template": "https://track.example/q?no={tracking_no}", "sort_order": 1,
	})
	if code != http.StatusCreated {
		t.Fatalf("create method = %d %s", code, errCode(env))
	}
	aras := pickData[deliveryMethodResp](t, env)
	if code, _ := it.do("POST", "/v1/delivery-methods", hostOlex, staffTok, map[string]any{"code": arasCode, "name": "Again"}); code != http.StatusBadRequest {
		t.Fatalf("duplicate code = %d", code)
	}
	if code, _ := it.do("POST", "/v1/delivery-methods", hostOlex, distTok, map[string]any{"code": "own", "name": "Own"}); code != http.StatusCreated {
		t.Fatalf("distributor method = %d", code)
	}

	// 8. Ship with the method and a tracking number: the shipping note is
	// rendered and stored.
	if code, ec := it.transition(staffTok, a.UUID, "ready"); code != http.StatusOK {
		t.Fatalf("ready = %d %s", code, ec)
	}
	// Another organization's method is refused.
	if code, env = it.do("GET", "/v1/delivery-methods?q=own", hostOlex, distTok, nil); code != http.StatusOK {
		t.Fatalf("distributor methods = %d", code)
	}
	own := pickData[struct {
		Items []deliveryMethodResp `json:"items"`
	}](t, env).Items[0]
	if code, env := it.do("POST", "/v1/orders/"+a.UUID+"/transitions", hostOlex, staffTok, map[string]any{
		"status": "shipped", "delivery_method_uuid": own.UUID,
	}); code != http.StatusBadRequest {
		t.Fatalf("foreign method = %d %s", code, errCode(env))
	}
	code, env = it.do("POST", "/v1/orders/"+a.UUID+"/transitions", hostOlex, staffTok, map[string]any{
		"status": "shipped", "delivery_method_uuid": aras.UUID, "tracking_no": "AR 123",
	})
	if code != http.StatusOK {
		t.Fatalf("ship = %d %s", code, errCode(env))
	}
	shipped := pickData[shippingOrderResp](t, env)
	if shipped.Status != "shipped" || shipped.DeliveryMethod == nil || shipped.DeliveryMethod.Code != arasCode ||
		shipped.TrackingNo == nil || *shipped.TrackingNo != "AR 123" || shipped.TrackingURL == nil ||
		*shipped.TrackingURL != "https://track.example/q?no=AR%20123" || !shipped.HasShippingDocument {
		t.Fatalf("shipped = %+v", shipped)
	}
	doc := gotb.LastHTML()
	for _, want := range []string{pu[1].Barcode, fu.Barcode, "AR 123", "Aras Kargo", dist.Name} {
		if !strings.Contains(doc, want) {
			t.Fatalf("shipping note misses %q", want)
		}
	}
	rec := it.raw("GET", "/v1/orders/"+a.UUID+"/documents/shipping", distTok, "", nil, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatalf("download = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	calls := gotb.Calls.Load()
	if code, env := it.do("POST", "/v1/orders/"+a.UUID+"/shipping-document", hostOlex, staffTok, nil); code != http.StatusOK || gotb.Calls.Load() != calls+1 {
		t.Fatalf("regenerate = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/orders/"+a.UUID+"/shipping-document", hostOlex, distTok, nil); code != http.StatusForbidden {
		t.Fatalf("buyer regenerate = %d", code)
	}
	// A used method cannot be deleted.
	if code, env := it.do("DELETE", "/v1/delivery-methods/"+aras.UUID, hostOlex, staffTok, nil); code != http.StatusConflict || errCode(env) != "DELIVERY_METHOD_IN_USE" {
		t.Fatalf("delete used method = %d %s", code, errCode(env))
	}

	// 9. Receipt: the buyer receives and attaches the delivery receipt.
	if code, ec := it.transition(distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive = %d %s", code, ec)
	}
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32))
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "receipt.png")
	_, _ = part.Write(png)
	_ = mw.Close()
	rec = it.raw("POST", "/v1/orders/"+a.UUID+"/receipt-document", distTok, mw.FormDataContentType(), buf.Bytes(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("receipt = %d %s", rec.Code, rec.Body.String())
	}
	if got := decodeEnv[shippingOrderResp](t, rec); !got.HasReceiptDocument {
		t.Fatalf("receipt flag = %+v", got)
	}
	rec = it.raw("GET", "/v1/orders/"+a.UUID+"/documents/receipt", staffTok, "", nil, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("receipt download = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	// The seller cannot attach the buyer's receipt.
	rec = it.raw("POST", "/v1/orders/"+a.UUID+"/receipt-document", staffTok, mw.FormDataContentType(), buf.Bytes(), nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("seller receipt = %d", rec.Code)
	}
}
