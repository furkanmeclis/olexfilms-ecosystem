package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

type einvoiceResp struct {
	UUID              string  `json:"uuid"`
	Number            *string `json:"number"`
	Status            string  `json:"status"`
	Payable           string  `json:"payable"`
	XMLSHA256         *string `json:"xml_sha256"`
	SourceUUID        string  `json:"source_uuid"`
	BuyerOrganization *struct {
		UUID string `json:"uuid"`
	} `json:"buyer_organization"`
}

type billableResp struct {
	SourceType string `json:"source_type"`
	SourceUUID string `json:"source_uuid"`
	Payable    string `json:"payable"`
}

// TEC-503 acceptance (HTTP): the e_invoice add-on of the center gates the
// API (403 when off); distributor and dealer users get 403; settings need
// einvoice.settings; billable sources and invoices follow the list
// contract; a draft for an incomplete buyer is 422 with the missing
// fields, a second one for the same source 409; archive and void need a
// step-up; the archived XML streams with its sha256; a voided source is
// billable again.
func TestIntegrationEinvoices(t *testing.T) {
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory() })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t503-dist", "distributor", center)
	bare := it.org("t503-bare", "distributor", center)
	dealer := it.org("t503-dealer", "dealer", dist)
	for _, o := range []db.Organization{dist, bare} {
		if _, err := it.pool.Exec(ctx, `UPDATE organizations SET address = 'Kordon No:5', city = 'İzmir', district = 'Konak' WHERE id = $1`, o.ID); err != nil {
			t.Fatal(err)
		}
	}

	acc, apw := it.user("t503-acc")
	it.member(center, acc, "staff", rbac.RoleCenterAccounting)
	admin, adpw := it.user("t503-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff", rbac.RoleCenterAccounting)
	distOwner, dpw := it.user("t503-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerOwner, rpw := it.user("t503-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	accTok := it.loginOrg(acc, apw, center)
	adminTok := it.loginOrg(admin, adpw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)

	var orders []string
	order := func(buyer db.Organization, qty int, price string) string {
		t.Helper()
		p := it.product(center, fmt.Sprintf("T503-%d", len(orders)))
		var id int64
		var uid string
		if err := it.pool.QueryRow(ctx, `INSERT INTO orders (
			order_no, organization_id, brand_id, seller_org_id, buyer_org_id, status, currency, subtotal, total, received_at
		) VALUES ($1, $2, $3, $2, $4, 'received', 'TRY', $5::numeric * $6::int, $5::numeric * $6::int, NOW()) RETURNING id, uuid::text`,
			fmt.Sprintf("T503-%s-%d", it.suffix, len(orders)), center.ID, center.BrandID, buyer.ID, price, qty).Scan(&id, &uid); err != nil {
			t.Fatalf("order: %v", err)
		}
		if _, err := it.pool.Exec(ctx, `INSERT INTO order_items (
			order_id, organization_id, brand_id, product_id, quantity, unit_price, price_source, line_total
		) VALUES ($1, $2, $3, $4, $5::int, $6::numeric, 'list', $6::numeric * $5::int)`,
			id, center.ID, center.BrandID, p.ID, qty, price); err != nil {
			t.Fatalf("order item: %v", err)
		}
		orders = append(orders, uid)
		return uid
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = it.pool.Exec(bg, `UPDATE einvoices SET status = 'voided', voided_at = NOW(), void_reason = 'test cleanup', error = NULL
			WHERE organization_id = $1 AND buyer_org_id = ANY($2) AND status <> 'voided'`, center.ID, []int64{dist.ID, bare.ID})
		_, _ = it.pool.Exec(bg, `DELETE FROM einvoices WHERE organization_id = $1 AND buyer_org_id = ANY($2)`, center.ID, []int64{dist.ID, bare.ID})
		_, _ = it.pool.Exec(bg, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE uuid::text = ANY($1))`, orders)
		_, _ = it.pool.Exec(bg, `DELETE FROM orders WHERE uuid::text = ANY($1)`, orders)
		_, _ = it.srv.features.SetByAdmin(bg, 0, center.ID, features.ModuleEInvoice, false)
	})

	// Module off: 403 FEATURE_DISABLED.
	if code, env := it.do("GET", "/v1/einvoices", hostOlex, accTok, nil); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("module off = %d %s", code, errCode(env))
	}
	for _, o := range []db.Organization{center, dist, dealer} {
		if _, err := it.srv.features.SetByAdmin(ctx, 0, o.ID, features.ModuleEInvoice, true); err != nil {
			t.Fatal(err)
		}
	}
	// Distributor and dealer users: 403 even with the module on.
	for _, tok := range []string{distTok, dealerTok} {
		for _, path := range []string{"/v1/einvoices", "/v1/einvoices/billable", "/v1/einvoices/settings"} {
			if code, _ := it.do("GET", path, hostOlex, tok, nil); code != http.StatusForbidden {
				t.Fatalf("GET %s by a non-center user = %d, want 403", path, code)
			}
		}
		if code, _ := it.do("POST", "/v1/einvoices", hostOlex, tok, map[string]any{"source_type": "order", "source_uuid": dist.Uuid.String()}); code != http.StatusForbidden {
			t.Fatalf("non-center draft = %d", code)
		}
	}

	// Settings: einvoice.settings (super_admin) only.
	settings := map[string]any{
		"vkn": "9000068418", "tax_office": "Beşiktaş", "legal_name": "Olexfilms Merkez A.Ş.",
		"address": "Papatya Cad. No:21", "city": "İstanbul", "district": "Beşiktaş",
		"earchive_series": "EAR", "efatura_series": "EFN", "pdf_enabled": true,
	}
	if code, _ := it.do("PUT", "/v1/einvoices/settings", hostOlex, accTok, settings); code != http.StatusForbidden {
		t.Fatalf("accounting settings write = %d", code)
	}
	bad := map[string]any{}
	for k, v := range settings {
		bad[k] = v
	}
	bad["earchive_series"] = "TMP"
	if code, env := it.do("PUT", "/v1/einvoices/settings", hostOlex, adminTok, bad); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("reserved series = %d %s", code, errCode(env))
	}
	got := decodeData[struct {
		Configured bool `json:"configured"`
	}](t, mustDo(t, it, "PUT", "/v1/einvoices/settings", adminTok, settings, http.StatusOK))
	if !got.Configured {
		t.Fatal("settings not configured")
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM einvoice_settings WHERE organization_id = $1`, center.ID)
	})

	// Billable sources: list contract.
	o1 := order(dist, 1, "300.00")
	o2 := order(dist, 2, "50.00")
	o3 := order(bare, 1, "10.00")
	page := func(path string) listPage[billableResp] {
		return decodeData[listPage[billableResp]](t, mustDo(t, it, "GET", path, accTok, nil, http.StatusOK))
	}
	asc := page("/v1/einvoices/billable?buyer=" + dist.Uuid.String() + "&sort=payable")
	if asc.Total != 2 || asc.Items[0].SourceUUID != o2 || asc.Items[1].SourceUUID != o1 || asc.Items[0].Payable != "100.00" {
		t.Fatalf("billable asc = %+v", asc)
	}
	desc := page("/v1/einvoices/billable?buyer=" + dist.Uuid.String() + "&sort=-payable&source_type=order&limit=1")
	if desc.Total != 2 || len(desc.Items) != 1 || desc.Items[0].SourceUUID != o1 {
		t.Fatalf("billable desc = %+v", desc)
	}
	if none := page("/v1/einvoices/billable?buyer=" + dist.Uuid.String() + "&source_type=service_subscription"); none.Total != 0 {
		t.Fatalf("subscription filter = %+v", none)
	}
	for _, q := range []string{"sort=bogus", "source_type=invoice", "payable_min=5&payable_max=1", "buyer=nope"} {
		if code, env := it.do("GET", "/v1/einvoices/billable?"+q, hostOlex, accTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("billable %s = %d %s", q, code, errCode(env))
		}
	}

	// Draft: buyer without a tax id is 422 with the missing fields.
	rec := it.raw("POST", "/v1/einvoices", accTok, "application/json",
		[]byte(`{"source_type":"order","source_uuid":"`+o3+`"}`), nil)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"EINVOICE_BUYER_PROFILE_INCOMPLETE"`) ||
		!strings.Contains(rec.Body.String(), `"field":"invoice_vkn"`) {
		t.Fatalf("draft without profile = %d %s", rec.Code, rec.Body.String())
	}
	if code, env := it.do("POST", "/v1/einvoices", hostOlex, accTok, map[string]any{"source_type": "x", "source_uuid": "y"}); code != http.StatusBadRequest {
		t.Fatalf("bad draft body = %d %s", code, errCode(env))
	}
	// Buyer profile (einvoice.manage).
	if code, env := it.do("PUT", "/v1/platform/organizations/"+dist.Uuid.String()+"/invoice-profile", hostOlex, accTok,
		map[string]any{"invoice_vkn": "1234567891"}); code != http.StatusBadRequest {
		t.Fatalf("bad VKN = %d %s", code, errCode(env))
	}
	prof := decodeData[struct {
		Missing []string `json:"missing_fields"`
	}](t, mustDo(t, it, "PUT", "/v1/platform/organizations/"+dist.Uuid.String()+"/invoice-profile", accTok,
		map[string]any{"invoice_vkn": "1234567890", "invoice_tax_office": "Konak", "invoice_legal_name": "Ege Distribütör Ltd. Şti."}, http.StatusOK))
	if len(prof.Missing) != 0 {
		t.Fatalf("profile missing = %v", prof.Missing)
	}
	// TEC-504: the organization card reads the profile; the settings screen
	// renders a sample with the current stylesheet.
	read := decodeData[struct {
		VKN *string `json:"invoice_vkn"`
	}](t, mustDo(t, it, "GET", "/v1/platform/organizations/"+dist.Uuid.String()+"/invoice-profile", accTok, nil, http.StatusOK))
	if read.VKN == nil || *read.VKN != "1234567890" {
		t.Fatalf("profile read = %+v", read)
	}
	if rec := it.raw("GET", "/v1/einvoices/settings/xslt/preview", accTok, "", nil, nil); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `data-watermark="PREVIEW"`) {
		t.Fatalf("sample preview = %d", rec.Code)
	}

	d1 := decodeData[einvoiceResp](t, mustDo(t, it, "POST", "/v1/einvoices", accTok, map[string]any{"source_type": "order", "source_uuid": o1}, http.StatusCreated))
	d2 := decodeData[einvoiceResp](t, mustDo(t, it, "POST", "/v1/einvoices", accTok, map[string]any{"source_type": "order", "source_uuid": o2}, http.StatusCreated))
	if d1.Status != "draft" || d1.Number != nil || d1.Payable != "360.00" {
		t.Fatalf("draft = %+v", d1)
	}
	if code, env := it.do("POST", "/v1/einvoices", hostOlex, accTok, map[string]any{"source_type": "order", "source_uuid": o1}); code != http.StatusConflict || errCode(env) != "EINVOICE_SOURCE_ALREADY_INVOICED" {
		t.Fatalf("second draft = %d %s", code, errCode(env))
	}
	if left := page("/v1/einvoices/billable?buyer=" + dist.Uuid.String()); left.Total != 0 {
		t.Fatalf("billable after drafts = %+v", left)
	}

	// Preview: HTML with the watermark.
	rec = it.raw("GET", "/v1/einvoices/"+d1.UUID+"/preview", accTok, "", nil, nil)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(rec.Body.String(), `data-watermark="PREVIEW"`) {
		t.Fatalf("preview = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	// Archive: step-up.
	if code, env := it.do("POST", "/v1/einvoices/"+d1.UUID+"/archive", hostOlex, accTok, nil); code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("archive without step-up = %d %s", code, errCode(env))
	}
	it.stepUp(acc.Uuid)
	a1 := decodeData[einvoiceResp](t, mustDo(t, it, "POST", "/v1/einvoices/"+d1.UUID+"/archive", accTok, nil, http.StatusOK))
	if a1.Status != "archived" || a1.Number == nil || a1.XMLSHA256 == nil {
		t.Fatalf("archive = %+v", a1)
	}
	rec = it.raw("GET", "/v1/einvoices/"+d1.UUID+"/xml", accTok, "", nil, nil)
	sum := sha256.Sum256(rec.Body.Bytes())
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/xml" || hex.EncodeToString(sum[:]) != *a1.XMLSHA256 {
		t.Fatalf("xml download = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if code, env := it.do("GET", "/v1/einvoices/"+d2.UUID+"/xml", hostOlex, accTok, nil); code != http.StatusConflict || errCode(env) != "EINVOICE_NOT_ARCHIVED" {
		t.Fatalf("draft xml = %d %s", code, errCode(env))
	}
	if rec := it.raw("GET", "/v1/einvoices/"+d1.UUID+"/html", accTok, "", nil, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), *a1.Number) {
		t.Fatalf("html = %d", rec.Code)
	}

	// Invoice list: list contract.
	list := func(path string) listPage[einvoiceResp] {
		return decodeData[listPage[einvoiceResp]](t, mustDo(t, it, "GET", path, accTok, nil, http.StatusOK))
	}
	byPayable := list("/v1/einvoices?buyer=" + dist.Uuid.String() + "&sort=payable")
	if byPayable.Total != 2 || byPayable.Items[0].UUID != d2.UUID || byPayable.Items[1].UUID != d1.UUID {
		t.Fatalf("list asc = %+v", byPayable)
	}
	if desc := list("/v1/einvoices?buyer=" + dist.Uuid.String() + "&sort=-payable"); desc.Items[0].UUID != d1.UUID {
		t.Fatalf("list desc = %+v", desc)
	}
	if arch := list("/v1/einvoices?buyer=" + dist.Uuid.String() + "&status=archived&q=" + *a1.Number); arch.Total != 1 || arch.Items[0].UUID != d1.UUID {
		t.Fatalf("list archived = %+v", arch)
	}
	if multi := list("/v1/einvoices?buyer=" + dist.Uuid.String() + "&status=draft,archived"); multi.Total != 2 {
		t.Fatalf("list multi status = %+v", multi)
	}
	for _, q := range []string{"sort=bogus", "status=paid", "issue_date_from=2026-02-01&issue_date_to=2026-01-01", "profile=X"} {
		if code, env := it.do("GET", "/v1/einvoices?"+q, hostOlex, accTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("list %s = %d %s", q, code, errCode(env))
		}
	}
	detail := decodeData[einvoiceResp](t, mustDo(t, it, "GET", "/v1/einvoices/"+d1.UUID, accTok, nil, http.StatusOK))
	if detail.SourceUUID != o1 || detail.BuyerOrganization == nil || detail.BuyerOrganization.UUID != dist.Uuid.String() {
		t.Fatalf("detail = %+v", detail)
	}

	// Void: step-up, reason required; the source is billable again.
	if code, env := it.do("POST", "/v1/einvoices/"+d1.UUID+"/void", hostOlex, accTok, map[string]any{"reason": ""}); code != http.StatusBadRequest {
		t.Fatalf("void without reason = %d %s", code, errCode(env))
	}
	v := decodeData[einvoiceResp](t, mustDo(t, it, "POST", "/v1/einvoices/"+d1.UUID+"/void", accTok, map[string]any{"reason": "Yanlış tutar"}, http.StatusOK))
	if v.Status != "voided" || v.Number == nil || *v.Number != *a1.Number {
		t.Fatalf("void = %+v", v)
	}
	if again := page("/v1/einvoices/billable?buyer=" + dist.Uuid.String()); again.Total != 1 || again.Items[0].SourceUUID != o1 {
		t.Fatalf("billable after void = %+v", again)
	}
	if code, env := it.do("POST", "/v1/einvoices/"+d1.UUID+"/void", hostOlex, accTok, map[string]any{"reason": "x"}); code != http.StatusConflict || errCode(env) != "EINVOICE_INVALID_STATUS" {
		t.Fatalf("second void = %d %s", code, errCode(env))
	}
	// Step-up is per user: the super admin's archive needs its own.
	if code, env := it.do("POST", "/v1/einvoices/"+d2.UUID+"/archive", hostOlex, adminTok, nil); code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("admin archive without step-up = %d %s", code, errCode(env))
	}
}
