package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/redis/go-redis/v9"
)

func TestAllowStrictFailsClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if ok, _, err := ratelimit.New(nil, "test").AllowStrict(ctx, "otp_send", "+905551234567", 5, time.Hour); ok || err == nil {
		t.Fatal("nil redis must fail closed")
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lim := ratelimit.New(rdb, "test")
	for i := 0; i < 2; i++ {
		if ok, _, err := lim.AllowStrict(ctx, "otp_send", "x", 2, time.Hour); !ok || err != nil {
			t.Fatalf("hit %d: %v %v", i, ok, err)
		}
	}
	if ok, retry, err := lim.AllowStrict(ctx, "otp_send", "x", 2, time.Hour); ok || err != nil || retry <= 0 {
		t.Fatalf("third hit: %v %v %v", ok, retry, err)
	}
	mr.Close()
	if ok, _, err := lim.AllowStrict(ctx, "otp_send", "y", 2, time.Hour); ok || err == nil {
		t.Fatal("redis down must fail closed")
	}
}
