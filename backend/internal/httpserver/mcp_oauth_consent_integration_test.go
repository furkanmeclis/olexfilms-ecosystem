package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-401: /oauth/authorize, consent (panel + portal realms), connected
// apps and the platform client list on the full server.
func TestIntegrationMCPOAuthConsent(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()
	ip := fmt.Sprintf("198.19.%d.%d", time.Now().UnixNano()%250, time.Now().UnixNano()/1000%250)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE created_ip = $1`, ip)
	})

	center := it.brandCenter("olex")
	dealer := it.org("mcp401", rbac.OrgTypeDealer, center)
	foreign := it.org("mcp401-foreign", rbac.OrgTypeDealer, center)
	for _, o := range []db.Organization{dealer, foreign} {
		if _, err := it.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
			Scope: "org", OrganizationID: pgtype.Int8{Int64: o.ID, Valid: true},
			ModuleKey: features.ModuleMCP, Enabled: true, Source: "admin",
		}); err != nil {
			t.Fatalf("enable mcp: %v", err)
		}
	}
	owner, pw := it.user("mcp401-owner")
	it.member(dealer, owner, "owner")
	panel := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": owner.Email.String, "password": pw, "organization_slug": dealer.Slug,
	})).AccessToken
	customer := portalOTPLogin(t, it, fw).AccessToken

	const redirect = "https://claude.ai/api/mcp/auth_callback"
	const verifier = "tec401-integration-verifier-0123456789abcdefghijklmnop"
	register := func(name string) string {
		rec := it.raw("POST", "/oauth/register", "", "application/json",
			[]byte(`{"client_name":"`+name+`","redirect_uris":["`+redirect+`"]}`), map[string]string{"X-Forwarded-For": ip})
		if rec.Code != http.StatusCreated {
			t.Fatalf("register = %d %s", rec.Code, rec.Body)
		}
		var c struct {
			ClientID string `json:"client_id"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &c)
		return c.ClientID
	}
	clientID := register("Claude TEC401")
	authorize := func(redirectURI, resource string) (int, string) {
		v := url.Values{
			"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"},
			"code_challenge": {usecase.S256(verifier)}, "code_challenge_method": {"S256"},
			"state": {"st"}, "resource": {"http://localhost:3000" + resource},
		}
		rec := it.raw("GET", "/oauth/authorize?"+v.Encode(), "", "", nil, map[string]string{"X-Forwarded-For": ip})
		return rec.Code, rec.Header().Get("Location")
	}
	request := func(resource string) string {
		code, loc := authorize(redirect, resource)
		const prefix = "http://localhost:3000/oauth/consent?request="
		if code != http.StatusFound || !strings.HasPrefix(loc, prefix) {
			t.Fatalf("authorize = %d %q", code, loc)
		}
		return strings.TrimPrefix(loc, prefix)
	}

	// Unregistered redirect_uri: an error page, no redirect.
	if code, loc := authorize("https://evil.example/cb", "/mcp/dealer"); code != http.StatusBadRequest || loc != "" {
		t.Fatalf("unregistered redirect_uri = %d %q", code, loc)
	}

	// The customer realm cannot see or decide a /mcp/dealer request; panel
	// tokens are refused on the portal twin and vice versa.
	req := request("/mcp/dealer")
	if code, env := it.do("GET", "/v1/portal/oauth/requests/"+req, hostOlex, customer, nil); code != http.StatusForbidden {
		t.Fatalf("customer consent = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/portal/oauth/requests/"+req+"/decide", hostOlex, customer, map[string]any{"decision": "approve"}); code != http.StatusForbidden {
		t.Fatalf("customer decide = %d %s", code, errCode(env))
	}
	if code, _ := it.do("GET", "/v1/oauth/requests/"+req, hostOlex, customer, nil); code == http.StatusOK {
		t.Fatal("portal token accepted on the panel consent route")
	}

	code, env := it.do("GET", "/v1/oauth/requests/"+req, hostOlex, panel, nil)
	var info struct {
		ClientName    string `json:"client_name"`
		RedirectHost  string `json:"redirect_host"`
		Realm         string `json:"realm"`
		Organizations []struct {
			UUID string `json:"uuid"`
		} `json:"organizations"`
	}
	_ = json.Unmarshal(env.Data, &info)
	if code != http.StatusOK || info.ClientName != "Claude TEC401" || info.RedirectHost != "claude.ai" || info.Realm != "dealer" ||
		len(info.Organizations) != 1 || info.Organizations[0].UUID != dealer.Uuid.String() {
		t.Fatalf("consent = %d %s", code, env.Data)
	}
	if code, env := it.do("POST", "/v1/oauth/requests/"+req+"/decide", hostOlex, panel, map[string]any{"decision": "approve", "organization_uuid": foreign.Uuid}); code != http.StatusForbidden {
		t.Fatalf("foreign org approve = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/oauth/requests/"+req+"/decide", hostOlex, panel, map[string]any{"decision": "approve"}); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("approve without org = %d %s", code, errCode(env))
	}
	code, env = it.do("POST", "/v1/oauth/requests/"+req+"/decide", hostOlex, panel, map[string]any{"decision": "approve", "organization_uuid": dealer.Uuid})
	var dec struct {
		RedirectURL string `json:"redirect_url"`
	}
	_ = json.Unmarshal(env.Data, &dec)
	u, _ := url.Parse(dec.RedirectURL)
	if code != http.StatusOK || !strings.HasPrefix(dec.RedirectURL, redirect+"?") || u.Query().Get("code") == "" || u.Query().Get("state") != "st" {
		t.Fatalf("approve = %d %s", code, env.Data)
	}
	form := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {u.Query().Get("code")},
		"redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {"http://localhost:3000/mcp/dealer"},
	}
	rec := it.raw("POST", "/oauth/token", "", "application/x-www-form-urlencoded", []byte(form.Encode()), map[string]string{"X-Forwarded-For": ip})
	var tok struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tok)
	if rec.Code != http.StatusOK || tok.RefreshToken == "" {
		t.Fatalf("token = %d %s", rec.Code, rec.Body)
	}

	// Deny: back to the client with error=access_denied.
	code, env = it.do("POST", "/v1/oauth/requests/"+request("/mcp/user")+"/decide", hostOlex, panel, map[string]any{"decision": "deny"})
	_ = json.Unmarshal(env.Data, &dec)
	if u, _ := url.Parse(dec.RedirectURL); code != http.StatusOK || u.Query().Get("error") != "access_denied" || u.Query().Get("state") != "st" {
		t.Fatalf("deny = %d %s", code, env.Data)
	}

	// Connected apps: list contract and revocation.
	if code, env := it.do("GET", "/v1/oauth/grants?sort=uuid", hostOlex, panel, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("unknown grant sort = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/oauth/grants?sort=-last_used_at", hostOlex, panel, nil)
	var grants struct {
		Items []struct {
			UUID             string `json:"uuid"`
			OrganizationUUID string `json:"organization_uuid"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &grants)
	if code != http.StatusOK || grants.Total != 1 || grants.Items[0].OrganizationUUID != dealer.Uuid.String() {
		t.Fatalf("grants = %d %s", code, env.Data)
	}
	if code, _ := it.do("GET", "/v1/portal/oauth/grants", hostOlex, customer, nil); code != http.StatusOK {
		t.Fatalf("portal grants = %d", code)
	}
	if code, _ := it.do("DELETE", "/v1/portal/oauth/grants/"+grants.Items[0].UUID, hostOlex, customer, nil); code != http.StatusNotFound {
		t.Fatalf("foreign grant revoke = %d", code)
	}
	if code, env := it.do("DELETE", "/v1/oauth/grants/"+grants.Items[0].UUID, hostOlex, panel, nil); code != http.StatusNoContent {
		t.Fatalf("revoke grant = %d %s", code, errCode(env))
	}
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {tok.RefreshToken}}
	if rec := it.raw("POST", "/oauth/token", "", "application/x-www-form-urlencoded", []byte(refresh.Encode()), map[string]string{"X-Forwarded-For": ip}); rec.Code != http.StatusBadRequest {
		t.Fatalf("refresh after revoke = %d %s", rec.Code, rec.Body)
	}

	// Platform clients: super admin only, list contract, revoke.
	if code, _ := it.do("GET", "/v1/platform/oauth/clients", hostOlex, panel, nil); code != http.StatusForbidden {
		t.Fatalf("dealer on platform clients = %d", code)
	}
	admin := it.waAdminToken()
	second := register("Another TEC401")
	if code, env := it.do("GET", "/v1/platform/oauth/clients?sort=redirect_uris", hostOlex, admin, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("unknown client sort = %d %s", code, errCode(env))
	}
	listClients := func(query string) []struct {
		UUID     string `json:"uuid"`
		ClientID string `json:"client_id"`
		Status   string `json:"status"`
	} {
		code, env := it.do("GET", "/v1/platform/oauth/clients?q=TEC401&"+query, hostOlex, admin, nil)
		var page struct {
			Items []struct {
				UUID     string `json:"uuid"`
				ClientID string `json:"client_id"`
				Status   string `json:"status"`
			} `json:"items"`
		}
		if code != http.StatusOK || json.Unmarshal(env.Data, &page) != nil {
			t.Fatalf("clients %s = %d %s", query, code, env.Data)
		}
		return page.Items
	}
	if got := listClients("sort=client_name"); len(got) != 2 || got[0].ClientID != second || got[1].ClientID != clientID {
		t.Fatalf("clients by name = %+v", got)
	}
	if got := listClients("sort=-client_name"); len(got) != 2 || got[0].ClientID != clientID {
		t.Fatalf("clients by -name = %+v", got)
	}
	victim := listClients("sort=client_name")[1]
	if code, env := it.do("DELETE", "/v1/platform/oauth/clients/"+victim.UUID, hostOlex, admin, nil); code != http.StatusNoContent {
		t.Fatalf("revoke client = %d %s", code, errCode(env))
	}
	if got := listClients("status=revoked"); len(got) != 1 || got[0].ClientID != clientID || got[0].Status != "revoked" {
		t.Fatalf("revoked clients = %+v", got)
	}
	if code, loc := authorize(redirect, "/mcp/dealer"); code != http.StatusBadRequest || loc != "" {
		t.Fatalf("authorize with a revoked client = %d %q", code, loc)
	}
}
