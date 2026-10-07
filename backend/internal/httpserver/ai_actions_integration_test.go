package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// mountAIActions mounts POST /v1/tenant/_test/ai-actions/{op} behind the
// real auth and organization chain. op is propose (body: tool, tool_use_id,
// input), confirm (body: action_uuid, edits) or cancel (action_uuid); the
// handler answers like the F4-01f chat endpoints will (ErrorStatus). The
// X-Test-Revoke header drops a permission from the loaded principal.
func (it *itest) mountAIActions() {
	authn := middleware.Authenticate(it.srv.tokens, it.srv.loader)
	org := middleware.RequireOrganization(it.srv.tokens, it.q)
	it.srv.mux.Handle("POST /v1/tenant/_test/ai-actions/{op}", middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := authctx.MustPrincipal(r.Context())
			if slug := r.Header.Get("X-Test-Revoke"); slug != "" {
				scopes := map[string]rbac.Scope{}
				for k, v := range p.PermissionScopes {
					if k != slug {
						scopes[k] = v
					}
				}
				p.PermissionScopes = scopes
				var perms []string
				for _, s := range p.Permissions {
					if s != slug {
						perms = append(perms, s)
					}
				}
				p.Permissions = perms
			}
			scope := orgctx.MustScope(r.Context())
			tp := aitools.Principal{Auth: p, Org: &scope, Realm: aitools.RealmPanel}
			var body struct {
				Tool       string          `json:"tool"`
				ToolUseID  string          `json:"tool_use_id"`
				Input      json.RawMessage `json:"input"`
				ActionUUID uuid.UUID       `json:"action_uuid"`
				Edits      map[string]any  `json:"edits"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			var (
				out any
				err error
			)
			switch r.PathValue("op") {
			case "propose":
				out, err = it.srv.aiActions.Propose(r.Context(), aiusecase.ProposeCall{Principal: tp, Source: model.SourcePanel,
					SourceRef: "t387-conv", ToolUseID: body.ToolUseID, ToolName: body.Tool, Input: body.Input})
			case "confirm":
				out, err = it.srv.aiActions.Confirm(r.Context(), tp, body.ActionUUID, body.Edits)
			case "cancel":
				out, err = it.srv.aiActions.Cancel(r.Context(), tp, body.ActionUUID)
			}
			if err != nil {
				status, code, ok := aiusecase.ErrorStatus(err)
				if !ok {
					response.InternalErr(w, r, err, "ai action")
					return
				}
				response.Error(w, r, status, code, err.Error())
				return
			}
			response.JSON(w, r, http.StatusOK, out)
		}), authn, org))
}

func (it *itest) aiAction(tok, op string, body any, revoke ...string) (int, json.RawMessage, string) {
	it.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/tenant/_test/ai-actions/"+op, strings.NewReader(string(raw)))
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	if len(revoke) > 0 {
		req.Header.Set("X-Test-Revoke", revoke[0])
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	code := ""
	if env.Error != nil {
		code = env.Error.Code
	}
	return rec.Code, env.Data, code
}

// TEC-387 acceptance on the real use cases, repository and activity log:
// a proposed lead is not written; another user of the organization cannot
// confirm it (404); the owner confirms it with an edited field (the lead
// is created once with the edit, activity log via = ai); a second
// confirmation is 409; a follow-up confirmed after leads.write was revoked
// is failed with 403 and changes nothing.
func TestIntegrationAIActionsConfirm(t *testing.T) {
	it := newIntegration(t)
	it.mountAIActions()
	ctx := context.Background()

	dealer := it.org("t387-dealer", "dealer", it.brandCenter("olex"))
	owner, pw := it.user("t387-owner")
	it.member(dealer, owner, "owner")
	tok := it.loginOrg(owner, pw, dealer)
	other, pw2 := it.user("t387-other")
	it.member(dealer, other, "owner")
	tokOther := it.loginOrg(other, pw2, dealer)

	name := "T387 " + it.suffix
	leads := func(contact string) int {
		var n int
		if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM leads WHERE organization_id = $1 AND candidate_contact_name = $2`,
			dealer.ID, contact).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	status, data, code := it.aiAction(tok, "propose", map[string]any{
		"tool": aitools.ToolCreateLead, "tool_use_id": "toolu_t387_1",
		"input": map[string]any{"contact_name": name, "phone": "+905321234567"},
	})
	var proposed aiusecase.ProposeOutcome
	_ = json.Unmarshal(data, &proposed)
	if status != http.StatusOK || proposed.Card == nil || proposed.Card.Status != model.ActionPending {
		t.Fatalf("propose = %d %s %s", status, code, data)
	}
	id := proposed.Card.ActionUUID
	if leads(name) != 0 {
		t.Fatal("a proposal wrote the lead")
	}

	if status, _, _ := it.aiAction(tokOther, "confirm", map[string]any{"action_uuid": id}); status != http.StatusNotFound {
		t.Fatalf("another user's confirm = %d, want 404", status)
	}
	if status, _, code := it.aiAction(tok, "confirm", map[string]any{"action_uuid": id,
		"edits": map[string]any{"contact_name": strings.Repeat("x", 201)}}); status != http.StatusBadRequest || code != "VALIDATION_ERROR" {
		t.Fatalf("schema-breaking edit = %d %s, want 400", status, code)
	}

	edited := name + " B"
	status, data, code = it.aiAction(tok, "confirm", map[string]any{"action_uuid": id,
		"edits": map[string]any{"contact_name": edited}})
	var done aiusecase.Outcome
	_ = json.Unmarshal(data, &done)
	if status != http.StatusOK || done.Status != model.ActionConfirmed || done.Link == nil || done.Link.Kind != "lead" {
		t.Fatalf("confirm = %d %s %s", status, code, data)
	}
	if leads(edited) != 1 || leads(name) != 0 {
		t.Fatalf("lead rows: edited=%d original=%d", leads(edited), leads(name))
	}
	var via, actionUUID string
	var actor int64
	if err := it.pool.QueryRow(ctx, `SELECT actor_user_id, payload->>'via', payload->>'ai_action_uuid' FROM activity_events
		WHERE action = 'ai.action.confirmed' AND resource_uuid = $1`, id).Scan(&actor, &via, &actionUUID); err != nil {
		t.Fatalf("activity event: %v", err)
	}
	if actor != owner.ID || via != "ai" || actionUUID != id.String() {
		t.Fatalf("activity: actor=%d via=%q action=%q", actor, via, actionUUID)
	}
	if status, _, code := it.aiAction(tok, "confirm", map[string]any{"action_uuid": id}); status != http.StatusConflict || code != aiusecase.CodeActionResolved {
		t.Fatalf("second confirm = %d %s, want 409", status, code)
	}

	// Follow-up on the new lead, confirmed after leads.write was revoked.
	status, data, _ = it.aiAction(tok, "propose", map[string]any{
		"tool": aitools.ToolSetLeadFollowUp, "tool_use_id": "toolu_t387_2",
		"input": map[string]any{"lead_uuid": done.Link.UUID, "follow_up_date": time.Now().AddDate(0, 0, 10).Format(time.DateOnly)},
	})
	proposed = aiusecase.ProposeOutcome{}
	_ = json.Unmarshal(data, &proposed)
	if status != http.StatusOK || proposed.Card == nil {
		t.Fatalf("propose follow-up = %d %s", status, data)
	}
	if status, _, code := it.aiAction(tok, "confirm", map[string]any{"action_uuid": proposed.Card.ActionUUID},
		rbac.PermLeadsWrite); status != http.StatusForbidden || code != "FORBIDDEN" {
		t.Fatalf("confirm after revoke = %d %s, want 403", status, code)
	}
	var st string
	var followUp *string
	if err := it.pool.QueryRow(ctx, `SELECT status FROM ai_pending_actions WHERE uuid = $1`, proposed.Card.ActionUUID).Scan(&st); err != nil || st != model.ActionFailed {
		t.Fatalf("revoked action status %q (%v)", st, err)
	}
	if err := it.pool.QueryRow(ctx, `SELECT follow_up_date::text FROM leads WHERE uuid = $1`, done.Link.UUID).Scan(&followUp); err != nil || followUp != nil {
		t.Fatalf("revoked follow-up was written: %v %v", followUp, err)
	}

	// Cancel: a third card is cancelled and never runs.
	_, data, _ = it.aiAction(tok, "propose", map[string]any{
		"tool": aitools.ToolCreateLead, "tool_use_id": "toolu_t387_3", "input": map[string]any{"contact_name": name + " C"},
	})
	proposed = aiusecase.ProposeOutcome{}
	_ = json.Unmarshal(data, &proposed)
	if status, _, _ := it.aiAction(tok, "cancel", map[string]any{"action_uuid": proposed.Card.ActionUUID}); status != http.StatusOK {
		t.Fatalf("cancel = %d", status)
	}
	if leads(name+" C") != 0 {
		t.Fatal("a cancelled action wrote")
	}
}
