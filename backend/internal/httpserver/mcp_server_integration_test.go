package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	mcpmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/mcp"
	oauthusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpToken stores a live MCP access token of the user bound to org for
// the endpoint (as the TEC-400 token endpoint would issue it) and returns
// the raw token and its family.
func (it *itest) mcpToken(u db.User, org db.Organization, resource string) (string, uuid.UUID) {
	it.t.Helper()
	ctx := context.Background()
	clientID := "tec402-" + uuid.NewString()
	if _, err := it.q.CreateOAuthClient(ctx, db.CreateOAuthClientParams{
		ClientID: clientID, ClientName: "TEC402 test", RedirectUris: []string{"https://claude.ai/api/mcp/auth_callback"},
	}); err != nil {
		it.t.Fatalf("oauth client: %v", err)
	}
	it.t.Cleanup(func() {
		ctx := context.Background()
		_, _ = it.pool.Exec(ctx, `DELETE FROM oauth_tokens WHERE client_id = $1`, clientID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM oauth_grants WHERE client_id = $1`, clientID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM oauth_clients WHERE client_id = $1`, clientID)
	})
	g, err := it.q.UpsertOAuthGrant(ctx, db.UpsertOAuthGrantParams{
		UserID: u.ID, ClientID: clientID, OrganizationID: org.ID, BrandID: org.BrandID, Resource: resource, Scopes: []string{"mcp"},
	})
	if err != nil {
		it.t.Fatalf("oauth grant: %v", err)
	}
	raw, family := "tec402-"+uuid.NewString()+uuid.NewString(), uuid.New()
	if err := it.q.CreateOAuthToken(ctx, db.CreateOAuthTokenParams{
		TokenHash: oauthusecase.HashToken(raw), Kind: "access", Family: family, GrantID: g.ID, ClientID: clientID,
		UserID: u.ID, OrganizationID: org.ID, BrandID: org.BrandID, Resource: resource, Scopes: []string{"mcp"},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		it.t.Fatalf("oauth token: %v", err)
	}
	return raw, family
}

// mcpTransport adds the Bearer token and the brand host to every request
// of the go-sdk client.
type mcpTransport struct{ token string }

func (m mcpTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if m.token != "" {
		r.Header.Set("Authorization", "Bearer "+m.token)
	}
	r.Header.Set("X-Forwarded-Host", hostOlex)
	return http.DefaultTransport.RoundTrip(r)
}

func mcpConnect(ctx context.Context, ts *httptest.Server, path, token string) (*mcpsdk.ClientSession, error) {
	c := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "tec402-test", Version: "1"}, nil)
	return c.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: ts.URL + path, HTTPClient: &http.Client{Transport: mcpTransport{token: token}},
		MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
}

func toolNames(t *testing.T, ctx context.Context, cs *mcpsdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	out := make([]string, 0, len(res.Tools))
	for _, tl := range res.Tools {
		out = append(out, tl.Name)
	}
	return out
}

func resultText(r *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// TEC-402 acceptance (F4-03c) with the go-sdk client against the full
// server: no token → 401 + WWW-Authenticate; a dealer token never lists a
// customer tool; a customer token returns only the own service; a write
// tool creates a pending action (source mcp) and writes nothing; the
// organization limit answers 429 with a JSON-RPC error; with the mcp
// module switched off the next call is 403.
func TestIntegrationMCPServer(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	ts := httptest.NewServer(it.handler)
	t.Cleanup(ts.Close)
	sfx := strings.ToUpper(it.suffix[len(it.suffix)-6:])

	center := it.brandCenter("olex")
	dealer := it.org("t402-dealer", rbac.OrgTypeDealer, center)
	for _, o := range []db.Organization{dealer, center} {
		if _, err := it.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
			Scope: "org", OrganizationID: pgtype.Int8{Int64: o.ID, Valid: true},
			ModuleKey: features.ModuleMCP, Enabled: true, Source: "admin",
		}); err != nil {
			t.Fatalf("enable mcp: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM module_flags WHERE scope = 'org' AND organization_id = $1 AND module_key = $2`,
			center.ID, features.ModuleMCP)
	})
	owner, _ := it.user("t402-owner")
	it.member(dealer, owner, "owner")

	// Two customers of the dealer, one service each.
	p := it.product(center, "T402")
	plateA, plateB := "34MA"+sfx, "34MB"+sfx
	service := func(name, plate string, seq int) (db.User, db.Service) {
		cust, veh := it.svcCustomer(dealer, name, plate)
		u := it.stockChain().unit(center, p, 40200+seq)
		svc := it.directService(dealer, cust, veh, 402000+seq,
			db.CreateServiceItemParams{ProductID: p.ID, UnitID: u.ID, Kind: "full"})
		if _, err := it.pool.Exec(ctx, `UPDATE services SET plate = $1, plate_country = 'TR' WHERE id = $2`, plate, svc.ID); err != nil {
			t.Fatal(err)
		}
		return cust, svc
	}
	custA, _ := service("t402-cust-a", plateA, 1)
	_, svcB := service("t402-cust-b", plateB, 2)

	dealerTok, dealerFamily := it.mcpToken(owner, dealer, "/mcp/dealer")
	customerTok, _ := it.mcpToken(custA, center, "/mcp/customer")

	// 1. No token: 401 with the RFC 9728 challenge; the SDK client fails.
	rec := it.raw("POST", "/mcp/dealer", "", "application/json",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), nil)
	const metadata = `resource_metadata="http://localhost:3000/.well-known/oauth-protected-resource/mcp/dealer"`
	if rec.Code != http.StatusUnauthorized || !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Bearer ") ||
		!strings.Contains(rec.Header().Get("WWW-Authenticate"), metadata) {
		t.Fatalf("no token = %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	if _, err := mcpConnect(ctx, ts, "/mcp/dealer", ""); err == nil {
		t.Fatal("client connected without a token")
	}
	// A token of another endpoint (audience) or an unknown token: 401 invalid_token.
	for _, c := range []struct{ path, tok string }{{"/mcp/customer", dealerTok}, {"/mcp/dealer", "nope"}} {
		rec := it.raw("POST", c.path, c.tok, "application/json", []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
			t.Fatalf("%s with a foreign token = %d %q", c.path, rec.Code, rec.Header().Get("WWW-Authenticate"))
		}
	}

	// 2. Dealer token: panel tools only, never a customer tool.
	dealerCS, err := mcpConnect(ctx, ts, "/mcp/dealer", dealerTok)
	if err != nil {
		t.Fatalf("dealer connect: %v", err)
	}
	defer func() { _ = dealerCS.Close() }()
	if caps := dealerCS.InitializeResult().Capabilities; caps.Tools == nil || caps.Tools.ListChanged {
		t.Fatalf("tools capability = %+v, want listChanged false", caps.Tools)
	}
	names := toolNames(t, ctx, dealerCS)
	if !slices.Contains(names, "list_leads") || !slices.Contains(names, aitools.ToolCreateLead) {
		t.Fatalf("dealer tools lack panel tools: %v", names)
	}
	for _, n := range []string{"my_services", "my_service_detail", "my_vehicles"} {
		if slices.Contains(names, n) {
			t.Fatalf("dealer token lists customer tool %s: %v", n, names)
		}
	}
	// Calling a customer tool anyway is refused like the registry does.
	res, err := dealerCS.CallTool(ctx, &mcpsdk.CallToolParams{Name: "my_services", Arguments: map[string]any{}})
	if err != nil || !res.IsError || !strings.Contains(resultText(res), aitools.CodeToolNotAllowed) {
		t.Fatalf("dealer my_services = %+v %v", res, err)
	}

	// 3. Customer token: customer tools, only the own service.
	custCS, err := mcpConnect(ctx, ts, "/mcp/customer", customerTok)
	if err != nil {
		t.Fatalf("customer connect: %v", err)
	}
	defer func() { _ = custCS.Close() }()
	names = toolNames(t, ctx, custCS)
	if !slices.Contains(names, "my_services") || slices.Contains(names, "list_leads") || slices.Contains(names, aitools.ToolCreateLead) {
		t.Fatalf("customer tools: %v", names)
	}
	res, err = custCS.CallTool(ctx, &mcpsdk.CallToolParams{Name: "my_services", Arguments: map[string]any{}})
	if err != nil || res.IsError || !strings.Contains(resultText(res), plateA) || strings.Contains(resultText(res), plateB) {
		t.Fatalf("customer my_services = %+v %v", res, err)
	}
	if _, ok := res.StructuredContent.(map[string]any); !ok {
		t.Fatalf("structured content = %T", res.StructuredContent)
	}
	res, err = custCS.CallTool(ctx, &mcpsdk.CallToolParams{Name: "my_service_detail", Arguments: map[string]any{"service": svcB.ServiceNo}})
	if err != nil || !res.IsError || strings.Contains(resultText(res), plateB) {
		t.Fatalf("customer A read B's service: %+v %v", res, err)
	}

	// 4. Write tool: a pending action (source mcp), no lead. TEC-461: the
	// "waiting for approval" summary is in the user's language.
	if _, err := it.pool.Exec(ctx, `UPDATE users SET locale = 'tr' WHERE id = $1`, owner.ID); err != nil {
		t.Fatal(err)
	}
	contact := "T402 " + it.suffix
	res, err = dealerCS.CallTool(ctx, &mcpsdk.CallToolParams{Name: aitools.ToolCreateLead,
		Arguments: map[string]any{"contact_name": contact, "phone": "+905321234567"}})
	if err != nil || res.IsError || !strings.Contains(resultText(res), "/t/"+dealer.Slug+mcpmodule.ApprovalsPath+"?action=") {
		t.Fatalf("create_lead = %+v %v", res, err)
	}
	var pa struct {
		Status     string    `json:"status"`
		ActionUUID uuid.UUID `json:"action_uuid"`
		Summary    string    `json:"summary"`
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if json.Unmarshal(raw, &pa) != nil || pa.Status != mcpmodule.CodePendingApproval || pa.ActionUUID == uuid.Nil {
		t.Fatalf("create_lead structured = %s", raw)
	}
	if want := contact + " için lead oluşturulsun."; pa.Summary != want || !strings.Contains(resultText(res), want) {
		t.Fatalf("create_lead summary = %q, want %q", pa.Summary, want)
	}
	var source, status string
	var userID, orgID int64
	if err := it.pool.QueryRow(ctx, `SELECT source, status, user_id, organization_id FROM ai_pending_actions WHERE uuid = $1`,
		pa.ActionUUID).Scan(&source, &status, &userID, &orgID); err != nil {
		t.Fatalf("pending action: %v", err)
	}
	if source != aimodel.SourceMCP || status != aimodel.ActionPending || userID != owner.ID || orgID != dealer.ID {
		t.Fatalf("pending action = %s %s user %d org %d", source, status, userID, orgID)
	}
	var leads int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM leads WHERE organization_id = $1 AND candidate_contact_name = $2`,
		dealer.ID, contact).Scan(&leads); err != nil || leads != 0 {
		t.Fatalf("leads written by an MCP write tool: %d %v", leads, err)
	}
	// Activity log: tool, duration, outcome; personal arguments masked.
	var payload []byte
	if err := it.pool.QueryRow(ctx, `SELECT payload FROM activity_events WHERE action = $1 AND actor_user_id = $2
		AND payload->>'tool' = $3 ORDER BY id DESC LIMIT 1`, mcpmodule.ActionToolCalled, owner.ID, aitools.ToolCreateLead).Scan(&payload); err != nil {
		t.Fatalf("activity: %v", err)
	}
	var act struct {
		Result     string            `json:"result"`
		DurationMS *int64            `json:"duration_ms"`
		Arguments  map[string]string `json:"arguments"`
	}
	if json.Unmarshal(payload, &act) != nil || act.Result != mcpmodule.OutcomePendingApproval || act.DurationMS == nil ||
		act.Arguments["contact_name"] != mcpmodule.Masked || act.Arguments["phone"] != mcpmodule.Masked ||
		strings.Contains(string(payload), "905321234567") || strings.Contains(string(payload), contact) {
		t.Fatalf("activity payload = %s", payload)
	}

	// 5. Limits: the connection's 60/min, then the organization's hourly
	// limit (default 600) → 429 with a JSON-RPC error for the request id.
	limitKey := func(action, subject string) string { return fmt.Sprintf("app:test:rl:%s:%s", action, subject) }
	assert429 := func(what string) {
		t.Helper()
		rec := it.raw("POST", "/mcp/dealer", dealerTok, "application/json",
			[]byte(`{"jsonrpc":"2.0","id":42,"method":"tools/list"}`), map[string]string{"Accept": "application/json, text/event-stream"})
		var body struct {
			ID    int `json:"id"`
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusTooManyRequests || body.ID != 42 || body.Error.Code != mcpmodule.CodeRateLimited || rec.Header().Get("Retry-After") == "" {
			t.Fatalf("%s limit = %d %s", what, rec.Code, rec.Body)
		}
		if _, err := dealerCS.ListTools(ctx, nil); err == nil {
			t.Fatalf("%s limit: client call succeeded", what)
		}
	}
	if err := it.rdb.Set(ctx, limitKey("mcp_token", dealerFamily.String()), mcpmodule.TokenRequestsPerMinute, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	assert429("token")
	it.rdb.Del(ctx, limitKey("mcp_token", dealerFamily.String()))
	if err := it.rdb.Set(ctx, limitKey("mcp_org", fmt.Sprint(dealer.ID)), 600, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	assert429("organization")
	// Another organization's connection is not affected.
	if _, err := custCS.ListTools(ctx, nil); err != nil {
		t.Fatalf("customer after the dealer limit: %v", err)
	}
	it.rdb.Del(ctx, limitKey("mcp_org", fmt.Sprint(dealer.ID)))
	if _, err := dealerCS.ListTools(ctx, nil); err != nil {
		t.Fatalf("dealer after the window: %v", err)
	}

	// 6. The mcp module switched off: the next call is 403.
	admin, _ := it.user("t402-admin", rbac.RoleSuperAdmin)
	if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, dealer.ID, features.ModuleMCP, false); err != nil {
		t.Fatalf("disable mcp: %v", err)
	}
	if _, err := dealerCS.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_leads", Arguments: map[string]any{}}); err == nil {
		t.Fatal("call succeeded with the mcp module off")
	}
	rec = it.raw("POST", "/mcp/dealer", dealerTok, "application/json",
		[]byte(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}`), map[string]string{"Accept": "application/json, text/event-stream"})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), fmt.Sprint(mcpmodule.CodeForbidden)) {
		t.Fatalf("module off = %d %s", rec.Code, rec.Body)
	}
}
