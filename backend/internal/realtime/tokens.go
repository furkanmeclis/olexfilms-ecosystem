package realtime

import (
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

// TokenIssuer mints Centrifugo connection and subscription JWTs.
// Connection JWT sub is the user UUID string (not a numeric id).
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	wsURL  string
}

// NewTokenIssuer creates a JWT issuer from Centrifugo config.
// Returns (nil, nil) when Centrifugo is disabled.
func NewTokenIssuer(cfg config.CentrifugoConfig) (*TokenIssuer, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.TokenHMAC == "" {
		return nil, fmt.Errorf("realtime: CENTRIFUGO_TOKEN_HMAC_SECRET is required when enabled")
	}
	ttl := cfg.TokenTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &TokenIssuer{
		secret: []byte(cfg.TokenHMAC),
		ttl:    ttl,
		wsURL:  cfg.WSURL,
	}, nil
}

// Enabled reports whether the issuer is active.
func (i *TokenIssuer) Enabled() bool {
	return i != nil
}

// WSURL returns the public WebSocket URL for clients.
func (i *TokenIssuer) WSURL() string {
	if i == nil {
		return ""
	}
	return i.wsURL
}

// ConnectionToken issues a Centrifugo connection JWT for sub=userUUID.
func (i *TokenIssuer) ConnectionToken(userUUID string) (string, time.Time, error) {
	if i == nil {
		return "", time.Time{}, fmt.Errorf("realtime: token issuer disabled")
	}
	if userUUID == "" {
		return "", time.Time{}, fmt.Errorf("realtime: user uuid is required")
	}
	now := time.Now().UTC()
	exp := now.Add(i.ttl)
	claims := jwt.MapClaims{
		"sub": userUUID,
		"exp": exp.Unix(),
		"iat": now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("realtime: sign connection token: %w", err)
	}
	return signed, exp, nil
}

// SubscriptionToken issues a channel subscription JWT.
func (i *TokenIssuer) SubscriptionToken(userUUID, channel string) (string, time.Time, error) {
	if i == nil {
		return "", time.Time{}, fmt.Errorf("realtime: token issuer disabled")
	}
	if userUUID == "" {
		return "", time.Time{}, fmt.Errorf("realtime: user uuid is required")
	}
	if channel == "" {
		return "", time.Time{}, fmt.Errorf("realtime: channel is required")
	}
	now := time.Now().UTC()
	exp := now.Add(i.ttl)
	claims := jwt.MapClaims{
		"sub":     userUUID,
		"channel": channel,
		"exp":     exp.Unix(),
		"iat":     now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("realtime: sign subscription token: %w", err)
	}
	return signed, exp, nil
}
