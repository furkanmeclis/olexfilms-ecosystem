package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-392: POST /v1/warranty-claims/{uuid}/ai-triage re-runs the AI first
// triage for warranty_claims.decide holders; the status never changes and
// the center's ai_assistant module gates it.
func TestIntegrationWarrantyClaimAITriageRetrigger(t *testing.T) {
	provider := fake.New(fake.ToolCall("tu_1", "record_triage", map[string]any{
		"damage_type": "bubbling", "summary": "Kapıda hava kabarcıkları var.", "confidence": 0.74,
	}, llm.Usage{InputTokens: 500, OutputTokens: 40}))
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.LLM = provider })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t392-dealer", "dealer", center)
	p := it.product(center, "T392")
	it.setWarrantyMonths(p, 12)
	tag := dt7Tag(it)
	cust, veh := it.svcCustomer(dealer, "t392-cust", "34T"+tag[4:])
	w := it.warrantyFor(dealer, center, p, cust, veh, 39201)
	c, err := it.q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
		OrganizationID: w.OrganizationID, BrandID: w.BrandID, WarrantyID: w.ID, ServiceID: w.ServiceID,
		VehicleID: w.VehicleID, CustomerUserID: w.HolderUserID, Description: "Kapı " + tag, Status: "open",
		CoverageCheck: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	staff, spw := it.user("t392-center")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	centerTok := it.loginOrg(staff, spw, center)
	owner, opw := it.user("t392-dealer")
	it.member(dealer, owner, "owner")
	dealerTok := it.loginOrg(owner, opw, dealer)
	path := "/v1/warranty-claims/" + c.Uuid.String() + "/ai-triage"

	admin, _ := it.user("t392-admin", rbac.RoleSuperAdmin)
	if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, center.ID, features.ModuleAIAssistant, false); err != nil {
		t.Fatalf("disable ai_assistant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = it.srv.features.ClearByAdmin(context.Background(), center.ID, features.ModuleAIAssistant)
	})
	if code, env := it.do("POST", path, hostOlex, centerTok, nil); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("module off = %d %s", code, errCode(env))
	}
	if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, center.ID, features.ModuleAIAssistant, true); err != nil {
		t.Fatalf("enable ai_assistant: %v", err)
	}

	if code, _ := it.do("POST", path, hostOlex, dealerTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer without decide = %d", code)
	}
	code, env := it.do("POST", path, hostOlex, centerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("retrigger = %d %s", code, errCode(env))
	}
	got := decodeData[struct {
		Status       string  `json:"status"`
		AIDamageType *string `json:"ai_damage_type"`
		AIConfidence *string `json:"ai_confidence"`
		AITriagedAt  *string `json:"ai_triaged_at"`
		Events       []struct {
			EventType string `json:"event_type"`
		} `json:"events"`
	}](t, env)
	if got.Status != "open" || got.AIDamageType == nil || *got.AIDamageType != "bubbling" ||
		got.AIConfidence == nil || got.AITriagedAt == nil {
		t.Fatalf("claim = %+v", got)
	}
	var triaged int
	for _, e := range got.Events {
		if e.EventType == "ai_triaged" {
			triaged++
		}
	}
	if triaged != 1 {
		t.Fatalf("ai_triaged events = %d", triaged)
	}
}
