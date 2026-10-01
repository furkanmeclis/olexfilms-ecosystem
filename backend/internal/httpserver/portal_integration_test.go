package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// portalOTPLogin signs a new phone customer in through the OTP flow.
func portalOTPLogin(t *testing.T, it *itest, fw *fakeWuzapi) tokenPair {
	t.Helper()
	ph := itPhone()
	if code, env := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": ph}); code != http.StatusAccepted {
		t.Fatalf("otp request: %d %s", code, errCode(env))
	}
	c, _ := sentCode(t, fw, ph)
	tp := it.tokensFrom(it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{"phone": ph, "code": c}))
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM users WHERE phone_e164 = $1", ph)
	})
	return tp
}

func (it *itest) realm(access string) string {
	it.t.Helper()
	claims, err := it.tokens.ParseAccess(access)
	if err != nil {
		it.t.Fatalf("parse access: %v", err)
	}
	return claims.Realm()
}

// TEC-90 acceptance 2: portal tokens are refused on panel routes and panel
// tokens on portal routes; refresh keeps the realm.
func TestIntegrationPortalRealmSeparation(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	portal := portalOTPLogin(t, it, fw)
	if it.realm(portal.AccessToken) != jwt.AudiencePortal {
		t.Fatal("OTP login must issue aud=portal")
	}

	center := it.brandCenter("olex")
	org := it.org("portal-realm", "dealer", center)
	staff, pw := it.user("portal-staff")
	it.member(org, staff, "owner")
	panel := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": staff.Email.String, "password": pw, "organization_slug": org.Slug,
	}))
	if it.realm(panel.AccessToken) != jwt.AudiencePanel {
		t.Fatal("password login must issue aud=panel")
	}

	panelOnly := []struct{ method, path string }{
		{"GET", "/v1/platform/users"},
		{"GET", "/v1/tenant/organizations"},
		{"GET", "/v1/tenant/settings"},
		{"GET", "/v1/me/organizations"},
		{"GET", "/v1/notifications"},
		{"POST", "/v1/auth/impersonation/stop"},
	}
	for _, rt := range panelOnly {
		if code, env := it.do(rt.method, rt.path, hostOlex, portal.AccessToken, nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
			t.Fatalf("portal token on %s %s: %d %s", rt.method, rt.path, code, errCode(env))
		}
	}
	// Organization context switch never mints a panel session from the portal.
	if code, env := it.do("POST", "/v1/auth/organization-context", hostOlex, portal.AccessToken,
		map[string]string{"organization_slug": org.Slug}); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("portal org switch: %d %s", code, errCode(env))
	}
	// Account routes and portal routes work with the portal token.
	if code, env := it.do("GET", "/v1/auth/me", hostOlex, portal.AccessToken, nil); code != http.StatusOK {
		t.Fatalf("portal me: %d %s", code, errCode(env))
	}
	if code, env := it.do("GET", "/v1/portal/consents/pending", hostOlex, portal.AccessToken, nil); code != http.StatusOK {
		t.Fatalf("portal consents: %d %s", code, errCode(env))
	}
	// Panel token on a portal route: 403.
	if code, env := it.do("GET", "/v1/portal/consents/pending", hostOlex, panel.AccessToken, nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel token on portal route: %d %s", code, errCode(env))
	}
	if code, env := it.do("GET", "/v1/tenant/settings", hostOlex, panel.AccessToken, nil); code != http.StatusOK {
		t.Fatalf("panel token on tenant route: %d %s", code, errCode(env))
	}

	// Refresh keeps aud on both sides.
	rp := it.tokensFrom(it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": portal.RefreshToken}))
	if it.realm(rp.AccessToken) != jwt.AudiencePortal {
		t.Fatal("refresh must keep aud=portal")
	}
	if code, _ := it.do("GET", "/v1/platform/users", hostOlex, rp.AccessToken, nil); code != http.StatusForbidden {
		t.Fatalf("refreshed portal token on platform: %d", code)
	}
	rpanel := it.tokensFrom(it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": panel.RefreshToken}))
	if it.realm(rpanel.AccessToken) != jwt.AudiencePanel {
		t.Fatal("refresh must keep aud=panel")
	}

	// Logging out of one realm leaves the other session alive.
	if code, _ := it.do("POST", "/v1/auth/logout", hostOlex, rpanel.AccessToken, map[string]string{"refresh_token": rpanel.RefreshToken}); code != http.StatusOK {
		t.Fatalf("panel logout: %d", code)
	}
	if code, env := it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": rp.RefreshToken}); code != http.StatusOK {
		t.Fatalf("portal session must survive panel logout: %d %s", code, errCode(env))
	}
}

// TEC-90 acceptance 3: fleet signs in to the portal with e-mail + password
// and is refused by the panel; staff are refused by the portal.
func TestIntegrationPortalPasswordRealm(t *testing.T) {
	it := newIntegration(t)
	fleet, fpw := it.user("fleet", rbac.RoleFleet)
	tp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": fleet.Email.String, "password": fpw, "realm": "portal",
	}))
	if it.realm(tp.AccessToken) != jwt.AudiencePortal {
		t.Fatal("fleet portal login must issue aud=portal")
	}
	if code, env := it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": fleet.Email.String, "password": fpw,
	}); code != http.StatusForbidden || errCode(env) != "NO_PANEL_ACCESS" {
		t.Fatalf("fleet panel login: %d %s", code, errCode(env))
	}
	// A wrong password never reveals the realm rule.
	if code, env := it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": fleet.Email.String, "password": "wrong-Passw0rd!",
	}); code != http.StatusUnauthorized || errCode(env) != "INVALID_CREDENTIALS" {
		t.Fatalf("fleet wrong password: %d %s", code, errCode(env))
	}

	admin, apw := it.user("portal-admin", rbac.RoleSuperAdmin)
	if code, env := it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw, "realm": "portal",
	}); code != http.StatusForbidden || errCode(env) != "NO_PORTAL_ACCESS" {
		t.Fatalf("staff portal login: %d %s", code, errCode(env))
	}
	center := it.brandCenter("olex")
	org := it.org("portal-dealer", "dealer", center)
	dealer, dpw := it.user("portal-dealer")
	it.member(org, dealer, "owner")
	if code, env := it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": dealer.Email.String, "password": dpw, "realm": "portal",
	}); code != http.StatusForbidden || errCode(env) != "NO_PORTAL_ACCESS" {
		t.Fatalf("dealer portal login: %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": dealer.Email.String, "password": dpw, "realm": "elsewhere",
	}); code != http.StatusBadRequest {
		t.Fatalf("unknown realm: %d", code)
	}
	// A customer upgraded to dealer (K11: role change) keeps panel access.
	upgraded, upw := it.user("portal-upgraded", rbac.RoleCustomer)
	it.member(org, upgraded, "staff")
	if code, env := it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": upgraded.Email.String, "password": upw,
	}); code != http.StatusOK {
		t.Fatalf("upgraded customer panel login: %d %s", code, errCode(env))
	}
}

type pendingText struct {
	Kind    string `json:"kind"`
	Locale  string `json:"locale"`
	Version int32  `json:"version"`
	Body    string `json:"body"`
}

func (it *itest) pendingConsents(access string) []pendingText {
	it.t.Helper()
	code, env := it.do("GET", "/v1/portal/consents/pending?locale=tr", hostOlex, access, nil)
	if code != http.StatusOK {
		it.t.Fatalf("pending: %d %s", code, errCode(env))
	}
	var out struct {
		Items []pendingText `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		it.t.Fatal(err)
	}
	return out.Items
}

// TEC-90 acceptance 4 (K22): asked on first sign-in, once per text version;
// a decline is recorded and does not lock the portal; a new version asks again.
func TestIntegrationPortalConsents(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	portal := portalOTPLogin(t, it, fw)

	items := it.pendingConsents(portal.AccessToken)
	if len(items) != 1 || items[0].Kind != "ai_guidelines" || items[0].Locale != "tr" || items[0].Body == "" {
		t.Fatalf("first sign-in pending = %+v", items)
	}
	first := items[0]
	code, env := it.do("POST", "/v1/portal/consents", hostOlex, portal.AccessToken, map[string]any{
		"kind": first.Kind, "locale": first.Locale, "version": first.Version, "accepted": false,
	})
	if code != http.StatusCreated {
		t.Fatalf("decline: %d %s", code, errCode(env))
	}
	var row struct {
		ip       *string
		accepted bool
		version  int32
	}
	// ip and decided_at are evidence (K22).
	if err := it.pool.QueryRow(context.Background(), `SELECT c.ip, c.accepted, c.text_version FROM consents c
		JOIN users u ON u.id = c.user_id WHERE u.uuid = $1::uuid`,
		mustSubject(t, it, portal.AccessToken)).Scan(&row.ip, &row.accepted, &row.version); err != nil {
		t.Fatalf("consent row: %v", err)
	}
	if row.accepted || row.version != first.Version {
		t.Fatalf("consent row = %+v", row)
	}
	// Second sign-in: not asked again.
	if items := it.pendingConsents(portal.AccessToken); len(items) != 0 {
		t.Fatalf("asked again: %+v", items)
	}
	// The portal is not locked after a decline.
	if code, _ := it.do("GET", "/v1/auth/me", hostOlex, portal.AccessToken, nil); code != http.StatusOK {
		t.Fatalf("me after decline: %d", code)
	}

	// Admin publishes a new tr version: asked again, the stale version is refused.
	admin, apw := it.user("legal-admin", rbac.RoleSuperAdmin)
	atok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{"email": admin.Email.String, "password": apw})).AccessToken
	code, env = it.do("PUT", "/v1/platform/legal-texts/ai_guidelines", hostOlex, atok, map[string]string{
		"locale": "tr", "body": "## Yönerge\n\nGüncel metin " + it.suffix,
	})
	if code != http.StatusOK {
		t.Fatalf("publish: %d %s", code, errCode(env))
	}
	items = it.pendingConsents(portal.AccessToken)
	if len(items) != 1 || items[0].Version <= first.Version {
		t.Fatalf("new version pending = %+v", items)
	}
	if code, env := it.do("POST", "/v1/portal/consents", hostOlex, portal.AccessToken, map[string]any{
		"kind": "ai_guidelines", "locale": "tr", "version": first.Version, "accepted": true,
	}); code != http.StatusConflict || errCode(env) != "LEGAL_TEXT_STALE" {
		t.Fatalf("stale answer: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/portal/consents", hostOlex, portal.AccessToken, map[string]any{
		"kind": "ai_guidelines", "locale": "tr", "version": items[0].Version, "accepted": true,
	}); code != http.StatusCreated {
		t.Fatalf("accept: %d %s", code, errCode(env))
	}
	if items := it.pendingConsents(portal.AccessToken); len(items) != 0 {
		t.Fatalf("pending after accept: %+v", items)
	}

	// Editor permission.
	if code, env := it.do("GET", "/v1/platform/legal-texts/ai_guidelines", hostOlex, atok, nil); code != http.StatusOK {
		t.Fatalf("admin get: %d %s", code, errCode(env))
	}
	plain, ppw := it.user("legal-plain")
	ptok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{"email": plain.Email.String, "password": ppw})).AccessToken
	if code, _ := it.do("PUT", "/v1/platform/legal-texts/ai_guidelines", hostOlex, ptok, map[string]string{"locale": "tr", "body": "x"}); code != http.StatusForbidden {
		t.Fatalf("editor without permission: %d", code)
	}
}

func mustSubject(t *testing.T, it *itest, access string) string {
	t.Helper()
	claims, err := it.tokens.ParseAccess(access)
	if err != nil {
		t.Fatal(err)
	}
	return claims.Subject
}

// TEC-90 acceptance 5: there is no endless customer hash link (the old hub's
// Crypt::encrypt(id) URL); short links (/s/{token}) arrive with F2.
func TestIntegrationNoPermanentCustomerHash(t *testing.T) {
	it := newIntegration(t)
	for _, p := range []string{
		"/s/abc", "/v1/public/s/abc", "/v1/public/customer/abc", "/v1/public/customers/abc",
		"/v1/customer/abc", "/customer/abc", "/v1/portal/customer/abc",
	} {
		if code, _ := it.do("GET", p, hostOlex, "", nil); code != http.StatusNotFound {
			t.Fatalf("%s must not exist: %d", p, code)
		}
	}
}
