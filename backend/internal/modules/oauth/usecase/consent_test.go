package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-401 acceptance: authorize, realm rules, organization choice, deny,
// grant revocation and the list contract. Permissions come from the real
// auth usecase (member roles in the chosen organization).

// consentFixture extends the TEC-400 fixture: f.user owns f.dealer
// (dealer_owner, which holds mcp.connect).
type consentFixture struct {
	*fixture
	center db.Organization
	brand  int64
}

func newConsentFixture(t *testing.T) *consentFixture {
	t.Helper()
	f := newFixture(t)
	f.svc.SetAccess(oauth.AuthAccess{UC: authusecase.New(repository.NewPostgres(nil, f.q), nil)})
	center, err := f.q.GetBrandCenter(f.ctx, f.dealer.BrandID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	cf := &consentFixture{fixture: f, center: center, brand: f.dealer.BrandID}
	cf.member(t, f.dealer, f.user, rbac.RoleDealerOwner)
	return cf
}

func (f *consentFixture) member(t *testing.T, org db.Organization, u db.User, roleSlug string) {
	t.Helper()
	m, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{OrganizationID: org.ID, UserID: u.ID, Role: "owner"})
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	if err := f.q.AssignMemberRoleBySlug(f.ctx, db.AssignMemberRoleBySlugParams{MemberID: m.ID, Slug: roleSlug}); err != nil {
		t.Fatalf("member role: %v", err)
	}
}

func (f *consentFixture) org(t *testing.T, name, typ string) db.Organization {
	t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: "tec401-" + name + "-" + uuid.NewString()[:8], Name: "TEC401 " + name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true},
		BrandID: f.brand, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	return o
}

func (f *consentFixture) newUser(t *testing.T, name string) db.User {
	t.Helper()
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "TEC401", Surname: name, Status: "active",
		Email: pgtype.Text{String: "tec401-" + name + "-" + uuid.NewString()[:8] + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return u
}

func panelActor(u db.User) usecase.Actor {
	return usecase.Actor{UserID: u.ID, Realm: jwt.AudiencePanel}
}

func (f *consentFixture) portalActor(u db.User) usecase.Actor {
	return usecase.Actor{UserID: u.ID, Realm: jwt.AudiencePortal, Roles: []string{rbac.RoleCustomer}, BrandID: f.brand}
}

// request runs /oauth/authorize for resource and returns the request id.
func (f *consentFixture) request(t *testing.T, clientID, resource string) uuid.UUID {
	t.Helper()
	to, err := f.svc.Authorize(f.ctx, "203.0.113.9", f.authorizeInput(clientID, resource))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	prefix := issuer + model.ConsentPath + "?request="
	if !strings.HasPrefix(to, prefix) {
		t.Fatalf("authorize redirect = %q, want the consent screen", to)
	}
	id, err := uuid.Parse(strings.TrimPrefix(to, prefix))
	if err != nil {
		t.Fatalf("request id: %v", err)
	}
	return id
}

func (f *consentFixture) approve(t *testing.T, a usecase.Actor, id uuid.UUID, org *uuid.UUID) url.Values {
	t.Helper()
	out, err := f.svc.Decide(f.ctx, a, id, model.DecideInput{Decision: model.DecisionApprove, OrganizationUUID: org})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	u, err := url.Parse(out.RedirectURL)
	if err != nil || !strings.HasPrefix(out.RedirectURL, f.redirect+"?") {
		t.Fatalf("approve redirect = %q", out.RedirectURL)
	}
	return u.Query()
}

func (f *consentFixture) activityCount(t *testing.T, action string, actor int64) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM activity_events WHERE action = $1 AND actor_user_id = $2`, action, actor).Scan(&n); err != nil {
		t.Fatalf("activity: %v", err)
	}
	return n
}

func wantForbidden(t *testing.T, err error) {
	t.Helper()
	var fe *usecase.ForbiddenError
	if !errors.As(err, &fe) {
		t.Fatalf("want 403 ForbiddenError, got %v", err)
	}
}

func TestAuthorizeUnregisteredRedirectURIShowsErrorPage(t *testing.T) {
	f := newConsentFixture(t)
	c := f.register(t)

	in := f.authorizeInput(c.ClientID, model.ResourceDealer)
	in.RedirectURI = "https://attacker.example/callback"
	to, err := f.svc.Authorize(f.ctx, "203.0.113.9", in)
	oe := wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidRequest)
	if oe.Redirectable || to != "" {
		t.Fatalf("an unregistered redirect_uri must not be redirected to (to=%q)", to)
	}

	in = f.authorizeInput("mcp_unknown", model.ResourceDealer)
	to, err = f.svc.Authorize(f.ctx, "203.0.113.9", in)
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidClient)
	if to != "" {
		t.Fatalf("unknown client redirected to %q", to)
	}

	// After client + redirect_uri are verified, errors go back to the client.
	in = f.authorizeInput(c.ClientID, model.ResourceDealer)
	in.Resource = issuer + "/mcp/unknown"
	to, err = f.svc.Authorize(f.ctx, "203.0.113.9", in)
	if err != nil {
		t.Fatalf("redirectable error: %v", err)
	}
	u, _ := url.Parse(to)
	if !strings.HasPrefix(to, f.redirect+"?") || u.Query().Get("error") != model.ErrInvalidTarget || u.Query().Get("state") != "xyz" || u.Query().Get("iss") != issuer {
		t.Fatalf("error redirect = %q", to)
	}

	// A valid request lands on the consent screen.
	f.request(t, c.ClientID, model.ResourceDealer)
}

func TestCustomerCannotConsentToDealerEndpoint(t *testing.T) {
	f := newConsentFixture(t)
	c := f.register(t)
	customer := f.newUser(t, "customer")
	id := f.request(t, c.ClientID, model.ResourceDealer)

	_, err := f.svc.Consent(f.ctx, f.portalActor(customer), id)
	wantForbidden(t, err)
	_, err = f.svc.Decide(f.ctx, f.portalActor(customer), id, model.DecideInput{Decision: model.DecisionApprove, OrganizationUUID: &f.dealer.Uuid})
	wantForbidden(t, err)
	_, err = f.svc.Decide(f.ctx, f.portalActor(customer), id, model.DecideInput{Decision: model.DecisionDeny})
	wantForbidden(t, err)
	// The request is still pending for its rightful user.
	if _, err := f.svc.GetAuthRequest(f.ctx, id); err != nil {
		t.Fatalf("request consumed by a forbidden decision: %v", err)
	}

	// /mcp/user is panel only too; /mcp/customer is portal only.
	_, err = f.svc.Consent(f.ctx, f.portalActor(customer), f.request(t, c.ClientID, model.ResourceUser))
	wantForbidden(t, err)
	custReq := f.request(t, c.ClientID, model.ResourceCustomer)
	_, err = f.svc.Consent(f.ctx, panelActor(f.user), custReq)
	wantForbidden(t, err)

	// The customer connects /mcp/customer; the token is bound to the center.
	info, err := f.svc.Consent(f.ctx, f.portalActor(customer), custReq)
	if err != nil {
		t.Fatalf("customer consent: %v", err)
	}
	if info.Realm != model.RealmCustomer || len(info.Organizations) != 1 || info.Organizations[0].UUID != f.center.Uuid || info.RedirectHost != "client.example" {
		t.Fatalf("customer consent = %+v", info)
	}
	code := f.approve(t, f.portalActor(customer), custReq, nil).Get("code")
	tok, err := f.exchange(c.ClientID, code, model.ResourceCustomer, verifier)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	at, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceCustomer)
	if err != nil || at.OrganizationID != f.center.ID || at.UserID != customer.ID {
		t.Fatalf("customer token = %+v, %v", at, err)
	}
}

func TestDealerUserCannotChooseForeignOrganization(t *testing.T) {
	f := newConsentFixture(t)
	c := f.register(t)
	foreign := f.org(t, "foreign", rbac.OrgTypeDealer)
	staffOnly := f.org(t, "staff", rbac.OrgTypeDealer)
	f.member(t, staffOnly, f.user, rbac.RoleDealerStaff) // no mcp.connect there
	f.member(t, f.center, f.user, rbac.RoleCenterStaff)  // not a dealer org
	id := f.request(t, c.ClientID, model.ResourceDealer)

	info, err := f.svc.Consent(f.ctx, panelActor(f.user), id)
	if err != nil {
		t.Fatalf("consent: %v", err)
	}
	if info.Realm != model.RealmDealer || info.ClientName != "Claude" || info.Resource != model.ResourceDealer ||
		info.ResourceURL != issuer+model.ResourceDealer || len(info.Organizations) != 1 || info.Organizations[0].UUID != f.dealer.Uuid {
		t.Fatalf("consent = %+v, want only the dealer with mcp.connect", info)
	}

	for _, org := range []uuid.UUID{foreign.Uuid, staffOnly.Uuid, f.center.Uuid, uuid.New()} {
		_, err := f.svc.Decide(f.ctx, panelActor(f.user), id, model.DecideInput{Decision: model.DecisionApprove, OrganizationUUID: &org})
		wantForbidden(t, err)
	}
	// The organization is required for a panel approval.
	_, err = f.svc.Decide(f.ctx, panelActor(f.user), id, model.DecideInput{Decision: model.DecisionApprove})
	var ve *apiquery.ValidationError
	if !errors.As(err, &ve) || ve.Details[0].Field != "organization_uuid" {
		t.Fatalf("missing organization = %v", err)
	}

	// The center is selectable on /mcp/user (center_staff holds mcp.connect).
	userInfo, err := f.svc.Consent(f.ctx, panelActor(f.user), f.request(t, c.ClientID, model.ResourceUser))
	if err != nil {
		t.Fatalf("user consent: %v", err)
	}
	if len(userInfo.Organizations) != 2 {
		t.Fatalf("user endpoint organizations = %+v, want center + dealer", userInfo.Organizations)
	}

	// The mcp module off: the organization is not offered.
	f.features.set(f.dealer.ID, false)
	info, err = f.svc.Consent(f.ctx, panelActor(f.user), id)
	if err != nil || len(info.Organizations) != 0 {
		t.Fatalf("module off consent = %+v, %v", info, err)
	}
	f.features.set(f.dealer.ID, true)

	q := f.approve(t, panelActor(f.user), id, &f.dealer.Uuid)
	if q.Get("code") == "" || q.Get("state") != "xyz" || q.Get("iss") != issuer {
		t.Fatalf("approve redirect query = %v", q)
	}
	if n := f.activityCount(t, model.ActionConsentApproved, f.user.ID); n != 1 {
		t.Fatalf("approved activity rows = %d", n)
	}
	// The request is consumed.
	_, err = f.svc.Decide(f.ctx, panelActor(f.user), id, model.DecideInput{Decision: model.DecisionApprove, OrganizationUUID: &f.dealer.Uuid})
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("second decision = %v", err)
	}
}

func TestDenyRedirectsWithAccessDenied(t *testing.T) {
	f := newConsentFixture(t)
	c := f.register(t)
	id := f.request(t, c.ClientID, model.ResourceDealer)

	if _, err := f.svc.Decide(f.ctx, panelActor(f.user), id, model.DecideInput{Decision: "maybe"}); err == nil {
		t.Fatal("unknown decision accepted")
	}
	out, err := f.svc.Decide(f.ctx, panelActor(f.user), id, model.DecideInput{Decision: model.DecisionDeny})
	if err != nil {
		t.Fatalf("deny: %v", err)
	}
	u, _ := url.Parse(out.RedirectURL)
	q := u.Query()
	if !strings.HasPrefix(out.RedirectURL, f.redirect+"?") || q.Get("error") != model.ErrAccessDenied || q.Get("state") != "xyz" || q.Get("iss") != issuer || q.Get("code") != "" {
		t.Fatalf("deny redirect = %q", out.RedirectURL)
	}
	if n := f.activityCount(t, model.ActionConsentDenied, f.user.ID); n != 1 {
		t.Fatalf("denied activity rows = %d", n)
	}
	if _, err := f.svc.Consent(f.ctx, panelActor(f.user), id); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("denied request still pending: %v", err)
	}
	var grants int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM oauth_grants WHERE user_id = $1`, f.user.ID).Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("deny created %d grants (%v)", grants, err)
	}
}

func TestRevokeGrantMakesAccessToken401(t *testing.T) {
	f := newConsentFixture(t)
	c := f.register(t)
	code := f.approve(t, panelActor(f.user), f.request(t, c.ClientID, model.ResourceDealer), &f.dealer.Uuid).Get("code")
	tok, err := f.exchange(c.ClientID, code, model.ResourceDealer, verifier)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceDealer); err != nil {
		t.Fatalf("token before revoke: %v", err)
	}
	// A second family (refresh rotation keeps it; a new consent adds one).
	code2 := f.approve(t, panelActor(f.user), f.request(t, c.ClientID, model.ResourceDealer), &f.dealer.Uuid).Get("code")
	tok2, err := f.exchange(c.ClientID, code2, model.ResourceDealer, verifier)
	if err != nil {
		t.Fatalf("exchange 2: %v", err)
	}

	filter, _ := usecase.ParseGrantListFilter(url.Values{})
	grants, total, err := f.svc.ListGrants(f.ctx, f.user.ID, filter)
	if err != nil || total != 1 || len(grants) != 1 || grants[0].OrganizationUUID != f.dealer.Uuid || grants[0].Realm != model.RealmDealer {
		t.Fatalf("grants = %+v total %d, %v", grants, total, err)
	}
	// Another user cannot revoke it.
	other := f.newUser(t, "other")
	if err := f.svc.RevokeGrant(f.ctx, panelActor(other), grants[0].UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign revoke = %v", err)
	}

	if err := f.svc.RevokeGrant(f.ctx, panelActor(f.user), grants[0].UUID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for _, at := range []string{tok.AccessToken, tok2.AccessToken} {
		if _, err := f.svc.ValidateAccessToken(f.ctx, at, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
			t.Fatalf("token after revoke = %v, want ErrUnauthorized (401)", err)
		}
	}
	_, err = f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{GrantType: model.GrantRefreshToken, ClientID: c.ClientID, RefreshToken: tok.RefreshToken})
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
	if n := f.activityCount(t, model.ActionGrantRevoked, f.user.ID); n != 1 {
		t.Fatalf("revoke activity rows = %d", n)
	}
	if _, total, _ := f.svc.ListGrants(f.ctx, f.user.ID, filter); total != 0 {
		t.Fatalf("revoked grant still listed (%d)", total)
	}
	if err := f.svc.RevokeGrant(f.ctx, panelActor(f.user), grants[0].UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("second revoke = %v", err)
	}

	// Consenting again reconnects with fresh tokens; the old ones stay dead.
	code3 := f.approve(t, panelActor(f.user), f.request(t, c.ClientID, model.ResourceDealer), &f.dealer.Uuid).Get("code")
	tok3, err := f.exchange(c.ClientID, code3, model.ResourceDealer, verifier)
	if err != nil {
		t.Fatalf("exchange 3: %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok3.AccessToken, model.ResourceDealer); err != nil {
		t.Fatalf("reconnected token: %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("old token revived by re-consent: %v", err)
	}
}

func TestRevokeClientRevokesEverything(t *testing.T) {
	f := newConsentFixture(t)
	c := f.register(t)
	tok := f.consentTokens(t, c.ClientID)
	pending := f.request(t, c.ClientID, model.ResourceDealer)

	filter, _ := usecase.ParseClientListFilter(url.Values{"q": {c.ClientID}})
	clients, total, err := f.svc.ListClients(f.ctx, filter)
	if err != nil || total != 1 || clients[0].Status != model.ClientStatusActive || clients[0].ActiveGrants != 1 {
		t.Fatalf("clients = %+v total %d, %v", clients, total, err)
	}
	admin := panelActor(f.user)
	if err := f.svc.RevokeClient(f.ctx, admin, uuid.New()); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("unknown client revoke = %v", err)
	}
	if err := f.svc.RevokeClient(f.ctx, admin, clients[0].UUID); err != nil {
		t.Fatalf("revoke client: %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("token of a revoked client = %v", err)
	}
	if _, err := f.svc.Consent(f.ctx, admin, pending); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("pending request of a revoked client = %v", err)
	}
	var live int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM oauth_tokens WHERE client_id = $1 AND revoked_at IS NULL`, c.ClientID).Scan(&live); err != nil || live != 0 {
		t.Fatalf("live tokens after client revoke = %d (%v)", live, err)
	}
	filter.Statuses = []string{model.ClientStatusRevoked}
	clients, _, _ = f.svc.ListClients(f.ctx, filter)
	if len(clients) != 1 || clients[0].Status != model.ClientStatusRevoked || clients[0].RevokedAt == nil || clients[0].ActiveGrants != 0 {
		t.Fatalf("revoked client = %+v", clients)
	}
	if n := f.activityCount(t, model.ActionClientRevoked, f.user.ID); n != 1 {
		t.Fatalf("client revoke activity rows = %d", n)
	}
}

func (f *consentFixture) consentTokens(t *testing.T, clientID string) model.TokenResponse {
	t.Helper()
	code := f.approve(t, panelActor(f.user), f.request(t, clientID, model.ResourceDealer), &f.dealer.Uuid).Get("code")
	tok, err := f.exchange(clientID, code, model.ResourceDealer, verifier)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	return tok
}

func TestGrantListContract(t *testing.T) {
	f := newConsentFixture(t)
	names := []string{"Bravo", "Alpha", "Charlie"}
	var grantIDs []uuid.UUID
	for i, name := range names {
		c, err := f.svc.Register(f.ctx, "203.0.113.1", model.RegisterInput{ClientName: name, RedirectURIs: []string{f.redirect}})
		if err != nil {
			t.Fatal(err)
		}
		resource := model.ResourceDealer
		if i == 2 {
			resource = model.ResourceUser
		}
		f.approve(t, panelActor(f.user), f.request(t, c.ClientID, resource), &f.dealer.Uuid)
		var g uuid.UUID
		if err := f.tx.QueryRow(f.ctx, `SELECT uuid FROM oauth_grants WHERE client_id = $1`, c.ClientID).Scan(&g); err != nil {
			t.Fatal(err)
		}
		grantIDs = append(grantIDs, g)
	}
	// Distinct created_at / last_used_at values (one transaction shares NOW()).
	for i, g := range grantIDs {
		if _, err := f.tx.Exec(f.ctx, `UPDATE oauth_grants SET created_at = NOW() - make_interval(hours => $2::int), last_used_at = CASE WHEN $2::int = 1 THEN NULL ELSE NOW() - make_interval(mins => $2::int) END WHERE uuid = $1`, g, i+1); err != nil {
			t.Fatal(err)
		}
	}
	list := func(params url.Values) []string {
		t.Helper()
		fl, err := usecase.ParseGrantListFilter(params)
		if err != nil {
			t.Fatalf("parse %v: %v", params, err)
		}
		items, total, err := f.svc.ListGrants(f.ctx, f.user.ID, fl)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it.ClientName)
		}
		if int(total) < len(out) {
			t.Fatalf("total %d < items %d", total, len(out))
		}
		return out
	}
	want := func(params url.Values, exp ...string) {
		t.Helper()
		if got := list(params); strings.Join(got, ",") != strings.Join(exp, ",") {
			t.Fatalf("%v = %v, want %v", params, got, exp)
		}
	}
	want(url.Values{}, "Bravo", "Alpha", "Charlie") // default -created_at
	want(url.Values{"sort": {"client_name"}}, "Alpha", "Bravo", "Charlie")
	want(url.Values{"sort": {"-client_name"}}, "Charlie", "Bravo", "Alpha")
	want(url.Values{"sort": {"created_at"}}, "Charlie", "Alpha", "Bravo")
	// last_used_at: Bravo never used (NULLS LAST both ways).
	want(url.Values{"sort": {"last_used_at"}}, "Charlie", "Alpha", "Bravo")
	want(url.Values{"sort": {"-last_used_at"}}, "Alpha", "Charlie", "Bravo")
	want(url.Values{"realm": {"user"}}, "Charlie")
	want(url.Values{"realm": {"dealer,user"}, "sort": {"client_name"}}, "Alpha", "Bravo", "Charlie")
	want(url.Values{"q": {"alp"}}, "Alpha")
	want(url.Values{"sort": {"client_name"}, "limit": {"1"}, "offset": {"1"}}, "Bravo")

	for _, bad := range []url.Values{{"sort": {"uuid"}}, {"realm": {"admin"}}} {
		_, err := usecase.ParseGrantListFilter(bad)
		var ve *apiquery.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%v = %v, want validation error", bad, err)
		}
	}
	if _, err := usecase.ParseClientListFilter(url.Values{"sort": {"redirect_uris"}}); err == nil {
		t.Fatal("unknown client sort accepted")
	}
}

type fakeToolCounter struct {
	calls []string
	err   error
}

func (c *fakeToolCounter) CountTools(_ context.Context, userID, orgID, brandID int64, resource string) (int, error) {
	c.calls = append(c.calls, fmt.Sprintf("%d/%d/%d%s", userID, orgID, brandID, resource))
	return int(orgID%7) + 3, c.err
}

// TEC-403: every selectable organization carries the tool count of the
// requested endpoint there; without a counter it is null.
func TestConsentToolCount(t *testing.T) {
	f := newConsentFixture(t)
	c := f.register(t)
	id := f.request(t, c.ClientID, model.ResourceDealer)

	info, err := f.svc.Consent(f.ctx, panelActor(f.user), id)
	if err != nil || len(info.Organizations) != 1 || info.Organizations[0].ToolCount != nil {
		t.Fatalf("without counter: %+v %v", info, err)
	}
	counter := &fakeToolCounter{}
	f.svc.SetToolCounter(counter)
	info, err = f.svc.Consent(f.ctx, panelActor(f.user), id)
	if err != nil || len(info.Organizations) != 1 {
		t.Fatalf("with counter: %+v %v", info, err)
	}
	want := int(f.dealer.ID%7) + 3
	if got := info.Organizations[0].ToolCount; got == nil || *got != want {
		t.Fatalf("tool_count = %v, want %d", got, want)
	}
	if len(counter.calls) != 1 || counter.calls[0] != fmt.Sprintf("%d/%d/%d%s", f.user.ID, f.dealer.ID, f.dealer.BrandID, model.ResourceDealer) {
		t.Fatalf("counter calls = %v", counter.calls)
	}
	counter.err = errors.New("boom")
	if _, err := f.svc.Consent(f.ctx, panelActor(f.user), id); err == nil {
		t.Fatal("counter error swallowed")
	}
}
