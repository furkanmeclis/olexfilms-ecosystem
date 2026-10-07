package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-406: the approval chain and scheduling over HTTP. A dealer campaign
// waits in its distributor's queue; another distributor gets 404; reject
// needs a reason (400); a too early schedule is 422; cancel twice is 409.
func TestIntegrationCampaignApprovalFlow(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("cmp406-dist", rbac.OrgTypeDistributor, center)
	otherDist := it.org("cmp406-dist2", rbac.OrgTypeDistributor, center)
	dealer := it.org("cmp406-dealer", rbac.OrgTypeDealer, dist)
	orgs := []db.Organization{dist, otherDist, dealer}
	ids := []int64{}
	for _, o := range orgs {
		ids = append(ids, o.ID)
		if _, err := it.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
			Scope: "org", OrganizationID: pgtype.Int8{Int64: o.ID, Valid: true},
			ModuleKey: features.ModuleCampaigns, Enabled: true, Source: "admin",
		}); err != nil {
			t.Fatalf("enable campaigns: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM campaigns WHERE organization_id = ANY($1)`, ids)
	})
	login := func(name string, org db.Organization) string {
		u, pw := it.user(name)
		it.member(org, u, "owner")
		return it.loginOrg(u, pw, org)
	}
	dealerTok := login("cmp406-dealer-owner", dealer)
	distTok := login("cmp406-dist-owner", dist)
	otherTok := login("cmp406-dist2-owner", otherDist)

	id := it.createCampaign(dealerTok, "Kış", "whatsapp")
	if code, env := it.do("PUT", "/v1/campaigns/"+id+"/contents/tr", hostOlex, dealerTok, map[string]any{"title": "", "body": "Merhaba"}); code != http.StatusOK {
		t.Fatalf("content = %d %s", code, errCode(env))
	}
	code, env := it.do("POST", "/v1/campaigns/"+id+"/submit", hostOlex, dealerTok, nil)
	var camp struct {
		Status                   string  `json:"status"`
		ApproverOrganizationUUID *string `json:"approver_organization_uuid"`
		Timezone                 string  `json:"timezone"`
		ScheduledAt              *string `json:"scheduled_at"`
		Events                   []struct {
			EventType string `json:"event_type"`
		} `json:"events"`
	}
	_ = json.Unmarshal(env.Data, &camp)
	if code != http.StatusOK || camp.Status != "pending_approval" || camp.ApproverOrganizationUUID == nil ||
		*camp.ApproverOrganizationUUID != dist.Uuid.String() || len(camp.Events) != 1 {
		t.Fatalf("submit = %d %s %+v", code, errCode(env), camp)
	}

	queue := func(tok string) []string {
		t.Helper()
		code, env := it.do("GET", "/v1/campaigns/approvals?sort=-created_at&channel=whatsapp", hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("approvals = %d %s", code, errCode(env))
		}
		var page struct {
			Items []struct {
				UUID string `json:"uuid"`
			} `json:"items"`
		}
		_ = json.Unmarshal(env.Data, &page)
		out := []string{}
		for _, i := range page.Items {
			out = append(out, i.UUID)
		}
		return out
	}
	if !slices.Contains(queue(distTok), id) {
		t.Fatal("distributor queue misses the dealer campaign")
	}
	if slices.Contains(queue(otherTok), id) {
		t.Fatal("other distributor queue holds the campaign")
	}
	if code, ec := it.status("GET", "/v1/campaigns/approvals?sort=bogus", distTok); code != http.StatusBadRequest {
		t.Fatalf("unknown sort = %d %s", code, ec)
	}
	if code, ec := it.status("GET", "/v1/campaigns/approvals", dealerTok); code != http.StatusForbidden {
		t.Fatalf("dealer queue = %d %s, want 403 (no campaigns.approve)", code, ec)
	}
	if code, ec := it.status("POST", "/v1/campaigns/"+id+"/approve", otherTok); code != http.StatusNotFound {
		t.Fatalf("other distributor approve = %d %s, want 404", code, ec)
	}
	if code, ec := it.status("GET", "/v1/campaigns/"+id, distTok); code != http.StatusOK {
		t.Fatalf("approver get = %d %s", code, ec)
	}
	if code, env := it.do("POST", "/v1/campaigns/"+id+"/reject", hostOlex, distTok, map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("reject without reason = %d %s, want 400", code, errCode(env))
	}
	if code, ec := it.status("POST", "/v1/campaigns/"+id+"/approve", distTok); code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, ec)
	}

	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if code, env := it.do("POST", "/v1/campaigns/"+id+"/schedule", hostOlex, dealerTok, map[string]any{"scheduled_at": past}); code != http.StatusUnprocessableEntity ||
		errCode(env) != "CAMPAIGN_SCHEDULE_TOO_SOON" {
		t.Fatalf("past schedule = %d %s, want 422", code, errCode(env))
	}
	at := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	code, env = it.do("POST", "/v1/campaigns/"+id+"/schedule", hostOlex, dealerTok, map[string]any{"scheduled_at": at.Format(time.RFC3339)})
	camp.ScheduledAt = nil
	_ = json.Unmarshal(env.Data, &camp)
	if code != http.StatusOK || camp.Status != "scheduled" || camp.ScheduledAt == nil || camp.Timezone != "Europe/Istanbul" {
		t.Fatalf("schedule = %d %s %+v", code, errCode(env), camp)
	}
	if got, _ := time.Parse(time.RFC3339, *camp.ScheduledAt); !got.Equal(at) {
		t.Fatalf("scheduled_at = %s, want %s", *camp.ScheduledAt, at)
	}
	if code, ec := it.status("POST", "/v1/campaigns/"+id+"/cancel", dealerTok); code != http.StatusOK {
		t.Fatalf("cancel = %d %s", code, ec)
	}
	if code, ec := it.status("POST", "/v1/campaigns/"+id+"/cancel", dealerTok); code != http.StatusConflict || ec != "CAMPAIGN_INVALID_STATUS" {
		t.Fatalf("second cancel = %d %s, want 409", code, ec)
	}
}
