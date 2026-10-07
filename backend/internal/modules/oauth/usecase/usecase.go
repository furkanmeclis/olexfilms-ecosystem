// Package usecase is the MCP OAuth 2.1 authorization server (TEC-400,
// F4-03a): dynamic client registration (RFC 7591), authorization code +
// PKCE S256 exchange, rotating refresh tokens with family revocation on
// reuse, token revocation (RFC 7009) and resource-bound (RFC 8707) access
// token validation for the MCP endpoints.
//
// The authorize endpoint and the consent decision (F4-03b) use
// CreateAuthRequest and IssueCode; the MCP server (F4-03c) uses
// ValidateAccessToken.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// maxRedirectURIs bounds a registration (chk_oauth_clients_redirect_uris).
const maxRedirectURIs = 10

// clientIDPrefix marks the client ids this server issues.
const clientIDPrefix = "mcp_"

var (
	// ErrUnauthorized: the Bearer token is unknown, expired, revoked, of a
	// revoked grant or client, of an inactive user, or of another resource.
	// The MCP endpoint answers 401 with WWW-Authenticate.
	ErrUnauthorized = errors.New("oauth: invalid access token")
	// ErrFeatureDisabled: the token is valid but the mcp module is off for
	// its organization. The MCP endpoint answers 403.
	ErrFeatureDisabled = errors.New("oauth: mcp module is disabled for the organization")
)

// DB is the connection the service runs on: a pool in production, a test
// transaction in tests (Begin opens a savepoint there).
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// FeatureChecker reports whether a module is on for an organization
// (features.Service).
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// Limiter is the per-IP fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Service is the authorization server.
type Service struct {
	db       DB
	q        *db.Queries
	features FeatureChecker
	limiter  Limiter
	access   AccessChecker
	tools    ToolCounter
	issuer   string
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service. issuer is the public origin clients reach (the
// frontend, which proxies /oauth/*, /.well-known/* and /mcp/* to Go).
func New(conn DB, featureChecker FeatureChecker, limiter Limiter, issuer string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		db: conn, q: db.New(conn), features: featureChecker, limiter: limiter,
		issuer: strings.TrimRight(issuer, "/"), log: log, now: time.Now,
	}
}

// SetClock overrides the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Issuer is the authorization server identifier (RFC 8414 issuer).
func (s *Service) Issuer() string { return s.issuer }

// ResourceURL is the absolute URI of an MCP endpoint path.
func (s *Service) ResourceURL(path string) string { return s.issuer + path }

// --- rate limits ---------------------------------------------------------------

func (s *Service) allow(ctx context.Context, action, ip string, limit int) error {
	if s.limiter == nil {
		return nil
	}
	ok, retry := s.limiter.Allow(ctx, "oauth_"+action, ip, limit, model.RateWindow)
	if ok {
		return nil
	}
	return &model.Error{
		Status: http.StatusTooManyRequests, Code: model.ErrSlowDown,
		Description: "too many requests, try again later", RetryAfter: retry,
	}
}

// AllowAuthorize applies the per-IP authorize limit (F4-03b endpoint).
func (s *Service) AllowAuthorize(ctx context.Context, ip string) error {
	return s.allow(ctx, "authorize", ip, model.AuthorizePerHour)
}

// --- helpers -------------------------------------------------------------------

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is the stored form of codes and tokens (SHA-256, hex).
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func serverError(err error) *model.Error {
	return &model.Error{Status: http.StatusInternalServerError, Code: model.ErrServerError, Description: "internal server error", Cause: err}
}

// validVerifier checks RFC 7636 §4.1 code_verifier syntax.
func validVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~':
		default:
			return false
		}
	}
	return true
}

// S256 is the PKCE S256 code challenge of a verifier.
func S256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// validRedirect accepts https URLs and loopback http URLs (native clients),
// without a fragment (RFC 6749 §3.1.2, OAuth 2.1 §2.3.1).
func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Host == "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

// resourcePath resolves an RFC 8707 resource indicator to an MCP endpoint
// path: an absolute URI on the issuer's origin naming one of the endpoints.
func (s *Service) resourcePath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.RawQuery != "" || !u.IsAbs() {
		return "", false
	}
	iss, err := url.Parse(s.issuer)
	if err != nil || !strings.EqualFold(u.Scheme, iss.Scheme) || !strings.EqualFold(u.Host, iss.Host) {
		return "", false
	}
	p := strings.TrimRight(u.Path, "/")
	if !model.IsResource(p) {
		return "", false
	}
	return p, true
}

// scopes resolves the requested scope string; only "mcp" exists.
func scopes(raw string) ([]string, bool) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return []string{model.ScopeMCP}, true
	}
	for _, f := range fields {
		if f != model.ScopeMCP {
			return nil, false
		}
	}
	return []string{model.ScopeMCP}, true
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// mcpEnabled reports whether the mcp module is on for the organization.
func (s *Service) mcpEnabled(ctx context.Context, orgID int64) (bool, error) {
	if s.features == nil {
		return false, errors.New("oauth: feature checker missing")
	}
	return s.features.Enabled(ctx, orgID, features.ModuleMCP)
}

// --- registration (RFC 7591) -----------------------------------------------------

// Register stores a public client. Only token_endpoint_auth_method none,
// the authorization_code and refresh_token grants and the code response type
// are accepted.
func (s *Service) Register(ctx context.Context, ip string, in model.RegisterInput) (model.Client, error) {
	if err := s.allow(ctx, "register", ip, model.RegisterPerHour); err != nil {
		return model.Client{}, err
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > maxRedirectURIs {
		return model.Client{}, model.BadRequest(model.ErrInvalidRedirectURI, "1 to 10 redirect_uris are required")
	}
	for _, u := range in.RedirectURIs {
		if !validRedirect(u) {
			return model.Client{}, model.BadRequest(model.ErrInvalidRedirectURI, "redirect URIs must be https (or http on a loopback host) without a fragment")
		}
	}
	if in.TokenEndpointAuthMethod != "" && in.TokenEndpointAuthMethod != "none" {
		return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "only public clients (token_endpoint_auth_method=none) are supported")
	}
	for _, g := range in.GrantTypes {
		if g != model.GrantAuthorizationCode && g != model.GrantRefreshToken {
			return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "only the authorization_code and refresh_token grants are supported")
		}
	}
	for _, rt := range in.ResponseTypes {
		if rt != "code" {
			return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "only response_type code is supported")
		}
	}
	if _, ok := scopes(in.Scope); !ok {
		return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "only the mcp scope is supported")
	}
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	var createdIP pgtype.Text
	if ip = strings.TrimSpace(ip); ip != "" {
		if len(ip) > 64 {
			ip = ip[:64]
		}
		createdIP = pgtype.Text{String: ip, Valid: true}
	}
	c, err := s.q.CreateOAuthClient(ctx, db.CreateOAuthClientParams{
		ClientID: clientIDPrefix + randomToken(18), ClientName: name,
		RedirectUris: in.RedirectURIs, CreatedIp: createdIP,
	})
	if err != nil {
		return model.Client{}, fmt.Errorf("oauth: register client: %w", err)
	}
	return model.Client{ClientID: c.ClientID, ClientName: c.ClientName, RedirectURIs: c.RedirectUris, IssuedAt: c.CreatedAt.Time}, nil
}

// --- authorization request -------------------------------------------------------

// CreateAuthRequest validates an authorize request and stores it for
// consent (15 minutes). An unknown client or an unregistered redirect_uri is
// a non-redirectable error; every later error is Redirectable. PKCE is
// required with code_challenge_method S256 (plain is rejected) and resource
// must name one of the MCP endpoints.
func (s *Service) CreateAuthRequest(ctx context.Context, in model.AuthorizeInput) (model.AuthRequest, error) {
	client, err := s.q.GetOAuthClient(ctx, in.ClientID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && client.RevokedAt.Valid) {
		return model.AuthRequest{}, model.BadRequest(model.ErrInvalidClient, "unknown client_id")
	}
	if err != nil {
		return model.AuthRequest{}, fmt.Errorf("oauth: client: %w", err)
	}
	if !slices.Contains(client.RedirectUris, in.RedirectURI) {
		return model.AuthRequest{}, model.BadRequest(model.ErrInvalidRequest, "redirect_uri is not registered for this client")
	}
	fail := func(code, desc string) error {
		e := model.BadRequest(code, desc)
		e.Redirectable = true
		return e
	}
	if in.ResponseType != "code" {
		return model.AuthRequest{}, fail(model.ErrUnsupportedResponseType, "only response_type=code is supported")
	}
	if in.CodeChallengeMethod != "S256" {
		return model.AuthRequest{}, fail(model.ErrInvalidRequest, "PKCE with code_challenge_method=S256 is required")
	}
	if n := len(in.CodeChallenge); n != 43 {
		return model.AuthRequest{}, fail(model.ErrInvalidRequest, "code_challenge must be a base64url SHA-256 digest")
	}
	resource, ok := s.resourcePath(in.Resource)
	if !ok {
		return model.AuthRequest{}, fail(model.ErrInvalidTarget, "resource must be one of the MCP endpoint URLs")
	}
	sc, ok := scopes(in.Scope)
	if !ok {
		return model.AuthRequest{}, fail(model.ErrInvalidScope, "only the mcp scope is supported")
	}
	state := pgtype.Text{String: in.State, Valid: in.State != ""}
	req, err := s.q.CreateOAuthAuthRequest(ctx, db.CreateOAuthAuthRequestParams{
		ClientID: client.ClientID, RedirectUri: in.RedirectURI, CodeChallenge: in.CodeChallenge,
		State: state, Scopes: sc, Resource: resource, ExpiresAt: ts(s.now().Add(model.AuthRequestTTL)),
	})
	if err != nil {
		return model.AuthRequest{}, fmt.Errorf("oauth: auth request: %w", err)
	}
	return model.AuthRequest{
		ID: req.ID, ClientID: client.ClientID, ClientName: client.ClientName, RedirectURI: req.RedirectUri,
		State: in.State, Resource: resource, ExpiresAt: req.ExpiresAt.Time,
	}, nil
}

// GetAuthRequest returns a pending (unexpired) request; pgx.ErrNoRows when
// missing.
func (s *Service) GetAuthRequest(ctx context.Context, id uuid.UUID) (model.AuthRequest, error) {
	r, err := s.q.GetOAuthAuthRequest(ctx, id)
	if err != nil {
		return model.AuthRequest{}, err
	}
	return model.AuthRequest{
		ID: r.ID, ClientID: r.ClientID, ClientName: r.ClientName, RedirectURI: r.RedirectUri,
		State: r.State.String, Resource: r.Resource, ExpiresAt: r.ExpiresAt.Time,
	}, nil
}

// IssueCode approves a pending request: it claims the request, records the
// grant (connected app) and returns the client redirect carrying the code,
// state and iss (RFC 9207). The caller has checked realm, membership and
// permissions. The mcp module must be on for the organization.
func (s *Service) IssueCode(ctx context.Context, in model.IssueCodeInput) (string, error) {
	return s.issueCode(ctx, in, nil)
}

// issueCode is IssueCode with an optional hook that runs in the same
// transaction once the grant exists (the consent decision's activity row).
func (s *Service) issueCode(ctx context.Context, in model.IssueCodeInput, after func(q *db.Queries, grant db.OauthGrant) error) (string, error) {
	on, err := s.mcpEnabled(ctx, in.OrganizationID)
	if err != nil {
		return "", err
	}
	if !on {
		return "", &model.Error{Status: http.StatusForbidden, Code: model.ErrAccessDenied, Description: "the mcp module is not enabled for this organization"}
	}
	code := randomToken(32)
	var redirect string
	err = s.inTx(ctx, func(q *db.Queries) error {
		req, err := q.GetOAuthAuthRequest(ctx, in.RequestID)
		if err != nil {
			return err
		}
		n, err := q.DeleteOAuthAuthRequest(ctx, in.RequestID)
		if err != nil {
			return err
		}
		if n == 0 {
			return pgx.ErrNoRows
		}
		grant, err := q.UpsertOAuthGrant(ctx, db.UpsertOAuthGrantParams{
			UserID: in.UserID, ClientID: req.ClientID, OrganizationID: in.OrganizationID,
			BrandID: in.BrandID, Resource: req.Resource, Scopes: req.Scopes,
		})
		if err != nil {
			return err
		}
		if after != nil {
			if err := after(q, grant); err != nil {
				return err
			}
		}
		if err := q.CreateOAuthCode(ctx, db.CreateOAuthCodeParams{
			CodeHash: HashToken(code), GrantID: grant.ID, ClientID: req.ClientID, UserID: in.UserID,
			OrganizationID: in.OrganizationID, BrandID: in.BrandID, RedirectUri: req.RedirectUri,
			CodeChallenge: req.CodeChallenge, Resource: req.Resource, Scopes: req.Scopes,
			ExpiresAt: ts(s.now().Add(model.CodeTTL)),
		}); err != nil {
			return err
		}
		u, err := url.Parse(req.RedirectUri)
		if err != nil {
			return err
		}
		v := u.Query()
		v.Set("code", code)
		if req.State.Valid && req.State.String != "" {
			v.Set("state", req.State.String)
		}
		v.Set("iss", s.issuer)
		u.RawQuery = v.Encode()
		redirect = u.String()
		return nil
	})
	if err != nil {
		return "", err
	}
	return redirect, nil
}

// --- token endpoint --------------------------------------------------------------

// Exchange serves the token endpoint: authorization_code (PKCE S256) and
// refresh_token (rotating). A replayed code or rotated refresh token revokes
// every token of its family.
func (s *Service) Exchange(ctx context.Context, ip string, in model.TokenInput) (model.TokenResponse, error) {
	if err := s.allow(ctx, "token", ip, model.TokenPerHour); err != nil {
		return model.TokenResponse{}, err
	}
	switch in.GrantType {
	case model.GrantAuthorizationCode:
		return s.exchangeCode(ctx, in)
	case model.GrantRefreshToken:
		return s.refresh(ctx, in)
	case "":
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidRequest, "grant_type is required")
	default:
		return model.TokenResponse{}, model.BadRequest(model.ErrUnsupportedGrantType, "use authorization_code or refresh_token")
	}
}

// activeClient loads a non-revoked client.
func (s *Service) activeClient(ctx context.Context, clientID string) error {
	if clientID == "" {
		return model.BadRequest(model.ErrInvalidRequest, "client_id is required")
	}
	c, err := s.q.GetOAuthClient(ctx, clientID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.RevokedAt.Valid) {
		return &model.Error{Status: http.StatusUnauthorized, Code: model.ErrInvalidClient, Description: "unknown or revoked client"}
	}
	if err != nil {
		return serverError(err)
	}
	return nil
}

// grantAllowed checks the user and the organization behind a grant at issue
// time: the user must be active and the mcp module on.
func (s *Service) grantAllowed(ctx context.Context, q *db.Queries, userID, orgID int64) error {
	u, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return model.BadRequest(model.ErrInvalidGrant, "the user of this grant is not available")
	}
	if u.Status != "active" {
		return model.BadRequest(model.ErrInvalidGrant, "the user of this grant is not active")
	}
	on, err := s.mcpEnabled(ctx, orgID)
	if err != nil {
		return serverError(err)
	}
	if !on {
		return model.BadRequest(model.ErrInvalidGrant, "the mcp module is not enabled for this organization")
	}
	return nil
}

func (s *Service) exchangeCode(ctx context.Context, in model.TokenInput) (model.TokenResponse, error) {
	if err := s.activeClient(ctx, in.ClientID); err != nil {
		return model.TokenResponse{}, err
	}
	if in.Code == "" || in.RedirectURI == "" || in.CodeVerifier == "" {
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidRequest, "code, redirect_uri and code_verifier are required")
	}
	resource, ok := s.resourcePath(in.Resource)
	if !ok {
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidTarget, "resource must be one of the MCP endpoint URLs")
	}
	codeHash := HashToken(in.Code)
	family := uuid.New()
	var out model.TokenResponse
	// Claim, check and issue in one transaction: a failed check leaves the
	// code unused, a successful exchange commits the claim with the tokens.
	err := s.inTx(ctx, func(q *db.Queries) error {
		code, err := q.UseOAuthCode(ctx, db.UseOAuthCodeParams{CodeHash: codeHash, Family: pgtype.UUID{Bytes: family, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return errCodeNotUsable
		}
		if err != nil {
			return serverError(err)
		}
		if code.ClientID != in.ClientID || code.RedirectUri != in.RedirectURI {
			return model.BadRequest(model.ErrInvalidGrant, "client_id or redirect_uri does not match the code")
		}
		if !validVerifier(in.CodeVerifier) || S256(in.CodeVerifier) != code.CodeChallenge {
			return model.BadRequest(model.ErrInvalidGrant, "PKCE verification failed")
		}
		if code.Resource != resource {
			return model.BadRequest(model.ErrInvalidTarget, "resource does not match the authorization")
		}
		if err := s.grantAllowed(ctx, q, code.UserID, code.OrganizationID); err != nil {
			return err
		}
		out, err = s.issue(ctx, q, issueArgs{
			family: family, grantID: code.GrantID, clientID: code.ClientID, userID: code.UserID,
			orgID: code.OrganizationID, brandID: code.BrandID, resource: code.Resource, scopes: code.Scopes,
		})
		return err
	})
	if errors.Is(err, errCodeNotUsable) {
		// A replayed code: revoke whatever was issued from it (RFC 6749 §4.1.2).
		if fam, ferr := s.q.GetUsedOAuthCodeFamily(ctx, codeHash); ferr == nil && fam.Valid {
			if rerr := s.q.RevokeOAuthFamily(ctx, uuid.UUID(fam.Bytes)); rerr != nil {
				s.log.Error("oauth_code_reuse_revoke_failed", "error", rerr)
			}
			s.log.Warn("oauth_code_reuse", "client_id", in.ClientID)
		}
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the code is invalid, expired or already used")
	}
	if err != nil {
		return model.TokenResponse{}, asOAuthError(err)
	}
	s.touchClient(ctx, in.ClientID)
	return out, nil
}

var (
	errCodeNotUsable = errors.New("oauth: code not usable")
	errRefreshRaced  = errors.New("oauth: refresh token already rotated")
)

func (s *Service) refresh(ctx context.Context, in model.TokenInput) (model.TokenResponse, error) {
	if err := s.activeClient(ctx, in.ClientID); err != nil {
		return model.TokenResponse{}, err
	}
	if in.RefreshToken == "" {
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidRequest, "refresh_token is required")
	}
	h := HashToken(in.RefreshToken)
	tok, err := s.q.GetActiveOAuthToken(ctx, db.GetActiveOAuthTokenParams{TokenHash: h, Kind: model.KindRefresh})
	if errors.Is(err, pgx.ErrNoRows) {
		// A rotated (revoked) refresh token used again: revoke the family.
		if old, oerr := s.q.GetOAuthTokenAnyState(ctx, db.GetOAuthTokenAnyStateParams{TokenHash: h, Kind: model.KindRefresh}); oerr == nil && old.RevokedAt.Valid {
			if rerr := s.q.RevokeOAuthFamily(ctx, old.Family); rerr != nil {
				s.log.Error("oauth_refresh_reuse_revoke_failed", "error", rerr)
			}
			s.log.Warn("oauth_refresh_reuse", "client_id", old.ClientID, "user_id", old.UserID)
		}
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the refresh token is invalid or expired")
	}
	if err != nil {
		return model.TokenResponse{}, serverError(err)
	}
	if tok.ClientID != in.ClientID {
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the refresh token was issued to another client")
	}
	if in.Resource != "" {
		if p, ok := s.resourcePath(in.Resource); !ok || p != tok.Resource {
			return model.TokenResponse{}, model.BadRequest(model.ErrInvalidTarget, "resource does not match the grant")
		}
	}
	if in.Scope != "" {
		if _, ok := scopes(in.Scope); !ok {
			return model.TokenResponse{}, model.BadRequest(model.ErrInvalidScope, "only the mcp scope is supported")
		}
	}
	var out model.TokenResponse
	raced := false
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := s.grantAllowed(ctx, q, tok.UserID, tok.OrganizationID); err != nil {
			return err
		}
		// Revoking is the claim: of two concurrent refreshes only one wins,
		// and the loser is treated as reuse.
		n, err := q.RevokeOAuthToken(ctx, tok.ID)
		if err != nil {
			return serverError(err)
		}
		if n == 0 {
			raced = true
			return errRefreshRaced
		}
		// Retire the old access token with the old refresh token.
		if err := q.RevokeOAuthFamily(ctx, tok.Family); err != nil {
			return serverError(err)
		}
		out, err = s.issue(ctx, q, issueArgs{
			family: tok.Family, grantID: tok.GrantID, clientID: tok.ClientID, userID: tok.UserID,
			orgID: tok.OrganizationID, brandID: tok.BrandID, resource: tok.Resource, scopes: tok.Scopes,
		})
		return err
	})
	if raced {
		if rerr := s.q.RevokeOAuthFamily(ctx, tok.Family); rerr != nil {
			s.log.Error("oauth_refresh_race_revoke_failed", "error", rerr)
		}
		s.log.Warn("oauth_refresh_race", "client_id", tok.ClientID, "user_id", tok.UserID)
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the refresh token is invalid or expired")
	}
	if err != nil {
		return model.TokenResponse{}, asOAuthError(err)
	}
	s.touchClient(ctx, in.ClientID)
	return out, nil
}

type issueArgs struct {
	family             uuid.UUID
	grantID, userID    int64
	orgID, brandID     int64
	clientID, resource string
	scopes             []string
}

func (s *Service) issue(ctx context.Context, q *db.Queries, a issueArgs) (model.TokenResponse, error) {
	access, refresh := randomToken(32), randomToken(32)
	now := s.now()
	for _, t := range []struct {
		raw, kind string
		ttl       time.Duration
	}{{access, model.KindAccess, model.AccessTTL}, {refresh, model.KindRefresh, model.RefreshTTL}} {
		if err := q.CreateOAuthToken(ctx, db.CreateOAuthTokenParams{
			TokenHash: HashToken(t.raw), Kind: t.kind, Family: a.family, GrantID: a.grantID,
			ClientID: a.clientID, UserID: a.userID, OrganizationID: a.orgID, BrandID: a.brandID,
			Resource: a.resource, Scopes: a.scopes, ExpiresAt: ts(now.Add(t.ttl)),
		}); err != nil {
			return model.TokenResponse{}, serverError(err)
		}
	}
	return model.TokenResponse{
		AccessToken: access, TokenType: "Bearer", ExpiresIn: int(model.AccessTTL.Seconds()),
		RefreshToken: refresh, Scope: strings.Join(a.scopes, " "),
	}, nil
}

func (s *Service) touchClient(ctx context.Context, clientID string) {
	if err := s.q.TouchOAuthClient(ctx, clientID); err != nil {
		s.log.Warn("oauth_client_touch_failed", "error", err)
	}
}

func asOAuthError(err error) error {
	var oe *model.Error
	if errors.As(err, &oe) {
		return oe
	}
	return serverError(err)
}

// --- revocation (RFC 7009) ---------------------------------------------------------

// Revoke revokes the family of an access or refresh token. Unknown tokens
// and tokens of another client are ignored (the endpoint always answers
// 200).
func (s *Service) Revoke(ctx context.Context, ip, clientID, token string) error {
	if err := s.allow(ctx, "revoke", ip, model.RevokePerHour); err != nil {
		return err
	}
	if token == "" {
		return model.BadRequest(model.ErrInvalidRequest, "token is required")
	}
	h := HashToken(token)
	for _, kind := range []string{model.KindAccess, model.KindRefresh} {
		t, err := s.q.GetOAuthTokenAnyState(ctx, db.GetOAuthTokenAnyStateParams{TokenHash: h, Kind: kind})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return serverError(err)
		}
		if clientID != "" && t.ClientID != clientID {
			return nil
		}
		if err := s.q.RevokeOAuthFamily(ctx, t.Family); err != nil {
			return serverError(err)
		}
	}
	return nil
}

// --- resource server ----------------------------------------------------------------

// ValidateAccessToken resolves a raw Bearer token for the MCP endpoint at
// resource (path, e.g. /mcp/dealer). The token must be live, of a live
// grant and client, of an active user and issued for that endpoint
// (audience); otherwise ErrUnauthorized. With the mcp module off for the
// token's organization it returns ErrFeatureDisabled.
func (s *Service) ValidateAccessToken(ctx context.Context, raw, resource string) (model.AccessToken, error) {
	if raw == "" {
		return model.AccessToken{}, ErrUnauthorized
	}
	t, err := s.q.GetActiveOAuthToken(ctx, db.GetActiveOAuthTokenParams{TokenHash: HashToken(raw), Kind: model.KindAccess})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AccessToken{}, ErrUnauthorized
	}
	if err != nil {
		return model.AccessToken{}, fmt.Errorf("oauth: token: %w", err)
	}
	if t.UserStatus != "active" || t.Resource != resource {
		return model.AccessToken{}, ErrUnauthorized
	}
	on, err := s.mcpEnabled(ctx, t.OrganizationID)
	if err != nil {
		return model.AccessToken{}, err
	}
	if !on {
		return model.AccessToken{}, ErrFeatureDisabled
	}
	if err := s.q.TouchOAuthToken(ctx, t.ID); err != nil {
		s.log.Warn("oauth_token_touch_failed", "error", err)
	}
	return model.AccessToken{
		ID: t.ID, Family: t.Family, GrantID: t.GrantID, ClientID: t.ClientID, UserID: t.UserID,
		OrganizationID: t.OrganizationID, BrandID: t.BrandID, Resource: t.Resource, Scopes: t.Scopes,
		ExpiresAt: t.ExpiresAt.Time,
	}, nil
}

// --- metadata -------------------------------------------------------------------------

// AuthorizationServerMetadata is the RFC 8414 document.
func (s *Service) AuthorizationServerMetadata() map[string]any {
	return map[string]any{
		"issuer":                                         s.issuer,
		"authorization_endpoint":                         s.issuer + "/oauth/authorize",
		"token_endpoint":                                 s.issuer + "/oauth/token",
		"registration_endpoint":                          s.issuer + "/oauth/register",
		"revocation_endpoint":                            s.issuer + "/oauth/revoke",
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{model.GrantAuthorizationCode, model.GrantRefreshToken},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"revocation_endpoint_auth_methods_supported":     []string{"none"},
		"scopes_supported":                               []string{model.ScopeMCP},
		"authorization_response_iss_parameter_supported": true,
	}
}

// ProtectedResourceMetadata is the RFC 9728 document of an MCP endpoint.
func (s *Service) ProtectedResourceMetadata(resource string) map[string]any {
	return map[string]any{
		"resource":                 s.ResourceURL(resource),
		"authorization_servers":    []string{s.issuer},
		"scopes_supported":         []string{model.ScopeMCP},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "Olexfilms MCP " + strings.TrimPrefix(resource, "/mcp/"),
	}
}

// ResourceMetadataURL is the RFC 9728 metadata URL of an MCP endpoint (for
// the WWW-Authenticate challenge of F4-03c).
func (s *Service) ResourceMetadataURL(resource string) string {
	return s.issuer + "/.well-known/oauth-protected-resource" + resource
}

// --- maintenance ------------------------------------------------------------------------

// Cleanup removes expired requests, codes, tokens and abandoned client
// registrations (periodic worker task).
func (s *Service) Cleanup(ctx context.Context) error {
	var total int64
	for _, f := range []func(context.Context) (int64, error){
		s.q.CleanupOAuthAuthRequests, s.q.CleanupOAuthCodes, s.q.CleanupOAuthTokens, s.q.CleanupOAuthClients,
	} {
		n, err := f(ctx)
		if err != nil {
			return fmt.Errorf("oauth: cleanup: %w", err)
		}
		total += n
	}
	if total > 0 {
		s.log.Info("oauth_cleanup", "deleted", total)
	}
	return nil
}
