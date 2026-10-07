// Package model holds the MCP OAuth 2.1 authorization server types
// (TEC-400, F4-03a).
package model

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Lifetimes of the OAuth artifacts (F4-03a).
const (
	AccessTTL      = time.Hour
	RefreshTTL     = 30 * 24 * time.Hour
	CodeTTL        = 5 * time.Minute
	AuthRequestTTL = 15 * time.Minute
)

// Per-IP limits per hour of the public endpoints.
const (
	RegisterPerHour  = 20
	AuthorizePerHour = 60
	TokenPerHour     = 120
	RevokePerHour    = 120
	RateWindow       = time.Hour
)

// ScopeMCP is the only scope: the token acts with the user's permissions
// inside the chosen organization.
const ScopeMCP = "mcp"

// Token kinds (oauth_tokens.kind).
const (
	KindAccess  = "access"
	KindRefresh = "refresh"
)

// Grant types of the token endpoint.
const (
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
)

// The protected resources (RFC 8707 audiences): the three MCP endpoints.
const (
	ResourceDealer   = "/mcp/dealer"
	ResourceCustomer = "/mcp/customer"
	ResourceUser     = "/mcp/user"
)

// Resources lists every MCP endpoint path.
var Resources = []string{ResourceDealer, ResourceCustomer, ResourceUser}

// IsResource reports whether path is one of the MCP endpoints.
func IsResource(path string) bool {
	for _, r := range Resources {
		if r == path {
			return true
		}
	}
	return false
}

// OAuth error codes (RFC 6749 §5.2, RFC 7591 §3.2.2, RFC 8707 §2).
const (
	ErrInvalidRequest          = "invalid_request"
	ErrInvalidClient           = "invalid_client"
	ErrInvalidGrant            = "invalid_grant"
	ErrUnsupportedGrantType    = "unsupported_grant_type"
	ErrUnsupportedResponseType = "unsupported_response_type"
	ErrInvalidScope            = "invalid_scope"
	ErrInvalidTarget           = "invalid_target"
	ErrInvalidRedirectURI      = "invalid_redirect_uri"
	ErrInvalidClientMetadata   = "invalid_client_metadata"
	ErrInvalidToken            = "invalid_token"
	ErrAccessDenied            = "access_denied"
	ErrSlowDown                = "slow_down"
	ErrServerError             = "server_error"
)

// Error is an OAuth protocol error: the handler writes it as
// {"error", "error_description"} with Status.
type Error struct {
	Status      int
	Code        string
	Description string
	// Redirectable is set by authorize request validation once client and
	// redirect_uri are known good: the error may go back to the client's
	// redirect_uri. Otherwise it must be shown to the user (RFC 6749 §4.1.2.1).
	Redirectable bool
	// RetryAfter is set on rate limit errors.
	RetryAfter time.Duration
	// Cause is the internal error behind a server_error (logged, never sent).
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("oauth: %s: %v", e.Code, e.Cause)
	}
	return fmt.Sprintf("oauth: %s: %s", e.Code, e.Description)
}

// Unwrap returns the internal cause.
func (e *Error) Unwrap() error { return e.Cause }

// BadRequest builds a 400 OAuth error.
func BadRequest(code, desc string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Description: desc}
}

// Client is a registered (RFC 7591) public client.
type Client struct {
	ClientID     string    `json:"client_id"`
	ClientName   string    `json:"client_name"`
	RedirectURIs []string  `json:"redirect_uris"`
	IssuedAt     time.Time `json:"-"`
}

// RegisterInput is the client metadata of a registration request.
type RegisterInput struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

// AuthorizeInput is an /oauth/authorize request (F4-03b serves the endpoint).
type AuthorizeInput struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	CodeChallenge       string
	CodeChallengeMethod string
	State               string
	Scope               string
	Resource            string
}

// AuthRequest is a validated authorize request waiting for consent.
type AuthRequest struct {
	ID          uuid.UUID
	ClientID    string
	ClientName  string
	RedirectURI string
	State       string
	Resource    string
	ExpiresAt   time.Time
}

// IssueCodeInput is an approved consent: the user and the organization the
// token is bound to (F4-03b checks realm, membership and mcp.connect first).
type IssueCodeInput struct {
	RequestID      uuid.UUID
	UserID         int64
	OrganizationID int64
	BrandID        int64
}

// TokenInput is the form of a token endpoint request.
type TokenInput struct {
	GrantType    string
	ClientID     string
	Code         string
	RedirectURI  string
	CodeVerifier string
	RefreshToken string
	Resource     string
	Scope        string
}

// TokenResponse is the RFC 6749 §5.1 success body.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// AccessToken is a validated Bearer token of an MCP endpoint.
type AccessToken struct {
	ID             int64
	Family         uuid.UUID
	GrantID        int64
	ClientID       string
	UserID         int64
	OrganizationID int64
	BrandID        int64
	Resource       string
	Scopes         []string
	ExpiresAt      time.Time
}

// --- consent and connected apps (TEC-401, F4-03b) ---------------------------

// Realms of the MCP endpoints: who may connect to each.
const (
	RealmCustomer = "customer" // /mcp/customer: portal (customer realm) sessions
	RealmDealer   = "dealer"   // /mcp/dealer: distributor / dealer members with mcp.connect
	RealmUser     = "user"     // /mcp/user: every panel user with mcp.connect
)

// RealmOf maps an MCP endpoint path to its realm.
func RealmOf(resource string) string {
	switch resource {
	case ResourceCustomer:
		return RealmCustomer
	case ResourceDealer:
		return RealmDealer
	default:
		return RealmUser
	}
}

// Consent decisions.
const (
	DecisionApprove = "approve"
	DecisionDeny    = "deny"
)

// ConsentPath is the frontend consent screen /oauth/authorize redirects to.
const ConsentPath = "/oauth/consent"

// Activity log actions.
const (
	ActionConsentApproved = "oauth.consent_approved"
	ActionConsentDenied   = "oauth.consent_denied"
	ActionGrantRevoked    = "oauth.grant_revoked"
	ActionClientRevoked   = "oauth.client_revoked"
)

// ConsentOrganization is an organization the token may be bound to.
type ConsentOrganization struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// Consent is what the consent screen shows for a pending request.
type Consent struct {
	RequestID     uuid.UUID             `json:"request_uuid"`
	ClientID      string                `json:"client_id"`
	ClientName    string                `json:"client_name"`
	RedirectHost  string                `json:"redirect_host"`
	Resource      string                `json:"resource"`
	ResourceURL   string                `json:"resource_url"`
	Realm         string                `json:"realm"`
	Organizations []ConsentOrganization `json:"organizations"`
	ExpiresAt     time.Time             `json:"expires_at"`
}

// DecideInput is the body of a consent decision.
type DecideInput struct {
	Decision         string     `json:"decision"`
	OrganizationUUID *uuid.UUID `json:"organization_uuid"`
}

// Decision is the result of a consent decision: where to send the browser.
type Decision struct {
	RedirectURL string `json:"redirect_url"`
}

// Grant is one of the user's connected apps.
type Grant struct {
	UUID             uuid.UUID  `json:"uuid"`
	ClientID         string     `json:"client_id"`
	ClientName       string     `json:"client_name"`
	Resource         string     `json:"resource"`
	Realm            string     `json:"realm"`
	Scopes           []string   `json:"scopes"`
	OrganizationUUID uuid.UUID  `json:"organization_uuid"`
	OrganizationName string     `json:"organization_name"`
	OrganizationType string     `json:"organization_type"`
	CreatedAt        time.Time  `json:"created_at"`
	LastUsedAt       *time.Time `json:"last_used_at"`
}

// ClientSummary is a registered client in the platform list.
type ClientSummary struct {
	UUID         uuid.UUID  `json:"uuid"`
	ClientID     string     `json:"client_id"`
	ClientName   string     `json:"client_name"`
	RedirectURIs []string   `json:"redirect_uris"`
	CreatedIP    *string    `json:"created_ip"`
	Status       string     `json:"status"`
	ActiveGrants int64      `json:"active_grants"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
}

// Client statuses of the platform list.
const (
	ClientStatusActive  = "active"
	ClientStatusRevoked = "revoked"
)
