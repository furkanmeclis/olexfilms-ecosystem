package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	servicecatalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-508 (F5-10a): the module chain end to end over HTTP (real DB). Each
// scenario is its own test and uses fresh organizations; only the system
// switch test closes a module system wide (efficiency) and reopens it.

type featureMetaItem struct {
	features.State
	Description     string `json:"description"`
	FreeDefault     bool   `json:"free_default"`
	ContactForPrice bool   `json:"contact_for_price"`
	Price           *struct {
		Amount     string `json:"amount"`
		Currency   string `json:"currency"`
		Recurrence string `json:"recurrence"`
		ItemName   string `json:"item_name"`
	} `json:"price"`
	Request *struct {
		UUID         string `json:"uuid"`
		Status       string `json:"status"`
		DecisionNote string `json:"decision_note"`
	} `json:"request"`
}

type featureMetaPayload struct {
	Items   []featureMetaItem `json:"items"`
	Enabled []string          `json:"enabled"`
}

// featureMeta reads GET /v1/features with an Accept-Language header.
func (it *itest) featureMeta(access, lang string) featureMetaPayload {
	it.t.Helper()
	req := httptest.NewRequest("GET", "/v1/features", nil)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Authorization", "Bearer "+access)
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if rec.Code != http.StatusOK {
		it.t.Fatalf("GET /v1/features (%s) = %d %s", lang, rec.Code, errCode(env))
	}
	var p featureMetaPayload
	if err := json.Unmarshal(env.Data, &p); err != nil {
		it.t.Fatal(err)
	}
	return p
}

func (p featureMetaPayload) item(key string) (featureMetaItem, bool) {
	for _, it := range p.Items {
		if it.Key == key {
			return it, true
		}
	}
	return featureMetaItem{}, false
}

// notGated asserts that a module route does not answer FEATURE_DISABLED.
func (it *itest) notGated(method, path, tok string) {
	it.t.Helper()
	if code, ec := it.status(method, path, tok); code == http.StatusForbidden && ec == response.CodeFeatureDisabled {
		it.t.Fatalf("%s %s = 403 FEATURE_DISABLED, want the module open", method, path)
	}
}

// gated asserts 403 FEATURE_DISABLED.
func (it *itest) gated(method, path, tok string) {
	it.t.Helper()
	if code, ec := it.status(method, path, tok); code != http.StatusForbidden || ec != response.CodeFeatureDisabled {
		it.t.Fatalf("%s %s = %d %s, want 403 FEATURE_DISABLED", method, path, code, ec)
	}
}

func (it *itest) adminSet(admin string, org db.Organization, key string, enabled bool) {
	it.t.Helper()
	code, env := it.do("PUT", "/v1/platform/organizations/"+org.Uuid.String()+"/modules/"+key, hostOlex, admin, map[string]any{"enabled": enabled})
	if code != http.StatusOK {
		it.t.Fatalf("admin %s=%v on %s = %d %s", key, enabled, org.Name, code, errCode(env))
	}
}

// Chain 1: the admin closes a module system wide; the distributor cannot
// open it (409) and it is off at the dealer even with an admin value.
func TestIntegrationF5ChainSystemCloseBlocksDistributor(t *testing.T) {
	it := newIntegration(t)
	key := features.ModuleEfficiency
	it.reopenSystem(key)
	net := it.featureNet(1)
	dealer := net.dealers[0]
	it.adminSet(net.admin, net.dist, key, true)
	it.adminSet(net.admin, dealer, key, true)
	it.notGated("GET", "/v1/efficiency/summary", net.dealerTok)

	code, env := it.do("PATCH", "/v1/platform/modules/"+key, hostOlex, net.admin, map[string]any{"enabled": false})
	if code != http.StatusOK {
		t.Fatalf("system close = %d %s", code, errCode(env))
	}
	code, env = it.do("PUT", "/v1/tenant/modules/dealer-standard/"+key, hostOlex, net.distTok, map[string]any{"enabled": true})
	if code != http.StatusConflict || errCode(env) != response.CodeFeatureDisabled {
		t.Fatalf("distributor standard = %d %s, want 409", code, errCode(env))
	}
	code, env = it.do("PUT", "/v1/tenant/modules/dealers/"+dealer.Uuid.String()+"/"+key, hostOlex, net.distTok, map[string]any{"enabled": true})
	if code != http.StatusConflict {
		t.Fatalf("distributor dealer = %d %s, want 409", code, errCode(env))
	}
	if hasKey(it.featureMeta(net.dealerTok, "").Enabled, key) {
		t.Fatal("dealer keeps a system-closed module")
	}
	it.gated("GET", "/v1/efficiency/summary", net.dealerTok)
	it.gated("GET", "/v1/efficiency/summary", net.distTok)
}

// Chain 2: the distributor opens its dealer standard; a dealer opened
// afterwards inherits it.
func TestIntegrationF5ChainDealerStandardInheritedByNewDealer(t *testing.T) {
	it := newIntegration(t)
	key := features.ModuleCertificates
	net := it.featureNet(0)
	it.adminSet(net.admin, net.dist, key, true)
	code, env := it.do("PUT", "/v1/tenant/modules/dealer-standard/"+key, hostOlex, net.distTok, map[string]any{"enabled": true})
	if code != http.StatusOK {
		t.Fatalf("standard = %d %s", code, errCode(env))
	}
	fresh := it.org("chain-fresh", "dealer", net.dist)
	owner, pw := it.user("chain-fresh-owner")
	it.member(fresh, owner, "owner")
	tok := it.loginOrg(owner, pw, fresh)
	st, ok := it.featureMeta(tok, "").item(key)
	if !ok || !st.Enabled || st.Source != features.FromStandard {
		t.Fatalf("new dealer state = %+v (listed %v), want on from the standard", st.State, ok)
	}
	it.notGated("GET", "/v1/certificates", tok)
}

// Chain 3: the admin opens a module for a dealer regardless of the
// distributor; it stays on while the distributor has it off.
func TestIntegrationF5ChainAdminOpensDealerIndependently(t *testing.T) {
	it := newIntegration(t)
	key := features.ModulePerformance
	net := it.featureNet(2)
	it.adminSet(net.admin, net.dist, key, false)
	it.adminSet(net.admin, net.dealers[0], key, true)

	st, ok := it.featureMeta(net.dealerTok, "").item(key)
	if !ok || !st.Enabled || !st.AdminOverride || st.UpstreamEnabled {
		t.Fatalf("dealer state = %+v, want on by the admin under a closed distributor", st.State)
	}
	it.notGated("GET", "/v1/performance/dashboard", net.dealerTok)
	if hasKey(it.featureMeta(net.distTok, "").Enabled, key) {
		t.Fatal("distributor must stay closed")
	}
	it.gated("GET", "/v1/performance/dashboard", net.distTok)
	code, env := it.do("PUT", "/v1/tenant/modules/dealers/"+net.dealers[0].Uuid.String()+"/"+key, hostOlex, net.distTok, map[string]any{"enabled": false})
	if code != http.StatusConflict || errCode(env) != response.CodeModuleAdminOverride {
		t.Fatalf("distributor override = %d %s, want 409 MODULE_ADMIN_OVERRIDE", code, errCode(env))
	}
	ok2, err := it.srv.features.Enabled(context.Background(), net.dealers[1].ID, key)
	if err != nil || ok2 {
		t.Fatalf("sibling dealer = %v, %v, want off", ok2, err)
	}
}

// bundleItem creates an active module_bundle catalog item of the center.
func (it *itest) bundleItem(name, price string, keys ...string) db.ServiceCatalogItem {
	it.t.Helper()
	ctx := context.Background()
	center := it.brandCenter("olex")
	var p pgtype.Numeric
	_ = p.Scan(price)
	var fee pgtype.Numeric
	_ = fee.Scan("0")
	item, err := it.q.CreateServiceCatalogItem(ctx, db.CreateServiceCatalogItemParams{
		OrganizationID: center.ID, BrandID: center.BrandID, Name: name + " " + it.suffix, Description: "bundle",
		Category: servicecatalogusecase.CategoryModuleBundle, DefaultPrice: p, Currency: "TRY", Recurrence: "monthly",
		CancellationFee: fee, IsActive: true,
	})
	if err != nil {
		it.t.Fatalf("bundle item: %v", err)
	}
	it.t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM service_subscriptions WHERE item_id = $1", item.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM service_catalog_items WHERE id = $1", item.ID)
	})
	for _, k := range keys {
		if err := it.q.AddServiceCatalogModule(ctx, db.AddServiceCatalogModuleParams{ItemID: item.ID, ModuleKey: k}); err != nil {
			it.t.Fatalf("bundle module %s: %v", k, err)
		}
	}
	return item
}

func (it *itest) assignBundle(tok string, item db.ServiceCatalogItem, org db.Organization, starts, ends time.Time) string {
	it.t.Helper()
	code, env := it.do("POST", "/v1/service-subscriptions", hostOlex, tok, map[string]any{
		"item_uuid": item.Uuid.String(), "organization_uuid": org.Uuid.String(),
		"starts_on": starts.Format(time.DateOnly), "ends_on": ends.Format(time.DateOnly),
	})
	if code != http.StatusCreated && code != http.StatusOK {
		it.t.Fatalf("assign %s = %d %s", item.Name, code, errCode(env))
	}
	var sub struct {
		UUID string `json:"uuid"`
	}
	_ = json.Unmarshal(env.Data, &sub)
	return sub.UUID
}

// Chain 4: a module bundle subscription opens the module; when it expires
// or is cancelled the module closes unless another active subscription
// still covers it.
func TestIntegrationF5ChainModuleBundleSubscription(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	key := features.ModuleFleet
	net := it.featureNet(0)
	center := it.brandCenter("olex")
	staff, spw := it.user("chain-center-staff")
	it.member(center, staff, "staff")
	centerTok := it.loginOrg(staff, spw, center)
	expiry := servicecatalogusecase.New(it.q).WithLifecycle(it.pool, nil, nil, it.srv.features)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	isOn := func() bool {
		on, err := it.srv.features.Enabled(ctx, net.dist.ID, key)
		if err != nil {
			t.Fatal(err)
		}
		return on
	}
	if isOn() {
		t.Fatal("fleet starts off")
	}

	long := it.bundleItem("Fleet yearly", "900.00", key)
	short := it.bundleItem("Fleet trial", "0.00", key, features.ModuleCertificates)
	longSub := it.assignBundle(centerTok, long, net.dist, today.AddDate(0, 0, -1), today.AddDate(1, 0, 0))
	if !isOn() {
		t.Fatal("subscription must open the module")
	}
	it.notGated("GET", "/v1/fleets", net.distTok)
	// A second bundle that ended yesterday expires; the yearly one still
	// covers the module.
	it.assignBundle(centerTok, short, net.dist, today.AddDate(0, 0, -10), today.AddDate(0, 0, -1))
	if n, err := expiry.ExpireDue(ctx, today); err != nil || n < 1 {
		t.Fatalf("expire = %d, %v", n, err)
	}
	if !isOn() {
		t.Fatal("another active subscription keeps the module open")
	}
	if on, _ := it.srv.features.Enabled(ctx, net.dist.ID, features.ModuleCertificates); on {
		t.Fatal("the expired bundle's own module must close")
	}

	// Cancelling the yearly one closes the module.
	code, env := it.do("POST", "/v1/service-subscriptions/"+longSub+"/cancel-request", hostOlex, net.distTok, map[string]any{"reason": "done"})
	if code != http.StatusCreated {
		t.Fatalf("cancel request = %d %s", code, errCode(env))
	}
	var cr struct {
		UUID string `json:"uuid"`
	}
	_ = json.Unmarshal(env.Data, &cr)
	code, env = it.do("POST", "/v1/service-subscriptions/cancel-requests/"+cr.UUID+"/approve", hostOlex, centerTok, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("approve cancel = %d %s", code, errCode(env))
	}
	if isOn() {
		t.Fatal("cancelled subscription must close the module")
	}
	it.gated("GET", "/v1/fleets", net.distTok)

	// A lone subscription that ends closes the module at expiry.
	it.assignBundle(centerTok, long, net.dist, today.AddDate(0, 0, -5), today.AddDate(0, 0, -1))
	if !isOn() {
		t.Fatal("assignment opens the module")
	}
	if _, err := expiry.ExpireDue(ctx, today); err != nil {
		t.Fatal(err)
	}
	if isOn() {
		t.Fatal("expired subscription must close the module")
	}
}

type requestRow struct {
	UUID             string `json:"uuid"`
	OrganizationUUID string `json:"organization_uuid"`
	ModuleKey        string `json:"module_key"`
	Status           string `json:"status"`
	Note             string `json:"note"`
	DecisionNote     string `json:"decision_note"`
	DecidedByName    string `json:"decided_by_name"`
}

func (it *itest) requestQueue(path, tok string) []requestRow {
	it.t.Helper()
	code, env := it.do("GET", path, hostOlex, tok, nil)
	if code != http.StatusOK {
		it.t.Fatalf("GET %s = %d %s", path, code, errCode(env))
	}
	var page struct {
		Items []requestRow `json:"items"`
		Total int64        `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		it.t.Fatal(err)
	}
	return page.Items
}

func findRequest(rows []requestRow, id string) (requestRow, bool) {
	for _, r := range rows {
		if r.UUID == id {
			return r, true
		}
	}
	return requestRow{}, false
}

func (it *itest) requestModule(tok, key, note string) string {
	it.t.Helper()
	code, env := it.do("POST", "/v1/features/"+key+"/request", hostOlex, tok, map[string]any{"note": note})
	if code != http.StatusAccepted {
		it.t.Fatalf("request %s = %d %s", key, code, errCode(env))
	}
	var out struct {
		Request struct {
			UUID   string `json:"uuid"`
			Status string `json:"status"`
		} `json:"request"`
	}
	_ = json.Unmarshal(env.Data, &out)
	if out.Request.Status != features.RequestPending || out.Request.UUID == "" {
		it.t.Fatalf("request = %+v", out.Request)
	}
	return out.Request.UUID
}

// Chain 5: a dealer requests a module, the distributor approves it and the
// module is on; the request is stored and the requester is notified.
func TestIntegrationF5ChainModuleRequestApproved(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	if err := notifusecase.SyncCatalog(ctx, it.q); err != nil {
		t.Fatalf("notification catalog: %v", err)
	}
	key := features.ModuleStockForecast
	net := it.featureNet(1)
	dealer := net.dealers[0]

	id := it.requestModule(net.dealerTok, key, "please")
	if again := it.requestModule(net.dealerTok, key, "please, again"); again != id {
		t.Fatalf("second request = %s, want the open one %s", again, id)
	}
	rows := it.requestQueue("/v1/tenant/modules/requests?status=pending", net.distTok)
	r, ok := findRequest(rows, id)
	if !ok || r.OrganizationUUID != dealer.Uuid.String() || r.Note != "please, again" {
		t.Fatalf("distributor queue = %+v", rows)
	}
	// Another distributor neither sees nor decides it.
	otherOwner, opw := it.user("chain-other-owner")
	it.member(net.other, otherOwner, "owner")
	otherTok := it.loginOrg(otherOwner, opw, net.other)
	if _, ok := findRequest(it.requestQueue("/v1/tenant/modules/requests", otherTok), id); ok {
		t.Fatal("foreign distributor sees the request")
	}
	if code, ec := it.status("POST", "/v1/tenant/modules/requests/"+id+"/approve", otherTok); code != http.StatusNotFound {
		t.Fatalf("foreign approve = %d %s, want 404", code, ec)
	}
	// The distributor does not have the module itself: ask upstream first.
	code, env := it.do("POST", "/v1/tenant/modules/requests/"+id+"/approve", hostOlex, net.distTok, map[string]any{})
	if code != http.StatusConflict || errCode(env) != response.CodeModuleBlockedByParent {
		t.Fatalf("approve under closed distributor = %d %s, want 409 MODULE_BLOCKED_BY_PARENT", code, errCode(env))
	}
	it.adminSet(net.admin, net.dist, key, true)
	code, env = it.do("POST", "/v1/tenant/modules/requests/"+id+"/approve", hostOlex, net.distTok, map[string]any{"note": "ok"})
	if code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, errCode(env))
	}
	st, ok := it.featureMeta(net.dealerTok, "").item(key)
	if !ok || !st.Enabled || st.Source != features.SourceDistributor || st.Request == nil || st.Request.Status != features.RequestApproved {
		t.Fatalf("dealer after approval = %+v request=%+v", st.State, st.Request)
	}
	it.notGated("GET", "/v1/stock-forecasts", net.dealerTok)
	if code, env := it.do("POST", "/v1/tenant/modules/requests/"+id+"/reject", hostOlex, net.distTok, map[string]any{}); code != http.StatusConflict {
		t.Fatalf("decide twice = %d %s, want 409", code, errCode(env))
	}
	if r, _ := findRequest(it.requestQueue("/v1/tenant/modules/requests?status=approved&module_key="+key, net.distTok), id); r.DecisionNote != "ok" || r.DecidedByName == "" {
		t.Fatalf("approved row = %+v", r)
	}
	var status string
	if err := it.pool.QueryRow(ctx, `
		SELECT d.status FROM notification_deliveries d
		JOIN organization_members om ON om.user_id = d.user_id AND om.organization_id = $1
		WHERE d.event_code = 'features.module_request_decided' AND d.channel = 'inapp'
		ORDER BY d.id DESC LIMIT 1`, dealer.ID).Scan(&status); err != nil {
		t.Fatalf("decision notification: %v", err)
	}
	if status != "delivered" {
		t.Fatalf("decision delivery = %s", status)
	}
}

// Requests: reject, cancel, validation, the platform queue (distributor
// requests) and automatic approval when the module opens another way.
func TestIntegrationModuleRequestsRejectCancelPlatformAndAuto(t *testing.T) {
	it := newIntegration(t)
	net := it.featureNet(1)

	// Reject.
	rej := it.requestModule(net.dealerTok, features.ModuleEfficiency, "")
	code, env := it.do("POST", "/v1/tenant/modules/requests/"+rej+"/reject", hostOlex, net.distTok, map[string]any{"note": "not now"})
	if code != http.StatusOK {
		t.Fatalf("reject = %d %s", code, errCode(env))
	}
	if on, _ := it.srv.features.Enabled(context.Background(), net.dealers[0].ID, features.ModuleEfficiency); on {
		t.Fatal("rejected module opened")
	}
	// Cancel by the requester; nothing pending afterwards.
	it.requestModule(net.dealerTok, features.ModuleCertificates, "")
	if code, env := it.do("DELETE", "/v1/features/"+features.ModuleCertificates+"/request", hostOlex, net.dealerTok, nil); code != http.StatusOK {
		t.Fatalf("cancel = %d %s", code, errCode(env))
	}
	if code, _ := it.do("DELETE", "/v1/features/"+features.ModuleCertificates+"/request", hostOlex, net.dealerTok, nil); code != http.StatusNotFound {
		t.Fatalf("cancel twice = %d, want 404", code)
	}
	// Core modules and modules already on cannot be requested.
	if code, env := it.do("POST", "/v1/features/"+features.ModuleServices+"/request", hostOlex, net.dealerTok, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("core request = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/features/"+features.ModuleLeads+"/request", hostOlex, net.dealerTok, nil); code != http.StatusConflict {
		t.Fatalf("enabled request = %d %s, want 409", code, errCode(env))
	}
	// List contract: unknown sort / status → 400.
	if code, _ := it.status("GET", "/v1/tenant/modules/requests?sort=bogus", net.distTok); code != http.StatusBadRequest {
		t.Fatalf("bad sort = %d", code)
	}
	if code, _ := it.status("GET", "/v1/tenant/modules/requests?status=bogus", net.distTok); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d", code)
	}
	// A dealer owner cannot read the queue.
	if code, _ := it.status("GET", "/v1/tenant/modules/requests", net.dealerTok); code != http.StatusForbidden {
		t.Fatalf("dealer queue = %d, want 403", code)
	}

	// Platform queue: the distributor's own request goes to the center.
	distReq := it.requestModule(net.distTok, features.ModuleMCP, "for our dealers")
	if _, ok := findRequest(it.requestQueue("/v1/tenant/modules/requests", net.distTok), distReq); ok {
		t.Fatal("a distributor's own request is not in its dealer queue")
	}
	if _, ok := findRequest(it.requestQueue("/v1/platform/modules/requests?q="+strings.ReplaceAll(net.dist.Name, " ", "%20"), net.admin), distReq); !ok {
		t.Fatal("platform queue misses the distributor's request")
	}
	if code, env := it.do("POST", "/v1/platform/modules/requests/"+distReq+"/approve", hostOlex, net.admin, map[string]any{}); code != http.StatusOK {
		t.Fatalf("platform approve = %d %s", code, errCode(env))
	}
	if !hasKey(it.featureMeta(net.distTok, "").Enabled, features.ModuleMCP) {
		t.Fatal("platform approval must open the module")
	}
	if code, _ := it.status("GET", "/v1/platform/modules/requests", net.distTok); code != http.StatusForbidden {
		t.Fatalf("distributor platform queue = %d, want 403", code)
	}

	// Automatic approval: the distributor opens its dealer standard.
	key := features.ModuleDealerShowcase
	it.adminSet(net.admin, net.dist, key, true)
	auto := it.requestModule(net.dealerTok, key, "")
	code, env = it.do("PUT", "/v1/tenant/modules/dealer-standard/"+key, hostOlex, net.distTok, map[string]any{"enabled": true})
	if code != http.StatusOK {
		t.Fatalf("standard = %d %s", code, errCode(env))
	}
	r, _ := findRequest(it.requestQueue("/v1/tenant/modules/requests?module_key="+key, net.distTok), auto)
	if r.Status != features.RequestApproved || r.DecisionNote != features.AutoDecisionNote || r.DecidedByName != "" {
		t.Fatalf("auto-approved row = %+v", r)
	}
}

// A module bundle that opens a requested module approves the request.
func TestIntegrationModuleRequestAutoApprovedBySubscription(t *testing.T) {
	it := newIntegration(t)
	net := it.featureNet(0)
	center := it.brandCenter("olex")
	staff, spw := it.user("auto-center-staff")
	it.member(center, staff, "staff")
	centerTok := it.loginOrg(staff, spw, center)
	key := features.ModuleWhatsApp
	id := it.requestModule(net.distTok, key, "")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	it.assignBundle(centerTok, it.bundleItem("WA bundle", "10.00", key), net.dist, today, today.AddDate(0, 1, 0))
	r, _ := findRequest(it.requestQueue("/v1/platform/modules/requests?module_key="+key, net.admin), id)
	if r.Status != features.RequestApproved || r.DecisionNote != features.AutoDecisionNote {
		t.Fatalf("request after subscription = %+v", r)
	}
}

// GET /v1/features: a description in all 13 locales, bundle price with the
// distributor override, free_default and contact_for_price.
func TestIntegrationFeaturesMetaDescriptionsAndPrice(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	net := it.featureNet(1)
	center := it.brandCenter("olex")
	bundled := func(key string) bool {
		var ok bool
		if err := it.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM service_catalog_modules m
			JOIN service_catalog_items i ON i.id = m.item_id
			WHERE m.module_key = $1 AND i.is_active AND i.brand_id = $2)`, key, center.BrandID).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	// Other packages' tests may leave bundles behind: use paid add-ons that
	// no active bundle contains.
	var free []string
	for _, k := range []string{features.ModuleEInvoice, features.ModuleShortURL, features.ModuleCampaigns,
		features.ModuleMCP, features.ModuleAIAssistant, features.ModuleEfficiency} {
		if !bundled(k) {
			free = append(free, k)
		}
	}
	if len(free) < 2 {
		t.Skipf("every candidate add-on is in a bundle: %v", free)
	}
	priced, unpriced := free[0], free[1]
	item := it.bundleItem("Priced bundle", "120.00", priced)
	var ov pgtype.Numeric
	_ = ov.Scan("99.50")
	if _, err := it.q.UpsertServicePriceOverride(ctx, db.UpsertServicePriceOverrideParams{
		ItemID: item.ID, OrganizationID: net.dist.ID, BrandID: item.BrandID, Price: ov, Currency: "TRY",
	}); err != nil {
		t.Fatal(err)
	}
	it.adminSet(net.admin, net.dist, priced, true)
	it.adminSet(net.admin, net.dist, unpriced, true)

	en := it.featureMeta(net.dealerTok, "en")
	for _, l := range i18n.Supported {
		p := it.featureMeta(net.dealerTok, string(l))
		for i, fi := range p.Items {
			if strings.TrimSpace(fi.Description) == "" {
				t.Errorf("%s %s: empty description", l, fi.Key)
			}
			if l != i18n.LocaleEN && fi.Description == en.Items[i].Description {
				t.Errorf("%s %s: description is the en text", l, fi.Key)
			}
		}
	}

	// The dealer sees its distributor's override of the bundle price.
	pr, ok := en.item(priced)
	if !ok || pr.Price == nil || pr.Price.Amount != "99.50" || pr.Price.Currency != "TRY" ||
		pr.Price.Recurrence != "monthly" || pr.ContactForPrice || pr.FreeDefault {
		t.Fatalf("%s price = %+v contact=%v", priced, pr.Price, pr.ContactForPrice)
	}
	up, ok := en.item(unpriced)
	if !ok || up.Price != nil || !up.ContactForPrice || up.FreeDefault {
		t.Fatalf("%s price = %+v contact=%v", unpriced, up.Price, up.ContactForPrice)
	}
	leads, _ := en.item(features.ModuleLeads)
	if !leads.FreeDefault || leads.ContactForPrice {
		t.Fatalf("leads free_default=%v contact=%v", leads.FreeDefault, leads.ContactForPrice)
	}
}
