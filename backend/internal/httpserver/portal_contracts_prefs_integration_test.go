package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type portalPrefs struct {
	EmailEnabled    bool `json:"email_enabled"`
	InappEnabled    bool `json:"inapp_enabled"`
	RealtimeEnabled bool `json:"realtime_enabled"`
	PushEnabled     bool `json:"push_enabled"`
}

type portalCountPage struct {
	Items []map[string]any `json:"items"`
	Total int64            `json:"total"`
}

// TEC-245 acceptance:
//   - a customer without a contract gets 200 and an empty list on
//     GET /v1/portal/contracts;
//   - the customer reads and updates the notification preferences from the
//     portal (/v1/portal/notification-preferences; the panel path stays
//     closed to the portal realm);
//   - a fleet session (e-mail + password, read only) gets 200 on the vehicle
//     list and the transfer list but 403 PORTAL_READ_ONLY when it starts,
//     verifies or cancels a transfer.
func TestIntegrationPortalContractsPrefsFleet(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t245-dist", "distributor", center)
	dealer := it.org("t245-dealer", "dealer", dist)
	tail := it.suffix[len(it.suffix)-4:]

	cust, _ := it.svcCustomer(dealer, "t245-cust", "34C245"+tail)
	custTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: cust.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Contracts: none recorded yet, so an empty list with 200.
	cp := decodeData[portalCountPage](t, it.custDo("GET", "/v1/portal/contracts", custTok, nil, http.StatusOK))
	if cp.Total != 0 || cp.Items == nil || len(cp.Items) != 0 {
		t.Fatalf("contracts = %+v, want an empty list", cp)
	}

	// 2. Notification preferences from the portal: read, update, read back.
	prefsPath := "/v1/portal/notification-preferences"
	before := decodeData[portalPrefs](t, it.custDo("GET", prefsPath, custTok, nil, http.StatusOK))
	want := portalPrefs{EmailEnabled: !before.EmailEnabled, InappEnabled: true, RealtimeEnabled: true}
	saved := decodeData[portalPrefs](t, it.custDo("PUT", prefsPath, custTok, want, http.StatusOK))
	if saved.EmailEnabled != want.EmailEnabled || !saved.InappEnabled {
		t.Fatalf("saved = %+v, want %+v", saved, want)
	}
	if got := decodeData[portalPrefs](t, it.custDo("GET", prefsPath, custTok, nil, http.StatusOK)); got.EmailEnabled != want.EmailEnabled {
		t.Fatalf("read back = %+v", got)
	}
	if code, env := it.do("GET", "/v1/notification-preferences", hostOlex, custTok, nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel preferences path with a portal token = %d %s", code, errCode(env))
	}

	// 3. Fleet session: reads open, transfer writes 403 PORTAL_READ_ONLY.
	fleet, fpw := it.user("t245-fleet", rbac.RoleFleet)
	_, veh := it.svcCustomer(dealer, "t245-fleet-veh", "34F245"+tail)
	if _, err := it.pool.Exec(ctx, `UPDATE vehicles SET user_id = $2 WHERE id = $1`, veh.ID, fleet.ID); err != nil {
		t.Fatalf("fleet vehicle: %v", err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM vehicles WHERE id = $1`, veh.ID)
	})
	fleetTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": fleet.Email.String, "password": fpw, "realm": "portal",
	})).AccessToken

	vp := decodeData[portalCountPage](t, it.custDo("GET", "/v1/portal/vehicles", fleetTok, nil, http.StatusOK))
	if vp.Total != 1 || len(vp.Items) != 1 || vp.Items[0]["uuid"] != veh.Uuid.String() {
		t.Fatalf("fleet vehicles = %+v", vp)
	}
	vPath := "/v1/portal/vehicles/" + veh.Uuid.String() + "/transfers"
	it.custDo("GET", vPath, fleetTok, nil, http.StatusOK)
	it.custDo("GET", "/v1/portal/contracts", fleetTok, nil, http.StatusOK)
	it.custDo("GET", prefsPath, fleetTok, nil, http.StatusOK)

	tr := "/v1/portal/vehicle-transfers/" + uuid.NewString()
	for _, c := range []struct {
		path string
		body any
	}{
		{vPath, map[string]any{"phone": itPhone()}},
		{"/v1/portal/appointments", map[string]any{}},
		{tr + "/verify", map[string]any{"from_code": "123456"}},
		{tr + "/cancel", nil},
	} {
		if code, env := it.do("POST", c.path, hostOlex, fleetTok, c.body); code != http.StatusForbidden || errCode(env) != "PORTAL_READ_ONLY" {
			t.Fatalf("fleet POST %s = %d %s, want 403 PORTAL_READ_ONLY", c.path, code, errCode(env))
		}
	}
	var transfers int64
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM vehicle_transfers WHERE vehicle_id = $1`, veh.ID).Scan(&transfers); err != nil || transfers != 0 {
		t.Fatalf("fleet started a transfer: %d %v", transfers, err)
	}
}

func TestIntegrationPortalContractPDFOwnership(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t288-dist", "distributor", center)
	dealer := it.org("t288-dealer", "dealer", dist)
	tail := it.suffix[len(it.suffix)-4:]

	owner, veh := it.svcCustomer(dealer, "t288-owner", "34C288"+tail)
	stranger, _ := it.svcCustomer(dealer, "t288-stranger", "34S288"+tail)
	svc := it.directService(dealer, owner, veh, 288)
	tpl, err := it.q.CreateContractTemplate(ctx, db.CreateContractTemplateParams{
		OrganizationID: center.ID, BrandID: center.BrandID, Name: "TEC288 " + it.suffix,
		Kind: "vehicle_intake", IsActive: true,
	})
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	body := "<p>executed</p>"
	sum := sha256.Sum256([]byte(body))
	executed, err := it.q.CreateContractInstance(ctx, db.CreateContractInstanceParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, ContractNo: 288001,
		SubjectType: "service", SubjectID: svc.ID, TemplateID: tpl.ID, Kind: "vehicle_intake", Locale: "tr",
		TemplateVersion: 1, Status: "pending", RenderedHtml: pgtype.Text{String: body, Valid: true},
		ContentSha256: pgtype.Text{String: hex.EncodeToString(sum[:]), Valid: true},
	})
	if err != nil {
		t.Fatalf("contract instance: %v", err)
	}
	executed, err = it.q.ExecuteContractInstance(ctx, db.ExecuteContractInstanceParams{
		ID: executed.ID, RenderedHtml: body, ContentSha256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatalf("execute contract: %v", err)
	}
	key := storage.ContractExecutedPDFObjectKey(dealer.Uuid, executed.Uuid)
	if err := store.Upload(ctx, storage.File{
		Body: bytes.NewReader([]byte("%PDF-1.7\ntec288\n")), Size: 16, ContentType: "application/pdf", Filename: "contract.pdf",
	}, key); err != nil {
		t.Fatalf("upload pdf: %v", err)
	}
	if _, err := it.q.SetContractInstancePDFKey(ctx, db.SetContractInstancePDFKeyParams{ID: executed.ID, PdfKey: key}); err != nil {
		t.Fatalf("set pdf key: %v", err)
	}
	pending, err := it.q.CreateContractInstance(ctx, db.CreateContractInstanceParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, ContractNo: 288002,
		SubjectType: "service", SubjectID: svc.ID, TemplateID: tpl.ID, Kind: "vehicle_intake", Locale: "tr",
		TemplateVersion: 1, Status: "pending",
	})
	if err != nil {
		t.Fatalf("pending contract: %v", err)
	}

	ownerTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: owner.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
	if err != nil {
		t.Fatal(err)
	}
	strangerTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: stranger.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
	if err != nil {
		t.Fatal(err)
	}

	page := decodeData[portalCountPage](t, it.custDo("GET", "/v1/portal/contracts", ownerTok, nil, http.StatusOK))
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0]["contract_uuid"] != executed.Uuid.String() || page.Items[0]["pdf_ready"] != true {
		t.Fatalf("portal contracts = %+v, want executed PDF-ready contract only", page)
	}
	path := "/v1/portal/contracts/" + executed.Uuid.String() + "/pdf"
	rec := it.raw("GET", path, ownerTok, "", nil, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("owner pdf = %d %s %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if rec := it.raw("GET", path, strangerTok, "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("stranger pdf = %d, want 404", rec.Code)
	}
	if rec := it.raw("GET", "/v1/portal/contracts/"+pending.Uuid.String()+"/pdf", ownerTok, "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("owner pending contract pdf = %d, want 404", rec.Code)
	}
	strangerPage := decodeData[portalCountPage](t, it.custDo("GET", "/v1/portal/contracts", strangerTok, nil, http.StatusOK))
	if strangerPage.Total != 0 || len(strangerPage.Items) != 0 {
		t.Fatalf("stranger portal contracts = %+v, want empty", strangerPage)
	}
	// The API process drains the outbox too: contract.executed must reach the
	// PDF scheduler here as well as in cmd/worker.
	if bus, ok := it.srv.events.(*events.MemoryBus); !ok || bus.HandlerCount(events.ContractExecuted) != 1 {
		t.Fatalf("contract.executed handlers on the API bus: ok=%v", ok)
	}
}
