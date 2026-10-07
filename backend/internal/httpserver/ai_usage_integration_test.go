package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	airepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-389 routes on the real server: the panel usage report is limited to
// the caller's ai.usage.read reach (another organization is 404 on list,
// summary and export), the list contract answers 400 for an unknown sort,
// and the platform settings / quota routes are super_admin only.
func TestIntegrationAIUsageReport(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealerA := it.org("t389-a", "dealer", center)
	dealerB := it.org("t389-b", "dealer", center)
	owner, pw := it.user("t389-owner")
	it.member(dealerA, owner, "owner")
	other, _ := it.user("t389-other")
	store := airepo.New(it.pool)
	for _, u := range []struct {
		orgID, brandID, userID int64
	}{{dealerA.ID, dealerA.BrandID, owner.ID}, {dealerB.ID, dealerB.BrandID, other.ID}} {
		uid := u.userID
		if _, _, err := store.RecordUsage(ctx, airepo.Usage{OrganizationID: u.orgID, BrandID: u.brandID, Pool: model.PoolOrg,
			UserID: &uid, Channel: model.UsageChannelPanel, Purpose: model.PurposeChat, Model: "claude-sonnet-5-5",
			InputTokens: 100, OutputTokens: 20}); err != nil {
			t.Fatal(err)
		}
	}
	tok := it.loginOrg(owner, pw, dealerA)

	code, env := it.do("GET", "/v1/ai/usage?sort=-tokens", hostOlex, tok, nil)
	var page struct {
		Items []struct {
			Tokens       int64 `json:"tokens"`
			Organization struct {
				UUID string `json:"uuid"`
			} `json:"organization"`
		} `json:"items"`
		Total int `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &page)
	if code != http.StatusOK || page.Total != 1 || page.Items[0].Tokens != 120 || page.Items[0].Organization.UUID != dealerA.Uuid.String() {
		t.Fatalf("own usage = %d %+v", code, page)
	}
	if code, env := it.do("GET", "/v1/ai/usage?sort=-model", hostOlex, tok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("unknown sort = %d %s", code, errCode(env))
	}
	foreign := dealerB.Uuid.String()
	for _, path := range []string{"/v1/ai/usage?organization=" + foreign, "/v1/ai/usage/summary?organization=" + foreign} {
		if code, env := it.do("GET", path, hostOlex, tok, nil); code != http.StatusNotFound {
			t.Fatalf("%s = %d %s, want 404", path, code, errCode(env))
		}
	}
	if code, env := it.do("POST", "/v1/ai/usage/export", hostOlex, tok, map[string]any{
		"format": "csv", "query": map[string]string{"organization": foreign},
	}); code != http.StatusNotFound {
		t.Fatalf("foreign export = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/ai/usage/summary", hostOlex, tok, nil)
	var sum struct {
		Quota struct {
			Used int64 `json:"used"`
		} `json:"quota"`
		ByUser []struct {
			Tokens int64 `json:"tokens"`
		} `json:"by_user"`
	}
	_ = json.Unmarshal(env.Data, &sum)
	if code != http.StatusOK || sum.Quota.Used != 120 || len(sum.ByUser) != 1 {
		t.Fatalf("summary = %d %+v", code, sum)
	}
	for _, path := range []string{"/v1/platform/ai/settings", "/v1/platform/ai/orgs", "/v1/platform/ai/usage"} {
		if code, _ := it.do("GET", path, hostOlex, tok, nil); code != http.StatusForbidden {
			t.Fatalf("dealer owner %s = %d, want 403", path, code)
		}
	}

	admin, apw := it.user("t389-admin", rbac.RoleSuperAdmin)
	atok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	})).AccessToken
	if code, env := it.do("GET", "/v1/platform/ai/settings", hostOlex, atok, nil); code != http.StatusOK {
		t.Fatalf("settings = %d %s", code, errCode(env))
	}
	if code, env := it.do("PUT", "/v1/platform/ai/orgs/"+dealerA.Uuid.String(), hostOlex, atok, map[string]any{
		"monthly_token_quota": 0,
	}); code != http.StatusOK {
		t.Fatalf("put org = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/platform/ai/orgs?q=t389-a-"+it.suffix+"&org_type=dealer&sort=-quota", hostOlex, atok, nil)
	var orgs struct {
		Items []struct {
			Quota   int64    `json:"quota"`
			Used    int64    `json:"used"`
			Percent *float64 `json:"percent"`
		} `json:"items"`
		Total int `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &orgs)
	if code != http.StatusOK || orgs.Total != 1 || orgs.Items[0].Quota != 0 || orgs.Items[0].Used != 120 || orgs.Items[0].Percent != nil {
		t.Fatalf("orgs = %d %+v", code, orgs)
	}
	if code, env := it.do("GET", "/v1/platform/ai/orgs?org_type=customer", hostOlex, atok, nil); code != http.StatusBadRequest {
		t.Fatalf("bad org_type = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/platform/ai/usage?organization="+dealerA.Uuid.String()+","+foreign, hostOlex, atok, nil)
	_ = json.Unmarshal(env.Data, &page)
	if code != http.StatusOK || page.Total != 2 {
		t.Fatalf("platform usage = %d %+v", code, page)
	}
}
