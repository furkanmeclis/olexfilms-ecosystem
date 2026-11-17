package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type t1226Adjustment struct {
	UUID                   string `json:"uuid"`
	AdjustmentNo           string `json:"adjustment_no"`
	Type                   string `json:"type"`
	Source                 string `json:"source"`
	Status                 string `json:"status"`
	TotalQuantity          int32  `json:"total_quantity"`
	TotalMeters            string `json:"total_meters"`
	RequiresSecondApproval bool   `json:"requires_second_approval"`
	CanApprove             *bool  `json:"can_approve"`
	Lines                  []struct {
		UUID   string  `json:"uuid"`
		Kind   string  `json:"kind"`
		Meters *string `json:"meters"`
		Posted bool    `json:"posted"`
	} `json:"lines"`
	Photos []struct {
		UUID string `json:"uuid"`
	} `json:"photos"`
}

// noPrice fails when a warehouse response carries a price or amount field.
func noPrice(t *testing.T, what string, raw []byte) {
	t.Helper()
	for _, k := range []string{`"price"`, `"unit_price"`, `"line_total"`, `"amount"`, `"currency"`, `"cost"`} {
		if bytes.Contains(raw, []byte(k)) {
			t.Fatalf("%s carries %s: %s", what, k, raw)
		}
	}
}

// pngPhoto is a 1x1 PNG.
var pngPhoto = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func (it *itest) uploadPhoto(path, bearer string, body []byte) (int, envelope) {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "photo.png")
	_, _ = fw.Write(body)
	_ = mw.Close()
	req := httptest.NewRequest("POST", path, &buf)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

// TEC-1226 acceptance: a received return is quarantined (not counted, not
// usable, listed for inspection); the inspection restocks or scraps it; a
// damage adjustment needs a photo and, above the threshold, a second person;
// no response carries a price; the ledger rebuild check stays clean.
func TestIntegrationStockAdjustmentsAndReturnInspection(t *testing.T) {
	c, _ := t229Setup(t)
	it := c.it
	ctx := context.Background()
	call := func(method, path, tok string, body any, want int) []byte {
		t.Helper()
		code, env := it.do(method, path, hostOlex, tok, body)
		if code != want {
			t.Fatalf("%s %s = %d, want %d: %+v %s", method, path, code, want, env.Error, env.Data)
		}
		return env.Data
	}
	errCode := func(method, path, tok string, body any, want int) string {
		t.Helper()
		code, env := it.do(method, path, hostOlex, tok, body)
		if code != want || env.Error == nil {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, code, want, env.Data)
		}
		return env.Error.Code
	}
	adj := func(raw []byte) t1226Adjustment {
		t.Helper()
		var a t1226Adjustment
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		return a
	}

	// 1. The dealer returns both rolls; the distributor receives them.
	items := []map[string]any{{"barcode": c.rolls[0].Barcode}, {"barcode": c.rolls[1].Barcode}}
	r, _ := it.transferCall("POST", "/v1/stock-transfers", c.dealerTok,
		map[string]any{"kind": "return", "items": items, "note": "Müşteri iadesi"}, http.StatusCreated)
	for _, m := range []struct{ tok, status string }{
		{c.distTok, "approved"}, {c.dealerTok, "shipped"}, {c.distTok, "received"},
	} {
		if code, ec := it.transferMove(m.tok, r.UUID, m.status); code != http.StatusOK {
			t.Fatalf("return -> %s = %d %s", m.status, code, ec)
		}
	}
	for _, u := range c.rolls {
		st, err := it.q.GetUnitCurrentState(ctx, u.ID)
		if err != nil || st.Status != "quarantine" || st.HolderOrgID != c.dist.ID {
			t.Fatalf("received return %s = %+v, %v", u.Barcode, st, err)
		}
	}
	// Not counted, not transferable before the inspection.
	if got := c.stockOf(c.distTok, c.dist); got != 0 {
		t.Fatalf("distributor stock with quarantined returns = %d, want 0", got)
	}
	if code, env := it.do("POST", "/v1/stock-transfers", hostOlex, c.distTok,
		map[string]any{"kind": "return", "items": []map[string]any{{"barcode": c.rolls[0].Barcode}}}); code != http.StatusBadRequest ||
		env.Error == nil || !strings.Contains(env.Error.Message, "not in this organization's stock") {
		t.Fatalf("returning a quarantined unit onwards = %d %+v %s, want 400 (not in stock)", code, env.Error, env.Data)
	}

	// 2. A distributor warehouse user (no price permission) sees the queue.
	wu, wpw := it.user("t1226-dist-wh")
	it.member(c.dist, wu, "staff", rbac.RoleDistributorWarehouseStaff)
	whTok := it.loginOrg(wu, wpw, c.dist)
	raw := call("GET", "/v1/warehouse/return-inspections", whTok, nil, http.StatusOK)
	noPrice(t, "inspection queue", raw)
	var queue struct {
		Items []struct {
			Unit struct {
				Barcode string `json:"barcode"`
			} `json:"unit"`
			Return *struct {
				TransferNo string  `json:"transfer_no"`
				Note       *string `json:"note"`
			} `json:"return"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(raw, &queue); err != nil || queue.Total != 2 || queue.Items[0].Return == nil ||
		queue.Items[0].Return.TransferNo != r.TransferNo {
		t.Fatalf("queue = %s (%v)", raw, err)
	}
	raw = call("GET", "/v1/warehouse/return-inspections/summary", whTok, nil, http.StatusOK)
	if !strings.Contains(string(raw), `"units":2`) {
		t.Fatalf("quarantine summary = %s", raw)
	}

	// 3. Restock the first roll; scrap the second (a scrap document).
	it.restockReturn(whTok, c.rolls[0].Barcode)
	if got := c.stockOf(c.distTok, c.dist); got != 1 {
		t.Fatalf("stock after restock = %d, want 1", got)
	}
	raw = call("POST", "/v1/warehouse/return-inspections", whTok, map[string]any{
		"barcode": c.rolls[1].Barcode, "decision": "scrap", "return_reason": "defective", "note": "Yapışkan bozuk",
	}, http.StatusOK)
	noPrice(t, "scrap decision", raw)
	var scrap struct {
		Status     string           `json:"status"`
		Adjustment *t1226Adjustment `json:"adjustment"`
	}
	if err := json.Unmarshal(raw, &scrap); err != nil || scrap.Adjustment == nil || scrap.Status != "quarantine" ||
		scrap.Adjustment.Source != "return_inspection" || scrap.Adjustment.Status != "pending_approval" {
		t.Fatalf("scrap = %s (%v)", raw, err)
	}
	if code := errCode("POST", "/v1/warehouse/return-inspections", whTok, map[string]any{
		"barcode": c.rolls[1].Barcode, "decision": "restock", "return_reason": "other",
	}, http.StatusConflict); code != "ADJUSTMENT_UNIT_BUSY" {
		t.Fatalf("second decision = %s", code)
	}
	a := adj(call("POST", "/v1/warehouse/adjustments/"+scrap.Adjustment.UUID+"/approve", c.distTok, nil, http.StatusOK))
	if a.Status != "posted" || len(a.Lines) != 1 || !a.Lines[0].Posted {
		t.Fatalf("approved scrap = %+v", a)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, c.rolls[1].ID); err != nil || st.Status != "void" {
		t.Fatalf("scrapped roll = %+v, %v", st, err)
	}

	// 4. Damage adjustment of the restocked roll: 2 m above a 1 m threshold.
	call("PUT", "/v1/warehouse/adjustment-settings", c.distTok,
		map[string]any{"threshold_quantity": 5, "threshold_meters": "1"}, http.StatusOK)
	a = adj(call("POST", "/v1/warehouse/adjustments", whTok, map[string]any{
		"type": "damage", "reason_code": "forklift", "note": "Rulo ezildi",
	}, http.StatusCreated))
	a = adj(call("POST", "/v1/warehouse/adjustments/"+a.UUID+"/lines", whTok,
		map[string]any{"barcode": c.rolls[0].Barcode, "meters": "2"}, http.StatusOK))
	if len(a.Lines) != 1 || a.Lines[0].Kind != "meters" || a.TotalMeters != "2.00" {
		t.Fatalf("damage draft = %+v", a)
	}
	if code := errCode("POST", "/v1/warehouse/adjustments/"+a.UUID+"/submit", whTok, nil,
		http.StatusUnprocessableEntity); code != "ADJUSTMENT_PHOTO_REQUIRED" {
		t.Fatalf("submit without photo = %s", code)
	}
	if code, env := it.uploadPhoto("/v1/warehouse/adjustments/"+a.UUID+"/photos", whTok, pngPhoto); code != http.StatusOK {
		t.Fatalf("photo upload = %d %+v", code, env.Error)
	}
	a = adj(call("POST", "/v1/warehouse/adjustments/"+a.UUID+"/submit", whTok, nil, http.StatusOK))
	if a.Status != "pending_approval" || !a.RequiresSecondApproval || len(a.Photos) != 1 ||
		a.CanApprove == nil || *a.CanApprove {
		t.Fatalf("submitted damage = %+v", a)
	}
	if code := errCode("POST", "/v1/warehouse/adjustments/"+a.UUID+"/approve", whTok, nil,
		http.StatusForbidden); code != "ADJUSTMENT_SELF_APPROVAL" {
		t.Fatalf("self approval = %s", code)
	}
	raw = call("POST", "/v1/warehouse/adjustments/"+a.UUID+"/approve", c.distTok, nil, http.StatusOK)
	noPrice(t, "approved adjustment", raw)
	if a = adj(raw); a.Status != "posted" {
		t.Fatalf("approved damage = %+v", a)
	}
	u, err := it.q.GetUnit(ctx, c.rolls[0].ID)
	if err != nil || !u.RemainingMeters.Valid {
		t.Fatal(err)
	}
	if v, _ := u.RemainingMeters.Float64Value(); v.Float64 != 23 {
		t.Fatalf("roll after damage = %v, want 23", v.Float64)
	}
	raw = call("GET", "/v1/warehouse/adjustments?type=damage,scrap&sort=-created_at", whTok, nil, http.StatusOK)
	noPrice(t, "adjustment list", raw)
	if !strings.Contains(string(raw), `"total":2`) && !strings.Contains(string(raw), `"total": 2`) {
		t.Fatalf("adjustment list = %s", raw)
	}

	// 5. The ledger rebuild check of the distributor is clean.
	rep, err := rebuild.New(it.pool, it.q).Check(ctx, c.dist.ID)
	if err != nil || rep.DiffCount != 0 || len(rep.Anomalies) != 0 {
		t.Fatalf("rebuild check: %v %+v %v", err, rep.Diffs, rep.Anomalies)
	}
}
