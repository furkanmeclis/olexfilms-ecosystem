package authrevoke

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return New(rdb, "test", 15*time.Minute), mr
}

func TestRevokeSession(t *testing.T) {
	ctx := context.Background()
	s, mr := newStore(t)
	user, sid, other := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	if revoked, err := s.IsRevoked(ctx, user, sid, now); err != nil || revoked {
		t.Fatalf("fresh session revoked=%v err=%v", revoked, err)
	}
	if err := s.RevokeSession(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if revoked, _ := s.IsRevoked(ctx, user, sid, now); !revoked {
		t.Fatal("logged-out session still accepted")
	}
	if revoked, _ := s.IsRevoked(ctx, user, other, now); revoked {
		t.Fatal("logout of one device revoked another")
	}
	// The marker only lives as long as an access token could.
	mr.FastForward(17 * time.Minute)
	if revoked, _ := s.IsRevoked(ctx, user, sid, now); revoked {
		t.Fatal("marker outlived the access token lifetime")
	}
}

func TestRevokeUserCutoff(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	fixed := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return fixed }
	user := uuid.New()

	if err := s.RevokeUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if revoked, _ := s.IsRevoked(ctx, user, uuid.New(), fixed.Add(-time.Minute)); !revoked {
		t.Fatal("token issued before password change still accepted")
	}
	if revoked, _ := s.IsRevoked(ctx, user, uuid.New(), fixed); revoked {
		t.Fatal("token issued at the cutoff second was rejected")
	}
	if revoked, _ := s.IsRevoked(ctx, uuid.New(), uuid.New(), fixed.Add(-time.Minute)); revoked {
		t.Fatal("cutoff leaked to another user")
	}
}

func TestNilStoreIsNoop(t *testing.T) {
	var s *Store
	ctx := context.Background()
	if err := s.RevokeSession(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if revoked, err := s.IsRevoked(ctx, uuid.New(), uuid.New(), time.Now()); err != nil || revoked {
		t.Fatalf("nil store revoked=%v err=%v", revoked, err)
	}
}
