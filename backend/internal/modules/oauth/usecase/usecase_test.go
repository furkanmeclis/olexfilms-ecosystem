package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// TEC-400 acceptance. Every database test runs in one rolled-back
// transaction; the mcp module state comes from a fake feature checker.

const issuer = "https://olexfilms.test"

const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk-tec400-verifier"

type fakeFeatures struct {
	mu  sync.Mutex
	off map[int64]bool
}

func (f *fakeFeatures) Enabled(_ context.Context, orgID int64, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key != features.ModuleMCP {
		return false, fmt.Errorf("unexpected module %q", key)
	}
	return !f.off[orgID], nil
}

func (f *fakeFeatures) set(orgID int64, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.off[orgID] = !on
}

type fixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	svc      *usecase.Service
	features *fakeFeatures
	dealer   db.Organization
	user     db.User
	redirect string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &fixture{ctx: ctx, tx: tx, q: db.New(tx), features: &fakeFeatures{off: map[int64]bool{}}, redirect: "https://client.example/callback"}
	mr := miniredis.RunT(t)
	limiter := ratelimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")
	f.svc = usecase.New(tx, f.features, limiter, issuer, nil)

	brand, err := f.q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := f.q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	if f.dealer, err = f.q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "tec400-dealer-" + suffix, Name: "TEC400 dealer", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
		BrandID: brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	}); err != nil {
		t.Fatalf("dealer: %v", err)
	}
	if f.user, err = f.q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "TEC400", Surname: "owner", Status: "active",
		Email: pgtype.Text{String: "tec400-" + suffix + "@example.test", Valid: true},
	}); err != nil {
		t.Fatalf("user: %v", err)
	}
	return f
}

func (f *fixture) register(t *testing.T) model.Client {
	t.Helper()
	c, err := f.svc.Register(f.ctx, "203.0.113.1", model.RegisterInput{ClientName: "Claude", RedirectURIs: []string{f.redirect}})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return c
}

func (f *fixture) authorizeInput(clientID, resource string) model.AuthorizeInput {
	return model.AuthorizeInput{
		ClientID: clientID, RedirectURI: f.redirect, ResponseType: "code",
		CodeChallenge: usecase.S256(verifier), CodeChallengeMethod: "S256",
		State: "xyz", Resource: issuer + resource,
	}
}

// code runs authorize + consent for resource and returns the code.
func (f *fixture) code(t *testing.T, clientID, resource string) string {
	t.Helper()
	req, err := f.svc.CreateAuthRequest(f.ctx, f.authorizeInput(clientID, resource))
	if err != nil {
		t.Fatalf("auth request: %v", err)
	}
	redirect, err := f.svc.IssueCode(f.ctx, model.IssueCodeInput{
		RequestID: req.ID, UserID: f.user.ID, OrganizationID: f.dealer.ID, BrandID: f.dealer.BrandID,
	})
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	u, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("redirect: %v", err)
	}
	if got := u.Query().Get("state"); got != "xyz" {
		t.Fatalf("state = %q", got)
	}
	if got := u.Query().Get("iss"); got != issuer {
		t.Fatalf("iss = %q", got)
	}
	return u.Query().Get("code")
}

func (f *fixture) exchange(clientID, code, resource, codeVerifier string) (model.TokenResponse, error) {
	return f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{
		GrantType: model.GrantAuthorizationCode, ClientID: clientID, Code: code,
		RedirectURI: f.redirect, CodeVerifier: codeVerifier, Resource: issuer + resource,
	})
}

func (f *fixture) tokens(t *testing.T, clientID, resource string) model.TokenResponse {
	t.Helper()
	out, err := f.exchange(clientID, f.code(t, clientID, resource), resource, verifier)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	return out
}

// wantOAuthError returns a copy (not an error value) for field checks.
func wantOAuthError(t *testing.T, err error, status int, code string) model.Error {
	t.Helper()
	var oe *model.Error
	if !errors.As(err, &oe) {
		t.Fatalf("want OAuth error %s, got %v", code, err)
	}
	if oe.Status != status || oe.Code != code {
		t.Fatalf("want %d %s, got %d %s (%s)", status, code, oe.Status, oe.Code, oe.Description)
	}
	return *oe
}

func TestAuthorizeRejectsPKCEPlain(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	in := f.authorizeInput(c.ClientID, model.ResourceDealer)
	in.CodeChallengeMethod = "plain"
	in.CodeChallenge = verifier
	_, err := f.svc.CreateAuthRequest(f.ctx, in)
	oe := wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidRequest)
	if !oe.Redirectable {
		t.Fatal("a PKCE error of a known client is returned to its redirect_uri")
	}
	in.CodeChallengeMethod = ""
	_, err = f.svc.CreateAuthRequest(f.ctx, in)
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidRequest)
}

func TestAuthorizeRequiresResource(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	for _, res := range []string{"", "/mcp/dealer", "https://evil.example/mcp/dealer", issuer + "/mcp/other"} {
		in := f.authorizeInput(c.ClientID, model.ResourceDealer)
		in.Resource = res
		_, err := f.svc.CreateAuthRequest(f.ctx, in)
		wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidTarget)
	}
	// An unregistered redirect_uri is never redirected to.
	in := f.authorizeInput(c.ClientID, model.ResourceDealer)
	in.RedirectURI = "https://evil.example/cb"
	_, err := f.svc.CreateAuthRequest(f.ctx, in)
	if oe := wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidRequest); oe.Redirectable {
		t.Fatal("an unregistered redirect_uri must not be redirectable")
	}
}

func TestTokenWrongVerifierInvalidGrant(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	code := f.code(t, c.ClientID, model.ResourceDealer)
	_, err := f.exchange(c.ClientID, code, model.ResourceDealer, strings.Repeat("a", 43))
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
	// A failed verification does not burn the code for the real client.
	if _, err := f.exchange(c.ClientID, code, model.ResourceDealer, verifier); err != nil {
		t.Fatalf("exchange with the right verifier: %v", err)
	}
}

func TestTokenResourceMustMatchCode(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	code := f.code(t, c.ClientID, model.ResourceDealer)
	_, err := f.exchange(c.ClientID, code, model.ResourceUser, verifier)
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidTarget)
	_, err = f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{
		GrantType: model.GrantAuthorizationCode, ClientID: c.ClientID, Code: code,
		RedirectURI: f.redirect, CodeVerifier: verifier,
	})
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidTarget)
}

func TestTokenCodeReuseRevokesFamily(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	code := f.code(t, c.ClientID, model.ResourceDealer)
	first, err := f.exchange(c.ClientID, code, model.ResourceDealer, verifier)
	if err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	if first.TokenType != "Bearer" || first.ExpiresIn != 3600 || first.Scope != model.ScopeMCP {
		t.Fatalf("token response = %+v", first)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, first.AccessToken, model.ResourceDealer); err != nil {
		t.Fatalf("access token before replay: %v", err)
	}
	_, err = f.exchange(c.ClientID, code, model.ResourceDealer, verifier)
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
	if _, err := f.svc.ValidateAccessToken(f.ctx, first.AccessToken, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("access token after code replay: want ErrUnauthorized, got %v", err)
	}
	_, err = f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{GrantType: model.GrantRefreshToken, ClientID: c.ClientID, RefreshToken: first.RefreshToken})
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
}

func TestTokenRefreshRotation(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	first := f.tokens(t, c.ClientID, model.ResourceDealer)
	refresh := func(token string) (model.TokenResponse, error) {
		return f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{
			GrantType: model.GrantRefreshToken, ClientID: c.ClientID, RefreshToken: token, Resource: issuer + model.ResourceDealer,
		})
	}
	second, err := refresh(first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
		t.Fatal("refresh must rotate both tokens")
	}
	// The old access token retires with the old refresh token.
	if _, err := f.svc.ValidateAccessToken(f.ctx, first.AccessToken, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("old access token: want ErrUnauthorized, got %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, second.AccessToken, model.ResourceDealer); err != nil {
		t.Fatalf("new access token: %v", err)
	}
	// The rotated refresh token is invalid; using it revokes the family.
	_, err = refresh(first.RefreshToken)
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
	if _, err := f.svc.ValidateAccessToken(f.ctx, second.AccessToken, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("new access token after reuse: want ErrUnauthorized, got %v", err)
	}
	_, err = refresh(second.RefreshToken)
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
}

func TestRefreshRejectsOtherClientAndResource(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	other := f.register(t)
	tok := f.tokens(t, c.ClientID, model.ResourceDealer)
	_, err := f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{GrantType: model.GrantRefreshToken, ClientID: other.ClientID, RefreshToken: tok.RefreshToken})
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
	_, err = f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{GrantType: model.GrantRefreshToken, ClientID: c.ClientID, RefreshToken: tok.RefreshToken, Resource: issuer + model.ResourceUser})
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidTarget)
}

func TestAccessTokenAudience(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	tok := f.tokens(t, c.ClientID, model.ResourceDealer)
	got, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceDealer)
	if err != nil {
		t.Fatalf("dealer endpoint: %v", err)
	}
	if got.UserID != f.user.ID || got.OrganizationID != f.dealer.ID || got.BrandID != f.dealer.BrandID || got.Resource != model.ResourceDealer {
		t.Fatalf("token = %+v", got)
	}
	for _, res := range []string{model.ResourceUser, model.ResourceCustomer} {
		if _, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, res); !errors.Is(err, usecase.ErrUnauthorized) {
			t.Fatalf("%s: want ErrUnauthorized, got %v", res, err)
		}
	}
	// The refresh token is not a Bearer token.
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok.RefreshToken, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("refresh token as bearer: want ErrUnauthorized, got %v", err)
	}
}

func TestMCPModuleOffNoToken(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	code := f.code(t, c.ClientID, model.ResourceDealer)
	f.features.set(f.dealer.ID, false)
	_, err := f.exchange(c.ClientID, code, model.ResourceDealer, verifier)
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
	var n int
	if err := f.tx.QueryRow(f.ctx, `SELECT count(*) FROM oauth_tokens WHERE organization_id = $1`, f.dealer.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("tokens issued for a disabled org: %d", n)
	}
	// Consent is refused as well.
	req, err := f.svc.CreateAuthRequest(f.ctx, f.authorizeInput(c.ClientID, model.ResourceDealer))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.IssueCode(f.ctx, model.IssueCodeInput{RequestID: req.ID, UserID: f.user.ID, OrganizationID: f.dealer.ID, BrandID: f.dealer.BrandID})
	wantOAuthError(t, err, http.StatusForbidden, model.ErrAccessDenied)

	// Switching the module off later blocks validation and refresh.
	f.features.set(f.dealer.ID, true)
	tok, err := f.exchange(c.ClientID, code, model.ResourceDealer, verifier)
	if err != nil {
		t.Fatalf("exchange with the module on: %v", err)
	}
	f.features.set(f.dealer.ID, false)
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceDealer); !errors.Is(err, usecase.ErrFeatureDisabled) {
		t.Fatalf("validation with the module off: want ErrFeatureDisabled, got %v", err)
	}
	_, err = f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{GrantType: model.GrantRefreshToken, ClientID: c.ClientID, RefreshToken: tok.RefreshToken})
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidGrant)
}

func TestRevokeRevokesFamily(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	tok := f.tokens(t, c.ClientID, model.ResourceUser)
	if err := f.svc.Revoke(f.ctx, "203.0.113.1", c.ClientID, tok.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceUser); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("after revoke: want ErrUnauthorized, got %v", err)
	}
	if err := f.svc.Revoke(f.ctx, "203.0.113.1", c.ClientID, "unknown"); err != nil {
		t.Fatalf("unknown token: %v", err)
	}
}

func TestRevokedClientCannotExchange(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	tok := f.tokens(t, c.ClientID, model.ResourceDealer)
	if _, err := f.tx.Exec(f.ctx, `UPDATE oauth_clients SET revoked_at = NOW() WHERE client_id = $1`, c.ClientID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ValidateAccessToken(f.ctx, tok.AccessToken, model.ResourceDealer); !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("revoked client token: want ErrUnauthorized, got %v", err)
	}
	_, err := f.svc.Exchange(f.ctx, "203.0.113.1", model.TokenInput{GrantType: model.GrantRefreshToken, ClientID: c.ClientID, RefreshToken: tok.RefreshToken})
	wantOAuthError(t, err, http.StatusUnauthorized, model.ErrInvalidClient)
}

func TestRegisterValidation(t *testing.T) {
	f := newFixture(t)
	for _, in := range []model.RegisterInput{
		{RedirectURIs: nil},
		{RedirectURIs: []string{"http://client.example/cb"}},
		{RedirectURIs: []string{"https://client.example/cb#frag"}},
	} {
		_, err := f.svc.Register(f.ctx, "203.0.113.9", in)
		wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidRedirectURI)
	}
	_, err := f.svc.Register(f.ctx, "203.0.113.9", model.RegisterInput{RedirectURIs: []string{"https://c.example/cb"}, TokenEndpointAuthMethod: "client_secret_basic"})
	wantOAuthError(t, err, http.StatusBadRequest, model.ErrInvalidClientMetadata)
	c, err := f.svc.Register(f.ctx, "203.0.113.9", model.RegisterInput{RedirectURIs: []string{"http://127.0.0.1:33418/callback"}})
	if err != nil {
		t.Fatalf("loopback redirect: %v", err)
	}
	if !strings.HasPrefix(c.ClientID, "mcp_") || c.ClientName != "MCP client" {
		t.Fatalf("client = %+v", c)
	}
}

func TestRegisterRateLimit(t *testing.T) {
	f := newFixture(t)
	in := model.RegisterInput{ClientName: "Claude", RedirectURIs: []string{f.redirect}}
	for i := 1; i <= model.RegisterPerHour; i++ {
		if _, err := f.svc.Register(f.ctx, "198.51.100.7", in); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	_, err := f.svc.Register(f.ctx, "198.51.100.7", in)
	oe := wantOAuthError(t, err, http.StatusTooManyRequests, model.ErrSlowDown)
	if oe.RetryAfter <= 0 {
		t.Fatal("429 must carry Retry-After")
	}
	// Another IP has its own budget.
	if _, err := f.svc.Register(f.ctx, "198.51.100.8", in); err != nil {
		t.Fatalf("other ip: %v", err)
	}
}

func TestCleanupRemovesExpiredRows(t *testing.T) {
	f := newFixture(t)
	c := f.register(t)
	tok := f.tokens(t, c.ClientID, model.ResourceDealer)
	if _, err := f.svc.CreateAuthRequest(f.ctx, f.authorizeInput(c.ClientID, model.ResourceDealer)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE oauth_auth_requests SET expires_at = NOW() - INTERVAL '1 minute' WHERE client_id = $1`,
		`UPDATE oauth_codes SET expires_at = NOW() - INTERVAL '2 days' WHERE client_id = $1`,
		`UPDATE oauth_tokens SET expires_at = NOW() - INTERVAL '2 days' WHERE client_id = $1 AND kind = 'access'`,
	} {
		if _, err := f.tx.Exec(f.ctx, q, c.ClientID); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.svc.Cleanup(f.ctx); err != nil {
		t.Fatal(err)
	}
	var reqs, codes, access, refresh int
	if err := f.tx.QueryRow(f.ctx, `SELECT
		(SELECT count(*) FROM oauth_auth_requests WHERE client_id = $1),
		(SELECT count(*) FROM oauth_codes WHERE client_id = $1),
		(SELECT count(*) FROM oauth_tokens WHERE client_id = $1 AND kind = 'access'),
		(SELECT count(*) FROM oauth_tokens WHERE client_id = $1 AND kind = 'refresh')`, c.ClientID).Scan(&reqs, &codes, &access, &refresh); err != nil {
		t.Fatal(err)
	}
	if reqs != 0 || codes != 0 || access != 0 || refresh != 1 {
		t.Fatalf("after cleanup: requests=%d codes=%d access=%d refresh=%d", reqs, codes, access, refresh)
	}
	_ = tok
}
