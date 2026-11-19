package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type cutPreview struct {
	ClosedBox       bool    `json:"closed_box"`
	SourceMeters    string  `json:"source_meters"`
	CutMeters       string  `json:"cut_meters"`
	RemainingMeters string  `json:"remaining_meters"`
	Whole           bool    `json:"whole"`
	Remnant         bool    `json:"remnant"`
	MinCutMeters    *string `json:"min_cut_meters"`
	MissingMeters   string  `json:"missing_meters"`
	Partial         bool    `json:"partial"`
	Credits         int32   `json:"credits"`
	LineCredits     int32   `json:"line_credits"`
	UnitPrice       *string `json:"unit_price"`
}

type cutAssign struct {
	Split *struct {
		Meters                string `json:"meters"`
		SourceRemainingMeters string `json:"source_remaining_meters"`
		NewBarcode            string `json:"new_barcode"`
		BoxOpened             bool   `json:"box_opened"`
		SourceRemnant         bool   `json:"source_remnant"`
		Replayed              bool   `json:"replayed"`
	} `json:"split"`
	Items []struct {
		UUID         string  `json:"uuid"`
		Assigned     string  `json:"assigned"`
		MinCutMeters *string `json:"min_cut_meters"`
		UnitPrice    *string `json:"unit_price"`
		Units        []struct {
			Barcode   string  `json:"barcode"`
			Meters    *string `json:"meters"`
			Credits   *int32  `json:"credits"`
			PieceNote *string `json:"piece_note"`
		} `json:"units"`
	} `json:"items"`
	DetachedCutPieces []struct {
		Barcode   string `json:"barcode"`
		CutMeters string `json:"cut_meters"`
	} `json:"detached_cut_pieces"`
}

// TEC-1241 acceptance (kutu/metraj 4/7, K5/K7/K11/K12 of TEC-1236): a meter
// line of a box product is picked by cutting one piece — from the smallest
// open roll that covers it, or from a closed box opened on confirmation
// (open and cut in one transaction); a remainder shorter than the minimum
// cut is flagged as a remnant; a shorter piece needs the partial
// confirmation and a note and gets its share of the line credits; a box
// reserved by another order is refused before it is opened; the warehouse
// role sees no price; cut pieces left after a cancel are listed.
func TestIntegrationOrdersCutMeterLine(t *testing.T) {
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
	dist := it.org("t1241-dist", "distributor", center)
	staff, spw := it.user("t1241-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	wh, wpw := it.user("t1241-center-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t1241-dist-owner")
	it.member(dist, distOwner, "owner")
	staffTok := it.loginOrg(staff, spw, center)
	whTok := it.loginOrg(wh, wpw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)

	film := it.product(center, "T1241F")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET default_roll_meters = 15, min_cut_meters = 2,
		sellable_by_meter = true, usage_credits = 5 WHERE id = $1`, film.ID); err != nil {
		t.Fatal(err)
	}
	it.setListPrice(film, cur, "400")

	chain := it.stockChain()
	loc := chain.location(center, "T1241")
	boxA := chain.unit(center, film, 12411)
	chain.post(ledger.TypeEntry, boxA, chain.nextRef(), loc)
	boxB := chain.unit(center, film, 12412)
	chain.post(ledger.TypeEntry, boxB, chain.nextRef(), loc)

	preview := func(o, line, barcode, meters string) (int, string, cutPreview) {
		q := url.Values{"barcode": {barcode}}
		if meters != "" {
			q.Set("meters", meters)
		}
		code, env := it.do("GET", "/v1/orders/"+o+"/items/"+line+"/cut-preview?"+q.Encode(), hostOlex, whTok, nil)
		var p cutPreview
		if code == http.StatusOK {
			if err := json.Unmarshal(env.Data, &p); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(env.Data), "price") {
				t.Fatalf("preview carries a price: %s", env.Data)
			}
		}
		return code, errCode(env), p
	}
	assign := func(o, line string, body map[string]any) (int, string, cutAssign) {
		code, env := it.do("POST", "/v1/orders/"+o+"/items/"+line+"/units", hostOlex, whTok, body)
		var a cutAssign
		if code == http.StatusOK {
			if err := json.Unmarshal(env.Data, &a); err != nil {
				t.Fatal(err)
			}
		}
		return code, errCode(env), a
	}

	// 1. 3 m from a closed box: the preview warns, the assignment without
	// open_box is refused; confirmed, the box opens and 3 m are cut; 12 m
	// stay as an open roll.
	o1 := it.openPreparing(distTok, staffTok, []any{map[string]any{"product_uuid": film.Uuid.String(), "meters": "3"}})
	l1 := o1.Items[0].UUID
	code, env := it.do("GET", "/v1/orders/"+o1.UUID+"/items/"+l1+"/suggestions?include_unlocated=true", hostOlex, whTok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"open_box":true`) {
		t.Fatalf("suggestion before any open roll = %d %s", code, env.Data)
	}
	code, ec, p := preview(o1.UUID, l1, boxA.Barcode, "")
	if code != http.StatusOK || !p.ClosedBox || p.SourceMeters != "15.00" || p.CutMeters != "3.00" ||
		p.RemainingMeters != "12.00" || p.Whole || p.Remnant || p.Partial || p.Credits != 5 || p.LineCredits != 5 {
		t.Fatalf("box preview = %d %s %+v", code, ec, p)
	}
	if code, ec, _ := assign(o1.UUID, l1, map[string]any{"barcode": boxA.Barcode}); code != http.StatusConflict || ec != "BOX_OPEN_CONFIRM_REQUIRED" {
		t.Fatalf("box without confirmation = %d %s", code, ec)
	}
	key := "t1241-" + it.suffix
	code, ec, a := assign(o1.UUID, l1, map[string]any{"barcode": boxA.Barcode, "open_box": true, "idempotency_key": key})
	if code != http.StatusOK || a.Split == nil || !a.Split.BoxOpened || a.Split.Meters != "3.00" ||
		a.Split.SourceRemainingMeters != "12.00" || a.Split.SourceRemnant {
		t.Fatalf("open and cut = %d %s %+v", code, ec, a.Split)
	}
	if len(a.Items[0].Units) != 1 || a.Items[0].Units[0].Credits != nil || a.Items[0].MinCutMeters == nil ||
		a.Items[0].UnitPrice != nil {
		t.Fatalf("line after the cut = %+v", a.Items[0])
	}
	// A retry with the same key is a no-op.
	if code, ec, again := assign(o1.UUID, l1, map[string]any{"barcode": boxA.Barcode, "open_box": true, "idempotency_key": key}); code != http.StatusOK ||
		again.Split == nil || !again.Split.Replayed || len(again.Items[0].Units) != 1 {
		t.Fatalf("retried cut = %d %s %+v", code, ec, again.Split)
	}
	var reason string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(metadata->>'reason', '') FROM stock_movements
		WHERE unit_id = $1 AND type = 'measure_conversion'`, boxA.ID).Scan(&reason); err != nil || reason != "box_opened" {
		t.Fatalf("box opened movement = %q %v", reason, err)
	}
	cut := a.Split.NewBarcode
	var credits int
	if err := it.pool.QueryRow(ctx, `SELECT unit_credit_default(id) FROM units WHERE barcode = $1`, cut).Scan(&credits); err != nil || credits != 5 {
		t.Fatalf("cut piece credits = %d %v", credits, err)
	}
	// The cut piece's label carries meters and credits.
	if lbl, err := it.q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: center.BrandID, Barcode: cut}); err != nil || !lbl.RemainingMeters.Valid {
		t.Fatalf("cut piece = %+v %v", lbl, err)
	}

	// 2. 11 m from the 12 m open roll: smallest covering roll suggested;
	// the 1 m left is shorter than the 2 m minimum cut -> remnant.
	o2 := it.openPreparing(distTok, staffTok, []any{map[string]any{"product_uuid": film.Uuid.String(), "meters": "11"}})
	l2 := o2.Items[0].UUID
	code, env = it.do("GET", "/v1/orders/"+o2.UUID+"/items/"+l2+"/suggestions?include_unlocated=true", hostOlex, whTok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), boxA.Barcode) || strings.Contains(string(env.Data), `"open_box":true`) {
		t.Fatalf("roll suggestion = %d %s", code, env.Data)
	}
	code, ec, p = preview(o2.UUID, l2, boxA.Barcode, "")
	if code != http.StatusOK || p.ClosedBox || p.CutMeters != "11.00" || p.RemainingMeters != "1.00" || !p.Remnant {
		t.Fatalf("remnant preview = %d %s %+v", code, ec, p)
	}
	code, ec, a = assign(o2.UUID, l2, map[string]any{"barcode": boxA.Barcode})
	if code != http.StatusOK || a.Split == nil || a.Split.BoxOpened || !a.Split.SourceRemnant || a.Split.SourceRemainingMeters != "1.00" {
		t.Fatalf("cut leaving a remnant = %d %s %+v", code, ec, a.Split)
	}
	// The remnant is never suggested: a new 3 m line proposes the box.
	o3 := it.openPreparing(distTok, staffTok, []any{map[string]any{"product_uuid": film.Uuid.String(), "meters": "3"}})
	l3 := o3.Items[0].UUID
	code, env = it.do("GET", "/v1/orders/"+o3.UUID+"/items/"+l3+"/suggestions?include_unlocated=true", hostOlex, whTok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), boxB.Barcode) || strings.Contains(string(env.Data), boxA.Barcode) {
		t.Fatalf("suggestion skips the remnant = %d %s", code, env.Data)
	}

	// 3. A box reserved by another order is not opened.
	if code, ec, _ := assign(o3.UUID, l3, map[string]any{"barcode": boxB.Barcode, "open_box": true, "meters": "3"}); code != http.StatusOK {
		t.Fatalf("o3 box = %d %s", code, ec)
	}
	boxC := chain.unit(center, film, 12413)
	chain.post(ledger.TypeEntry, boxC, chain.nextRef(), loc)
	o4 := it.openPreparing(distTok, staffTok, []any{map[string]any{"product_uuid": film.Uuid.String(), "meters": "6"}})
	l4 := o4.Items[0].UUID
	// Reserve box C whole on a box line of another order first.
	o5 := it.openPreparing(distTok, staffTok, []any{map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1}})
	if code, ec, _ := it.assign(whTok, o5.UUID, o5.Items[0].UUID, map[string]any{"barcode": boxC.Barcode}); code != http.StatusOK {
		t.Fatalf("box line = %d %s", code, ec)
	}
	if code, ec, _ := preview(o4.UUID, l4, boxC.Barcode, ""); code != http.StatusConflict || ec != "ORDER_UNIT_RESERVED" {
		t.Fatalf("reserved box preview = %d %s", code, ec)
	}
	if code, ec, _ := assign(o4.UUID, l4, map[string]any{"barcode": boxC.Barcode, "open_box": true}); code != http.StatusConflict || ec != "ORDER_UNIT_RESERVED" {
		t.Fatalf("reserved box = %d %s", code, ec)
	}
	var boxCMeters pgtype.Numeric
	if err := it.pool.QueryRow(ctx, `SELECT initial_meters FROM units WHERE id = $1`, boxC.ID).Scan(&boxCMeters); err != nil || boxCMeters.Valid {
		t.Fatalf("reserved box was opened: %+v %v", boxCMeters, err)
	}

	// 4. Partial pieces (K11): 6 m from the 12 m left of box B as 4 m + 2 m.
	if code, ec, _ := assign(o4.UUID, l4, map[string]any{"barcode": boxB.Barcode, "meters": "4"}); code != http.StatusConflict || ec != "PARTIAL_PIECE_CONFIRM_REQUIRED" {
		t.Fatalf("partial without confirmation = %d %s", code, ec)
	}
	if code, ec, _ := assign(o4.UUID, l4, map[string]any{"barcode": boxB.Barcode, "meters": "4", "partial": true}); code != http.StatusBadRequest || !strings.Contains(ec, "VALIDATION") {
		t.Fatalf("partial without a note = %d %s", code, ec)
	}
	if code, ec, _ := assign(o4.UUID, l4, map[string]any{"barcode": boxB.Barcode, "meters": "1", "partial": true, "note": "x"}); code != http.StatusUnprocessableEntity || ec != "CUT_BELOW_MIN_CUT" {
		t.Fatalf("partial below the minimum cut = %d %s", code, ec)
	}
	code, ec, p = preview(o4.UUID, l4, boxB.Barcode, "4")
	if code != http.StatusOK || !p.Partial || p.Credits != 3 {
		t.Fatalf("partial preview = %d %s %+v", code, ec, p)
	}
	if code, ec, _ := assign(o4.UUID, l4, map[string]any{"barcode": boxB.Barcode, "meters": "4", "partial": true, "note": "iki parça", "credits": 6}); code != http.StatusBadRequest {
		t.Fatalf("credits above the line = %d %s", code, ec)
	}
	code, ec, a = assign(o4.UUID, l4, map[string]any{"barcode": boxB.Barcode, "meters": "4", "partial": true, "note": "iki parça"})
	if code != http.StatusOK || a.Items[0].Assigned != "4.00" || len(a.Items[0].Units) != 1 ||
		a.Items[0].Units[0].Credits == nil || *a.Items[0].Units[0].Credits != 3 ||
		a.Items[0].Units[0].PieceNote == nil || *a.Items[0].Units[0].PieceNote != "iki parça" {
		t.Fatalf("first partial piece = %d %s %+v", code, ec, a.Items)
	}
	code, ec, a = assign(o4.UUID, l4, map[string]any{"barcode": boxB.Barcode})
	if code != http.StatusOK || a.Items[0].Assigned != "6.00" || len(a.Items[0].Units) != 2 ||
		a.Items[0].Units[1].Credits == nil || *a.Items[0].Units[1].Credits != 2 {
		t.Fatalf("completing piece = %d %s %+v", code, ec, a.Items)
	}

	// 5. Cancel after the cut: the piece stays in stock and is listed.
	if code, ec := it.transition(staffTok, o3.UUID, "cancelled"); code != http.StatusOK {
		t.Fatalf("cancel = %d %s", code, ec)
	}
	code, env = it.do("GET", "/v1/orders/"+o3.UUID, hostOlex, whTok, nil)
	var cancelled cutAssign
	if code != http.StatusOK || json.Unmarshal(env.Data, &cancelled) != nil || len(cancelled.DetachedCutPieces) != 1 ||
		cancelled.DetachedCutPieces[0].CutMeters != "3.00" {
		t.Fatalf("detached pieces = %d %s", code, env.Data)
	}

	// 6. The first order ships: the piece leaves, the open roll stays.
	for _, step := range []struct{ tok, status string }{{whTok, "ready"}, {whTok, "shipped"}} {
		if code, ec := it.transition(step.tok, o1.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
}
