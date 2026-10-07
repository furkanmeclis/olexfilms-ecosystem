package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// aiToolAnswer is what the test route answers: the available tool names
// and, for a call, the tool result.
type aiToolAnswer struct {
	Available  []string `json:"available"`
	Content    string   `json:"content"`
	IsError    bool     `json:"is_error"`
	Code       string   `json:"code"`
	NotAllowed bool     `json:"not_allowed"`
}

// mountAITools mounts POST /v1/tenant/_test/ai-tools/{name} behind the real
// auth and organization chain: the handler builds the panel principal the
// chat will build (F4-01f) and runs Registry.Available and Registry.Call.
// The X-Test-Grant header adds "slug:scope" grants to the loaded principal
// (a role variant the seeded catalog does not have).
func (it *itest) mountAITools() {
	authn := middleware.Authenticate(it.srv.tokens, it.srv.loader)
	org := middleware.RequireOrganization(it.srv.tokens, it.q)
	it.srv.mux.Handle("POST /v1/tenant/_test/ai-tools/{name}", middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := authctx.MustPrincipal(r.Context())
			if g := r.Header.Get("X-Test-Grant"); g != "" {
				scopes := map[string]rbac.Scope{}
				for k, v := range p.PermissionScopes {
					scopes[k] = v
				}
				slug, sc := rbac.ParseGrant(g)
				scopes[slug] = sc
				p.PermissionScopes = scopes
				p.Permissions = append(append([]string{}, p.Permissions...), slug)
			}
			scope := orgctx.MustScope(r.Context())
			tp := aitools.Principal{Auth: p, Org: &scope, Realm: aitools.RealmPanel}
			ctx := r.Context()
			var out aiToolAnswer
			avail, err := it.srv.aiTools.Available(ctx, tp)
			if err != nil {
				response.InternalErr(w, r, err, "available")
				return
			}
			for _, t := range avail {
				out.Available = append(out.Available, t.Spec().Name)
			}
			if name := r.PathValue("name"); name != "_list" {
				body, _ := io.ReadAll(r.Body)
				res, err := it.srv.aiTools.Call(ctx, tp, name, body)
				if err != nil && !errors.Is(err, aitools.ErrToolNotAllowed) {
					response.InternalErr(w, r, err, "call")
					return
				}
				out.Content, out.IsError, out.Code = res.Content, res.IsError, res.Code
				out.NotAllowed = errors.Is(err, aitools.ErrToolNotAllowed)
			}
			response.JSON(w, r, http.StatusOK, out)
		}), authn, org))
}

func (it *itest) aiTool(tok, name, input string, grant ...string) aiToolAnswer {
	it.t.Helper()
	req := httptest.NewRequest("POST", "/v1/tenant/_test/ai-tools/"+name, strings.NewReader(input))
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	if len(grant) > 0 {
		req.Header.Set("X-Test-Grant", grant[0])
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if rec.Code != http.StatusOK {
		it.t.Fatalf("ai tool %s = %d %s", name, rec.Code, rec.Body.String())
	}
	var out aiToolAnswer
	if err := json.Unmarshal(env.Data, &out); err != nil {
		it.t.Fatalf("ai tool %s: %v", name, err)
	}
	return out
}

func hasTool(a aiToolAnswer, name string) bool {
	for _, n := range a.Available {
		if n == name {
			return true
		}
	}
	return false
}

// TEC-385 acceptance (F4-01c) on the real use cases and principals:
//  1. A dealer cannot find another organization's service by plate; the
//     distributor finds its subtree dealer's service.
//  2. Without pricing.purchase.read the stock tool output has no purchase
//     price; with it (dealer owner) the price is there.
//  3. On the Olex domain a Glorian product never comes back.
//  4. A module switched off for the organization removes its tool from
//     Available and a call answers TOOL_NOT_ALLOWED.
func TestIntegrationAIToolsScope(t *testing.T) {
	it := newIntegration(t)
	it.mountAITools()
	ctx := context.Background()
	sfx := strings.ToUpper(it.suffix[len(it.suffix)-6:])

	center := it.brandCenter("olex")
	dist := it.org("t385-dist", "distributor", center)
	dealerA := it.org("t385-a", "dealer", dist)
	dealerB := it.org("t385-b", "dealer", dist)
	otherDist := it.org("t385-dist2", "distributor", center)
	dealerC := it.org("t385-c", "dealer", otherDist)

	p := it.product(center, "T385")
	plateA, plateB, plateC := "34A"+sfx, "06B"+sfx, "35C"+sfx
	service := func(org db.Organization, name, plate string, seq int) db.Service {
		cust, veh := it.svcCustomer(org, name, plate)
		u := it.stockChain().unit(center, p, 38500+seq)
		svc := it.directService(org, cust, veh, 385000+seq,
			db.CreateServiceItemParams{ProductID: p.ID, UnitID: u.ID, Kind: "full"})
		if _, err := it.pool.Exec(ctx, `UPDATE services SET plate = $1, plate_country = 'TR' WHERE id = $2`, plate, svc.ID); err != nil {
			t.Fatal(err)
		}
		return svc
	}
	svcA := service(dealerA, "t385-cust-a", plateA, 1)
	service(dealerB, "t385-cust-b", plateB, 2)
	service(dealerC, "t385-cust-c", plateC, 3)

	login := func(org db.Organization, name, role string, roles ...string) string {
		u, pw := it.user(name)
		it.member(org, u, role, roles...)
		return it.loginOrg(u, pw, org)
	}
	tokA := login(dealerA, "t385-owner-a", "owner")
	tokAStaff := login(dealerA, "t385-staff-a", "staff")
	tokDist := login(dist, "t385-dist-owner", "owner")

	found := func(a aiToolAnswer, plate string) bool {
		return !a.IsError && strings.Contains(a.Content, plate)
	}
	search := func(tok, plate string) aiToolAnswer {
		return it.aiTool(tok, "search_services", `{"query":"`+plate+`"}`)
	}

	// 1. Dealer A: its own service, never the sibling's or another
	// distributor's dealer's.
	if a := search(tokA, plateA); !found(a, plateA) || !strings.Contains(a.Content, svcA.ServiceNo) {
		t.Fatalf("dealer A own plate: %+v", a)
	}
	for _, plate := range []string{plateB, plateC} {
		if a := search(tokA, plate); a.IsError || strings.Contains(a.Content, plate) || !strings.Contains(a.Content, `"total":0`) {
			t.Fatalf("dealer A found another organization's service %s: %+v", plate, a)
		}
	}
	// A spaced plate is matched too ("34 A..." -> "34A...").
	if a := search(tokA, "34 A"+sfx); !found(a, plateA) {
		t.Fatalf("dealer A spaced plate: %+v", a)
	}
	// The service detail by number: own yes, a sibling's no.
	if a := it.aiTool(tokA, "get_service", `{"service":"`+svcA.ServiceNo+`"}`); a.IsError || !strings.Contains(a.Content, plateA) {
		t.Fatalf("dealer A get_service: %+v", a)
	}
	// The distributor sees its subtree, not the other distributor's dealer.
	if a := search(tokDist, plateA); !found(a, plateA) {
		t.Fatalf("distributor subtree plate A: %+v", a)
	}
	if a := search(tokDist, plateB); !found(a, plateB) {
		t.Fatalf("distributor subtree plate B: %+v", a)
	}
	if a := search(tokDist, plateC); strings.Contains(a.Content, plateC) {
		t.Fatalf("distributor found another subtree's service: %+v", a)
	}
	if a := it.aiTool(tokDist, "get_service", `{"service":"`+svcA.Uuid.String()+`"}`); a.IsError {
		t.Fatalf("distributor get_service on subtree: %+v", a)
	}
	// Activity summary (legacy dealer/* parity) counts only the scope.
	if a := it.aiTool(tokA, "service_activity_summary", `{}`); a.IsError || !strings.Contains(a.Content, `"completed_count":1`) {
		t.Fatalf("dealer A activity: %+v", a)
	}
	if a := it.aiTool(tokDist, "service_activity_summary", `{}`); a.IsError || !strings.Contains(a.Content, `"completed_count":2`) {
		t.Fatalf("distributor activity: %+v", a)
	}

	// 2. Purchase price. K8: the dealer pays the distributor's dealer price.
	it.setListPrice(p, "TRY", "100")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: p.ID, BrandID: p.BrandID, DistributorOrgID: dist.ID, Currency: "TRY", Price: "120",
	}); err != nil {
		t.Fatal(err)
	}
	c := it.stockChain()
	cLoc := c.location(center, "C")
	dLoc := c.location(dist, "D")
	held := c.unit(center, p, 38599)
	c.post(ledger.TypeEntry, held, c.nextRef(), cLoc)
	c.ship(held, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	c.ship(held, ledger.TypeOrderOut, ledger.TypeReceived, dealerA,
		ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID})
	units := `{"product_uuid":"` + p.Uuid.String() + `"}`

	owner := it.aiTool(tokA, "stock_units", units)
	if owner.IsError || !strings.Contains(owner.Content, held.Barcode) || !strings.Contains(owner.Content, `"purchase_price":{"amount":"120`) {
		t.Fatalf("dealer owner stock units: %+v", owner)
	}
	// The seeded dealer staff has neither stock.read nor the price: no
	// stock tool at all.
	staff := it.aiTool(tokAStaff, "stock_units", units)
	if hasTool(staff, "stock_units") || !staff.NotAllowed || staff.Code != aitools.CodeToolNotAllowed {
		t.Fatalf("dealer staff stock_units: %+v", staff)
	}
	// A dealer staff granted stock.read (still without
	// pricing.purchase.read) sees the unit but no purchase price.
	staff = it.aiTool(tokAStaff, "stock_units", units, "stock.read:managed")
	if staff.IsError || !strings.Contains(staff.Content, held.Barcode) || strings.Contains(staff.Content, "purchase_price") {
		t.Fatalf("dealer staff with stock.read: %+v", staff)
	}

	// 3. Brand (K20): a Glorian product with a unique name never shows on
	// the Olex domain; the Olex product does.
	gp := it.product(it.brandCenter("glorian"), "T385G")
	marker := "Glorian385" + sfx
	if _, err := it.pool.Exec(ctx, `UPDATE products SET name = $1 WHERE id = $2`, marker, gp.ID); err != nil {
		t.Fatal(err)
	}
	if a := it.aiTool(tokA, "search_products", `{"query":"`+marker+`"}`); a.IsError || strings.Contains(a.Content, marker) || strings.Contains(a.Content, gp.Sku) {
		t.Fatalf("Olex domain returned a Glorian product: %+v", a)
	}
	if a := it.aiTool(tokA, "search_products", `{"query":"`+p.Sku+`"}`); a.IsError || !strings.Contains(a.Content, p.Sku) {
		t.Fatalf("Olex product search: %+v", a)
	}

	// 4. Leads (standard module) switched off for dealer A only.
	if a := it.aiTool(tokA, "_list", ""); !hasTool(a, "list_leads") {
		t.Fatalf("list_leads before switch-off: %v", a.Available)
	}
	admin, _ := it.user("t385-admin", rbac.RoleSuperAdmin)
	if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, dealerA.ID, features.ModuleLeads, false); err != nil {
		t.Fatalf("disable leads: %v", err)
	}
	a := it.aiTool(tokA, "list_leads", `{}`)
	if hasTool(a, "list_leads") || !a.NotAllowed || !a.IsError || a.Code != aitools.CodeToolNotAllowed ||
		!strings.Contains(a.Content, "TOOL_NOT_ALLOWED") {
		t.Fatalf("list_leads with the module off: %+v", a)
	}
	if !hasTool(a, "search_services") {
		t.Fatalf("other tools must stay: %v", a.Available)
	}
	// The sibling dealer keeps the module and the tool.
	tokB := login(dealerB, "t385-owner-b", "owner")
	if b := it.aiTool(tokB, "list_leads", `{}`); !hasTool(b, "list_leads") || b.IsError {
		t.Fatalf("dealer B list_leads: %+v", b)
	}
	// Center-only and distributor tools are not offered to a dealer.
	if hasTool(a, "my_tasks") || hasTool(a, "list_sub_organizations") {
		t.Fatalf("dealer got center/distributor tools: %v", a.Available)
	}
	if d := it.aiTool(tokDist, "list_sub_organizations", `{}`); d.IsError || !strings.Contains(d.Content, dealerA.Slug) ||
		!strings.Contains(d.Content, dealerB.Slug) || strings.Contains(d.Content, dealerC.Slug) {
		t.Fatalf("distributor sub organizations: %+v", d)
	}
}
