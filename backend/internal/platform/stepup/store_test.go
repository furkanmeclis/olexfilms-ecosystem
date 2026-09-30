package stepup_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestGrantTTLAndRevoke(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := stepup.NewStore(rdb, "test")
	user := uuid.New()

	expiresAt, err := store.SetGrant(context.Background(), user, "password", time.Hour)
	if err != nil {
		t.Fatalf("SetGrant: %v", err)
	}
	if expiresAt.Before(time.Now().UTC()) {
		t.Fatalf("expected future expiry")
	}

	valid, gotExpires, err := store.GetGrant(context.Background(), user)
	if err != nil {
		t.Fatalf("GetGrant: %v", err)
	}
	if !valid {
		t.Fatalf("expected valid grant")
	}
	if gotExpires.Unix() != expiresAt.Unix() {
		t.Fatalf("expires mismatch: got %d want %d", gotExpires.Unix(), expiresAt.Unix())
	}

	if err := store.RevokeGrant(context.Background(), user); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}
	valid, _, err = store.GetGrant(context.Background(), user)
	if err != nil {
		t.Fatalf("GetGrant after revoke: %v", err)
	}
	if valid {
		t.Fatalf("expected grant to be revoked")
	}
}

func TestRateLimit(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := stepup.NewStore(rdb, "test")
	user := uuid.New()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := store.IncrRateLimit(ctx, user); err != nil {
			t.Fatalf("IncrRateLimit: %v", err)
		}
	}
	limited, err := store.IsRateLimited(ctx, user)
	if err != nil {
		t.Fatalf("IsRateLimited: %v", err)
	}
	if !limited {
		t.Fatalf("expected rate limit")
	}
}
