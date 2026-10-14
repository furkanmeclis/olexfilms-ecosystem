package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/indexsync"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TEC-645: a lead the AI proposes over MCP and the user approves on the
// panel reaches the leads index through the outbox (leads.created), so
// list_leads(query=…) and the panel search find it; a panel edit refreshes
// the document through leads.updated.
func TestIntegrationLeadSearchIndexAfterAIApproval(t *testing.T) {
	finder := &lateFinder{}
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.SearchFinder = finder })
	ctx := context.Background()
	mem := newMemIndex(t, searchengine.NewRegistry(leadsusecase.NewSearchAdapter(it.q)))
	finder.m = mem
	ts := httptest.NewServer(it.handler)
	t.Cleanup(ts.Close)

	center := it.brandCenter("olex")
	dealer := it.org("t645-dealer", rbac.OrgTypeDealer, center)
	if _, err := it.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
		Scope: "org", OrganizationID: pgtype.Int8{Int64: dealer.ID, Valid: true},
		ModuleKey: features.ModuleMCP, Enabled: true, Source: "admin",
	}); err != nil {
		t.Fatalf("enable mcp: %v", err)
	}
	owner, pw := it.user("t645-owner")
	it.member(dealer, owner, "owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM ai_pending_actions WHERE user_id = $1`, owner.ID)
	})

	tok, _ := it.mcpToken(owner, dealer, "/mcp/dealer")
	cs, err := mcpConnect(ctx, ts, "/mcp/dealer", tok)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	// 1. create_lead over MCP, approved on the panel.
	token := "Bayi" + it.suffix[len(it.suffix)-6:]
	contact := "T645 " + token
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: aitools.ToolCreateLead,
		Arguments: map[string]any{"contact_name": contact, "phone": "+905321234567"}})
	if err != nil || res.IsError {
		t.Fatalf("create_lead = %+v %v", res, err)
	}
	var proposed struct {
		ActionUUID uuid.UUID `json:"action_uuid"`
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if json.Unmarshal(raw, &proposed) != nil || proposed.ActionUUID == uuid.Nil {
		t.Fatalf("structured = %s", raw)
	}
	ptok := it.loginOrg(owner, pw, dealer)
	if code, env := it.do("POST", "/v1/ai/pending-actions/"+proposed.ActionUUID.String()+"/confirm", hostOlex, ptok, nil); code != http.StatusOK {
		t.Fatalf("confirm = %d %s %+v", code, env.Data, env.Error)
	}
	var leadUUID uuid.UUID
	if err := it.pool.QueryRow(ctx, `SELECT uuid FROM leads WHERE organization_id = $1 AND candidate_contact_name = $2`,
		dealer.ID, contact).Scan(&leadUUID); err != nil {
		t.Fatalf("approved lead: %v", err)
	}

	panelSearch := func(q string) map[string]bool {
		t.Helper()
		code, env := it.do("GET", "/v1/leads?q="+url.QueryEscape(q), hostOlex, ptok, nil)
		if code != http.StatusOK {
			t.Fatalf("panel search = %d %+v", code, env.Error)
		}
		return listUUIDs(t, env)
	}
	mcpSearch := func(q string) string {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_leads", Arguments: map[string]any{"query": q}})
		if err != nil || res.IsError {
			t.Fatalf("list_leads = %+v %v", res, err)
		}
		return resultText(res)
	}
	// Before the outbox event reaches the bus the index does not know the
	// lead (what prod showed: the approved lead was never indexed).
	if got := panelSearch(token); got[leadUUID.String()] {
		t.Fatal("index answered before the outbox event")
	}

	// 2. The approval wrote leads.created; delivering it indexes the lead.
	sync := indexsync.New(it.q, mem, nil)
	deliver := func(name string) {
		t.Helper()
		for _, ev := range it.outboxEventsOf(name, leadUUID.String()) {
			if err := sync.HandleLead(ctx, ev); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	deliver(events.LeadsCreated)
	if got := panelSearch(token); !got[leadUUID.String()] || len(got) != 1 {
		t.Fatalf("panel search after leads.created = %v", got)
	}
	if text := mcpSearch(token); !strings.Contains(text, leadUUID.String()) {
		t.Fatalf("list_leads(query) after leads.created = %s", text)
	}
	if mem.queries[searchengine.SpecLeads] == 0 {
		t.Fatal("lead search did not query the index")
	}

	// 3. A panel edit writes leads.updated and refreshes the document.
	renamed := "T645 Yeni" + token
	if code, env := it.do("PATCH", "/v1/leads/"+leadUUID.String(), hostOlex, ptok,
		map[string]any{"candidate_contact_name": renamed}); code != http.StatusOK {
		t.Fatalf("patch = %d %+v", code, env.Error)
	}
	deliver(events.LeadsUpdated)
	if d, ok := mem.doc(searchengine.SpecLeads, leadUUID.String()); !ok || d.Title != renamed {
		t.Fatalf("lead doc after leads.updated = %+v (%v)", d, ok)
	}
}
