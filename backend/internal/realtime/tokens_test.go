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
