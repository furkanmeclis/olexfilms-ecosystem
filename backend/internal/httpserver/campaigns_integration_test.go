package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// campaignNet is a dealer (campaigns add-on enabled) with its owner and a
// second dealer of the same brand.
type campaignNet struct {
	dealer, other       db.Organization
	dealerTok, otherTok string
}

func (it *itest) campaignNet() campaignNet {
	it.t.Helper()
	ctx := context.Background()
	center := it.brandCenter("olex")
	n := campaignNet{
		dealer: it.org("cmp405", rbac.OrgTypeDealer, center),
		other:  it.org("cmp405-other", rbac.OrgTypeDealer, center),
	}
	for _, o := range []db.Organization{n.dealer, n.other} {
		if _, err := it.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
			Scope: "org", OrganizationID: pgtype.Int8{Int64: o.ID, Valid: true},
			ModuleKey: features.ModuleCampaigns, Enabled: true, Source: "admin",
		}); err != nil {
			it.t.Fatalf("enable campaigns: %v", err)
		}
	}
	it.t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM campaigns WHERE organization_id = ANY($1)`,
			[]int64{n.dealer.ID, n.other.ID})
	})
	owner, pw := it.user("cmp405-owner")
	it.member(n.dealer, owner, "owner")
	n.dealerTok = it.loginOrg(owner, pw, n.dealer)
	other, opw := it.user("cmp405-other-owner")
	it.member(n.other, other, "owner")
	n.otherTok = it.loginOrg(other, opw, n.other)
	return n
}

func (it *itest) createCampaign(tok, name string, channels ...string) string {
	it.t.Helper()
	code, env := it.do("POST", "/v1/campaigns", hostOlex, tok, map[string]any{
		"name": name, "channels": channels, "audience_filter": map[string]any{"audience_type": "customers"},
	})
	if code != http.StatusCreated {
		it.t.Fatalf("create campaign = %d %s", code, errCode(env))
	}
	var c struct {
		UUID string `json:"uuid"`
	}
	_ = json.Unmarshal(env.Data, &c)
	return c.UUID
}

// TEC-405: GET /v1/campaigns follows the list contract (sort whitelist,
// default -created_at with id tiebreak, multi-value status / channel,
// unknown sort 400) inside the caller's scope.
func TestIntegrationCampaignListContract(t *testing.T) {
	it := newIntegration(t)
	n := it.campaignNet()
	b := it.createCampaign(n.dealerTok, "b-kampanya", "push")
	a := it.createCampaign(n.dealerTok, "a-kampanya", "whatsapp", "email")
	c := it.createCampaign(n.dealerTok, "c-kampanya", "email")
	it.createCampaign(n.otherTok, "foreign", "push")
	it.exec(`UPDATE campaigns SET status = 'pending_approval', approver_org_id = organization_id WHERE uuid = $1`, c)

	lc := listSortCase{it: it, admin: n.dealerTok, base: "/v1/campaigns"}
	lc.expect(nil, c, a, b)
	lc.expect(sortParam("name"), a, b, c)
	lc.expect(sortParam("-name"), c, b, a)
	lc.expect(sortParam("status"), b, a, c)
	lc.expect(url.Values{"status": {"draft"}, "sort": {"name"}}, a, b)
	lc.expect(url.Values{"status": {"draft,pending_approval"}, "sort": {"name"}}, a, b, c)
	lc.expect(url.Values{"channel": {"email"}, "sort": {"name"}}, a, c)
	lc.expect(url.Values{"q": {"b-kamp"}}, b)
	lc.expect400(sortParam("bogus"), "sort")
	lc.expect400(url.Values{"status": {"draft,bogus"}}, "status")
	lc.expect400(url.Values{"channel": {"sms"}}, "channel")
}

// TEC-405: draft-only edits (409), media size limit (400), dealer audience
// restriction (400), preview and foreign scope (404) over HTTP.
func TestIntegrationCampaignAuthoring(t *testing.T) {
	it := newIntegration(t)
	n := it.campaignNet()
	id := it.createCampaign(n.dealerTok, "Bahar", "push", "whatsapp")

	code, env := it.do("POST", "/v1/campaigns", hostOlex, n.dealerTok, map[string]any{
		"name": "x", "channels": []string{"email"}, "audience_filter": map[string]any{"audience_type": "dealer_users"},
	})
	if code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("dealer_users audience = %d %s, want 400", code, errCode(env))
	}
	code, env = it.do("PUT", "/v1/campaigns/"+id+"/contents/tr", hostOlex, n.dealerTok, map[string]any{
		"title": "Bahar", "body": "İndirim",
	})
	if code != http.StatusOK {
		t.Fatalf("put content = %d %s", code, errCode(env))
	}
	code, env = it.do("POST", "/v1/campaigns/"+id+"/preview", hostOlex, n.dealerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("preview = %d %s", code, errCode(env))
	}
	var p struct {
		Total    int `json:"total"`
		Channels []struct {
			Channel string `json:"channel"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(env.Data, &p); err != nil || p.Total != 0 || len(p.Channels) != 2 {
		t.Fatalf("preview = %s (%v)", env.Data, err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "big.png")
	big := make([]byte, 6<<20)
	copy(big, "\x89PNG\x0D\x0A\x1A\x0A")
	_, _ = fw.Write(big)
	_ = mw.Close()
	rec := it.raw("POST", "/v1/campaigns/"+id+"/contents/tr/media", n.dealerTok, mw.FormDataContentType(), body.Bytes(), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("6 MB image = %d %s, want 400", rec.Code, rec.Body)
	}

	if code, ec := it.status("GET", "/v1/campaigns/"+id, n.otherTok); code != http.StatusNotFound {
		t.Fatalf("foreign get = %d %s, want 404", code, ec)
	}
	it.exec(`UPDATE campaigns SET status = 'pending_approval', approver_org_id = organization_id WHERE uuid = $1`, id)
	code, env = it.do("PATCH", "/v1/campaigns/"+id, hostOlex, n.dealerTok, map[string]any{"name": "Yeni"})
	if code != http.StatusConflict || errCode(env) != "CAMPAIGN_NOT_DRAFT" {
		t.Fatalf("patch non-draft = %d %s, want 409 CAMPAIGN_NOT_DRAFT", code, errCode(env))
	}
	if code, ec := it.status("DELETE", "/v1/campaigns/"+id, n.dealerTok); code != http.StatusConflict {
		t.Fatalf("delete non-draft = %d %s, want 409", code, ec)
	}
	draft := it.createCampaign(n.dealerTok, "Silinecek", "email")
	if code, ec := it.status("DELETE", "/v1/campaigns/"+draft, n.dealerTok); code != http.StatusNoContent {
		t.Fatalf("delete draft = %d %s", code, ec)
	}
}
