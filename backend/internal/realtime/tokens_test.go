package realtime_test

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestConnectionTokenShape(t *testing.T) {
	t.Parallel()
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{
		Enabled: true, TokenHMAC: "unit-test-hmac-secret", TokenTTL: time.Hour,
		WSURL: "ws://127.0.0.1:8000/connection/websocket",
	})
	if err != nil || issuer == nil {
		t.Fatalf("issuer: %v", err)
	}
	uid := uuid.New().String()
	token, exp, err := issuer.ConnectionToken(uid)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || exp.IsZero() {
		t.Fatal("empty token")
	}
	if issuer.WSURL() == "" {
		t.Fatal("empty ws url")
	}
	parsed, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		return []byte("unit-test-hmac-secret"), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("parse: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims type")
	}
	if claims["sub"] != uid {
		t.Fatalf("sub = %v want %s", claims["sub"], uid)
	}
}

func TestDisabledIssuer(t *testing.T) {
	t.Parallel()
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if issuer != nil {
		t.Fatal("expected nil issuer when disabled")
	}
}

// TEC-91: the QR guest token is anonymous, bound to one channel and short.
func TestGuestChannelTokens(t *testing.T) {
	t.Parallel()
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{
		Enabled: true, TokenHMAC: "unit-test-hmac-secret", TokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, sub, exp, err := issuer.GuestChannelTokens("qr:abc", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(exp); d > 2*time.Minute+time.Second || d < time.Minute {
		t.Fatalf("guest exp in %v", d)
	}
	parse := func(raw string) jwt.MapClaims {
		parsed, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return []byte("unit-test-hmac-secret"), nil })
		if err != nil || !parsed.Valid {
			t.Fatalf("parse: %v", err)
		}
		return parsed.Claims.(jwt.MapClaims)
	}
	c := parse(conn)
	if c["sub"] != "" {
		t.Fatalf("guest sub = %v", c["sub"])
	}
	chans, _ := c["channels"].([]any)
	if len(chans) != 1 || chans[0] != "qr:abc" {
		t.Fatalf("guest channels = %v", c["channels"])
	}
	if s := parse(sub); s["channel"] != "qr:abc" || s["sub"] != "" {
		t.Fatalf("guest subscription claims = %v", s)
	}
	if _, _, _, err := issuer.GuestChannelTokens("", time.Minute); err == nil {
		t.Fatal("empty channel must fail")
	}
}
