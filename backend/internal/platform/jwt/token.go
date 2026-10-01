package jwt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid token")

// Audiences (session realms). Panel logins (password, passkey, OAuth) get
// "panel"; customer WhatsApp OTP logins get "portal". Enforcement per route
// group is the portal work (TEC-90).
const (
	AudiencePanel  = "panel"
	AudiencePortal = "portal"
)

// NormalizeAudience maps "" and unknown values to panel.
func NormalizeAudience(aud string) string {
	if aud == AudiencePortal {
		return AudiencePortal
	}
	return AudiencePanel
}

// Claims are verified identity data carried by an access token.
type Claims struct {
	Roles          []string `json:"roles"`
	IsSuperAdmin   bool     `json:"is_super_admin"`
	ImpersonatorID *string  `json:"imp,omitempty"`
	SessionID      string   `json:"sid,omitempty"`
	OrganizationID *string  `json:"oid,omitempty"`
	jwtlib.RegisteredClaims
}

// AccessInput is the payload used when issuing an access token.
type AccessInput struct {
	UserID         uuid.UUID
	Roles          []string
	IsSuperAdmin   bool
	ImpersonatorID *uuid.UUID
	SessionID      uuid.UUID
	OrganizationID *uuid.UUID
	// Audience is the session realm (AudiencePanel when empty).
	Audience string
}

// Manager issues and validates access JWTs. Refresh tokens are opaque (not JWT).
type Manager struct {
	accessSecret []byte
	accessTTL    time.Duration
	refreshTTL   time.Duration
	now          func() time.Time
}

// NewManager creates a token manager from a non-empty access signing secret.
func NewManager(accessSecret string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	if accessSecret == "" {
		return nil, errors.New("jwt: access signing secret is required")
	}
	if accessTTL <= 0 || refreshTTL <= 0 {
		return nil, errors.New("jwt: token TTLs must be positive")
	}
	return &Manager{
		accessSecret: []byte(accessSecret),
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
		now:          time.Now,
	}, nil
}

// AccessTTL returns the configured access token lifetime.
func (m *Manager) AccessTTL() time.Duration { return m.accessTTL }

// RefreshTTL returns the configured opaque refresh token lifetime.
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

// IssueAccess creates a short-lived access token.
func (m *Manager) IssueAccess(in AccessInput) (string, time.Time, error) {
	if in.UserID == uuid.Nil {
		return "", time.Time{}, errors.New("jwt: user id is required")
	}
	now := m.now().UTC()
	expiresAt := now.Add(m.accessTTL)
	roles := in.Roles
	if roles == nil {
		roles = []string{}
	}
	var imp *string
	if in.ImpersonatorID != nil && *in.ImpersonatorID != uuid.Nil {
		s := in.ImpersonatorID.String()
		imp = &s
	}
	var sid string
	if in.SessionID != uuid.Nil {
		sid = in.SessionID.String()
	}
	var oid *string
	if in.OrganizationID != nil && *in.OrganizationID != uuid.Nil {
		s := in.OrganizationID.String()
		oid = &s
	}
	claims := Claims{
		Roles:          roles,
		IsSuperAdmin:   in.IsSuperAdmin,
		ImpersonatorID: imp,
		SessionID:      sid,
		OrganizationID: oid,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   in.UserID.String(),
			Audience:  jwtlib.ClaimStrings{NormalizeAudience(in.Audience)},
			ExpiresAt: jwtlib.NewNumericDate(expiresAt),
			IssuedAt:  jwtlib.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	signed, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString(m.accessSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseAccess validates an access token and returns claims.
func (m *Manager) ParseAccess(token string) (Claims, error) {
	var claims Claims
	parsed, err := jwtlib.ParseWithClaims(token, &claims, func(token *jwtlib.Token) (any, error) {
		if token.Method != jwtlib.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.accessSecret, nil
	})
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Roles == nil {
		claims.Roles = []string{}
	}
	return claims, nil
}

// UserUUID returns the subject as a UUID.
// Realm returns the token audience (panel for tokens issued before aud).
func (c Claims) Realm() string {
	for _, a := range c.Audience {
		if a == AudiencePortal || a == AudiencePanel {
			return a
		}
	}
	return AudiencePanel
}

func (c Claims) UserUUID() (uuid.UUID, error) {
	return uuid.Parse(c.Subject)
}

// ImpersonatorUUID returns the optional impersonator claim as a UUID.
func (c Claims) ImpersonatorUUID() (*uuid.UUID, error) {
	if c.ImpersonatorID == nil || strings.TrimSpace(*c.ImpersonatorID) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*c.ImpersonatorID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// SessionUUID returns the optional refresh-session id bound to this access token.
func (c Claims) SessionUUID() uuid.UUID {
	if strings.TrimSpace(c.SessionID) == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(c.SessionID)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// OrganizationUUID returns the optional active organization claim.
func (c Claims) OrganizationUUID() (*uuid.UUID, error) {
	if c.OrganizationID == nil || strings.TrimSpace(*c.OrganizationID) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*c.OrganizationID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
