package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TEC-403 (F4-03d): an MCP write tool waits on the panel screen "pending
// AI actions"; the list follows the list contract (source, q, sort), chat
// cards are not listed there, confirm runs the action once and a second
// decision is AI_ACTION_RESOLVED.
func TestIntegrationAIPendingActionsScreen(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	ts := httptest.NewServer(it.handler)
	t.Cleanup(ts.Close)

	center := it.brandCenter("olex")
	dealer := it.org("t403-dealer", rbac.OrgTypeDealer, center)
	if _, err := it.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
		Scope: "org", OrganizationID: pgtype.Int8{Int64: dealer.ID, Valid: true},
		ModuleKey: features.ModuleMCP, Enabled: true, Source: "admin",
	}); err != nil {
		t.Fatalf("enable mcp: %v", err)
	}
	owner, pw := it.user("t403-owner")
	it.member(dealer, owner, "owner")

	tok, _ := it.mcpToken(owner, dealer, "/mcp/dealer")
	cs, err := mcpConnect(ctx, ts, "/mcp/dealer", tok)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = cs.Close() }()
	contact := "T403 " + it.suffix
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
	// A chat card of the same user is decided in its conversation only.
	chat, err := it.q.CreateAIPendingAction(ctx, db.CreateAIPendingActionParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, UserID: owner.ID, Source: aimodel.SourcePanel,
		SourceRef: pgtype.Text{String: uuid.NewString(), Valid: true}, ToolUseID: "toolu_t403", ToolName: aitools.ToolCreateLead,
		Input: []byte(`{}`), Preview: []byte(`{}`), IdempotencyKey: "t403-" + it.suffix,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("chat card: %v", err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM ai_pending_actions WHERE user_id = $1`, owner.ID)
	})

	ptok := it.loginOrg(owner, pw, dealer)
	type page struct {
		Items []struct {
			ActionUUID uuid.UUID `json:"action_uuid"`
			Source     string    `json:"source"`
			ToolName   string    `json:"tool_name"`
			Preview    struct {
				Summary string `json:"summary"`
			} `json:"preview"`
		} `json:"items"`
		Total int `json:"total"`
	}
	list := func(path string) (int, page, envelope) {
		code, env := it.do("GET", path, hostOlex, ptok, nil)
		var p page
		_ = json.Unmarshal(env.Data, &p)
		return code, p, env
	}
	code, p, env := list("/v1/ai/pending-actions?source=mcp&sort=-expires_at&q=lead")
	if code != http.StatusOK || p.Total != 1 || len(p.Items) != 1 || p.Items[0].ActionUUID != proposed.ActionUUID ||
		p.Items[0].Source != aimodel.SourceMCP || p.Items[0].Preview.Summary == "" {
		t.Fatalf("list = %d %+v %+v", code, p, env.Error)
	}
	if code, p, _ := list("/v1/ai/pending-actions?source=whatsapp"); code != http.StatusOK || p.Total != 0 {
		t.Fatalf("whatsapp filter = %d %+v", code, p)
	}
	for _, bad := range []string{"?source=panel", "?sort=input"} {
		if code, _, env := list("/v1/ai/pending-actions" + bad); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("%s = %d %s", bad, code, errCode(env))
		}
	}
	if code, env := it.do("POST", "/v1/ai/pending-actions/"+chat.Uuid.String()+"/confirm", hostOlex, ptok, nil); code != http.StatusNotFound {
		t.Fatalf("chat card confirm = %d %s", code, errCode(env))
	}

	code, env = it.do("POST", "/v1/ai/pending-actions/"+proposed.ActionUUID.String()+"/confirm", hostOlex, ptok, nil)
	var out struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &out)
	if code != http.StatusOK || out.Status != aimodel.ActionConfirmed {
		t.Fatalf("confirm = %d %s %+v", code, env.Data, env.Error)
	}
	var leads int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM leads WHERE organization_id = $1 AND candidate_contact_name = $2`,
		dealer.ID, contact).Scan(&leads); err != nil || leads != 1 {
		t.Fatalf("leads after confirm = %d %v", leads, err)
	}
	if code, env := it.do("POST", "/v1/ai/pending-actions/"+proposed.ActionUUID.String()+"/cancel", hostOlex, ptok, nil); code != http.StatusConflict ||
		errCode(env) != "AI_ACTION_RESOLVED" {
		t.Fatalf("second decision = %d %s", code, errCode(env))
	}
	if code, p, _ := list("/v1/ai/pending-actions"); code != http.StatusOK || p.Total != 0 {
		t.Fatalf("list after confirm = %d %+v", code, p)
	}

}
