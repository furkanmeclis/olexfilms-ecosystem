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
// "panel"; customer WhatsApp OTP logins get "portal" (TEC-90); the mobile
// app's Bearer sessions get "mobile" (TEC-91). Enforcement per route group:
// middleware.RealmAllows.
const (
	AudiencePanel  = "panel"
	AudiencePortal = "portal"
	AudienceMobile = "mobile"
)

// NormalizeAudience maps "" and unknown values to panel.
func NormalizeAudience(aud string) string {
	switch aud {
	case AudiencePortal, AudienceMobile:
		return aud
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
	// Legacy marks a long-lived access token of the old hub mobile app
	// (TEC-284, F2-FIX-3): aud=mobile, valid until its device session
	// expires. Only a manager from AcceptLegacy parses it; every other
	// route rejects it as an invalid token.
	Legacy bool `json:"legacy,omitempty"`
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
	// acceptLegacy: ParseAccess also accepts legacy tokens (AcceptLegacy).
	acceptLegacy bool
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

// SetClock replaces the clock used to issue and validate tokens (tests
// that move time forward). nil restores time.Now. Copies made by
// AcceptLegacy afterwards share the new clock.
func (m *Manager) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	m.now = now
}

// AcceptLegacy returns a manager that also parses legacy tokens (Claims.Legacy).
// Only the old hub mobile app's alias routes (/v1/mobile/legacy/*) use it.
func (m *Manager) AcceptLegacy() *Manager {
	c := *m
	c.acceptLegacy = true
	return &c
}

// IssueAccess creates a short-lived access token.
func (m *Manager) IssueAccess(in AccessInput) (string, time.Time, error) {
	now := m.now().UTC()
	return m.issue(in, now, now.Add(m.accessTTL), false)
}

// IssueLegacyAccess creates the old hub mobile app's access token (TEC-284):
// aud=mobile, the legacy claim, and expiry at expiresAt (the end of its
// device session; the old app has no refresh flow). It needs a session id,
// so the token can be revoked with its session.
func (m *Manager) IssueLegacyAccess(in AccessInput, expiresAt time.Time) (string, time.Time, error) {
	now := m.now().UTC()
	if in.SessionID == uuid.Nil {
		return "", time.Time{}, errors.New("jwt: legacy token needs a session id")
	}
	if !expiresAt.After(now) {
		return "", time.Time{}, errors.New("jwt: legacy token expiry is in the past")
	}
	in.Audience = AudienceMobile
	in.ImpersonatorID = nil
	return m.issue(in, now, expiresAt.UTC(), true)
}

func (m *Manager) issue(in AccessInput, now, expiresAt time.Time, legacy bool) (string, time.Time, error) {
	if in.UserID == uuid.Nil {
		return "", time.Time{}, errors.New("jwt: user id is required")
	}
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
		Legacy:         legacy,
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

// ParseAccess validates an access token and returns claims. A legacy token
// (Claims.Legacy) is invalid unless the manager comes from AcceptLegacy.
func (m *Manager) ParseAccess(token string) (Claims, error) {
	var claims Claims
	parsed, err := jwtlib.ParseWithClaims(token, &claims, func(token *jwtlib.Token) (any, error) {
		if token.Method != jwtlib.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.accessSecret, nil
	}, jwtlib.WithTimeFunc(m.now))
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	if claims.Legacy && (!m.acceptLegacy || claims.Realm() != AudienceMobile || claims.SessionUUID() == uuid.Nil) {
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
		if a == AudiencePortal || a == AudiencePanel || a == AudienceMobile {
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
